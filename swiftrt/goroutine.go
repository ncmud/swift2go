//go:build arm64 || amd64

package swiftrt

// getg returns the address of the running goroutine's runtime descriptor.
func getg() uintptr

// goroutineID identifies the running goroutine without walking its stack. A finished goroutine's
// descriptor can be reused, so every entry keyed by it must be removed before its goroutine ends.
func goroutineID() uint64 { return uint64(getg()) }
