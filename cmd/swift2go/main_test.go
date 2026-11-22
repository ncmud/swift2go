package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/swifttest"
)

// TestMappedModules transpiles testdata/mapping/swift with Toy replaced by a hand-written Go package and Glue,
// which is not transpiled, mapped to another, then runs the generated App against both. App reaches Glue
// through a function, a static, a struct, a generic struct and function, an enum and a lazy sequence.
func TestMappedModules(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	pkgDir, err := filepath.Abs("testdata/mapping/swift")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	outDir := filepath.Join("out", "mappingtest")
	t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })

	// Transpiling Glue on its own leaves its .swiftmodule for the next run; its Go output is discarded.
	if err := run("transpile", []string{"Glue"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir), "", mapFlag{}, overlayFlag{}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(outDir, "glue")); err != nil {
		t.Fatal(err)
	}
	const goPkgs = "github.com/ncmud/swift2go/cmd/swift2go/testdata/mapping/go/"
	maps := mapFlag{"Toy": goPkgs + "toy", "Glue": goPkgs + "glue"}
	if err := run("transpile", []string{"Toy", "App"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir), "", maps, overlayFlag{}); err != nil {
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
	if got, want := strings.TrimSpace(string(out)), "hi, go: 84 12 dark tagged"; got != want {
		t.Errorf("Report(\"go\") = %q, want %q", got, want)
	}
}

// TestOverlay transpiles testdata/overlay/swift with Platform.swift replaced by a hand-written Go file, then runs
// the generated Banner.swift against it.
func TestOverlay(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	pkgDir, err := filepath.Abs("testdata/overlay/swift")
	if err != nil {
		t.Fatal(err)
	}
	overlayDir, err := filepath.Abs("testdata/overlay/go/server")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	outDir := filepath.Join("out", "overlaytest")
	t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })

	overlays := overlayFlag{"Server": overlayDir}
	if err := run("transpile", []string{"Server"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir), "", mapFlag{}, overlays); err != nil {
		t.Fatal(err)
	}
	platform, err := os.ReadFile(filepath.Join(outDir, "server", "platform.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(platform), "// Hand-written replacement") {
		t.Errorf("platform.go is not the overlay file:\n%s", platform)
	}
	main := "package main\n\nimport \"github.com/ncmud/swift2go/out/overlaytest/server\"\n\nfunc main() { print(server.Banner(\"ncmud\"), \"; \", server.Note()) }\n"
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
	if got, want := strings.TrimSpace(string(out)), "ncmud on go on "+runtime.GOOS+"; copied"; got != want {
		t.Errorf("Banner(\"ncmud\") = %q, want %q", got, want)
	}

	// An overlay for a target that is not being transpiled is a mistake in the command line.
	err = run("transpile", []string{"Server"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir), "", mapFlag{}, overlayFlag{"Sever": overlayDir})
	if err == nil || !strings.Contains(err.Error(), "not in this run") {
		t.Errorf("overlay for an unknown target: err = %v", err)
	}
}
