package gorfc

import "testing"

// The reference reports the SDK's version here. There is no SDK, so this
// reports erpl-proto's own -- see the doc comment on GetNWRFCLibVersion.
func TestGetNWRFCLibVersion(t *testing.T) {
	major, minor, patch := GetNWRFCLibVersion()
	if major == 0 && minor == 0 && patch == 0 {
		t.Fatalf("version reported as 0.0.0, want erpl-proto's own version")
	}
	t.Logf("erpl-gorfc reports version %d.%d.%d", major, minor, patch)
}
