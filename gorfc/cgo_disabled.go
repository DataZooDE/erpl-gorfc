//go:build !cgo

// Package gorfc requires cgo.
//
// Without it every other file in this package is excluded by its build
// constraints, the package compiles to nothing, and a caller's first use fails
// as an undefined symbol far from the cause. This file makes the build fail
// here instead, with the reason in the identifier.
//
// cgo is not incidental. It is what lets the RFC implementation link
// statically into your binary rather than be loaded at run time, which is the
// whole difference from the library this replaces.
package gorfc

func init() {
	_ = erpl_gorfc_requires_CGO_ENABLED_1_and_a_C_compiler
}
