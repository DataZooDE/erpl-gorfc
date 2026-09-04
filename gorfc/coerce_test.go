package gorfc

import (
	"math"
	"strings"
	"testing"
	"time"
)

// The library reads exactly eight characters for a DATE and six for a TIME,
// so a value of the wrong length has to be refused here -- passing it on is an
// out-of-bounds read of our own buffer.
func TestDateAndTimeCoercion(t *testing.T) {
	when := time.Date(2026, 9, 4, 13, 45, 59, 0, time.UTC)

	if got, err := asDate(when); err != nil || got != "20260904" {
		t.Errorf("asDate(time.Time) = %q, %v", got, err)
	}
	if got, err := asTime(when); err != nil || got != "134559" {
		t.Errorf("asTime(time.Time) = %q, %v", got, err)
	}
	if got, err := asDate("20260904"); err != nil || got != "20260904" {
		t.Errorf("asDate(string) = %q, %v", got, err)
	}
	// Empty means "leave it at the ABAP initial", and nothing is sent.
	if got, err := asDate(nil); err != nil || got != "" {
		t.Errorf("asDate(nil) = %q, %v", got, err)
	}
	// Anything else of the wrong length is refused here, because RfcSetDate
	// reads eight units from a buffer sized to the string we give it.
	for _, bad := range []string{"2026", "20260904 ", "2026-09-04", "0"} {
		if got, err := asDate(bad); err == nil {
			t.Errorf("asDate(%q) = %q, want an error", bad, got)
		}
	}
	for _, bad := range []string{"13", "134559 ", "13:45:59"} {
		if got, err := asTime(bad); err == nil {
			t.Errorf("asTime(%q) = %q, want an error", bad, got)
		}
	}
	if _, err := asDate(42); err == nil {
		t.Error("asDate(int) was accepted")
	}
	if _, err := asTime(42); err == nil {
		t.Error("asTime(int) was accepted")
	}
}

func TestNumericCoercion(t *testing.T) {
	for _, c := range []struct {
		in   interface{}
		want int64
	}{
		{int(7), 7}, {int8(7), 7}, {int64(-7), -7}, {uint16(7), 7},
		{float64(7), 7}, {"7", 7}, {" 7 ", 7},
	} {
		got, err := asInt(c.in)
		if err != nil || got != c.want {
			t.Errorf("asInt(%#v) = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
	// A float that is not whole is a lost value, not a rounding decision.
	if _, err := asInt(7.5); err == nil {
		t.Error("asInt(7.5) was accepted")
	}
	if _, err := asInt("not a number"); err == nil {
		t.Error("asInt of a non-numeric string was accepted")
	}
	// INT8 past 2^53 survives, which is the whole reason it is not a float.
	const big = int64(9007199254740993)
	if got, err := asInt(big); err != nil || got != big {
		t.Errorf("asInt(%d) = %d, %v", big, got, err)
	}
}

// Decimals travel as text so that no digit is lost. A float64 rendered with
// %g must not gain an exponent or lose precision for ordinary values.
func TestTextCoercion(t *testing.T) {
	for _, c := range []struct {
		in   interface{}
		want string
	}{
		{"abc", "abc"},
		{1.23456789, "1.23456789"},
		{int64(42), "42"},
		{uint8(7), "7"},
		{"1234567890123456789012345678901234", "1234567890123456789012345678901234"},
		{nil, ""},
		{true, "X"},
		{false, " "},
	} {
		if got := asText(c.in); got != c.want {
			t.Errorf("asText(%#v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A decimal must never reach ABAP in exponent form.
func TestDecimalTextIsNeverExponential(t *testing.T) {
	for _, c := range []struct {
		in   interface{}
		want string
	}{
		{1e-7, "0.0000001"},
		{-1e-7, "-0.0000001"},
		{1e21, "1000000000000000000000"},
		{1.23456789, "1.23456789"},
		{0.0, "0"},
		{"1234567890123456789012345678901234", "1234567890123456789012345678901234"},
		{int64(42), "42"},
	} {
		if got := asDecimalText(c.in); got != c.want {
			t.Errorf("asDecimalText(%#v) = %q, want %q", c.in, got, c.want)
		}
		if strings.ContainsAny(asDecimalText(c.in), "eE") {
			t.Errorf("asDecimalText(%#v) = %q, which is exponent form", c.in, asDecimalText(c.in))
		}
	}
}

func TestBytesCoercion(t *testing.T) {
	if got, err := asBytes([]byte{1, 2}); err != nil || len(got) != 2 {
		t.Errorf("asBytes([]byte) = % x, %v", got, err)
	}
	if got, err := asBytes("ab"); err != nil || string(got) != "ab" {
		t.Errorf("asBytes(string) = % x, %v", got, err)
	}
	if _, err := asBytes(42); err == nil {
		t.Error("asBytes(int) was accepted")
	}
}

// Sscanf("%d") stops at the first character it cannot use and reports
// success, so the reference sent "12abc" to ABAP as 12. Every one of these
// must be refused, not truncated.
func TestAsIntRejectsTrailingGarbage(t *testing.T) {
	for _, bad := range []string{"12abc", "12.3", "0x10", "", " ", "1 2", "+-3", "12,5"} {
		if got, err := asInt(bad); err == nil {
			t.Errorf("asInt(%q) = %d, want an error", bad, got)
		}
	}
	for _, good := range []struct {
		in   string
		want int64
	}{{"12", 12}, {"-12", -12}, {"  7  ", 7}, {"+7", 7}, {"0", 0}} {
		if got, err := asInt(good.in); err != nil || got != good.want {
			t.Errorf("asInt(%q) = %d, %v; want %d", good.in, got, err, good.want)
		}
	}
}

// A uint64 past MaxInt64 wraps negative when converted, and a wrapped value
// is indistinguishable from one the caller meant.
func TestAsIntRejectsUnsignedOverflow(t *testing.T) {
	if got, err := asInt(uint64(1) << 63); err == nil {
		t.Errorf("asInt(1<<63) = %d, want an error", got)
	}
	if got, err := asInt(uint64(math.MaxInt64)); err != nil || got != math.MaxInt64 {
		t.Errorf("asInt(MaxInt64) = %d, %v", got, err)
	}
}

func TestAsIntRejectsUnrepresentableFloats(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1e30, 7.5} {
		if got, err := asInt(bad); err == nil {
			t.Errorf("asInt(%v) = %d, want an error", bad, got)
		}
	}
}

// The width checks are what stop an INT2 quietly taking 40000.
func TestIntInRange(t *testing.T) {
	if err := intInRange(40000, math.MinInt16, math.MaxInt16, "INT2"); err == nil {
		t.Error("40000 was accepted for an INT2")
	}
	if err := intInRange(-1, 0, 255, "INT1"); err == nil {
		t.Error("-1 was accepted for an INT1")
	}
	if err := intInRange(255, 0, 255, "INT1"); err != nil {
		t.Errorf("255 was refused for an INT1: %v", err)
	}
	if err := intInRange(math.MaxInt32, math.MinInt32, math.MaxInt32, "INT4"); err != nil {
		t.Errorf("MaxInt32 was refused for an INT4: %v", err)
	}
}
