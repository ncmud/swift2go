package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/swifttest"
)

// appendsToManyArrays fills several local arrays in one loop, which the emitter presizes in the order it
// finds their appends.
const appendsToManyArrays = `public func sortWords(_ words: [String]) -> [[String]] {
    var a: [String] = [], b: [String] = [], c: [String] = [], d: [String] = []
    var e: [String] = [], f: [String] = [], g: [String] = [], h: [String] = []
    for w in words {
        if w.isEmpty {
            a.append(w)
        } else if w.count == 1 {
            b.append(w)
        } else if w.count == 2 {
            c.append(w)
        } else if w.count == 3 {
            d.append(w)
        } else if w.count == 4 {
            e.append(w)
        } else if w.count == 5 {
            f.append(w)
        } else if w.count == 6 {
            g.append(w)
        } else {
            h.append(w)
        }
    }
    return [a, b, c, d, e, f, g, h]
}
`

// TestEmissionIsDeterministic emits the Go of the difftest cases ten times from one dump and checks that
// every emission writes the same bytes.
func TestEmissionIsDeterministic(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	pkgDir := t.TempDir()
	writeSource(t, filepath.Join(pkgDir, "Package.swift"), `// swift-tools-version:6.1
import PackageDescription
let package = Package(
    name: "Determinism",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "DiffBase", swiftSettings: [.swiftLanguageMode(.v5)]),
        .target(name: "Cases", dependencies: ["DiffBase"], swiftSettings: [.swiftLanguageMode(.v5)]),
    ])
`)
	copySwift := func(glob, dir string) {
		t.Helper()
		files, err := filepath.Glob(filepath.Join(root, "testdata", "diff", glob))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Swift files at %s: %v", glob, err)
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			writeSource(t, filepath.Join(pkgDir, "Sources", dir, filepath.Base(f)), string(data))
		}
	}
	copySwift("*.swift", "Cases")
	copySwift(filepath.Join("base", "*.swift"), "DiffBase")
	writeSource(t, filepath.Join(pkgDir, "Sources", "Cases", "AppendsToManyArrays.swift"), appendsToManyArrays)

	t.Chdir(root)
	outDir := filepath.Join("out", "deterministictest")
	t.Cleanup(func() { _ = os.RemoveAll(outDir) })
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(pkgDir, ".build")) })
	targets := []string{"DiffBase", "Cases"}
	transpile := func() map[string]string {
		t.Helper()
		var err error
		log := captureStderr(t, func() {
			err = run("transpile", targets, pkgDir, outDir, swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{}, true, false, swiftmod.Destination{})
		})
		if err != nil {
			t.Fatalf("%v\n%s", err, log)
		}
		return goFiles(t, outDir)
	}
	first := transpile()
	for n := 2; n <= 10; n++ {
		// Without the emit cache the next run emits every file again from the cached dump.
		for _, target := range targets {
			if err := os.Remove(filepath.Join(outDir, ".cache", target, "emit.gob")); err != nil {
				t.Fatal(err)
			}
		}
		got := transpile()
		for name, want := range first {
			if got[name] != want {
				t.Errorf("emission %d wrote %s differently:\n%s\nfirst emission:\n%s", n, name, got[name], want)
			}
		}
		if len(got) != len(first) {
			t.Errorf("emission %d wrote %d Go files, the first %d", n, len(got), len(first))
		}
	}
	if !strings.Contains(first[filepath.Join("cases", "appends_to_many_arrays.go")], "min(len(words)") {
		t.Errorf("appends_to_many_arrays.go reserves no capacity:\n%s", first[filepath.Join("cases", "appends_to_many_arrays.go")])
	}
}
