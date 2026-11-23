//go:build !arm64 && !amd64

package swiftrt

import "runtime"

// goroutineID identifies the running goroutine by the number in its stack header, "goroutine N [".
func goroutineID() uint64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	var id uint64
	for _, c := range buf[len("goroutine "):n] {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
