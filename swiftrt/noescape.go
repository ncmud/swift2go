//go:build amd64 || arm64 || wasm

package swiftrt

import "unsafe"

// noescape returns p unchanged, hiding it from escape analysis, for a callee reached indirectly that
// only writes through the pointer while it runs.
//
//go:noescape
func noescape(p unsafe.Pointer) unsafe.Pointer
