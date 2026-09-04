package gorfc

/*
#include <sapnwrfc.h>
#include <stdlib.h>

// The SDK's sapuc.h supplies mallocU and strlenU as macros over its own
// allocator. This header declares only what erpl-proto implements, so the two
// the binding needs are defined here against SAP_UC being UTF-16
// (sapnwrfc.h: "typedef uint16_t SAP_UC").
//
// The definitions repeat in the other files that need them: a cgo preamble is
// its own translation unit, and these are static, so the same name in each
// file is one function per file rather than a collision.
static SAP_UC *mallocU(unsigned size) {
	return (SAP_UC *)calloc(size, sizeof(SAP_UC));
}

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
	"strings"
	"unsafe"
)

// fillString converts a Go string into a freshly allocated, NUL-terminated
// UTF-16 string. The caller owns the result and must free it.
func fillString(gostr string) (sapuc *C.SAP_UC, err error) {
	// UTF-16 never needs more code units than UTF-8 needs bytes, so one
	// SAP_UC per byte plus a terminator is always enough. GoMallocU zeroes,
	// which is what terminates the empty string.
	sapucSize := C.uint(len(gostr) + 1)
	sapuc = C.mallocU(sapucSize)
	if sapuc == nil {
		return nil, goRfcError(fmt.Sprintf("could not allocate %d characters", sapucSize), nil)
	}
	if len(gostr) == 0 {
		return sapuc, nil
	}

	var errorInfo C.RFC_ERROR_INFO
	var resultLen C.uint
	cStr := C.CString(gostr)
	defer C.free(unsafe.Pointer(cStr))

	rc := C.RfcUTF8ToSAPUC((*C.uint8_t)(unsafe.Pointer(cStr)), C.uint(len(gostr)),
		sapuc, &sapucSize, &resultLen, &errorInfo)
	if rc != C.RFC_OK {
		// Freeing here rather than leaving it to the caller: every caller of
		// this function defers a free on the returned pointer, and on the
		// error path there is no pointer to defer on.
		C.free(unsafe.Pointer(sapuc))
		return nil, rfcError(errorInfo, "Could not fill the string %q", gostr)
	}
	return sapuc, nil
}

// wrapString converts a NUL-terminated UTF-16 string to Go.
func wrapString(sapuc *C.SAP_UC, strip bool) (string, error) {
	if sapuc == nil {
		return "", nil
	}
	return nWrapString(sapuc, C.strlenU(sapuc), strip)
}

// nWrapString converts sapucLength UTF-16 code units to Go. Fixed-width ABAP
// fields are blank-padded to their declared width, and strip removes that
// padding -- trailing blanks and NULs, nothing else.
func nWrapString(sapuc *C.SAP_UC, sapucLength C.uint, strip bool) (string, error) {
	if sapucLength == 0 || sapuc == nil {
		return "", nil
	}

	var errorInfo C.RFC_ERROR_INFO
	var resultLength C.uint

	// A UTF-16 code unit encodes to at most three UTF-8 bytes; a surrogate
	// pair is two units and four bytes, so three per unit is always enough.
	utf8size := C.uint(3*sapucLength + 1)
	utf8Str := (*C.uint8_t)(C.calloc(C.size_t(utf8size), 1))
	if utf8Str == nil {
		return "", goRfcError(fmt.Sprintf("could not allocate %d bytes", utf8size), nil)
	}
	defer C.free(unsafe.Pointer(utf8Str))

	rc := C.RfcSAPUCToUTF8(sapuc, sapucLength, utf8Str, &utf8size, &resultLength, &errorInfo)
	if rc != C.RFC_OK {
		return "", rfcError(errorInfo, "Could not convert %d characters to UTF-8", uint(sapucLength))
	}

	result := C.GoStringN((*C.char)(unsafe.Pointer(utf8Str)), C.int(resultLength))
	if strip {
		result = strings.TrimRight(result, "\x00 ")
	}
	return result, nil
}

// ucRoundTrip converts to UTF-16 and back. cgo is not permitted in _test.go
// files, so the conversion pair -- the one piece of this binding that can be
// exercised without a live system -- is driven from here.
func ucRoundTrip(s string, strip bool) (string, error) {
	uc, err := fillString(s)
	if err != nil {
		return "", err
	}
	defer C.free(unsafe.Pointer(uc))
	return wrapString(uc, strip)
}
