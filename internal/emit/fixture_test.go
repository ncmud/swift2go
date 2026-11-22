package emit

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/swiftmod"
)

const fixtureRT = "github.com/ncmud/swift2go/swiftrt"

// transpileFixture transpiles targets of the Swift package testdata/<fixture>, in order, into out/<test name>
// at the repository root, so the Go output builds inside this module. It returns that directory. Test targets
// are written into the package of the target they test.
func transpileFixture(t *testing.T, fixture string, targets ...string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("needs the Xcode Swift toolchain (swiftc, swift-demangle and swift-testing through xcrun)")
	}
	pkgDir, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	rel := path.Join("out", strings.ToLower(t.Name()))
	outDir := filepath.Join(root, filepath.FromSlash(rel))
	_ = os.RemoveAll(outDir)
	if os.Getenv("SWIFT2GO_KEEP_OUT") == "" {
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	}
	described, err := swiftmod.Describe(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	mods := filepath.Join(outDir, "modules")
	extra := swiftmod.TestingFlags()
	byName := map[string]*Module{}
	var done []*Module
	for _, name := range targets {
		tg, ok := described[name]
		if !ok {
			t.Fatalf("no target %s in %s", name, fixture)
		}
		if tg.Type != "test" {
			if err := swiftmod.EmitModule(tg, mods, extra...); err != nil {
				t.Fatal(err)
			}
		}
		data, err := swiftmod.DumpAST(tg, mods, extra...)
		if err != nil {
			t.Fatal(err)
		}
		files, err := ast.Load(data)
		if err != nil {
			t.Fatal(err)
		}
		m, err := New(name, files, fixtureRT)
		if err != nil {
			t.Fatal(err)
		}
		m.Deps = done
		pkgOut := strings.ToLower(name)
		if tg.Type == "test" {
			under := byName[tg.Deps[0]]
			m.SetTarget(under)
			pkgOut = under.Pkg
		}
		m.ImportPath = "github.com/ncmud/swift2go/" + rel + "/" + pkgOut
		if _, err := m.WriteGo(filepath.Join(outDir, pkgOut)); err != nil {
			t.Fatal(err)
		}
		if n := len(m.Unsupported); n > 0 {
			t.Errorf("%s: %d unsupported constructs:\n%s", name, n, m.Report())
		}
		if tg.Type != "test" {
			done = append(done, m)
			byName[name] = m
		}
	}
	return outDir
}

// goTest runs `go test` in a transpiled package directory and fails with its output on error.
func goTest(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-count=1", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test %s: %v\n%s", dir, err, out)
	}
}
