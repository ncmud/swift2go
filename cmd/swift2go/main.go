// Command swift2go transpiles a SwiftPM target to Go.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/emit"
	"github.com/ncmud/swift2go/internal/swiftmod"
)

const rtPath = "github.com/ncmud/swift2go/swiftrt"

func main() {
	pkgDir := flag.String("pkg", ".", "SwiftPM package directory")
	outDir := flag.String("out", "out", "output directory")
	modules := flag.String("modules", "", "directory of built .swiftmodule files for dependencies")
	buildDir := flag.String("build", "", "SwiftPM build directory with the package's built dependencies (default: the package's .build)")
	maps := mapFlag{}
	flag.Var(maps, "map", "use the Go package `Module=importpath` for a Swift module instead of transpiling it (repeatable)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: swift2go [flags] dump|transpile|stats <Target> [<TestTarget>]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}
	if *buildDir == "" {
		*buildDir = swiftmod.BuildDir(*pkgDir)
	}
	if err := run(flag.Arg(0), flag.Args()[1:], *pkgDir, *outDir, absPath(*buildDir), *modules, maps); err != nil {
		fmt.Fprintln(os.Stderr, "swift2go:", err)
		os.Exit(1)
	}
}

// mapFlag collects -map Module=importpath pairs.
type mapFlag map[string]string

func (f mapFlag) String() string { return "" }

func (f mapFlag) Set(v string) error {
	mod, importPath, ok := strings.Cut(v, "=")
	if !ok || mod == "" || importPath == "" {
		return fmt.Errorf("want Module=importpath, got %q", v)
	}
	f[mod] = importPath
	return nil
}

func run(cmd string, names []string, pkgDir, outDir, buildDir, modulesDir string, maps mapFlag) error {
	pkgDir = absPath(pkgDir)
	targets, err := swiftmod.Describe(pkgDir)
	if err != nil {
		return err
	}
	goModule, err := currentModule()
	if err != nil && cmd == "transpile" {
		return err
	}
	mappings, err := foreignMappings(maps, names)
	if err != nil && cmd == "transpile" {
		return err
	}
	mods := absPath(filepath.Join(outDir, "modules"))
	cflags, err := swiftmod.CModuleFlags(targets, buildDir, absPath(filepath.Join(outDir, "modulemaps")))
	if err != nil {
		return err
	}
	extra := append(append(swiftmod.TestingFlags(), swiftmod.PrebuiltFlags(buildDir)...), cflags...)
	searchDirs := []string{mods, filepath.Join(buildDir, "out", "Products", "Debug")}
	emitted := map[string]bool{}
	load := func(t swiftmod.Target) ([]ast.File, error) {
		if err := swiftmod.AddGenerated(&t, buildDir, absPath(filepath.Join(outDir, "derived"))); err != nil {
			return nil, err
		}
		if err := swiftmod.StaleModule(t, targets, emitted, searchDirs); err != nil {
			return nil, err
		}
		if t.Type != "test" {
			if err := swiftmod.EmitModule(t, mods, extra...); err != nil {
				return nil, err
			}
			emitted[t.Name] = true
		}
		data, err := swiftmod.DumpAST(t, mods, extra...)
		if err != nil {
			return nil, err
		}
		cache := filepath.Join(outDir, "ast", t.Name+".json")
		_ = os.MkdirAll(filepath.Dir(cache), 0o755)
		_ = os.WriteFile(cache, data, 0o644)
		return ast.Load(data)
	}
	var done []*emit.Module
	byName := map[string]*emit.Module{}
	failed := map[string]string{}
	var failures []string
	fail := func(name string, err error) {
		failed[name] = name
		failures = append(failures, name)
		fmt.Fprintf(os.Stderr, "swift2go: %s failed: %v\n", name, err)
	}
	for _, name := range names {
		t, ok := targets[name]
		if !ok {
			fail(name, fmt.Errorf("no target %q", name))
			continue
		}
		if dep := failedDep(t, targets, failed); dep != "" {
			failed[name] = failed[dep]
			failures = append(failures, name)
			fmt.Fprintf(os.Stderr, "swift2go: skipping %s: it depends on %s, which failed\n", name, failed[dep])
			continue
		}
		files, err := load(t)
		if err != nil {
			fail(name, err)
			continue
		}
		if cmd == "dump" {
			fmt.Printf("%s: %d files\n", name, len(files))
			continue
		}
		m, err := emit.New(name, files, rtPath)
		if err != nil {
			fail(name, err)
			continue
		}
		m.Deps = done
		m.Mappings = mappings
		if importPath, ok := maps[name]; ok {
			// A mapped target is read for its declarations only: dependents import the hand-written package.
			if t.Type == "test" {
				fail(name, fmt.Errorf("a test target cannot be mapped"))
				continue
			}
			m.ImportPath = importPath
			fmt.Printf("%s: mapped to %s\n", name, importPath)
			done = append(done, m)
			byName[name] = m
			continue
		}
		pkgOut := filepath.Join(outDir, strings.ToLower(name))
		if t.Type == "test" {
			if len(t.Deps) == 0 || byName[t.Deps[0]] == nil {
				fail(name, fmt.Errorf("transpile its target %v first", t.Deps))
				continue
			}
			under := byName[t.Deps[0]]
			if _, ok := maps[under.Name]; ok {
				fail(name, fmt.Errorf("its target %s is mapped to a Go package", under.Name))
				continue
			}
			m.SetTarget(under)
			pkgOut = filepath.Join(outDir, under.Pkg)
		}
		m.ImportPath = path.Join(goModule, filepath.ToSlash(pkgOut))
		if _, err := m.WriteGo(pkgOut); err != nil {
			fail(name, err)
			continue
		}
		fmt.Print(m.Report())
		if t.Type != "test" {
			done = append(done, m)
			byName[name] = m
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d targets not transpiled: %s", len(failures), len(names), strings.Join(failures, ", "))
	}
	return nil
}

// foreignMappings resolves the mapped modules that are not transpiled in this run. Their packages must be
// importable from the current Go module; their exported names replace the runtime's.
func foreignMappings(maps mapFlag, names []string) ([]emit.Mapping, error) {
	inRun := map[string]bool{}
	for _, n := range names {
		inRun[n] = true
	}
	var mods []string
	for mod := range maps {
		if !inRun[mod] {
			mods = append(mods, mod)
		}
	}
	slices.Sort(mods)
	var out []emit.Mapping
	for _, mod := range mods {
		dir, err := exec.Command("go", "list", "-f", "{{.Dir}}", maps[mod]).Output()
		if err != nil {
			return nil, fmt.Errorf("-map %s=%s: go list: %w", mod, maps[mod], err)
		}
		pkg, exports, generic, err := emit.PackageExports(strings.TrimSpace(string(dir)))
		if err != nil {
			return nil, fmt.Errorf("-map %s: %w", mod, err)
		}
		out = append(out, emit.Mapping{Module: mod, ImportPath: maps[mod], Package: pkg, Exports: exports, Generic: generic})
	}
	return out, nil
}

// failedDep returns a dependency of t, direct or not, that has failed in this run.
func failedDep(t swiftmod.Target, targets map[string]swiftmod.Target, failed map[string]string) string {
	seen := map[string]bool{}
	var visit func(names []string) string
	visit = func(names []string) string {
		for _, n := range names {
			if seen[n] {
				continue
			}
			seen[n] = true
			if _, ok := failed[n]; ok {
				return n
			}
			if d, ok := targets[n]; ok {
				if f := visit(d.AllDeps()); f != "" {
					return f
				}
			}
		}
		return ""
	}
	return visit(t.AllDeps())
}

func absPath(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// currentModule is the module path in ./go.mod: generated packages are written inside the Go
// module the command runs in, and import one another by paths under it.
func currentModule() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("run swift2go from the root of the Go module that will hold the output: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(mod), `"`), nil
		}
	}
	return "", fmt.Errorf("go.mod has no module line")
}
