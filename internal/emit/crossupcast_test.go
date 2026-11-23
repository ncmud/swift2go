package emit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCrossModuleUpcast checks that a class of one module, passed or assigned where its superclass from a
// second module is expected, is upcast when a third module does it, and that the override still runs.
func TestCrossModuleUpcast(t *testing.T) {
	out := transpileFixture(t, "crossupcast", "Base", "Kit", "App")
	test := `package app

import "testing"

func TestApp(t *testing.T) {
	got := App()
	want := []string{"captured x", "captured y", "captured x"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("App() = %q, want %q", got, want)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(out, "app", "app_test.go"), []byte(test), 0o600); err != nil {
		t.Fatal(err)
	}
	goTest(t, filepath.Join(out, "app"))
}
