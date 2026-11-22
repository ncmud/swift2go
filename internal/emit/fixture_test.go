package emit

import (
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/swifttest"
)

const (
	fixtureRT         = "github.com/ncmud/swift2go/swiftrt"
	fixtureImportRoot = "github.com/ncmud/swift2go/internal/emit/testdata"
)

// transpileFixture transpiles targets of the Swift package testdata/<fixture>, in order, into out/<test name>
// at the repository root, so the Go output builds inside this module. It returns that directory. Test targets
// are written into the package of the target they test.
func transpileFixture(t *testing.T, fixture string, targets ...string) string {
	t.Helper()
	return transpileMapped(t, fixture, nil, targets...)
}

// transpileMapped is transpileFixture with Swift modules replaced by Go packages: maps holds each module's
// package directory under testdata/<fixture>. The modules are compiled for their declarations only.
func transpileMapped(t *testing.T, fixture string, mapped map[string]string, targets ...string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
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
	described, err := swiftmod.Describe(pkgDir, "")
	if err != nil {
		t.Fatal(err)
	}
	mods := filepath.Join(outDir, "modules")
	extra := swiftmod.TestingFlags(swiftmod.Destination{})
	var mappings []Mapping
	for _, mod := range slices.Sorted(maps.Keys(mapped)) {
		if err := swiftmod.EmitModule(described[mod], mods, extra...); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(pkgDir, filepath.FromSlash(mapped[mod]))
		pkg, exports, generic, err := PackageExports(dir)
		if err != nil {
			t.Fatal(err)
		}
		reqs, err := swiftmod.ProtocolRequirements(mod, []string{"-I", mods})
		if err != nil {
			t.Fatal(err)
		}
		mappings = append(mappings, Mapping{Module: mod, ImportPath: path.Join(fixtureImportRoot, fixture, mapped[mod]), Package: pkg, Exports: exports, Generic: generic, Requirements: reqs})
	}
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
		data, err := swiftmod.DumpAST(tg, mods, t.TempDir(), extra...)
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
		m.Mappings = mappings
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

// readGo returns the concatenated Go sources of a transpiled package directory.
func readGo(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, ent := range entries {
		if strings.HasSuffix(ent.Name(), ".go") {
			data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			if err != nil {
				t.Fatal(err)
			}
			b.Write(data)
		}
	}
	return b.String()
}
