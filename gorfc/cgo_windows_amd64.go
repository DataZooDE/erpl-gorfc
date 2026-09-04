//go:build windows && amd64 && cgo

package gorfc

// windows/amd64, built for the MinGW ABI cgo links with -- an MSVC archive will not resolve.
//
// The archive under lib/ is built by scripts/build-gorfc-libs.sh and linked
// statically, so a consumer needs neither a Rust toolchain nor the SAP SDK.
// The README has the build story.

// #cgo CFLAGS: -I${SRCDIR}/../include
// #cgo LDFLAGS: -L${SRCDIR}/../lib/windows_amd64 -lsapnwrfc -lws2_32 -lbcrypt -luserenv -lntdll -ladvapi32
import "C"
