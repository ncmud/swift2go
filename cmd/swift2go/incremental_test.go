package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/swifttest"
)

// TestIncrementalMatchesACleanRun edits a two-target package in the ways an edit loop does and checks
// that each -incremental run writes the Go a clean run writes, and that a target's module is emitted
// again only when its declarations change.
func TestIncrementalMatchesACleanRun(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	pkgDir := t.TempDir()
	files := map[string]string{
		"Package.swift": `// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "incremental",
    targets: [
        .target(name: "Core"),
        .target(name: "App", dependencies: ["Core"]),
    ]
)
`,
		"Sources/Core/Counter.swift": `public struct Counter {
    public var count = 0
    public init() {}
    public mutating func bump() { count += 1 }
    public func describe() -> String { "count: \(count) of \(defaultName)" }
}
`,
		"Sources/Core/Names.swift": "public let defaultName = \"core\"\n",
		"Sources/App/App.swift": `import Core

public func greeting() -> String {
    var c = Counter()
    c.bump()
    return "hello " + c.describe()
}
`,
		"Sources/App/Other.swift": "func helper() -> Int { 42 }\n",
	}
	for name, src := range files {
		writeSource(t, filepath.Join(pkgDir, name), src)
	}
	t.Chdir("../..")
	outDir := filepath.Join("out", "incrementaltest")
	cleanDir := filepath.Join("out", "incrementalclean")
	t.Cleanup(func() { _ = os.RemoveAll(outDir); _ = os.RemoveAll(cleanDir) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })
	targets := []string{"Core", "App"}
	transpile := func(dir string, incremental bool) map[string]string {
		t.Helper()
		if err := run("transpile", targets, pkgDir, dir, swiftmod.BuildDir(pkgDir), "", mapFlag{}, overlayFlag{}, incremental); err != nil {
			t.Fatal(err)
		}
		return goFiles(t, dir)
	}
	moduleTime := func(target string) time.Time {
		t.Helper()
		info, err := os.Stat(filepath.Join(outDir, "modules", target+".swiftmodule"))
		if err != nil {
			t.Fatal(err)
		}
		return info.ModTime()
	}
	transpile(outDir, true)

	steps := []struct {
		name, file, old, new string
		emits                []string // the targets whose modules are emitted again
	}{
		{"a body edit", "Sources/App/App.swift", `"hello "`, `"hi "`, nil},
		{"a body edit in a dependency", "Sources/Core/Counter.swift", "count += 1", "count += 2", nil},
		{"a new initial value of the same type", "Sources/Core/Names.swift", `"core"`, `"kernel"`, nil},
		{"a new type for a variable another file uses", "Sources/Core/Names.swift", `"kernel"`, `7`, []string{"Core", "App"}},
		{"a new declaration in a dependency", "Sources/Core/Names.swift", "7\n", "7\npublic func shout(_ s: String) -> String { s + \"!\" }\n", []string{"Core", "App"}},
		{"a use of the new declaration", "Sources/App/App.swift", `return "hi "`, `return shout("hi") + " "`, nil},
	}
	for _, step := range steps {
		path := filepath.Join(pkgDir, step.file)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), step.old) {
			t.Fatalf("%s: %s does not contain %q", step.name, step.file, step.old)
		}
		writeSource(t, path, strings.Replace(string(src), step.old, step.new, 1))
		before := map[string]time.Time{}
		for _, target := range targets {
			before[target] = moduleTime(target)
		}
		got := transpile(outDir, true)
		_ = os.RemoveAll(cleanDir)
		want := transpile(cleanDir, false)
		for name, w := range want {
			w = strings.ReplaceAll(w, filepath.ToSlash(cleanDir), filepath.ToSlash(outDir))
			if got[name] != w {
				t.Errorf("after %s, the incremental %s differs from a clean run's:\n%s\nwant:\n%s", step.name, name, got[name], w)
			}
		}
		if len(got) != len(want) {
			t.Errorf("after %s, the incremental run wrote %d Go files, a clean run %d", step.name, len(got), len(want))
		}
		for _, target := range targets {
			emitted := !moduleTime(target).Equal(before[target])
			if want := strings.Contains(strings.Join(step.emits, " "), target); emitted != want {
				t.Errorf("after %s, %s's module emitted again: %v, want %v", step.name, target, emitted, want)
			}
		}
	}
}

func writeSource(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// goFiles reads the Go files under dir, keyed by their paths relative to it.
func goFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
