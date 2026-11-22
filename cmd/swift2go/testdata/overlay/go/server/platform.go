// Hand-written replacement for Sources/Server/Platform.swift.

package server

import "runtime"

// HostLabel names the host the server runs on.
func HostLabel() string { return "go on " + runtime.GOOS }
