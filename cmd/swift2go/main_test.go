package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/swiftmod"
)

// TestMappedModules transpiles testdata/mapping/swift with Toy replaced by a hand-written Go package and Glue,
// which is not transpiled, mapped to another, then runs the generated App against both. App reaches Glue
// through a function, a static, a struct, a generic struct and function, an enum and a lazy sequence.
func TestMappedModules(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("needs the Xcode Swift toolchain (swiftc and swift-demangle through xcrun)")
	}
	pkgDir, err := filepath.Abs("testdata/mapping/swift")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	outDir := filepath.Join("out", "mappingtest")
	t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })

	// Transpiling Glue on its own leaves its .swiftmodule for the next run; its Go output is discarded.
	if err := run("transpile", []string{"Glue"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir), "", mapFlag{}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(outDir, "glue")); err != nil {
		t.Fatal(err)
	}
	const goPkgs = "github.com/ncmud/swift2go/cmd/swift2go/testdata/mapping/go/"
	maps := mapFlag{"Toy": goPkgs + "toy", "Glue": goPkgs + "glue"}
	if err := run("transpile", []string{"Toy", "App"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir), "", maps); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "toy")); !os.IsNotExist(err) {
		t.Errorf("mapped target Toy was written to %s", outDir)
	}
	main := "package main\n\nimport \"github.com/ncmud/swift2go/out/mappingtest/app\"\n\nfunc main() { print(app.Report(\"go\")) }\n"
	if err := os.MkdirAll(filepath.Join(outDir, "check"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "check", "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "run", "./"+filepath.ToSlash(filepath.Join(outDir, "check"))).CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), "hi, go: 84 12 dark"; got != want {
		t.Errorf("Report(\"go\") = %q, want %q", got, want)
	}
}
