package swiftrt

import (
	"os"
	"strings"
	_ "unsafe" // for go:linkname
)

// ProcessInfo is Foundation `ProcessInfo`.
type ProcessInfo struct{}

// NSProcessInfoProcessInfo is Foundation `ProcessInfo.processInfo`.
var NSProcessInfoProcessInfo = ProcessInfo{}

// NSProcessInfoEnvironment is Foundation `ProcessInfo.environment`: the process's environment
// variables, read when called. An entry is split at its first `=`.
func NSProcessInfoEnvironment(ProcessInfo) map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

//go:linkname runtimeNanotime runtime.nanotime
func runtimeNanotime() int64

// NSProcessInfoSystemUptime is Foundation `ProcessInfo.systemUptime`: seconds on the system's
// monotonic clock (CLOCK_MONOTONIC on Linux), which counts from boot.
func NSProcessInfoSystemUptime(ProcessInfo) float64 {
	return float64(runtimeNanotime()) / 1e9
}
