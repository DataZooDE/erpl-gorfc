# Migrating from SAP gorfc

One page: what runs unchanged, what is deliberately absent, and where behaviour
differs. If your code opens connections and calls function modules, you can stop
after the first table.

## Supported today

| gorfc surface | Status |
|---|---|
| `ConnectionFromParams(ConnectionParameters{...})` | works |
| `ConnectionFromDest(dest)` | works — `sapnwrfc.ini` is parsed by this package |
| `conn.Call(name, map[string]interface{}{...})` | works |
| `conn.Call(name, someStruct)` — parameters as struct fields | works |
| `conn.Open()`, `Close()`, `Reopen()`, `Ping()`, `Alive()` | works |
| `conn.GetConnectionAttributes()` | works |
| `conn.GetFunctionDescription(name)` | works |
| `conn.RStrip(bool)`, `conn.ReturnImportParams(bool)` | works, chainable |
| `RfcError`, `GoRfcError`, `.ErrorInfo` fields | works |
| `FunctionDescription`, `ParameterDescription`, `TypeDescription`, `FieldDescription` | works |
| `GetNWRFCLibVersion()` | works — see below |
| `ashost`/`sysnr`/`client`/`user`/`passwd`/`lang` | works |
| `mshost`/`msserv`/`group` (load balancing) | works |
| `saprouter` | works |
| SNC parameter family | works |

## Type mapping

Unchanged from gorfc:

| ABAP | Go |
|---|---|
| CHAR, NUMC, STRING | `string` |
| BYTE (RAW), XSTRING | `[]byte` |
| INT1 | `uint8` |
| INT2 | `int16` |
| INT4 | `int32` |
| INT8 | `int64` |
| FLOAT | `float64` |
| BCD (packed), DECFLOAT16, DECFLOAT34 | `string` |
| DATE, TIME | `time.Time` |
| UTCLONG | `string` |
| STRUCTURE | `map[string]interface{}` |
| TABLE | `[]interface{}` of `map[string]interface{}` |

Decimals are `string` deliberately, as in gorfc: a `DECFLOAT34` carries 34
significant digits and an `INT8` goes past 2^53, and neither survives a
`float64`. On input the same fields accept `string`, any integer type, or a
`float64`.

## Where behaviour differs

Each of these is a place the reference did something that could not be
reproduced without also reproducing a defect.

**A failed logon returns no connection.** `ConnectionFromParams` builds its
parameter list, and the reference assigned the result of each conversion to one
`err` that it checked only after the loop — so a failure on any parameter but
the last was overwritten and the connection was opened with an incomplete list.
Here the first failure is returned.

**Empty connection parameters are an error, not a panic.** The reference indexed
`connParams[0]` unconditionally, so `ConnectionFromParams(nil)` panicked with an
index-out-of-range before reaching the library.

**Strings filled into a call are freed.** The reference deferred
`C.free(unsafe.Pointer(cValue))` before `cValue` was assigned. A deferred call
evaluates its arguments immediately, so it always freed `nil`, and every string,
date, time and decimal sent to ABAP leaked for the life of the process.

**A dropped connection is closed.** The reference's finalizer freed the
parameter strings but left the connection open until the gateway timed it out.

**DATE and TIME accept a string as well as `time.Time`.** The reference asserted
`value.(time.Time)` and panicked on anything else, which made passing a date
straight from one call into the next a crash rather than a round trip. Reading
is unchanged: you get a `time.Time`, and ABAP's initial date still reads as
`nil`.

**A pointer to a struct or slice is accepted.** `conn.Call(name, &Params{...})`
and `"TABLE": &rows` are ordinary Go — every encoder in the standard library
takes a pointer — and the reference refused them, reporting that parameters
must be a map or a struct for something that is a struct. A nil pointer or nil
slice means "no value", which leaves the field at its ABAP initial.

**A value of the wrong Go type is reported, not sent.** The reference reached
for `reflect.Value.String()`, which returns `"<int Value>"` rather than failing,
so a wrongly typed value was sent to ABAP as that text.

**Errors from inside a structure or table are returned.** The reference assigned
each row's error to one variable and returned whatever the last row left there,
so a failure on any row but the last was lost and the call reported success.

**A short or blank DATE does not panic.** The reference tested `value[1]` to
detect a blank date, which indexed out of range whenever the field came back
shorter than two characters. Likewise a `UTCLONG` was reassembled by slicing at
fixed offsets, which panicked on the initial value.

**An INT8 past 2^31 is not truncated.** The reference sent INT1, INT2, INT4 and
INT8 all through `RfcSetInt`, which is 32 bits wide, so an `int64` beyond
2,147,483,647 arrived at ABAP as whatever the narrowing produced. Each width
now uses its own setter.

**A value too wide for its ABAP integer is refused.** 40000 into an INT2, or 300
into an INT1, was accepted and mangled downstream with no error anywhere. It is
now a `*GoRfcError` naming the range.

**An integer given as a string must be a whole integer.** The reference parsed
with `fmt.Sscanf("%d")`, which stops at the first character it cannot use and
reports success — so `"12abc"` reached ABAP as `12` and `"0x10"` as `0`.

**A DATE or TIME string must be exactly eight or six characters.** The library
reads a fixed width from the buffer, so a shorter string is a read past the end
of the buffer rather than a short date. An empty string, or `nil`, means the
ABAP initial value and sends nothing.

**A nil table row is an empty row, not a panic.** `[]interface{}{nil}` reached
`reflect.Value.Interface()` on a zero Value, which panics and takes the calling
process with it.

**An empty `[]byte` clears the field.** It is written with length zero rather
than skipped, which is what the reference did and what "clear this" means.

**`UTCLONG` comes back in the reference's spelling**, `2026-08-30T23:59:58.1234567`
— ISO, with a `T`, and a dot for the decimal separator. That is what SAP's gorfc
returns, verified byte-for-byte by `examples/run_both.sh`; this library's own
text form uses a space instead of the `T` (spec VAL-8), and the binding
normalises. What you read can be sent straight back. The reference reassembled
the value by indexing position 19 unconditionally, which panicked on the initial
UTCLONG; anything too short to carry the separators is passed through here.

**`TypeDescription.NucLength` is nominal.** It is reported as half the Unicode
row width. The field descriptors halve only their text fields and keep the byte
width of everything else, so for a structure holding an INT4, a RAW or a packed
field the fields do not sum to it. Size a narrow-layout row from the field
descriptors. Nothing in this package reads it; it is reported because the SDK's
signature has the out-parameter.

**A decimal is never sent in exponent form.** `%g`, which the reference used,
switches to exponents outside a middle range, so `0.0000001` went to ABAP as
`"1e-07"` — not a number to a packed field.

**A duplicate `DEST=` in `sapnwrfc.ini` resolves to the first block**, as the
SDK does.

**`GoRfcError` unwraps.** `errors.Is` and `errors.As` see the underlying error,
so "no `sapnwrfc.ini`" can be told from "destination not in the file" without
matching on message text.

**Connection attributes have no NUL padding.** They come out of fixed-width
buffers, and the reference returned the value followed by the rest of the
buffer as NULs whenever `RStrip(false)` was set.

**`GetNWRFCLibVersion()` reports this package's version**, in the SDK's shape.
There is no SDK to report on. Code that logs it keeps working; code comparing it
against an SDK patch level was reading a number that no longer describes
anything.

## Deliberate non-goals

- **`RfcSetIniPath` and the SDK's ini search order.** `ConnectionFromDest` works,
  but the file is found via `RFC_INI` or `sapnwrfc.ini` in the working
  directory, and nothing else. The SDK's wider search cannot be reproduced
  faithfully because there is no SDK to reproduce it from.
- **Tracing.** `RfcSetTraceDir` and the trace levels are accepted and ignored.
- **Timeouts and `context.Context`.** `Open`, `Call` and `Ping` block until the
  server answers or the connection breaks. There is no deadline, no
  cancellation, and no `CallContext`. gorfc had none either, and the underlying
  library does support an I/O timeout, so this is a gap to close rather than a
  limit — but it is a gap today, and a caller that needs one must manage it
  around the call.
- **Server mode, tRFC/qRFC units, and cancel.** gorfc had none of these. They
  exist in `erpl-proto` and in the Python binding, and they are the next thing
  to surface here; they are not in this release.
- **`sapnwrfc.ini` options other than connection parameters.**

## Build requirements that are new

Nothing about the API, but the build is different in one direction and easier in
the other. You no longer need the SAP NW RFC SDK, `CGO_CFLAGS`, `CGO_LDFLAGS`,
or a `LD_LIBRARY_PATH` at run time. You do still need `CGO_ENABLED=1` and a C
compiler, you need MinGW-w64 on Windows, and on Alpine you must build with
`-tags musl`. See the README.
