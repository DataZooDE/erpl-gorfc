package gorfc

/*
#include <sapnwrfc.h>
#include <stdlib.h>

// See strings.go for why these two are defined rather than included: the
// definition repeats per file because a cgo preamble is its own translation
// unit and these are static.
static unsigned strlenU(const SAP_UC *str) {
	unsigned n = 0;
	while (str[n] != 0) {
		n++;
	}
	return n;
}
*/
import "C"

import (
	"fmt"
	"math"
	"reflect"
	"runtime"
	"unsafe"
)

// Call invokes a function module and returns its exports.
//
// Parameters are passed as a map[string]interface{} keyed by ABAP parameter
// name, or as a struct whose field names are the parameter names.
func (conn *Connection) Call(goFuncName string, params interface{}) (result map[string]interface{}, err error) {
	if !conn.alive {
		return nil, goRfcError("Call() method requires an open connection", nil)
	}

	funcName, err := fillString(goFuncName)
	if err != nil {
		return
	}
	defer C.free(unsafe.Pointer(funcName))

	var errorInfo C.RFC_ERROR_INFO
	funcDesc := C.RfcGetFunctionDesc(conn.handle, funcName, &errorInfo)
	runtime.KeepAlive(conn)
	if funcDesc == nil {
		return nil, rfcError(errorInfo, "Could not get function description for %q", goFuncName)
	}

	funcCont := C.RfcCreateFunction(funcDesc, &errorInfo)
	if funcCont == nil {
		return nil, rfcError(errorInfo, "Could not create function")
	}
	defer C.RfcDestroyFunction(funcCont, nil)

	if err = fillParameters(funcDesc, funcCont, params); err != nil {
		return nil, err
	}

	rc := C.RfcInvoke(conn.handle, funcCont, &errorInfo)
	runtime.KeepAlive(conn)
	if rc != C.RFC_OK {
		return nil, rfcError(errorInfo, "Could not invoke function %q", goFuncName)
	}

	if conn.returnImportParams {
		// Direction 0 matches no parameter, so nothing is filtered out.
		return wrapResult(funcDesc, funcCont, C.RFC_DIRECTION(0), conn.rstrip)
	}
	return wrapResult(funcDesc, funcCont, C.RFC_IMPORT, conn.rstrip)
}

// fillParameters writes a call's inputs, from a map or a struct.
func fillParameters(funcDesc C.RFC_FUNCTION_DESC_HANDLE, funcCont C.RFC_FUNCTION_HANDLE, params interface{}) error {
	if params == nil {
		return nil
	}
	paramsValue := deref(reflect.ValueOf(params))
	if !paramsValue.IsValid() {
		return nil
	}
	switch paramsValue.Kind() {
	case reflect.Map:
		if paramsValue.Type().Key().Kind() != reflect.String {
			return goRfcError("Parameters passed as a map must have string keys", nil)
		}
		iter := paramsValue.MapRange()
		for iter.Next() {
			if err := fillFunctionParameter(funcDesc, funcCont, iter.Key().String(), iter.Value().Interface()); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < paramsValue.NumField(); i++ {
			if !paramsValue.Type().Field(i).IsExported() {
				continue
			}
			name := paramsValue.Type().Field(i).Name
			if err := fillFunctionParameter(funcDesc, funcCont, name, paramsValue.Field(i).Interface()); err != nil {
				return err
			}
		}
	default:
		return goRfcError("Parameters can only be passed as map[string]interface{} or a struct", nil)
	}
	return nil
}

func fillFunctionParameter(funcDesc C.RFC_FUNCTION_DESC_HANDLE, container C.RFC_FUNCTION_HANDLE, goName string, value interface{}) error {
	name, err := fillString(goName)
	if err != nil {
		return err
	}
	defer C.free(unsafe.Pointer(name))

	var errorInfo C.RFC_ERROR_INFO
	var paramDesc C.RFC_PARAMETER_DESC
	if rc := C.RfcGetParameterDescByName(funcDesc, name, &paramDesc, &errorInfo); rc != C.RFC_OK {
		return rfcError(errorInfo, "Could not get the parameter description for %q", goName)
	}
	return fillVariable(paramDesc._type, C.DATA_CONTAINER_HANDLE(container), &paramDesc.name[0], value, paramDesc.typeDescHandle)
}

// fillVariable writes one value into a container.
//
// Every C string it allocates is freed on the way out of the branch that made
// it. The reference deferred the free before the pointer was assigned, so the
// deferred call always freed nil and every string filled into a call leaked.
func fillVariable(cType C.RFCTYPE, container C.DATA_CONTAINER_HANDLE, cName *C.SAP_UC, value interface{}, typeDesc C.RFC_TYPE_DESC_HANDLE) error {
	var errorInfo C.RFC_ERROR_INFO
	var rc C.RFC_RC

	// A nil is not a value of any ABAP type, and it is what a map literal
	// gives for a key whose value was never set. reflect.Value.Interface() on
	// the invalid Value it produces panics, so it is caught once here rather
	// than in each branch that would reach it. Leaving the field alone is what
	// "no value" already means: a fresh function container starts initial.
	if value == nil || !reflect.ValueOf(value).IsValid() {
		return nil
	}

	// checkRC turns the last return code into an error naming the parameter.
	checkRC := func() error {
		if rc != C.RFC_OK {
			goName, _ := wrapString(cName, true)
			return rfcError(errorInfo, "Could not fill %v of type %v", goName, cType)
		}
		return nil
	}

	// setText covers every type the library takes as text: the two
	// fixed-width character types, STRING, and the decimals, which travel as
	// text so that no digit is lost to a float.
	setText := func(text string, set func(*C.SAP_UC, C.uint) C.RFC_RC) error {
		cValue, units, err := fillStringN(text)
		if err != nil {
			return err
		}
		defer C.free(unsafe.Pointer(cValue))
		rc = set(cValue, units)
		return checkRC()
	}

	switch cType {
	case C.RFCTYPE_STRUCTURE:
		var structure C.RFC_STRUCTURE_HANDLE
		if rc = C.RfcGetStructure(container, cName, &structure, &errorInfo); rc != C.RFC_OK {
			return rfcError(errorInfo, "Could not get structure")
		}
		return fillStructure(typeDesc, structure, value)

	case C.RFCTYPE_TABLE:
		// Through the pointer first, so *[]row and a nil slice behave the way
		// the struct and structure cases do.
		rows := deref(reflect.ValueOf(value))
		if !rows.IsValid() {
			return nil
		}
		if kind := rows.Kind(); kind != reflect.Slice && kind != reflect.Array {
			return goRfcError(fmt.Sprintf("GO %s passed to ABAP TABLE parameter, expected GO array", reflect.TypeOf(value)), nil)
		}
		var table C.RFC_TABLE_HANDLE
		if rc = C.RfcGetTable(container, cName, &table, &errorInfo); rc != C.RFC_OK {
			return rfcError(errorInfo, "Could not get table")
		}
		return fillTable(typeDesc, table, value)

	case C.RFCTYPE_BYTE, C.RFCTYPE_XSTRING:
		bytes, err := asBytes(value)
		if err != nil {
			return err
		}
		// An empty slice is still a value: it means "clear this field", and
		// skipping the call would leave whatever the field already held.
		// C.CBytes always returns a valid pointer, including for length zero.
		bValue := (*C.SAP_RAW)(C.CBytes(bytes))
		defer C.free(unsafe.Pointer(bValue))
		if cType == C.RFCTYPE_BYTE {
			rc = C.RfcSetBytes(container, cName, bValue, C.uint(len(bytes)), &errorInfo)
		} else {
			rc = C.RfcSetXString(container, cName, bValue, C.uint(len(bytes)), &errorInfo)
		}
		return checkRC()

	case C.RFCTYPE_CHAR:
		return setText(asText(value), func(v *C.SAP_UC, n C.uint) C.RFC_RC {
			return C.RfcSetChars(container, cName, (*C.RFC_CHAR)(v), n, &errorInfo)
		})

	case C.RFCTYPE_NUM:
		return setText(asText(value), func(v *C.SAP_UC, n C.uint) C.RFC_RC {
			return C.RfcSetNum(container, cName, (*C.RFC_NUM)(v), n, &errorInfo)
		})

	case C.RFCTYPE_STRING, C.RFCTYPE_UTCLONG:
		return setText(asText(value), func(v *C.SAP_UC, n C.uint) C.RFC_RC {
			return C.RfcSetString(container, cName, v, n, &errorInfo)
		})

	case C.RFCTYPE_FLOAT:
		// Sent as text, like the decimals: a float64 rendered shortest-round-
		// trip loses nothing, and exponent form is a number ABAP accepts here.
		return setText(asText(value), func(v *C.SAP_UC, n C.uint) C.RFC_RC {
			return C.RfcSetString(container, cName, v, n, &errorInfo)
		})

	case C.RFCTYPE_BCD, C.RFCTYPE_DECF16, C.RFCTYPE_DECF34:
		// Text, because a packed or decimal-float field holds more digits than
		// a float64 has -- and spelled out rather than in exponent form, which
		// these fields will not take.
		return setText(asDecimalText(value), func(v *C.SAP_UC, n C.uint) C.RFC_RC {
			return C.RfcSetString(container, cName, v, n, &errorInfo)
		})

	// Each width gets its own setter and its own range check. The reference
	// sent all four through RfcSetInt, which is 32 bits: an INT8 past 2^31
	// was silently truncated, and an INT2 given 40000 was whatever the
	// narrowing produced.
	case C.RFCTYPE_INT1:
		n, err := asInt(value)
		if err != nil {
			return err
		}
		if err := intInRange(n, 0, 255, "INT1"); err != nil {
			return err
		}
		rc = C.RfcSetInt1(container, cName, C.RFC_INT1(n), &errorInfo)
		return checkRC()

	case C.RFCTYPE_INT2:
		n, err := asInt(value)
		if err != nil {
			return err
		}
		if err := intInRange(n, math.MinInt16, math.MaxInt16, "INT2"); err != nil {
			return err
		}
		rc = C.RfcSetInt2(container, cName, C.RFC_INT2(n), &errorInfo)
		return checkRC()

	case C.RFCTYPE_INT:
		n, err := asInt(value)
		if err != nil {
			return err
		}
		if err := intInRange(n, math.MinInt32, math.MaxInt32, "INT4"); err != nil {
			return err
		}
		rc = C.RfcSetInt(container, cName, C.RFC_INT(n), &errorInfo)
		return checkRC()

	case C.RFCTYPE_INT8:
		n, err := asInt(value)
		if err != nil {
			return err
		}
		rc = C.RfcSetInt8(container, cName, C.RFC_INT8(n), &errorInfo)
		return checkRC()

	case C.RFCTYPE_DATE:
		text, err := asDate(value)
		if err != nil {
			return err
		}
		// RfcSetDate reads exactly eight characters from the buffer, so a
		// shorter one is an out-of-bounds read rather than a short date.
		// Nothing to send is the initial value, which is what leaving the
		// field alone already means.
		if text == "" {
			return nil
		}
		if len(text) != 8 {
			goName, _ := wrapString(cName, true)
			return goRfcError(fmt.Sprintf("%q is not an ABAP date for %v: want eight characters, YYYYMMDD", text, goName), nil)
		}
		return setText(text, func(v *C.SAP_UC, _ C.uint) C.RFC_RC {
			return C.RfcSetDate(container, cName, (*C.RFC_CHAR)(v), &errorInfo)
		})

	case C.RFCTYPE_TIME:
		text, err := asTime(value)
		if err != nil {
			return err
		}
		// As RfcSetDate, six characters.
		if text == "" {
			return nil
		}
		if len(text) != 6 {
			goName, _ := wrapString(cName, true)
			return goRfcError(fmt.Sprintf("%q is not an ABAP time for %v: want six characters, HHMMSS", text, goName), nil)
		}
		return setText(text, func(v *C.SAP_UC, _ C.uint) C.RFC_RC {
			return C.RfcSetTime(container, cName, (*C.RFC_CHAR)(v), &errorInfo)
		})

	default:
		goName, _ := wrapString(cName, true)
		return goRfcError(fmt.Sprintf("Unknown RFC type %v when filling %v", cType, goName), nil)
	}
}

// deref follows pointers and interfaces to the value underneath. A caller
// writing &Params{...} is ordinary Go -- encoding/json and every other encoder
// takes one -- and the reference refused it, falling through to "parameters can
// only be passed as a map or a struct" for something that is a struct.
func deref(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func fillStructure(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_STRUCTURE_HANDLE, value interface{}) error {
	s := deref(reflect.ValueOf(value))
	// A nil row -- []interface{}{nil} -- gives an invalid Value, and the
	// default branch below calls s.Interface() on it, which panics with
	// "reflect.Value.Interface on zero Value" and takes the caller's process
	// with it. An empty row is a row of initial values.
	if !s.IsValid() {
		return nil
	}
	switch s.Kind() {
	case reflect.Map:
		if s.Type().Key().Kind() != reflect.String {
			return goRfcError("Could not fill structure passed as map with non-string keys", nil)
		}
		iter := s.MapRange()
		for iter.Next() {
			if err := fillStructureField(typeDesc, container, iter.Key().String(), iter.Value().Interface()); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < s.NumField(); i++ {
			if !s.Type().Field(i).IsExported() {
				continue
			}
			if err := fillStructureField(typeDesc, container, s.Type().Field(i).Name, s.Field(i).Interface()); err != nil {
				return err
			}
		}
	default:
		// A table of scalars: the row has one field, and the value is it.
		return fillStructureField(typeDesc, container, "", s.Interface())
	}
	return nil
}

func fillStructureField(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_STRUCTURE_HANDLE, fieldName string, fieldValue interface{}) error {
	cName, err := fillString(fieldName)
	if err != nil {
		return err
	}
	defer C.free(unsafe.Pointer(cName))

	var errorInfo C.RFC_ERROR_INFO
	var fieldDesc C.RFC_FIELD_DESC
	if rc := C.RfcGetFieldDescByName(typeDesc, cName, &fieldDesc, &errorInfo); rc != C.RFC_OK {
		return rfcError(errorInfo, "Could not get field description for %q", fieldName)
	}
	return fillVariable(fieldDesc._type, C.DATA_CONTAINER_HANDLE(container), &fieldDesc.name[0], fieldValue, fieldDesc.typeDescHandle)
}

// fillPlan is a row type's field descriptors, keyed by name and read once.
//
// Filling a table resolved every field of every row by name, and the by-name
// lookup in the library is itself a scan over the fields -- so an N-row,
// F-field table cost N x F allocations of the name and N x F x F comparisons
// to fill N x F values. The layout cannot change while the call is in hand.
type fillPlan struct {
	byName map[string]*C.RFC_FIELD_DESC
	descs  []C.RFC_FIELD_DESC
}

func planFill(typeDesc C.RFC_TYPE_DESC_HANDLE) (*fillPlan, error) {
	var errorInfo C.RFC_ERROR_INFO
	var fieldCount C.uint
	if rc := C.RfcGetFieldCount(typeDesc, &fieldCount, &errorInfo); rc != C.RFC_OK {
		return nil, rfcError(errorInfo, "Failed getting field count")
	}
	plan := &fillPlan{
		byName: make(map[string]*C.RFC_FIELD_DESC, fieldCount),
		descs:  make([]C.RFC_FIELD_DESC, fieldCount),
	}
	for i := C.uint(0); i < fieldCount; i++ {
		if rc := C.RfcGetFieldDescByIndex(typeDesc, i, &plan.descs[i], &errorInfo); rc != C.RFC_OK {
			return nil, rfcError(errorInfo, "Failed getting field description by index(%v)", i)
		}
		name, err := wrapString(&plan.descs[i].name[0], true)
		if err != nil {
			return nil, err
		}
		plan.byName[name] = &plan.descs[i]
	}
	return plan, nil
}

// fill writes one row through an already-resolved plan.
func (plan *fillPlan) fill(container C.RFC_STRUCTURE_HANDLE, value interface{}) error {
	s := deref(reflect.ValueOf(value))
	if !s.IsValid() {
		return nil
	}
	set := func(name string, v interface{}) error {
		desc, ok := plan.byName[name]
		if !ok {
			return goRfcError(fmt.Sprintf("the row type has no field %q", name), nil)
		}
		return fillVariable(desc._type, C.DATA_CONTAINER_HANDLE(container), &desc.name[0], v, desc.typeDescHandle)
	}
	switch s.Kind() {
	case reflect.Map:
		if s.Type().Key().Kind() != reflect.String {
			return goRfcError("Could not fill structure passed as map with non-string keys", nil)
		}
		iter := s.MapRange()
		for iter.Next() {
			if err := set(iter.Key().String(), iter.Value().Interface()); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < s.NumField(); i++ {
			if !s.Type().Field(i).IsExported() {
				continue
			}
			if err := set(s.Type().Field(i).Name, s.Field(i).Interface()); err != nil {
				return err
			}
		}
	default:
		// A table of scalars: the row has one field and the value is it.
		if len(plan.descs) != 1 {
			return goRfcError(fmt.Sprintf("GO %s passed as a row of %d fields", s.Type(), len(plan.descs)), nil)
		}
		desc := &plan.descs[0]
		return fillVariable(desc._type, C.DATA_CONTAINER_HANDLE(container), &desc.name[0], s.Interface(), desc.typeDescHandle)
	}
	return nil
}

func fillTable(typeDesc C.RFC_TYPE_DESC_HANDLE, container C.RFC_TABLE_HANDLE, lines interface{}) error {
	rows := deref(reflect.ValueOf(lines))
	if !rows.IsValid() || rows.Len() == 0 {
		return nil
	}
	// Once for the table, not once per field per row.
	plan, err := planFill(typeDesc)
	if err != nil {
		return err
	}
	var errorInfo C.RFC_ERROR_INFO
	for i := 0; i < rows.Len(); i++ {
		lineHandle := C.RfcAppendNewRow(container, &errorInfo)
		if lineHandle == nil {
			return rfcError(errorInfo, "Could not append new row to table")
		}
		if err := plan.fill(lineHandle, rows.Index(i).Interface()); err != nil {
			return err
		}
	}
	return nil
}
