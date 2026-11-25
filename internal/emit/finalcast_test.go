package emit

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestFinalClassCast checks that `is`, `as?`, `as!` and a `case let as` pattern cast to a final class,
// local or another module's, with the runtime's Final forms, which match the most-derived object
// without walking its superclass parts, and that a cast to a non-final class keeps the walking form.
func TestFinalClassCast(t *testing.T) {
	out := transpileFixture(t, "finalcast", "Base", "App")
	src := readGo(t, filepath.Join(out, "app"))
	for _, want := range []string{
		"swiftrt.IsFinal[base.Item](e)",
		"swiftrt.IsFinal[Player](e)",
		"swiftrt.AsRefFinal[Player](e)",
		"swiftrt.MustAsFinal[Player](e)",
		"swiftrt.Is[*Mobile](e)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("transpiled App lacks %q:\n%s", want, src)
		}
	}
	for _, bad := range []string{"swiftrt.Is[*Player]", "swiftrt.Is[*base.Item]", "swiftrt.AsRef[*Player]", "swiftrt.MustAs[*Player]"} {
		if strings.Contains(src, bad) {
			t.Errorf("transpiled App casts to a final class with %q:\n%s", bad, src)
		}
	}
}
