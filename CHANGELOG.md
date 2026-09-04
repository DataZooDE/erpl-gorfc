# Changelog

All notable changes to `erpl-gorfc`. Versions are `v0.YYYY.MMDD` — the erpl
projects' calendar versioning, folded into what Go modules accept. See the
README for why the shape is that and not `YYYY.M.D`.

## v0.2026.904 — 2026-09-04

First release. A drop-in replacement for SAP's archived gorfc, built as a
cgo binding over `crates/erpl-proto-nwrfc` — the same C API gorfc itself was
written against — with the native code linked statically from an archive
committed per platform, so `go get` needs neither a Rust toolchain nor the SAP
NW RFC SDK.

- Client connections: `ConnectionFromParams`, `ConnectionFromDest`, `Open`,
  `Close`, `Reopen`, `Ping`, `Alive`, `GetConnectionAttributes`,
  `GetFunctionDescription`, `Call`, `RStrip`, `ReturnImportParams`,
  `GetNWRFCLibVersion`, and the `RfcError`/`GoRfcError` types.
- `sapnwrfc.ini` is parsed by the binding, so `ConnectionFromDest` works without
  an SDK.
- Four entry points added to the shim's C ABI, which gorfc uses and which were
  `RFC_NOT_SUPPORTED` stubs: `RfcGetParameterDescByName`,
  `RfcGetFieldDescByName`, `RfcGetTypeLength`, `RfcGetFunctionName`. Each is
  defined as a scan over the matching by-index entry point, so the two can never
  disagree.
- Seven platform archives: linux amd64/arm64 in glibc and musl flavours, darwin
  amd64/arm64, windows amd64 (MinGW).

### Fixes to the shim, found by this binding

A second consumer of `crates/erpl-proto-nwrfc` found three defects that the
first one could not, because the Python binding reaches erpl-proto's Rust API
directly and never crosses the C ABI:

- **`UTCLONG` was reported as `RFCTYPE_CHAR`**, so the descriptor gave the
  field's eight bytes as eight characters against a twenty-seven character
  value, and every `RfcGetChars` on one answered `RFC_BUFFER_TOO_SMALL`. No
  consumer of the shim, erpl included, could read a UTCLONG.
- **`RfcGetConnectionAttributes` put the ISO language in `language`**, where
  the reference puts SAP's one-character code, and filled neither
  `isoLanguage` nor `dest`.
- **`RfcSAPUCToUTF8` wrote to an unchecked null output buffer.**
- **`RFC_BUFFER_TOO_SMALL` was never returned.** The too-small branches of
  `copy_out` and `copy_bytes` carried the right error *key* and the wrong
  return *code* (`RFC_INVALID_PARAMETER`), and never wrote the required
  length. The code is what a consumer branches on: the SDK's contract is
  "call once, and if the answer is 23, allocate the length it reported and
  call again", and SAP's own gorfc is written that way. Against this shim
  every value wider than a caller's first guess failed the whole call instead
  of being read on the second attempt. Pinned by a Rust test.

Also: `MAX_GENERATION` was being emitted into the public header over a private
`GEN_MASK`, so any consumer using the macro failed to compile; it is a
row-handle detail and is now excluded. `RfcGetParameterDescByName` and
`RfcGetFieldDescByName` read the caller's name bounded by the width SAP fixes
for one, rather than scanning up to a million units and echoing what they found
into an error message.

### Release gate

Run on 2026-09-04 against the a4h ABAP Platform Trial, kernel 7.58, on the
archives this release ships:

- 51 Go tests, live, no failures and **no skips**
- 513 Rust tests across the workspace, no failures
- `go test -race`, clean
- erpl's own suites through the C-ABI shim: **rfc 33/33** (1 skipped) and
  **bics 45/45**, every file, no failures. The bics files were run in batches
  with nothing else touching the machine: an earlier attempt reported 10 passed
  and 35 failed, but thirty-four of those carried no message -- the signature of
  a file killed by its own `timeout 600` -- because `./check.sh`, which runs the
  whole live Rust suite against the same system, was running alongside it. Every
  one of those files passes when it has the machine to itself.
- `examples/run_both.sh` against SAP's gorfc built on the NW RFC SDK:
  **no differences across 39 lines**, covering connection attributes, the echo
  and structure round trips, a table, the function description and an ABAP
  error's key

### Performance

Resolving a table's field descriptors once per table rather than once per row
took a 10 000-row `RFC_READ_TABLE` from 149 ms and 82 MB to 133 ms and 45 MB,
and halved its allocations. The same caching is on the write path. Numbers and
the reasoning are in the README.

### UTCLONG is spelled the way gorfc spells it

`2026-08-30T23:59:58.1234567`, measured against SAP's gorfc by
`examples/run_both.sh` rather than assumed. This library's own text form uses a
space where the SDK uses a `T` (spec VAL-8); the binding normalises, so the
classic form the spec records and the Python binding returns is untouched. What
a caller reads can be passed straight back — the round trip is in the
differential.

### Open: erpl parses UTCLONG differently again

Fixing the `RFCTYPE_UTCLONG` mapping made a pre-existing disagreement visible.
erpl maps that code to `TIMESTAMP` and parses the value as
`"YYYYMMDDHHMMSS,sssssss"` (`rfc/src/sap_type_conversion.cpp`); this library
renders UTCLONG as `"YYYY-MM-DD HH:MM:SS.fffffff"` (spec VAL-8). Before the fix
the field was reported as `CHAR` of eight characters against a twenty-seven
character value, so erpl could not read one at all -- the mismatch was hidden
behind a hard failure. It now reads and parses wrongly instead.

Reporting `CHAR` was a lie either way, so the mapping stays, and this binding
now matches gorfc exactly. erpl is the remaining consumer whose parser expects
a third spelling, and that is a decision spanning both repositories.

Behaviour that deviates from the reference, and why, is listed in
[MIGRATION.md](MIGRATION.md). Every deviation is a defect that could not be
reproduced without reproducing the defect.
