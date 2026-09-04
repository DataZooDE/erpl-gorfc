//go:build linux && amd64 && cgo && musl

package gorfc

// linux/amd64 against musl, selected by -tags musl -- Go cannot tell one libc from the other.
//
// The archive under lib/ is built by scripts/build-gorfc-libs.sh and linked
// statically, so a consumer needs neither a Rust toolchain nor the SAP SDK.
// The README has the build story.

// #cgo CFLAGS: -I${SRCDIR}/../include
// #cgo LDFLAGS: -L${SRCDIR}/../lib/linux_amd64_musl -lsapnwrfc -lm -lpthread
import "C"
