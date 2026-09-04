package gorfc

import "testing"

// The wire is UTF-16 throughout, so a round trip has to survive more than
// ASCII: a two-byte character, a three-byte one, and a surrogate pair.
func TestStringRoundTrip(t *testing.T) {
	for _, want := range []string{
		"",
		"STFC_CONNECTION",
		"Grüße aus Walldorf",
		"日本語テキスト",
		"emoji: 🐘",
		"trailing spaces   ",
	} {
		got, err := ucRoundTrip(want, false)
		if err != nil {
			t.Fatalf("round trip of %q: %v", want, err)
		}
		if got != want {
			t.Errorf("round trip of %q gave %q", want, got)
		}
	}
}

// rstrip removes trailing blanks and NULs, and nothing else. A string whose
// content ends in a space is indistinguishable from a padded fixed-width
// field once it is on the wire, which is why this is a per-connection choice.
func TestStringRoundTripStripped(t *testing.T) {
	got, err := ucRoundTrip("padded   ", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "padded" {
		t.Errorf("stripped round trip gave %q, want %q", got, "padded")
	}
}

// UTCLONG is rendered as SAP's gorfc renders it: ISO with a T, and a dot for
// the decimal separator. The reference indexed position 19 unconditionally,
// so a short value -- ABAP's initial UTCLONG among them -- panicked.
func TestUtclongLikeReference(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"2026-08-30 23:59:58.1234567", "2026-08-30T23:59:58.1234567"},
		{"2026-08-30T23:59:58,1234567", "2026-08-30T23:59:58.1234567"},
		{"2026-08-30T23:59:58.1234567", "2026-08-30T23:59:58.1234567"},
		// Too short to carry the separators: passed through, not indexed.
		{"", ""},
		{"2026-08-30", "2026-08-30"},
		{"2026-08-30 23:59:5", "2026-08-30 23:59:5"},
	} {
		if got := utclongLikeReference(c.in); got != c.want {
			t.Errorf("utclongLikeReference(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
