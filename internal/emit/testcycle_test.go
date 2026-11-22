package emit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTestTargetCycle checks a test target that imports both the target it tests and a target that depends on
// it: its tests get a package of their own, since joining the tested package would be an import cycle in Go,
// and still reach the tested target's internal declarations, including the initial values its implicit
// memberwise initializers take for arguments left out and its members whose names begin with an underscore.
func TestTestTargetCycle(t *testing.T) {
	out := transpileFixture(t, "testcycle", "Lib", "App", "LibTests")
	if _, err := os.Stat(filepath.Join(out, "libtests", "lib_tests_test.go")); err != nil {
		t.Fatalf("LibTests did not get its own package: %v", err)
	}
	goTest(t, filepath.Join(out, "libtests"))
}
