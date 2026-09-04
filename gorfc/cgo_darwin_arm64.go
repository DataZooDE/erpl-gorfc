//go:build darwin && arm64 && cgo

package gorfc

// darwin/arm64.
//
// The archive under lib/ is built by scripts/build-gorfc-libs.sh and linked
// statically, so a consumer needs neither a Rust toolchain nor the SAP SDK.
// The README has the build story.

// #cgo CFLAGS: -I${SRCDIR}/../include
// #cgo LDFLAGS: -L${SRCDIR}/../lib/darwin_arm64 -lsapnwrfc -lresolv -framework CoreFoundation -framework Security
import "C"
