package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/swifttest"
)

// TestOutputIsTheSameInAnyCheckout transpiles one package, with a resource bundle whose accessor swift2go
// generates outside the package, from two checkouts in different directories and checks that both write
// the same Go, which names neither directory.
func TestOutputIsTheSameInAnyCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("transpiles a Swift package")
	}
	swifttest.Require(t, "xcrun", "swift", "swiftc")
	files := map[string]string{
		"go.mod": "module example.com/repro\n\ngo 1.27.4\n",
		"swift/Package.swift": `// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "repro",
    targets: [
        .target(name: "Core", resources: [.copy("greeting.txt")]),
        .target(name: "App", dependencies: ["Core"]),
    ]
)
`,
		"swift/Sources/Core/greeting.txt": "hello\n",
		"swift/Sources/Core/Greeting.swift": `import Foundation

public func greeting() -> String {
    let url = Bundle.module.url(forResource: "greeting", withExtension: "txt")
    return "\(#filePath) \(#fileID) \(url == nil)"
}
`,
		"swift/Sources/App/App.swift": "import Core\n\npublic func run() -> String { greeting() + \" \" + #filePath }\n",
	}
	transpileIn := func(dir string) map[string]string {
		t.Helper()
		for name, src := range files {
			writeSource(t, filepath.Join(dir, name), src)
		}
		pkgDir := filepath.Join(dir, "swift")
		t.Chdir(dir)
		var err error
		log := captureStderr(t, func() {
			err = run("transpile", []string{"Core", "App"}, pkgDir, "out", swiftmod.BuildDir(pkgDir, swiftmod.Destination{}), "", mapFlag{}, overlayFlag{}, false, false, swiftmod.Destination{})
		})
		if err != nil {
			t.Fatalf("%v\n%s", err, log)
		}
		return goFiles(t, filepath.Join(dir, "out"))
	}
	dirA := filepath.Join(t.TempDir(), "first")
	dirB := filepath.Join(t.TempDir(), "elsewhere", "second")
	a := transpileIn(dirA)
	b := transpileIn(dirB)
	if len(a) == 0 {
		t.Fatal("the transpile wrote no Go")
	}
	for name, src := range a {
		if b[name] != src {
			t.Errorf("%s differs between checkouts:\n%s\nother checkout:\n%s", name, src, b[name])
		}
		for _, dir := range []string{dirA, dirB, filepath.Dir(dirA), filepath.Dir(filepath.Dir(dirB))} {
			if strings.Contains(src, dir) {
				t.Errorf("%s names the checkout directory %s:\n%s", name, dir, src)
			}
		}
	}
	if len(b) != len(a) {
		t.Errorf("the checkouts wrote %d and %d Go files", len(a), len(b))
	}
	if !strings.Contains(a[filepath.Join("core", "resource_bundle_accessor.go")], "swift2go from derived/Core/resource_bundle_accessor.swift.") {
		t.Errorf("the bundle accessor's header does not name it by its generated directory:\n%s", a[filepath.Join("core", "resource_bundle_accessor.go")])
	}
}
