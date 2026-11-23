// Package difftest checks translated programs against the output of the Swift originals.
//
// Each testdata/diff/<name>.swift declares `public func <name>()`. The expected output, recorded
// from Swift with `go test ./internal/difftest -update`, is testdata/diff/<name>.expected. The test
// transpiles every case into one Go package and runs each case's function as its own process. A case
// that traps records what it printed before the trap, then a line "TRAP"; the trap's own message
// differs between the languages and is not compared.
//
// Cases whose names start with "actor" run concurrent tasks on actors; they also run from a build with
// the race detector, which reports a race as a trap.
//
// testdata/diff/base holds the library DiffBase, which Cases depends on, for cases that use
// declarations of another package.
package difftest

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/ncmud/swift2go/internal/swifttest"
)

var update = flag.Bool("update", false, "record expected output by running the Swift cases")

const packageSwift = `// swift-tools-version:6.1
import PackageDescription
let package = Package(
    name: "DiffCases",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "DiffBase", swiftSettings: [.swiftLanguageMode(.v5)]),
        .target(name: "Cases", dependencies: ["DiffBase"], swiftSettings: [.swiftLanguageMode(.v5)]),
        .executableTarget(name: "Runner", dependencies: ["Cases"], swiftSettings: [.swiftLanguageMode(.v5)]),
    ])
`

func TestDiff(t *testing.T) {
	swifttest.Require(t, "xcrun", "swift", "swiftc")
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
	report := run(t, gomod, tool, "-pkg", pkg, "-out", "out", "transpile", "DiffBase", "Cases")
	if unsupported := regexp.MustCompile(`(?m)^\w+: [1-9]\d* unsupported$`); unsupported.Match(report) {
		t.Errorf("the cases use constructs swift2go reports as unsupported:\n%s", report)
	}

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
	raceBin := ""
	if strings.TrimSpace(string(run(t, gomod, "go", "env", "CGO_ENABLED"))) == "1" {
		raceBin = filepath.Join(work, "cases-race")
		run(t, gomod, "go", "build", "-race", "-o", raceBin, "./main")
	}

	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(root, "testdata", "diff", n+".expected"))
			if err != nil {
				t.Fatalf("no expected output; run go test ./internal/difftest -update: %v", err)
			}
			got, _ := caseOutput(bin, n)
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from Swift\n%s", lineDiff(string(want), string(got)))
			}
			if raceBin == "" || !strings.HasPrefix(n, "actor") {
				return
			}
			if got, _ := caseOutput(raceBin, n); !bytes.Equal(got, want) {
				report, _ := exec.Command(raceBin, n).CombinedOutput()
				t.Errorf("output under the race detector differs from Swift\n%s", report)
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
	base := filepath.Join(pkg, "Sources", "DiffBase")
	runner := filepath.Join(pkg, "Sources", "Runner")
	must(t, os.MkdirAll(cases, 0o755))
	must(t, os.MkdirAll(base, 0o755))
	must(t, os.MkdirAll(runner, 0o755))
	must(t, os.WriteFile(filepath.Join(pkg, "Package.swift"), []byte(packageSwift), 0o644))
	baseSources, err := filepath.Glob(filepath.Join(filepath.Dir(sources[0]), "base", "*.swift"))
	must(t, err)
	copyInto := func(dir string, files []string) {
		for _, s := range files {
			data, err := os.ReadFile(s)
			must(t, err)
			must(t, os.WriteFile(filepath.Join(dir, filepath.Base(s)), data, 0o644))
		}
	}
	copyInto(cases, sources)
	copyInto(base, baseSources)
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
		out, err := caseOutput(bin, n)
		if err != nil {
			t.Fatalf("Swift case %s failed: %v\n%s", n, err, out)
		}
		must(t, os.WriteFile(filepath.Join(root, "testdata", "diff", n+".expected"), out, 0o644))
	}
}

// caseOutput runs one case and returns its standard output followed by its standard error. A case
// that exits on a signal or with a nonzero status trapped: it returns the standard output followed
// by "TRAP".
func caseOutput(bin, name string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, name)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return stdout.Bytes(), ctx.Err()
	}
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		return append(stdout.Bytes(), "TRAP\n"...), nil
	}
	return append(stdout.Bytes(), stderr.Bytes()...), err
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

func run(t *testing.T, dir, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
