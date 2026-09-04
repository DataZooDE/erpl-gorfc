package gorfc

/*
#include <sapnwrfc.h>
#include <stdlib.h>

// See strings.go for why this repeats.
static SAP_UC *mallocU(unsigned size) {
	return (SAP_UC *)calloc(size, sizeof(SAP_UC));
}
*/
import "C"

import (
	"time"
	"unsafe"
)

// rcBufferTooSmall is what the library answers when a caller's buffer cannot
// hold the value; the required length comes back in resultLen. Taken from the
// header rather than written as 23, which is how this came to be dead code
// once -- the shim was answering a different code entirely.
const rcBufferTooSmall = C.RFC_RC(C.RFC_BUFFER_TOO_SMALL)

// wrapResult collects a call's parameters, skipping the one direction the
// caller asked to filter out. Direction 0 matches nothing, which is how
// ReturnImportParams(true) keeps everything.
func wrapResult(funcDesc C.RFC_FUNCTION_DESC_HANDLE, container C.RFC_FUNCTION_HANDLE, filterParameterDirection C.RFC_DIRECTION, strip bool) (map[string]interface{}, error) {
	var errorInfo C.RFC_ERROR_INFO
	var paramCount C.uint

	if rc := C.RfcGetParameterCount(funcDesc, &paramCount, &errorInfo); rc != C.RFC_OK {
		return nil, rfcError(errorInfo, "Failed getting parameter count")
	}

	result := make(map[string]interface{}, paramCount)
	for i := C.uint(0); i < paramCount; i++ {
		var paramDesc C.RFC_PARAMETER_DESC
		if rc := C.RfcGetParameterDescByIndex(funcDesc, i, &paramDesc, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting parameter description by index(%v)", i)
		}
		if paramDesc.direction == filterParameterDirection {
			continue
		}
		fieldName, err := wrapString(&paramDesc.name[0], strip)
		if err != nil {
			return nil, err
		}
		value, err := wrapVariable(paramDesc._type, C.DATA_CONTAINER_HANDLE(container),
			&paramDesc.name[0], paramDesc.nucLength, paramDesc.typeDescHandle, strip)
		if err != nil {
			return nil, err
		}
		result[fieldName] = value
	}
	return result, nil
}

// rowPlan is a row type's field descriptors, read once.
//
// The reference re-read them for every row: RfcGetFieldCount and one
// RfcGetFieldDescByIndex per field, plus a UTF-16 decode of each field name,
// on every row of the table. For a ten-thousand-row read that is tens of
// thousands of cgo crossings and two allocations per field per row, spent
// re-deriving a layout that cannot change while the call is in hand.
type rowPlan struct {
	descs []C.RFC_FIELD_DESC
	names []string
}

func planRow(typeDesc C.RFC_TYPE_DESC_HANDLE, strip bool) (*rowPlan, error) {
	var errorInfo C.RFC_ERROR_INFO
	var fieldCount C.uint

	if rc := C.RfcGetFieldCount(typeDesc, &fieldCount, &errorInfo); rc != C.RFC_OK {
		return nil, rfcError(errorInfo, "Failed getting field count")
	}

	plan := &rowPlan{
		descs: make([]C.RFC_FIELD_DESC, fieldCount),
		names: make([]string, fieldCount),
	}
	for i := C.uint(0); i < fieldCount; i++ {
		if rc := C.RfcGetFieldDescByIndex(typeDesc, i, &plan.descs[i], &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting field description by index(%v)", i)
		}
		name, err := wrapString(&plan.descs[i].name[0], strip)
		if err != nil {
			return nil, err
		}
		plan.names[i] = name
	}
	return plan, nil
}

func wrapStructure(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_STRUCTURE_HANDLE, strip bool) (map[string]interface{}, error) {
	plan, err := planRow(typeDesc, strip)
	if err != nil {
		return nil, err
	}
	return plan.wrap(container, strip)
}

// wrap reads one row through an already-resolved plan.
//
// The name pointer handed to C is into the plan's own Go memory. That is
// allowed: RFC_FIELD_DESC holds C handles, not Go pointers, so the buffer
// carries nothing the collector must know about.
func (plan *rowPlan) wrap(container C.RFC_STRUCTURE_HANDLE, strip bool) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(plan.descs))
	for i := range plan.descs {
		desc := &plan.descs[i]
		value, err := wrapVariable(desc._type, C.DATA_CONTAINER_HANDLE(container),
			&desc.name[0], desc.nucLength, desc.typeDescHandle, strip)
		if err != nil {
			return nil, err
		}
		result[plan.names[i]] = value
	}
	return result, nil
}

func wrapTable(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_TABLE_HANDLE, strip bool) ([]interface{}, error) {
	var errorInfo C.RFC_ERROR_INFO
	var lines C.uint

	if rc := C.RfcGetRowCount(container, &lines, &errorInfo); rc != C.RFC_OK {
		return nil, rfcError(errorInfo, "Failed getting row count")
	}
	if lines == 0 {
		return []interface{}{}, nil
	}

	// Once for the table, not once per row.
	plan, err := planRow(typeDesc, strip)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, lines)
	for i := C.uint(0); i < lines; i++ {
		if rc := C.RfcMoveTo(container, i, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed moving cursor to index(%v)", i)
		}
		structHandle := C.RfcGetCurrentRow(container, &errorInfo)
		if structHandle == nil {
			return nil, rfcError(errorInfo, "Failed getting row at index(%v)", i)
		}
		line, err := plan.wrap(structHandle, strip)
		if err != nil {
			return nil, err
		}
		result[i] = line
	}
	return result, nil
}

// wrapVariable reads one value out of a container.
//
// cLen is the field's non-Unicode length, which for the fixed-width character
// types is its length in characters -- the unit RfcGetChars and RfcGetNum
// count in.
func wrapVariable(cType C.RFCTYPE, container C.DATA_CONTAINER_HANDLE, cName *C.SAP_UC, cLen C.uint, typeDesc C.RFC_TYPE_DESC_HANDLE, strip bool) (interface{}, error) {
	var errorInfo C.RFC_ERROR_INFO

	switch cType {
	case C.RFCTYPE_STRUCTURE:
		var structure C.RFC_STRUCTURE_HANDLE
		if rc := C.RfcGetStructure(container, cName, &structure, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting structure")
		}
		return wrapStructure(typeDesc, structure, strip)

	case C.RFCTYPE_TABLE:
		var table C.RFC_TABLE_HANDLE
		if rc := C.RfcGetTable(container, cName, &table, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting table")
		}
		return wrapTable(typeDesc, table, strip)

	case C.RFCTYPE_CHAR:
		buf := C.mallocU(cLen + 1)
		defer C.free(unsafe.Pointer(buf))
		if rc := C.RfcGetChars(container, cName, (*C.RFC_CHAR)(buf), cLen, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting chars")
		}
		return nWrapString(buf, cLen, strip)

	case C.RFCTYPE_NUM:
		buf := C.mallocU(cLen + 1)
		defer C.free(unsafe.Pointer(buf))
		if rc := C.RfcGetNum(container, cName, (*C.RFC_NUM)(buf), cLen, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting num")
		}
		return nWrapString(buf, cLen, strip)

	case C.RFCTYPE_STRING:
		return getString(container, cName, strip, "STRING", 0)

	case C.RFCTYPE_BCD:
		// A packed field of cLen bytes holds at most 2*cLen-1 digits, plus a
		// sign and a decimal point.
		return getString(container, cName, strip, "BCD", 2*cLen+1)

	case C.RFCTYPE_DECF16, C.RFCTYPE_DECF34:
		// As BCD, plus room for an exponent.
		return getString(container, cName, strip, "DECF", 2*cLen+10)

	case C.RFCTYPE_UTCLONG:
		text, err := getString(container, cName, strip, "UTCLONG", 27)
		if err != nil {
			return nil, err
		}
		return utclongLikeReference(text), nil

	case C.RFCTYPE_BYTE:
		if cLen == 0 {
			return []byte{}, nil
		}
		buf := C.malloc(C.size_t(cLen))
		defer C.free(buf)
		if rc := C.RfcGetBytes(container, cName, (*C.SAP_RAW)(buf), cLen, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting bytes")
		}
		return C.GoBytes(buf, C.int(cLen)), nil

	case C.RFCTYPE_XSTRING:
		var strLen C.uint
		if rc := C.RfcGetStringLength(container, cName, &strLen, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting xstring length")
		}
		if strLen == 0 {
			return []byte{}, nil
		}
		buf := C.malloc(C.size_t(strLen))
		defer C.free(buf)
		var resultLen C.uint
		if rc := C.RfcGetXString(container, cName, (*C.SAP_RAW)(buf), strLen, &resultLen, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting xstring")
		}
		return C.GoBytes(buf, C.int(resultLen)), nil

	case C.RFCTYPE_FLOAT:
		var floatValue C.RFC_FLOAT
		if rc := C.RfcGetFloat(container, cName, &floatValue, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting FLOAT")
		}
		return float64(floatValue), nil

	case C.RFCTYPE_INT:
		var intValue C.RFC_INT
		if rc := C.RfcGetInt(container, cName, &intValue, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting INT")
		}
		return int32(intValue), nil

	case C.RFCTYPE_INT1:
		var int1Value C.RFC_INT1
		if rc := C.RfcGetInt1(container, cName, &int1Value, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting INT1")
		}
		return uint8(int1Value), nil

	case C.RFCTYPE_INT2:
		var int2Value C.RFC_INT2
		if rc := C.RfcGetInt2(container, cName, &int2Value, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting INT2")
		}
		return int16(int2Value), nil

	case C.RFCTYPE_INT8:
		var int8Value C.RFC_INT8
		if rc := C.RfcGetInt8(container, cName, &int8Value, &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting INT8")
		}
		return int64(int8Value), nil

	case C.RFCTYPE_DATE:
		buf := C.mallocU(9)
		defer C.free(unsafe.Pointer(buf))
		if rc := C.RfcGetDate(container, cName, (*C.RFC_CHAR)(buf), &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting DATE")
		}
		value, err := nWrapString(buf, 8, false)
		if err != nil {
			return nil, err
		}
		// ABAP's initial date, and a blank one, are not dates. The reference
		// returned nil for them; it also indexed value[1] to detect the blank,
		// which panicked whenever the field came back short.
		if len(value) < 8 || value == "00000000" || value[0] == ' ' || value[1] == ' ' {
			return nil, nil
		}
		goDate, err := time.Parse("20060102", value)
		if err != nil {
			return nil, goRfcError("Error parsing ABAP RFC_DATE field", err)
		}
		return goDate, nil

	case C.RFCTYPE_TIME:
		buf := C.mallocU(7)
		defer C.free(unsafe.Pointer(buf))
		if rc := C.RfcGetTime(container, cName, (*C.RFC_CHAR)(buf), &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting TIME")
		}
		value, err := nWrapString(buf, 6, false)
		if err != nil {
			return nil, err
		}
		if len(value) < 6 || value[0] == ' ' {
			return nil, nil
		}
		goTime, err := time.Parse("150405", value)
		if err != nil {
			return nil, goRfcError("Error parsing ABAP RFC_TIME field", err)
		}
		return goTime, nil
	}

	goName, _ := wrapString(cName, true)
	return nil, goRfcError("Unknown RFC type when wrapping "+goName, nil)
}

// utclongLikeReference renders a UTCLONG the way SAP's gorfc does.
//
// Measured against it rather than assumed. For the same value the reference
// prints 2026-08-30T23:59:58.1234567 where this library renders
// 2026-08-30 23:59:58.1234567 (spec VAL-8): the SDK hands gorfc an ISO-ish
// form whose decimal separator is a comma, and gorfc rewrites the character at
// index 19 to a dot (utc[:19] + "." + utc[20:]) -- the T is the SDK's own.
//
// Normalising here rather than in the shim is deliberate. The classic form is
// what the spec records and what the Python binding returns; a caller of *this*
// package is one whose code was written against gorfc, and what it expects is
// gorfc's spelling.
//
// The reference indexed 19 unconditionally, so ABAP's initial UTCLONG -- which
// comes back shorter -- panicked. Anything too short to carry the separators is
// passed through untouched.
func utclongLikeReference(text string) string {
	if len(text) < 20 {
		return text
	}
	b := []byte(text)
	if b[10] == ' ' {
		b[10] = 'T'
	}
	b[19] = '.'
	return string(b)
}

// getString reads a string-shaped value. hint is a first guess at the length;
// zero means ask. Either way a buffer that turns out to be too small is grown
// to the length the library reports and read again, so no value is truncated.
func getString(container C.DATA_CONTAINER_HANDLE, cName *C.SAP_UC, strip bool, what string, hint C.uint) (string, error) {
	var errorInfo C.RFC_ERROR_INFO
	var strLen C.uint

	if hint > 0 {
		strLen = hint
	} else if rc := C.RfcGetStringLength(container, cName, &strLen, &errorInfo); rc != C.RFC_OK {
		return "", rfcError(errorInfo, "Failed getting %v length", what)
	}

	for attempt := 0; attempt < 2; attempt++ {
		buf := C.mallocU(strLen + 1)
		var resultLen C.uint
		rc := C.RfcGetString(container, cName, buf, strLen+1, &resultLen, &errorInfo)
		if rc == C.RFC_OK {
			value, err := nWrapString(buf, resultLen, strip)
			C.free(unsafe.Pointer(buf))
			return value, err
		}
		C.free(unsafe.Pointer(buf))
		if rc != rcBufferTooSmall {
			return "", rfcError(errorInfo, "Failed getting %v", what)
		}
		strLen = resultLen
	}
	return "", rfcError(errorInfo, "Failed getting %v: buffer still too small at %d characters", what, uint(strLen))
}
