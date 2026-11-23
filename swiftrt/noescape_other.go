//go:build !amd64 && !arm64 && !wasm

package swiftrt

import "unsafe"

// noescape returns p. Elsewhere it hides p from escape analysis; here the value it points to moves to
// the heap.
func noescape(p unsafe.Pointer) unsafe.Pointer { return p }
