// Package gorfc is a drop-in replacement for SAP's archived gorfc: an RFC
// client for SAP ABAP systems, built on erpl-proto, a pure-Rust implementation
// of the RFC protocol.
//
// It needs no SAP NW RFC SDK and no ICU. The native code ships in the module as
// a static archive, one per platform, and links into your binary -- there is
// nothing to download, install, or find at run time.
//
// # Calling a function module
//
//	conn, err := gorfc.ConnectionFromParams(gorfc.ConnectionParameters{
//		"ashost": "sap.example.com", "sysnr": "00", "client": "001",
//		"user": "RFCUSER", "passwd": "...", "lang": "EN",
//	})
//	if err != nil {
//		return err
//	}
//	defer conn.Close()
//
//	result, err := conn.Call("STFC_CONNECTION", map[string]interface{}{
//		"REQUTEXT": "hello",
//	})
//	fmt.Println(result["ECHOTEXT"])
//
// Parameters go in as a map keyed by ABAP parameter name, or as a struct whose
// exported field names are the parameter names. A structure is a
// map[string]interface{}, a table a slice of them. Results come back the same
// way, with ABAP types mapped to Go ones: CHAR and STRING to string, RAW and
// XSTRING to []byte, DATE and TIME to time.Time, the four integer widths to
// uint8, int16, int32 and int64, and the decimal types to string so that no
// digit is lost to a float64.
//
// # Building
//
// This package uses cgo, which is what allows the native code to be linked
// statically. That means CGO_ENABLED=1 and a C compiler: the system compiler
// on Linux and macOS, MinGW-w64 on Windows. On Alpine and other musl systems,
// build with -tags musl -- Go cannot tell one libc from the other, and a glibc
// archive will not link there.
//
// # Differences from SAP's gorfc
//
// The API is the same. A handful of behaviours differ, each because the
// reference's could not be reproduced without reproducing a defect -- a leak,
// a panic, or a value silently changed on its way to ABAP. They are listed in
// MIGRATION.md.
//
// Adapted from SAP's gorfc, Apache-2.0. See the NOTICE file.
package gorfc

// #include <sapnwrfc.h>
import "C"

// GetNWRFCLibVersion reports the version of the library underneath.
//
// The reference returns the SAP NW RFC SDK's patch level. There is no SDK
// here, so this returns erpl-proto's own version, in the same shape. Code that
// logs it keeps working; code that compared it against an SDK patch level was
// reading a number that no longer describes anything.
func GetNWRFCLibVersion() (major, minor, patchLevel uint) {
	var cmaj, cmin, cpatch C.uint
	C.RfcGetVersion(&cmaj, &cmin, &cpatch)
	return uint(cmaj), uint(cmin), uint(cpatch)
}
