package gorfc

// #include <sapnwrfc.h>
import "C"

import "fmt"

// RfcError is an error the RFC library itself reported.
type RfcError struct {
	Description string
	ErrorInfo   rfcSDKError
}

func (err RfcError) Error() string {
	return fmt.Sprintf("NWRFC SDK error: %s | %s", err.Description, err.ErrorInfo)
}

func rfcError(errorInfo C.RFC_ERROR_INFO, format string, a ...interface{}) *RfcError {
	return &RfcError{fmt.Sprintf(format, a...), wrapError(&errorInfo)}
}

// GoRfcError is an error this binding produced rather than the library.
type GoRfcError struct {
	Description string
	GoError     error
}

func (err GoRfcError) Error() string {
	if err.GoError != nil {
		return fmt.Sprintf("GORFC error: %s | %s", err.Description, err.GoError.Error())
	}
	return fmt.Sprintf("GORFC error: %s", err.Description)
}

// Unwrap exposes the underlying error to errors.Is and errors.As. Without it
// a caller cannot tell "no sapnwrfc.ini" from "destination not in the file"
// except by matching on message text.
func (err GoRfcError) Unwrap() error { return err.GoError }

func goRfcError(description string, goerror error) *GoRfcError {
	return &GoRfcError{description, goerror}
}

// rfcSDKError carries the fields of RFC_ERROR_INFO. Field order and the
// String() rendering match the reference's, because both appear in error text
// that callers log and, occasionally, match on.
type rfcSDKError struct {
	Message       string
	Code          string
	Key           string
	AbapMsgClass  string
	AbapMsgType   string
	AbapMsgNumber string
	AbapMsgV1     string
	AbapMsgV2     string
	AbapMsgV3     string
	AbapMsgV4     string
}

func (err rfcSDKError) String() string {
	return fmt.Sprintf("rfcSDKError[%v, %v, %v, %v, %v, %v, %v, %v, %v, %v]",
		err.Message, err.Code, err.Key, err.AbapMsgClass, err.AbapMsgType,
		err.AbapMsgNumber, err.AbapMsgV1, err.AbapMsgV2, err.AbapMsgV3, err.AbapMsgV4)
}

func wrapError(errorInfo *C.RFC_ERROR_INFO) rfcSDKError {
	// A conversion failure here would mask the error actually being reported,
	// so each field degrades to empty rather than replacing the diagnosis.
	str := func(uc *C.SAP_UC) string {
		s, err := wrapString(uc, true)
		if err != nil {
			return ""
		}
		return s
	}
	return rfcSDKError{
		str(&errorInfo.message[0]),
		str(C.RfcGetRcAsString(errorInfo.code)),
		str(&errorInfo.key[0]),
		str(&errorInfo.abapMsgClass[0]),
		str(&errorInfo.abapMsgType[0]),
		str(&errorInfo.abapMsgNumber[0]),
		str(&errorInfo.abapMsgV1[0]),
		str(&errorInfo.abapMsgV2[0]),
		str(&errorInfo.abapMsgV3[0]),
		str(&errorInfo.abapMsgV4[0]),
	}
}
