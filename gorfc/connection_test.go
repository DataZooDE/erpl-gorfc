package gorfc

import (
	"strings"
	"testing"
)

// The reference indexes connParams[0] unconditionally, so an empty parameter
// set panics with an index-out-of-range before it ever reaches the library.
// A connection with no parameters is a caller error, and it is reported.
func TestConnectionFromParamsRejectsEmpty(t *testing.T) {
	conn, err := ConnectionFromParams(ConnectionParameters{})
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Fatal("empty connection parameters accepted, want an error")
	}
	if conn != nil {
		t.Errorf("a failed open returned a connection: %#v", conn)
	}
	if _, ok := err.(*GoRfcError); !ok {
		t.Errorf("want *GoRfcError, got %T: %v", err, err)
	}
}

// A logon that cannot succeed must report which parameters it tried with,
// and must not leave a half-open connection behind.
func TestConnectionFromParamsUnreachable(t *testing.T) {
	conn, err := ConnectionFromParams(ConnectionParameters{
		"ashost": "127.0.0.1", "sysnr": "99", "client": "001",
		"user": "NOBODY", "passwd": "wrong", "lang": "EN",
	})
	if err == nil {
		conn.Close()
		t.Fatal("connecting to a dead gateway succeeded")
	}
	if conn != nil {
		t.Errorf("a failed open returned a connection: %#v", conn)
	}
	if _, ok := err.(*RfcError); !ok {
		t.Fatalf("want *RfcError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "NWRFC SDK error") {
		t.Errorf("error text %q does not carry the reference's prefix", err.Error())
	}
}

// RStrip and ReturnImportParams are chainable setters returning the receiver,
// which is what makes conn.RStrip(false).Call(...) work.
func TestChainableSetters(t *testing.T) {
	conn := &Connection{}
	if conn.RStrip(false) != conn || conn.ReturnImportParams(true) != conn {
		t.Fatal("setters must return the receiver")
	}
	if conn.rstrip != false || conn.returnImportParams != true {
		t.Errorf("setters did not take effect: %#v", conn)
	}
	if conn.Alive() {
		t.Error("a connection that was never opened reports itself alive")
	}
}

// Call on a closed connection is refused by the binding, before any cgo.
func TestCallRequiresOpenConnection(t *testing.T) {
	conn := &Connection{}
	if _, err := conn.Call("STFC_CONNECTION", map[string]interface{}{}); err == nil {
		t.Fatal("Call on a closed connection succeeded")
	} else if _, ok := err.(*GoRfcError); !ok {
		t.Errorf("want *GoRfcError, got %T: %v", err, err)
	}
}
