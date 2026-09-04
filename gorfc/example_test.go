package gorfc_test

import (
	"fmt"
	"log"

	"github.com/DataZooDE/erpl-gorfc/gorfc"
)

// Opening a connection and calling a function module. Nothing has to be
// installed for this to build and run: the RFC implementation is linked in.
func Example() {
	conn, err := gorfc.ConnectionFromParams(gorfc.ConnectionParameters{
		"ashost": "sap.example.com",
		"sysnr":  "00",
		"client": "001",
		"user":   "RFCUSER",
		"passwd": "secret",
		"lang":   "EN",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	result, err := conn.Call("STFC_CONNECTION", map[string]interface{}{
		"REQUTEXT": "hello",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result["ECHOTEXT"])
}

// A table goes in as a slice of maps and comes back the same way; a structure
// is a map. Passing a struct works too, by field name.
func ExampleConnection_Call_table() {
	var conn *gorfc.Connection // from ConnectionFromParams

	result, err := conn.Call("STFC_STRUCTURE", map[string]interface{}{
		"IMPORTSTRUCT": map[string]interface{}{"RFCCHAR4": "ABCD", "RFCINT4": 42},
		"RFCTABLE": []interface{}{
			map[string]interface{}{"RFCCHAR4": "ROW1"},
			map[string]interface{}{"RFCCHAR4": "ROW2"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, row := range result["RFCTABLE"].([]interface{}) {
		fmt.Println(row.(map[string]interface{})["RFCCHAR4"])
	}
}

// An error from ABAP is an *gorfc.RfcError and carries the message fields the
// server sent; an error this binding produced is a *gorfc.GoRfcError.
func ExampleRfcError() {
	var conn *gorfc.Connection // from ConnectionFromParams

	if _, err := conn.Call("Z_NO_SUCH_MODULE", nil); err != nil {
		if rfcErr, ok := err.(*gorfc.RfcError); ok {
			fmt.Println("ABAP said:", rfcErr.ErrorInfo.Key)
		}
	}
}
