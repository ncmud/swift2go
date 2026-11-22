// Package swifttest gates tests that run the Swift toolchain.
package swifttest

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// Require skips t unless it runs on macOS with every named tool on PATH. When the CI environment
// variable is set on macOS, a missing tool fails t instead, so a broken toolchain cannot pass as a
// skipped test.
func Require(t testing.TB, tools ...string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("needs the Xcode Swift toolchain on macOS")
	}
	var missing []string
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) == 0 {
		return
	}
	msg := "needs the Xcode Swift toolchain; not on PATH: " + strings.Join(missing, ", ")
	if os.Getenv("CI") != "" {
		t.Fatal(msg)
	}
	t.Skip(msg)
}
