package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/swifttest"
)

// TestIncrementalMatchesACleanRun edits a two-target package in the ways an edit loop does and checks
// that each -incremental run writes the Go a clean run writes, that a target's module is emitted
// again only when its declarations change, and that only the Go files whose input changed are
// emitted again.
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
		"Sources/Core/Effects.swift": `nonisolated(unsafe) public var lastTotal = 0

public final class Box {
    public var items: [Int] = []
    public init() {}
}

public func total(_ xs: [Int]) -> Int {
    return xs.reduce(0, +)
}

func note(_ n: Int) { lastTotal = n }
`,
		"Sources/Core/Options.swift": `struct Options {
    var n: Int
    var tag: String = "a"
}
`,
		"Sources/Core/Use.swift": `func optionsTag() -> String { Options(n: 1).tag }

func coreTotal(_ b: Box) -> Int { total(b.items) }
`,
		"Sources/App/Use.swift": `import Core

func appTotal(_ b: Box) -> Int { total(b.items) }
`,
		"Sources/App/Hoist1.swift": `func pairOne() -> (Int, String) {
    func same<U>(_ u: U) -> U { u }
    return (same(1), same("a"))
}
`,
		"Sources/App/Hoist2.swift": `func pairTwo() -> (Int, String) {
    func keep<U>(_ u: U) -> U { u }
    return (keep(2), keep("b"))
}
`,
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
	var log string
	transpile := func(dir string, incremental bool) map[string]string {
		t.Helper()
		var err error
		log = captureStderr(t, func() {
			err = run("transpile", targets, pkgDir, dir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{}, incremental, swiftmod.Destination{})
		})
		if err != nil {
			t.Fatalf("%v\n%s", err, log)
		}
		return goFiles(t, dir)
	}
	// emittedGo is the number of Go files the last incremental run wrote again for each target.
	emittedGo := func() map[string]int {
		out := map[string]int{}
		for _, m := range keptLine.FindAllStringSubmatch(log, -1) {
			kept, _ := strconv.Atoi(m[2])
			all, _ := strconv.Atoi(m[3])
			out[m[1]] = all - kept
		}
		return out
	}
	moduleTime := func(target string) time.Time {
		t.Helper()
		info, err := os.Stat(filepath.Join(outDir, "modules", target+".swiftmodule"))
		if err != nil {
			t.Fatal(err)
		}
		return info.ModTime()
	}
	prev := transpile(outDir, true)

	const all = -1 // every Go file of the target
	steps := []struct {
		name, file, old, new string         // old is "" for a new file; new is "" to remove the file
		emits                []string       // the targets whose modules are emitted again
		emitsGo              map[string]int // the number of Go files each target writes again
		changes              []string       // Go files of other Swift files whose content must change
	}{
		{"a body edit", "Sources/App/App.swift", `"hello "`, `"hi "`, nil, map[string]int{"Core": 0, "App": 1}, nil},
		{"a body edit in a dependency", "Sources/Core/Counter.swift", "count += 1", "count += 2", nil, map[string]int{"Core": 1, "App": 0}, nil},
		{"a new initial value of the same type", "Sources/Core/Names.swift", `"core"`, `"kernel"`, nil, map[string]int{"Core": 1, "App": 0}, nil},
		// Callers lend a class's property to total without a copy only while total cannot write it.
		{"a function that starts writing shared state", "Sources/Core/Effects.swift", "return xs", "note(xs.count)\n    return xs", nil, map[string]int{"Core": 2, "App": 1}, []string{"core/use.go", "app/use.go"}},
		{"a function that stops writing shared state", "Sources/Core/Effects.swift", "note(xs.count)\n    return xs", "return xs", nil, map[string]int{"Core": 2, "App": 1}, []string{"core/use.go", "app/use.go"}},
		// The synthesized initializer's callers spell out the default.
		{"a new default for a stored property", "Sources/Core/Options.swift", `"a"`, `"b"`, nil, map[string]int{"Core": 2, "App": 0}, []string{"core/use.go"}},
		// Hoisted generic local functions are numbered across the files of the module.
		{"a new hoisted local function", "Sources/App/Hoist1.swift", "    return (same(1)", "    func twice<U>(_ u: U) -> [U] { [u, u] }\n    return (same(twice(1)[0])", nil, map[string]int{"Core": 0, "App": 2}, []string{"app/hoist2.go"}},
		// A global with an initializer that is not a literal is initialized lazily, and read through a cell.
		{"an initial value that is not a literal", "Sources/Core/Names.swift", `"kernel"`, `String("kernel")`, nil, map[string]int{"Core": all, "App": all}, []string{"core/counter.go"}},
		{"a new type for a variable another file uses", "Sources/Core/Names.swift", `String("kernel")`, `7`, []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil},
		{"a new declaration in a dependency", "Sources/Core/Names.swift", "7\n", "7\npublic func shout(_ s: String) -> String { s + \"!\" }\n", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil},
		{"a use of the new declaration", "Sources/App/App.swift", `return "hi "`, `return shout("hi") + " "`, nil, map[string]int{"Core": 0, "App": 1}, nil},
		{"a new file", "Sources/App/Extra.swift", "", "func extra() -> Int { helper() + 1 }\n", []string{"App"}, map[string]int{"Core": 0, "App": all}, nil},
		{"a removed file", "Sources/App/Extra.swift", "func extra", "", []string{"App"}, map[string]int{"Core": 0, "App": all}, nil},
	}
	for _, step := range steps {
		path := filepath.Join(pkgDir, step.file)
		src, err := os.ReadFile(path)
		if err != nil && step.old != "" {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), step.old) {
			t.Fatalf("%s: %s does not contain %q", step.name, step.file, step.old)
		}
		if step.new == "" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		} else {
			writeSource(t, path, strings.Replace(string(src), step.old, step.new, 1))
		}
		before := map[string]time.Time{}
		for _, target := range targets {
			before[target] = moduleTime(target)
		}
		got := transpile(outDir, true)
		emitted := emittedGo()
		for _, target := range targets {
			want := step.emitsGo[target]
			if want == all {
				want = len(sourcesOf(t, pkgDir, target))
			}
			if emitted[target] != want {
				t.Errorf("after %s, %s wrote %d Go files again, want %d", step.name, target, emitted[target], want)
			}
		}
		for _, name := range step.changes {
			if got[name] == prev[name] {
				t.Errorf("after %s, %s is unchanged:\n%s", step.name, name, got[name])
			}
		}
		prev = got
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

var keptLine = regexp.MustCompile(`swift2go: (\w+): kept (\d+) of (\d+) Go files`)

// sourcesOf lists the Swift files of target in the package at pkgDir.
func sourcesOf(t *testing.T, pkgDir, target string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(pkgDir, "Sources", target, "*.swift"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// captureStderr runs f and returns what it wrote to os.Stderr, which it also passes on.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	defer func() { os.Stderr = saved }()
	f()
	_ = w.Close()
	out := <-done
	_ = r.Close()
	_, _ = saved.WriteString(out)
	return out
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
