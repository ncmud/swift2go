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
		"Sources/Core/Point.swift": `public struct Point {
    public var x: Int
    public var y: Int
    public init(x: Int, y: Int) { self.x = x; self.y = y }
}
`,
		"Sources/App/Shapes.swift": `import Core

func origin() -> Point { Point(x: 0, y: 0) }
`,
		"Sources/Core/Find.swift": `public struct Spot {
    public var n: Int
    public init(n: Int) { self.n = n }
}

public func findSpot() -> Spot? { Spot(n: 1) }
`,
		"Sources/App/Find.swift": `import Core

func spotN() -> Int { findSpot()?.n ?? 0 }
`,
		"Sources/Core/Kinds.swift": `public enum Suit { case hearts, spades }

public typealias Coord = Int

public protocol Named { var name: String { get } }

public final class Tally {
    public var total: Int = 0
    public init() {}
    public func step() -> Int { 1 }
    public static func make() -> Tally { Tally() }
}

public func pick<T>(_ x: T) -> T { x }

public func load() -> Int { 1 }
`,
		"Sources/App/Kinds.swift": `import Core

func suitName(_ s: Suit) -> String {
    switch s {
    case .hearts: return "h"
    default: return "other"
    }
}

func coord() -> Coord { 3 }

struct Tag: Named { var name: String { "tag" } }

func tagName() -> String { Tag().name }

func tally() -> Int {
    let t = Tally.make()
    t.step()
    return pick(t.step())
}

func totalText() -> String { "\(Tally().total)" }

func loaded() -> Int { load() }
`,
	}
	for name, src := range files {
		writeSource(t, filepath.Join(pkgDir, name), src)
	}
	overlay := filepath.Join(pkgDir, "overlay", "core")
	if err := os.MkdirAll(overlay, 0o755); err != nil {
		t.Fatal(err)
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
			err = run("transpile", targets, pkgDir, dir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{"Core": overlay}, incremental, swiftmod.Destination{})
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
		name, file, old, new string         // an edit (see [edit])
		emits                []string       // the targets whose modules are emitted again
		emitsGo              map[string]int // the number of Go files each target writes again
		changes              []string       // Go files of other Swift files whose content must change
		also                 []edit         // edits made in other files at the same time
	}{
		{"a body edit", "Sources/App/App.swift", `"hello "`, `"hi "`, nil, map[string]int{"Core": 0, "App": 1}, nil, nil},
		{"a body edit in a dependency", "Sources/Core/Counter.swift", "count += 1", "count += 2", nil, map[string]int{"Core": 1, "App": 0}, nil, nil},
		{"a new initial value of the same type", "Sources/Core/Names.swift", `"core"`, `"kernel"`, nil, map[string]int{"Core": 1, "App": 0}, nil, nil},
		// Callers lend a class's property to total without a copy only while total cannot write it.
		{"a function that starts writing shared state", "Sources/Core/Effects.swift", "return xs", "note(xs.count)\n    return xs", nil, map[string]int{"Core": 2, "App": 1}, []string{"core/use.go", "app/use.go"}, nil},
		{"a function that stops writing shared state", "Sources/Core/Effects.swift", "note(xs.count)\n    return xs", "return xs", nil, map[string]int{"Core": 2, "App": 1}, []string{"core/use.go", "app/use.go"}, nil},
		// The synthesized initializer's callers spell out the default.
		{"a new default for a stored property", "Sources/Core/Options.swift", `"a"`, `"b"`, nil, map[string]int{"Core": 2, "App": 0}, []string{"core/use.go"}, nil},
		// Hoisted generic local functions are numbered across the files of the module.
		{"a new hoisted local function", "Sources/App/Hoist1.swift", "    return (same(1)", "    func twice<U>(_ u: U) -> [U] { [u, u] }\n    return (same(twice(1)[0])", nil, map[string]int{"Core": 0, "App": 2}, []string{"app/hoist2.go"}, nil},
		// A global with an initializer that is not a literal is initialized lazily, and read through a cell.
		{"an initial value that is not a literal", "Sources/Core/Names.swift", `"kernel"`, `String("kernel")`, nil, map[string]int{"Core": all, "App": all}, []string{"core/counter.go"}, nil},
		{"a new type for a variable another file uses", "Sources/Core/Names.swift", `String("kernel")`, `7`, []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a new declaration in a dependency", "Sources/Core/Names.swift", "7\n", "7\npublic func shout(_ s: String) -> String { s + \"!\" }\n", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a use of the new declaration", "Sources/App/App.swift", `return "hi "`, `return shout("hi") + " "`, nil, map[string]int{"Core": 0, "App": 1}, nil, nil},
		// The Go of a call to another target's initializer passes its defaults and depends on its parameters.
		{"a new defaulted parameter of an initializer another target calls", "Sources/Core/Point.swift", "init(x: Int, y: Int) { self.x = x; self.y = y }", "init(x: Int, y: Int, z: Int = 3) { self.x = x; self.y = y + z }", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, []string{"app/shapes.go"}, nil},
		{"a new default argument", "Sources/Core/Point.swift", "z: Int = 3", "z: Int = 4", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, []string{"app/shapes.go"}, nil},
		{"a new argument label", "Sources/Core/Point.swift", "init(x: Int,", "init(left x: Int,", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, []edit{{"Sources/App/Shapes.swift", "Point(x: 0", "Point(left: 0"}}},
		{"a new parameter type", "Sources/Core/Point.swift", "y: Int, z: Int = 4) { self.x = x; self.y = y + z }", "y: Int?, z: Int = 4) { self.x = x; self.y = (y ?? 0) + z }", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, []string{"app/shapes.go"}, nil},
		// Declarations another target uses.
		{"a new enum case", "Sources/Core/Kinds.swift", "case hearts, spades }", "case hearts, spades, clubs }", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a new type for a typealias", "Sources/Core/Kinds.swift", "Coord = Int\n", "Coord = Int32\n", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, []string{"app/kinds.go"}, nil},
		{"a new protocol requirement with a default", "Sources/Core/Kinds.swift", "var name: String { get } }", "var name: String { get }; func greet() -> String }\n\nextension Named { public func greet() -> String { \"hi \" + name } }", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a new @discardableResult", "Sources/Core/Kinds.swift", "public func step()", "@discardableResult public func step()", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a new property type", "Sources/Core/Kinds.swift", "total: Int = 0", "total: Double = 0", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a new generic constraint", "Sources/Core/Kinds.swift", "pick<T>(", "pick<T: Equatable>(", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a new access level", "Sources/Core/Kinds.swift", "public func load()", "package func load()", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, nil},
		{"a static method made an instance method", "Sources/Core/Kinds.swift", "public static func make()", "public func make()", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, []edit{{"Sources/App/Kinds.swift", "Tally.make()", "Tally().make()"}}},
		{"a function that starts throwing", "Sources/Core/Kinds.swift", "func load() -> Int", "func load() throws -> Int", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, []edit{{"Sources/App/Kinds.swift", "{ load() }", "{ (try? load()) ?? 0 }"}}},
		{"an async function", "Sources/Core/Kinds.swift", "func load() throws", "func load() async throws", []string{"Core", "App"}, map[string]int{"Core": all, "App": all}, nil, []edit{{"Sources/App/Kinds.swift", "func loaded() -> Int { (try? load())", "func loaded() async -> Int { (try? await load())"}}},
		// A caller takes the Optional result of a function an overlay replaces in its pointer form.
		{"a new overlay file in a dependency", "overlay/core/find.go", "", "package core\n\nfunc FindSpot() *Spot { return \u0026Spot{N: 1} }\n", nil, map[string]int{"Core": all, "App": all}, []string{"app/find.go"}, nil},
		{"a removed overlay file in a dependency", "overlay/core/find.go", "package core", "", nil, map[string]int{"Core": all, "App": all}, []string{"app/find.go"}, nil},
		// Go declares the methods of a conformance where the type is, in the dependency's package.
		{"a new retroactive conformance", "Sources/App/Retro.swift", "", "import Core\n\nprotocol Describes { func describeIt() -> String }\n\nextension Counter: Describes { func describeIt() -> String { \"c\" } }\n", []string{"App"}, map[string]int{"Core": 0, "App": all}, nil, nil},
		{"a removed retroactive conformance", "Sources/App/Retro.swift", "import Core", "", []string{"App"}, map[string]int{"Core": 0, "App": all}, nil, nil},
		{"a new file", "Sources/App/Extra.swift", "", "func extra() -> Int { helper() + 1 }\n", []string{"App"}, map[string]int{"Core": 0, "App": all}, nil, nil},
		{"a removed file", "Sources/App/Extra.swift", "func extra", "", []string{"App"}, map[string]int{"Core": 0, "App": all}, nil, nil},
	}
	for _, step := range steps {
		for _, e := range append([]edit{{step.file, step.old, step.new}}, step.also...) {
			e.apply(t, pkgDir, step.name)
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

// An edit replaces old with new in a file of the package; old is "" for a new file, and new is "" to
// remove the file.
type edit struct{ file, old, new string }

func (e edit) apply(t *testing.T, pkgDir, step string) {
	t.Helper()
	path := filepath.Join(pkgDir, e.file)
	src, err := os.ReadFile(path)
	if err != nil && e.old != "" {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), e.old) {
		t.Fatalf("%s: %s does not contain %q", step, e.file, e.old)
	}
	if e.new == "" {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		return
	}
	writeSource(t, path, strings.Replace(string(src), e.old, e.new, 1))
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
