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
	if err := run("transpile", []string{"Glue"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{}, false, false, swiftmod.Destination{}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(outDir, "glue")); err != nil {
		t.Fatal(err)
	}
	const goPkgs = "github.com/ncmud/swift2go/cmd/swift2go/testdata/mapping/go/"
	maps := mapFlag{"Toy": goPkgs + "toy", "Glue": goPkgs + "glue"}
	if err := run("transpile", []string{"Toy", "App"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", maps, overlayFlag{}, false, false, swiftmod.Destination{}); err != nil {
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
	if err := run("transpile", []string{"Server"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlays, false, false, swiftmod.Destination{}); err != nil {
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
	err = run("transpile", []string{"Server"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{"Sever": overlayDir}, false, false, swiftmod.Destination{})
	if err == nil || !strings.Contains(err.Error(), "not in this run") {
		t.Errorf("overlay for an unknown target: err = %v", err)
	}
}

// TestExecutable transpiles testdata/executable/swift, whose top-level code declares a type inside a do block,
// runs the program, and builds it for WASI.
func TestExecutable(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	pkgDir, err := filepath.Abs("testdata/executable/swift")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	outDir := filepath.Join("out", "executabletest")
	t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })

	if err := run("transpile", []string{"Tool"}, pkgDir, outDir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{}, false, false, swiftmod.Destination{}); err != nil {
		t.Fatal(err)
	}
	tool := "./" + filepath.ToSlash(filepath.Join(outDir, "tool"))
	for input, want := range map[string]string{`{"version": 7}`: "version 7", "{}": "not a header"} {
		cmd := exec.Command("go", "run", tool)
		cmd.Stdin = strings.NewReader(input + "\n")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go run: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("for %s: printed %q, want %q", input, got, want)
		}
	}

	wasm := exec.Command("go", "build", "-o", filepath.Join(t.TempDir(), "tool.wasm"), tool)
	wasm.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if out, err := wasm.CombinedOutput(); err != nil {
		t.Fatalf("go build for wasip1: %v\n%s", err, out)
	}
}

// TestSwiftSDK transpiles testdata/platform/swift for the host and for an installed Linux Swift SDK, and
// checks that each Go program takes the `#if os(...)` and `canImport` branches of its platform, finds its
// resources in the bundle SwiftPM names on that platform, and calls the Foundation names that platform declares.
func TestSwiftSDK(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	sdks, err := swiftmod.InstalledSwiftSDKs()
	if err != nil {
		t.Fatal(err)
	}
	var linux swiftmod.Destination
	for _, d := range sdks {
		if d.OS() == "linux" && (linux.Host() || strings.HasPrefix(d.Triple, runtimeArch())) {
			linux = d
		}
	}
	if linux.Host() {
		t.Skip("needs a Linux Swift SDK; see `swift sdk install`")
	}
	pkgDir, err := filepath.Abs("testdata/platform/swift")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })
	libc := "/musl"
	if !strings.HasSuffix(linux.Triple, "-musl") {
		libc = "/glibc"
	}
	for _, c := range []struct {
		dest   swiftmod.Destination
		want   string
		bundle string
	}{
		{swiftmod.Destination{}, "macos/darwin", "platform_Platform.bundle"},
		{linux, "linux" + libc, "platform_Platform.resources"},
	} {
		outDir := filepath.Join("out", "platformtest")
		_ = os.RemoveAll(outDir)
		t.Cleanup(func() { _ = os.RemoveAll(outDir) })
		build := swiftmod.BuildDir(pkgDir, c.dest)
		if !c.dest.Host() {
			build = t.TempDir()
		}
		if err := run("transpile", []string{"Platform"}, pkgDir, outDir, build, "", mapFlag{}, overlayFlag{}, false, false, c.dest); err != nil {
			t.Fatal(err)
		}
		check := filepath.Join(outDir, "check")
		main := "package main\n\nimport \"github.com/ncmud/swift2go/out/platformtest/platform\"\n\nfunc main() { print(platform.PlatformName(), \" \", platform.Motd(), \" \", platform.FileAttributes()) }\n"
		if err := os.MkdirAll(check, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(check, "main.go"), []byte(main), 0o644); err != nil {
			t.Fatal(err)
		}
		exe := filepath.Join(t.TempDir(), "check")
		if out, err := exec.Command("go", "build", "-o", exe, "./"+filepath.ToSlash(check)).CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
		bundle := filepath.Join(filepath.Dir(exe), c.bundle)
		if err := os.MkdirAll(bundle, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bundle, "motd.txt"), []byte("welcome\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(exe).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", exe, err, out)
		}
		if got, want := strings.TrimSpace(string(out)), c.want+" welcome 1000000 5"; got != want {
			t.Errorf("for %q: PlatformName(), Motd(), FileAttributes() = %q, want %q", c.dest.Triple, got, want)
		}
	}
}

// runtimeArch is the host's architecture as target triples spell it.
func runtimeArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}
