// Package difftest checks translated programs against the output of the Swift originals.
//
// Each testdata/diff/<name>.swift declares `public func <name>()`. The expected output, recorded
// from Swift with `go test ./internal/difftest -update`, is testdata/diff/<name>.expected. The test
// transpiles every case into one Go package and runs each case's function as its own process.
package difftest

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

var update = flag.Bool("update", false, "record expected output by running the Swift cases")

const packageSwift = `// swift-tools-version:6.1
import PackageDescription
let package = Package(
    name: "DiffCases",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "Cases", swiftSettings: [.swiftLanguageMode(.v5)]),
        .executableTarget(name: "Runner", dependencies: ["Cases"], swiftSettings: [.swiftLanguageMode(.v5)]),
    ])
`

func TestDiff(t *testing.T) {
	if _, err := exec.LookPath("xcrun"); err != nil || runtime.GOOS != "darwin" {
		t.Skip("swift2go needs the Swift toolchain on macOS")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sources, err := filepath.Glob(filepath.Join(root, "testdata", "diff", "*.swift"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("no cases in testdata/diff: %v", err)
	}
	var names []string
	for _, s := range sources {
		names = append(names, strings.TrimSuffix(filepath.Base(s), ".swift"))
	}
	readSources(t, root)
	work := t.TempDir()
	pkg := filepath.Join(work, "swift")
	writeSwiftPackage(t, pkg, sources, names)
	if *update {
		record(t, pkg, root, names)
	}

	gomod := filepath.Join(work, "go")
	must(t, os.MkdirAll(filepath.Join(gomod, "main"), 0o755))
	must(t, os.WriteFile(filepath.Join(gomod, "go.mod"), fmt.Appendf(nil,
		"module difftest\n\ngo 1.27.4\n\nrequire github.com/ncmud/swift2go v0.0.0\n\nreplace github.com/ncmud/swift2go => %s\n", root), 0o644))
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	must(t, err)
	must(t, os.WriteFile(filepath.Join(gomod, "go.sum"), sum, 0o644))
	tool := filepath.Join(work, "swift2go")
	run(t, root, "go", "build", "-o", tool, "./cmd/swift2go")
	run(t, gomod, tool, "-pkg", pkg, "-out", "out", "transpile", "Cases")

	var mainGo strings.Builder
	mainGo.WriteString("package main\n\nimport (\n\t\"os\"\n\n\tcases \"difftest/out/cases\"\n)\n\nvar all = map[string]func(){\n")
	for _, n := range names {
		fmt.Fprintf(&mainGo, "\t%q: cases.%s,\n", n, exported(n))
	}
	mainGo.WriteString("}\n\nfunc main() { all[os.Args[1]]() }\n")
	must(t, os.WriteFile(filepath.Join(gomod, "main", "main.go"), []byte(mainGo.String()), 0o644))
	bin := filepath.Join(work, "cases")
	run(t, gomod, "go", "mod", "tidy")
	run(t, gomod, "go", "build", "-o", bin, "./main")

	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(root, "testdata", "diff", n+".expected"))
			if err != nil {
				t.Fatalf("no expected output; run go test ./internal/difftest -update: %v", err)
			}
			got, _ := exec.Command(bin, n).CombinedOutput()
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from Swift\n%s", lineDiff(string(want), string(got)))
			}
		})
	}
}

// readSources reads the transpiler's and runtime's Go sources. The cases build and run them in
// subprocesses, which go test's result cache does not see; reading them here makes a change to
// them run the test again.
func readSources(t *testing.T, root string) {
	for _, dir := range []string{"cmd", "internal", "swiftrt"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
				_, err = os.ReadFile(path)
			}
			return err
		})
		must(t, err)
	}
}

func writeSwiftPackage(t *testing.T, pkg string, sources, names []string) {
	cases := filepath.Join(pkg, "Sources", "Cases")
	runner := filepath.Join(pkg, "Sources", "Runner")
	must(t, os.MkdirAll(cases, 0o755))
	must(t, os.MkdirAll(runner, 0o755))
	must(t, os.WriteFile(filepath.Join(pkg, "Package.swift"), []byte(packageSwift), 0o644))
	for _, s := range sources {
		data, err := os.ReadFile(s)
		must(t, err)
		must(t, os.WriteFile(filepath.Join(cases, filepath.Base(s)), data, 0o644))
	}
	var main strings.Builder
	main.WriteString("import Cases\n\nlet all: [String: () -> Void] = [\n")
	for _, n := range names {
		fmt.Fprintf(&main, "    %q: %s,\n", n, n)
	}
	main.WriteString("]\nall[CommandLine.arguments[1]]!()\n")
	must(t, os.WriteFile(filepath.Join(runner, "main.swift"), []byte(main.String()), 0o644))
}

func record(t *testing.T, pkg, root string, names []string) {
	run(t, pkg, "swift", "build")
	bin := filepath.Join(pkg, ".build", "debug", "Runner")
	for _, n := range names {
		out, err := exec.Command(bin, n).CombinedOutput()
		if err != nil {
			t.Fatalf("Swift case %s failed: %v\n%s", n, err, out)
		}
		must(t, os.WriteFile(filepath.Join(root, "testdata", "diff", n+".expected"), out, 0o644))
	}
}

func exported(name string) string {
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	for i := range max(len(w), len(g)) {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			fmt.Fprintf(&b, "line %d\n  swift: %q\n  go:    %q\n", i+1, wl, gl)
		}
	}
	return b.String()
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
