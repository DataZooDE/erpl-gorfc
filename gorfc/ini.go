//go:build cgo

package gorfc

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// iniPath is where a destination is looked up: the file named by RFC_INI, or
// sapnwrfc.ini in the working directory.
//
// The reference reads this file inside the SDK, and configures the search with
// RfcSetIniPath. There is no SDK here, so the file is parsed by this package;
// RfcSetIniPath is accepted and ignored by the library underneath, which is
// why the search is deliberately the small, documented one rather than the
// SDK's full order.
func iniPath() string {
	if p := os.Getenv("RFC_INI"); p != "" {
		return p
	}
	return "sapnwrfc.ini"
}

// destinationParams reads one destination's parameters from sapnwrfc.ini.
//
// The file is a sequence of blocks, each opened by a DEST= line and running to
// the next one. Keys are returned lowercased, which is the spelling the
// library uses, and DEST itself is kept -- it is what the system reports back
// as the connection's destination.
func destinationParams(dest string) (ConnectionParameters, error) {
	path := iniPath()
	file, err := os.Open(path)
	if err != nil {
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			abs = path
		}
		return nil, goRfcError(fmt.Sprintf("Could not read destination %q from %s", dest, abs), err)
	}
	defer file.Close()

	var current ConnectionParameters
	var found ConnectionParameters

	scanner := bufio.NewScanner(file)
	// A generated ini can carry a long SNC_LIB or saprouter string, and past
	// bufio's 64 KiB default the line comes back as a generic read failure
	// rather than as the long line it is.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, ";") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, goRfcError(fmt.Sprintf("%s:%d: %q is not a KEY=VALUE line", path, line, text), nil)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		if key == "dest" {
			current = ConnectionParameters{"dest": value}
			// The first definition wins, as the SDK's does. Taking the last
			// would make a file with a duplicated destination behave
			// differently here than under the library being replaced.
			if value == dest && found == nil {
				found = current
			}
			continue
		}
		if current == nil {
			return nil, goRfcError(fmt.Sprintf("%s:%d: %q appears before any DEST= line", path, line, key), nil)
		}
		current[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, goRfcError(fmt.Sprintf("Could not read %s", path), err)
	}

	if found == nil {
		return nil, goRfcError(fmt.Sprintf("Destination %q is not defined in %s", dest, path), nil)
	}
	return found, nil
}
