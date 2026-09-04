//go:build cgo

package gorfc

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// The coercions below are what the reference did with reflect.Value.String()
// and unchecked type assertions. Value.String() on a non-string returns
// "<int Value>" rather than failing, so a value of the wrong type was sent to
// ABAP as that text; a time.Time assertion on a string panicked. Each of these
// either converts or says which parameter it could not convert.

// asText renders a value for a character, numeric-text or decimal field.
func asText(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case time.Time:
		return v.Format("2006-01-02 15:04:05.0000000")
	case fmt.Stringer:
		return v.String()
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		// %g is the shortest decimal that reads back as the same float64.
		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return fmt.Sprintf("%d", rv.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return fmt.Sprintf("%d", rv.Uint())
		case reflect.Bool:
			if rv.Bool() {
				return "X"
			}
			return " "
		default:
			return fmt.Sprintf("%v", value)
		}
	}
}

// asDecimalText renders a value for a packed or decimal-float field.
//
// Never exponent form: ABAP takes a decimal literal there, and %g switches to
// exponents outside a middle range -- so 0.0000001 went across as "1e-07" and
// 1e21 as "1e+21", neither of which is a number to a packed field. 'f' with a
// precision of -1 gives the shortest decimal that reads back as the same
// float64, spelled out.
func asDecimalText(value interface{}) string {
	switch v := value.(type) {
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return asText(value)
	}
}

// asInt renders a value for an integer field.
func asInt(value interface{}) (int64, error) {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Past MaxInt64 the conversion wraps negative, and a wrapped value is
		// indistinguishable from one the caller meant.
		if u := rv.Uint(); u > math.MaxInt64 {
			return 0, goRfcError(fmt.Sprintf("GO %d is past the largest ABAP integer, %d", u, int64(math.MaxInt64)), nil)
		}
		return int64(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) || f > math.MaxInt64 || f < math.MinInt64 || f != math.Trunc(f) {
			return 0, goRfcError(fmt.Sprintf("GO %v passed to an ABAP integer parameter is not a whole number in range", value), nil)
		}
		return int64(f), nil
	case reflect.String:
		// ParseInt, not Sscanf: Sscanf("%d") stops at the first character it
		// cannot use and reports success, so "12abc" arrived at ABAP as 12 and
		// "0x10" as 0.
		text := strings.TrimSpace(rv.String())
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, goRfcError(fmt.Sprintf("GO string %q passed to an ABAP integer parameter", rv.String()), err)
		}
		return n, nil
	default:
		return 0, goRfcError(fmt.Sprintf("GO %s passed to an ABAP integer parameter", reflect.TypeOf(value)), nil)
	}
}

// intInRange checks a value against the width of the ABAP integer it is going
// into. Without this an INT2 quietly takes 40000 and ABAP sees whatever the
// narrowing produced.
func intInRange(n int64, low, high int64, what string) error {
	if n < low || n > high {
		return goRfcError(fmt.Sprintf("GO %d is out of range for an ABAP %s (%d..%d)", n, what, low, high), nil)
	}
	return nil
}

// asBytes renders a value for a RAW or XSTRING field.
func asBytes(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
			return rv.Bytes(), nil
		}
		return nil, goRfcError(fmt.Sprintf("GO %s passed to an ABAP binary parameter, expected []byte", reflect.TypeOf(value)), nil)
	}
}

// asDate renders a value for a DATE field, as ABAP's YYYYMMDD. An empty
// result means "send nothing", which leaves the field at its ABAP initial.
//
// The reference asserted time.Time and panicked on anything else, which made
// passing a date through unchanged -- reading one from a call and sending it
// back to another -- a crash rather than a round trip. Accepting the string
// form is what fixes that, and it is also what makes the length check below
// load-bearing: RfcSetDate reads exactly eight UTF-16 units from the buffer we
// hand it, with no length of its own, so a shorter string is a read past the
// end of our own allocation rather than a short date.
func asDate(value interface{}) (string, error) {
	switch v := value.(type) {
	case time.Time:
		return v.Format("20060102"), nil
	case string:
		return exactly(v, 8, "DATE", "YYYYMMDD")
	case nil:
		return "", nil
	default:
		return "", goRfcError(fmt.Sprintf("GO %s passed to an ABAP DATE parameter, expected time.Time or string", reflect.TypeOf(value)), nil)
	}
}

// asTime renders a value for a TIME field, as ABAP's HHMMSS. As asDate:
// RfcSetTime reads exactly six units.
func asTime(value interface{}) (string, error) {
	switch v := value.(type) {
	case time.Time:
		return v.Format("150405"), nil
	case string:
		return exactly(v, 6, "TIME", "HHMMSS")
	case nil:
		return "", nil
	default:
		return "", goRfcError(fmt.Sprintf("GO %s passed to an ABAP TIME parameter, expected time.Time or string", reflect.TypeOf(value)), nil)
	}
}

// exactly accepts a string of exactly n characters, or an empty one meaning
// the ABAP initial value. Anything else is refused: the library reads a fixed
// width from the buffer, so a wrong length is a memory error, not a data one.
func exactly(text string, n int, what, shape string) (string, error) {
	if text == "" {
		return "", nil
	}
	if len(text) != n {
		return "", goRfcError(fmt.Sprintf("%q is not an ABAP %s: want %d characters, %s", text, what, n, shape), nil)
	}
	return text, nil
}
