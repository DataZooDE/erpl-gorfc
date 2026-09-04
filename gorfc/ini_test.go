package gorfc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleIni = `# a comment
DEST=MME
USER=demo
PASSWD=welcome
ASHOST=10.68.110.51
SYSNR=00
CLIENT=620
LANG=EN

DEST=QM7
USER=NWRFCTEST
PASSWD=Welcome1
ASHOST=ldciqm7.wdf.sap.corp
SYSNR=20
GROUP=PUBLIC
CLIENT=002
LANG=EN
`

func writeIni(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sapnwrfc.ini")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RFC_INI", path)
	return path
}

// A destination's parameters come back lowercased, which is the spelling
// ConnectionFromParams and the library both use.
func TestDestinationParams(t *testing.T) {
	writeIni(t, sampleIni)

	params, err := destinationParams("QM7")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"dest": "QM7", "user": "NWRFCTEST", "passwd": "Welcome1",
		"ashost": "ldciqm7.wdf.sap.corp", "sysnr": "20", "group": "PUBLIC",
		"client": "002", "lang": "EN",
	}
	if len(params) != len(want) {
		t.Fatalf("got %d parameters %v, want %d", len(params), params, len(want))
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("%s = %q, want %q", k, params[k], v)
		}
	}
}

// The block boundary is the next DEST=, not a blank line: the file is allowed
// to have neither blank lines nor a trailing newline.
func TestDestinationParamsWithoutBlankLines(t *testing.T) {
	writeIni(t, "DEST=A\nASHOST=a\nDEST=B\nASHOST=b")

	params, err := destinationParams("B")
	if err != nil {
		t.Fatal(err)
	}
	if params["ashost"] != "b" {
		t.Errorf("ashost = %q, want %q", params["ashost"], "b")
	}
	if _, leaked := params["dest"]; !leaked || params["dest"] != "B" {
		t.Errorf("dest = %q, want B", params["dest"])
	}
}

// An unknown destination names itself and the file it was looked for in.
func TestDestinationParamsUnknown(t *testing.T) {
	path := writeIni(t, sampleIni)

	_, err := destinationParams("NOPE")
	if err == nil {
		t.Fatal("an unknown destination was accepted")
	}
	if _, ok := err.(*GoRfcError); !ok {
		t.Fatalf("want *GoRfcError, got %T", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "NOPE") || !strings.Contains(msg, path) {
		t.Errorf("error %q names neither the destination nor the file", msg)
	}
}

// No ini file at all is the common case for a user who never had an SDK, and
// the message has to say what was looked for rather than "not found".
func TestDestinationParamsNoFile(t *testing.T) {
	t.Setenv("RFC_INI", filepath.Join(t.TempDir(), "absent.ini"))

	_, err := destinationParams("MME")
	if err == nil {
		t.Fatal("a missing ini file was accepted")
	}
	if !strings.Contains(err.Error(), "absent.ini") {
		t.Errorf("error %q does not name the file it looked for", err.Error())
	}
}
