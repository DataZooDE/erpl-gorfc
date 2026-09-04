# Static archives

One `libsapnwrfc.a` per platform, under `<goos>_<goarch>/`. They are build
outputs of `crates/erpl-proto-nwrfc`, and they are **not committed in this
repository** — 30 MB per set, rebuilt on every release, in a repository that is
not the published module.

A fresh clone therefore has no archives, and `go build` fails with a linker
error naming `-lsapnwrfc`. Build them first:

```bash
scripts/build-gorfc-libs.sh --host     # this platform, for the dev loop
scripts/build-gorfc-libs.sh            # all seven, for a release
```

The published `erpl-gorfc` module does commit them: that is what makes
`go get` work with no Rust toolchain and no SAP SDK. `BUILD_INFO`, written
beside them, records the commit and the SHA-256 of each archive.
