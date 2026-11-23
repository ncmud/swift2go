package emit

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSharedValueCopies checks which reads of a value are copied. A frozen struct (every stored
// property a `let`) is returned and bound without a copy, and a private array that code only
// replaces whole is lent to a callee without one. A part read out of a frozen struct is still copied
// when it is returned, and an array that code appends to is still copied when lent to a callee
// that can append to it.
func TestSharedValueCopies(t *testing.T) {
	out := transpileFixture(t, "sharedvalues", "SharedValues")
	src := readGo(t, filepath.Join(out, "sharedvalues"))
	for _, absent := range []string{"swiftrt.Copy(*", "return swiftrt.Copy(cachedCatalog)", "swiftrt.Copy(self.Table)"} {
		if strings.Contains(src, absent) {
			t.Errorf("the Go copies where Swift shares: found %q in\n%s", absent, src)
		}
	}
	for _, present := range []string{"swiftrt.Copy(c.Names)", "swiftrt.Copy(self.Grown)"} {
		if !strings.Contains(src, present) {
			t.Errorf("the Go shares where it must copy: no %q in\n%s", present, src)
		}
	}
}
