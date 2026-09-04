package gorfc

/*
#include <sapnwrfc.h>
#include <stdlib.h>
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// Every cgo call that consumes conn.handle is followed by runtime.KeepAlive.
//
// Passing conn.handle to C does not keep conn reachable: the argument is a
// plain word copied out of it, and the collector may decide conn is dead the
// moment its last Go use is past. The finalizer would then close the
// connection while the call it was passed to is still using the handle. This
// is the one hazard of attaching a finalizer to a type that hands a raw handle
// to C, and it does not show up in testing -- it needs the collector to run in
// the window.

// ConnectionParameters are the logon parameters, by their RFC names --
// ashost, sysnr, client, user, passwd, lang, and the load-balancing, router
// and SNC families.
type ConnectionParameters map[string]string

// Connection is a client connection to an SAP system.
//
// A Connection is not safe for concurrent use. One call occupies the
// conversation until the server answers it, so two goroutines calling on one
// Connection would interleave on the same socket; the fields that track
// whether it is open are equally unguarded. Give each goroutine its own
// Connection, or serialise access yourself. This matches the library being
// replaced, and it is why a pool is the usual arrangement.
//
// A Connection that is dropped without Close is closed by a finalizer, which
// also frees the parameter strings it holds. That is a backstop rather than a
// plan: it runs whenever the collector gets to it, and until then the server
// keeps the conversation open.
type Connection struct {
	handle             C.RFC_CONNECTION_HANDLE
	rstrip             bool
	returnImportParams bool
	alive              bool
	paramCount         C.uint
	connParams         []C.RFC_CONNECTION_PARAMETER
}

// connectionFinalizer releases what the garbage collector cannot: the UTF-16
// parameter strings, and the connection itself if the caller never closed it.
// The reference freed the strings only, so a dropped connection stayed open on
// the server until the gateway timed it out.
func connectionFinalizer(conn *Connection) {
	if conn.alive {
		conn.alive = false
		// Closing is a Deallocate round trip to the gateway. The runtime runs
		// finalizers on one goroutine, so doing it here would stall every
		// other finalizer in the process -- files and sockets included --
		// behind an unresponsive peer.
		handle := conn.handle
		go func() {
			C.RfcCloseConnection(handle, nil)
		}()
	}
	conn.freeParams()
}

func (conn *Connection) freeParams() {
	for i := range conn.connParams {
		C.free(unsafe.Pointer(conn.connParams[i].name))
		C.free(unsafe.Pointer(conn.connParams[i].value))
		conn.connParams[i].name = nil
		conn.connParams[i].value = nil
	}
	conn.connParams = nil
	conn.paramCount = 0
}

// ConnectionFromParams opens a connection with the given parameters. It
// returns the open connection, or nil and the error that stopped it.
func ConnectionFromParams(connectionParams ConnectionParameters) (*Connection, error) {
	if len(connectionParams) == 0 {
		// The reference indexes connParams[0] unconditionally, so this
		// panicked with an index-out-of-range before reaching the library.
		return nil, goRfcError("Connection parameters are required", nil)
	}

	conn := new(Connection)
	conn.rstrip = true
	conn.returnImportParams = false
	conn.alive = false
	conn.paramCount = C.uint(len(connectionParams))
	conn.connParams = make([]C.RFC_CONNECTION_PARAMETER, conn.paramCount)
	runtime.SetFinalizer(conn, connectionFinalizer)

	i := 0
	for name, value := range connectionParams {
		// Each failure frees what was allocated so far. The reference
		// assigned err twice per iteration and checked it only after the
		// loop, so an early failure was overwritten by a later success and
		// the connection was opened with a parameter list full of holes.
		cName, err := fillString(name)
		if err != nil {
			conn.freeParams()
			return nil, err
		}
		cValue, err := fillString(value)
		if err != nil {
			C.free(unsafe.Pointer(cName))
			conn.freeParams()
			return nil, err
		}
		conn.connParams[i].name = cName
		conn.connParams[i].value = cValue
		i++
	}

	if err := conn.Open(); err != nil {
		conn.freeParams()
		return nil, err
	}
	return conn, nil
}

// ConnectionFromDest opens a connection to a destination defined in
// sapnwrfc.ini. See iniPath for where that file is looked for.
func ConnectionFromDest(dest string) (*Connection, error) {
	params, err := destinationParams(dest)
	if err != nil {
		return nil, err
	}
	return ConnectionFromParams(params)
}

// RStrip controls whether trailing blanks are removed from strings returned by
// a call. It defaults to true, and returns the connection so it can be
// chained.
func (conn *Connection) RStrip(rstrip bool) *Connection {
	conn.rstrip = rstrip
	return conn
}

// ReturnImportParams controls whether a call's result also carries the import
// parameters that were sent. It defaults to false, and returns the connection
// so it can be chained.
func (conn *Connection) ReturnImportParams(returnImportParams bool) *Connection {
	conn.returnImportParams = returnImportParams
	return conn
}

// Alive reports whether the connection is open.
func (conn *Connection) Alive() bool {
	return conn.alive
}

// Close closes the connection.
func (conn *Connection) Close() (err error) {
	var errorInfo C.RFC_ERROR_INFO
	if conn.alive {
		conn.alive = false
		rc := C.RfcCloseConnection(conn.handle, &errorInfo)
		runtime.KeepAlive(conn)
		conn.handle = nil
		if rc != C.RFC_OK {
			return rfcError(errorInfo, "Connection could not be closed")
		}
	}
	return
}

// Open opens the connection.
func (conn *Connection) Open() (err error) {
	if len(conn.connParams) == 0 {
		return goRfcError("Connection parameters are required", nil)
	}
	// Opening an open connection overwrote the handle and left the previous
	// conversation on the server until the gateway timed it out -- one leaked
	// per iteration of a reconnect loop. Reopen is the way to get a fresh one.
	if conn.alive {
		return nil
	}
	var errorInfo C.RFC_ERROR_INFO
	conn.handle = C.RfcOpenConnection(&conn.connParams[0], conn.paramCount, &errorInfo)
	runtime.KeepAlive(conn)
	if conn.handle == nil || errorInfo.code != C.RFC_OK {
		conn.handle = nil
		return rfcError(errorInfo, "Connection could not be opened")
	}
	conn.alive = true
	return
}

// Reopen closes the connection and opens it again.
func (conn *Connection) Reopen() (err error) {
	if err = conn.Close(); err != nil {
		return
	}
	return conn.Open()
}

// Ping sends an RFC_PING to the server, opening the connection first if it is
// not open.
func (conn *Connection) Ping() (err error) {
	if !conn.alive {
		if err = conn.Open(); err != nil {
			return
		}
	}
	var errorInfo C.RFC_ERROR_INFO
	rc := C.RfcPing(conn.handle, &errorInfo)
	runtime.KeepAlive(conn)
	if rc != C.RFC_OK {
		return rfcError(errorInfo, "Server could not be pinged")
	}
	return
}

// GetConnectionAttributes returns what the system reported about this
// connection at logon.
func (conn *Connection) GetConnectionAttributes() (connAttr ConnectionAttributes, err error) {
	if !conn.alive {
		return nil, goRfcError("GetConnectionAttributes() requires an open connection", nil)
	}
	var errorInfo C.RFC_ERROR_INFO
	var attributes C.RFC_ATTRIBUTES

	rc := C.RfcGetConnectionAttributes(conn.handle, &attributes, &errorInfo)
	runtime.KeepAlive(conn)
	if rc != C.RFC_OK || errorInfo.code != C.RFC_OK {
		return nil, rfcError(errorInfo, "Could not get connection attributes")
	}
	return wrapConnectionAttributes(attributes, conn.rstrip)
}
