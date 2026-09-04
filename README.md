# erpl-gorfc

A drop-in replacement for SAP's archived [gorfc](https://github.com/SAP-archive/gorfc),
built on `erpl-proto` — a pure-Rust implementation of the RFC protocol.

```go
import "github.com/DataZooDE/erpl-gorfc/gorfc"

conn, err := gorfc.ConnectionFromParams(gorfc.ConnectionParameters{
    "ashost": "sap.example.com", "sysnr": "00", "client": "001",
    "user": "RFCUSER", "passwd": "...", "lang": "EN",
})
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

result, err := conn.Call("STFC_CONNECTION", map[string]interface{}{"REQUTEXT": "hello"})
fmt.Println(result["ECHOTEXT"])
```

## Why

gorfc is archived and is cgo over the **SAP NW RFC SDK** — a licence-gated
download of roughly 30 MB of C libraries and ICU that may not be redistributed.
`go get github.com/sap/gorfc` never produced a working build on its own: you
first had to register with SAP, download the SDK, unpack it to the exact path
baked into the source, and set `CGO_CFLAGS`/`CGO_LDFLAGS`. Windows was not
supported at all.

`erpl-gorfc` has no SDK dependency and no ICU. The native code is a static
archive that ships **inside the module**, one per platform, and links into your
binary. Nothing to download, nothing to install, nothing to find at run time.

## Requirements

- Go 1.21 or later
- **`CGO_ENABLED=1` and a C compiler.** cgo is what makes static linking of the
  native code possible at all; there is no pure-Go path. On Linux and macOS the
  system compiler is enough. On Windows you need
  [MinGW-w64](https://www.mingw-w64.org/) — the archive is built for the MinGW
  ABI, not MSVC.
- On Alpine and other musl systems, build with **`-tags musl`**. Go cannot tell
  glibc from musl on its own, and a glibc archive will not link there.

Supported platforms: linux/amd64, linux/arm64 (glibc and musl), darwin/amd64,
darwin/arm64, windows/amd64.

## Performance

Measured on the a4h trial over loopback, 2026-09-04 (`go test -bench . -benchtime 5x`):

| Operation | Cost | Allocations |
|---|---|---|
| `Ping()` | 207 µs | — |
| `Call("STFC_CONNECTION")` | 454 µs | — |
| `RFC_READ_TABLE`, 100 rows × 3 columns | 2.6 ms | 0.6 MB |
| `RFC_READ_TABLE`, 1 000 rows | 15 ms | 4.6 MB |
| `RFC_READ_TABLE`, 10 000 rows | 133 ms | 45 MB |

This binding reads values through SAP's own C API, which is one call per field,
so a table crosses the cgo boundary rows × columns times. Two things are worth
knowing about that.

The crossing is not what a call costs. 200 twelve-field rows add 7.5 ms over a
one-row call — about 3 µs per field, against a cgo crossing of roughly 50–100 ns.
The time is in the round trip and the per-field value handling on both sides.

What *was* expensive is re-deriving the row layout. The reference called
`RfcGetFieldCount` and `RfcGetFieldDescByIndex` and decoded every field name
again for every row; resolving them once per table took a 10 000-row read from
149 ms and 82 MB to 133 ms and 45 MB. What remains is one buffer pair per field
per row, sized from the field's declared width. If that ever matters, the fix is
a bulk row accessor on the shim, not a different binding technique.

A whole table is materialised before `Call` returns, so a read of hundreds of
thousands of rows is bounded by memory rather than by anything here. Use
`ROWCOUNT`.

## Concurrency

A `Connection` must not be used from more than one goroutine at a time. One
call occupies the conversation until the server answers it, so two goroutines
sharing a connection would interleave on the same socket. Give each goroutine
its own connection, or pool them — the reference behaved the same way, and
pooling is the usual arrangement.

## What is and is not supported

See [MIGRATION.md](MIGRATION.md) — one page telling you whether your code will
run, including the deliberate non-goals and the handful of places behaviour
differs.

## Version

`v0.YYYY.MMDD` — `v0.2026.904` was cut on 2026-09-04. A release tells you when
it was cut, which for a library tracking someone else's wire protocol is the
useful fact.

The shape is dictated by Go. The other erpl projects version as `YYYY.M.D`, but
Go modules are semver and read the first component as the *major* version: any
major above 1 requires the import path to carry a `/vN` suffix, so `v2026.9.4`
would mean importing `github.com/DataZooDE/erpl-gorfc/v2026` — and a new import
path every January. Folding the date into the minor and patch keeps the major
at zero, keeps one import path forever, and still sorts correctly year over
year.

The consequence to know: a second release on one day has no room in the scheme
and takes the following day's number.

## Licence

**BUSL-1.1**, inherited from `erpl-proto`, and a change from gorfc's Apache-2.0.
If you are embedding this package in a product you distribute, read the LICENSE
at the root of the erpl-proto distribution and the Additional Use Grant before
you ship: the terms are not the ones gorfc came with, and that difference is
deliberate rather than an oversight. For a package-specific grant, contact
DataZoo.

The Go binding layer is adapted from SAP's gorfc under Apache-2.0; see
[NOTICE](NOTICE).

## Issues

Bugs and compatibility gaps: https://github.com/DataZooDE/erpl-gorfc/issues

## Source, issues and contributions

This repository is the published module: the Go binding, and the prebuilt
static archives it links. It is also the issue tracker -- please report bugs
and compatibility gaps here, ideally with the function module and the values
involved.

The RFC protocol implementation underneath is `erpl-proto`, which is not open
source; it reaches you only as the compiled archives under `lib/`. The binding
itself is what you see here.

## Compatibility

Compatibility with SAP's gorfc is not asserted, it is measured.
`examples/run_both.sh` builds one program twice -- once against SAP's gorfc and
the NW RFC SDK, once against this package -- and diffs the output. The last run
before release reported no differences across 42 lines, covering connection
attributes, the echo and structure round trips, a table, dates and times,
UTCLONG read and round-tripped, the function description, and an ABAP error's
key.
