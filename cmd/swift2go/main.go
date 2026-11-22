// Command swift2go transpiles a SwiftPM target to Go.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

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
	overlays := overlayFlag{}
	flag.Var(overlays, "overlay", "`Target=dir`: hand-written Go files in dir replace the target's Swift files of the same name (repeatable)")
	incremental := flag.Bool("incremental", false, "re-dump only the files whose content or dependencies changed since an earlier incremental run, which cached the ASTs under OUT/.cache")
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
	if err := run(flag.Arg(0), flag.Args()[1:], *pkgDir, *outDir, absPath(*buildDir), *modules, maps, overlays, *incremental); err != nil {
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

// overlayFlag collects -overlay Target=dir pairs.
type overlayFlag map[string]string

func (o overlayFlag) String() string { return "" }

func (o overlayFlag) Set(v string) error {
	target, dir, ok := strings.Cut(v, "=")
	if !ok || target == "" || dir == "" {
		return fmt.Errorf("want Target=dir, got %q", v)
	}
	o[target] = absPath(dir)
	return nil
}

func run(cmd string, names []string, pkgDir, outDir, buildDir, modulesDir string, maps mapFlag, overlays overlayFlag, incremental bool) error {
	pkgDir = absPath(pkgDir)
	describe := swiftmod.Describe
	if incremental {
		describe = func(dir string) (map[string]swiftmod.Target, error) {
			return swiftmod.DescribeCached(dir, absPath(filepath.Join(outDir, ".cache", "describe.gob")))
		}
	}
	targets, err := describe(pkgDir)
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
	for i, mp := range mappings {
		if t, ok := targets[mp.Module]; ok && t.ModuleType == "ClangTarget" {
			continue
		}
		reqs, err := swiftmod.ProtocolRequirements(mp.Module, append([]string{"-I", mods}, extra...))
		if err != nil {
			fmt.Fprintf(os.Stderr, "swift2go: -map %s: conformances register no witnesses: %v\n", mp.Module, err)
			continue
		}
		mappings[i].Requirements = reqs
	}
	searchDirs := []string{mods, filepath.Join(buildDir, "out", "Products", "Debug")}
	emitted := map[string]bool{}
	ifaces := map[string]string{}
	load := func(t swiftmod.Target) ([]ast.File, error) {
		if err := swiftmod.AddGenerated(&t, buildDir, absPath(filepath.Join(outDir, "derived"))); err != nil {
			return nil, err
		}
		if err := swiftmod.StaleModule(t, targets, emitted, searchDirs); err != nil {
			return nil, err
		}
		if incremental {
			c := cachedLoad{t: t, targets: targets, outDir: outDir, mods: mods, searchDirs: searchDirs, extra: extra}
			return c.load(ifaces, emitted)
		}
		if t.Type != "test" {
			if err := swiftmod.EmitModule(t, mods, extra...); err != nil {
				return nil, err
			}
			emitted[t.Name] = true
		}
		data, err := swiftmod.DumpAST(t, mods, absPath(filepath.Join(outDir, "derived")), extra...)
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
	for target := range overlays {
		if !slices.Contains(names, target) {
			return fmt.Errorf("-overlay %s: the target is not in this run", target)
		}
		if _, mapped := maps[target]; mapped {
			return fmt.Errorf("-overlay %s: the target is mapped to a Go package, which replaces all of it", target)
		}
	}
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
		m.Overlay = overlays[name]
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
			m.Overlay = overlays[t.Deps[0]]
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

// cachedLoad loads a target's AST through the cache under OUT/.cache/<Target>.
type cachedLoad struct {
	t          swiftmod.Target
	targets    map[string]swiftmod.Target
	outDir     string
	mods       string
	searchDirs []string
	extra      []string
}

// load dumps the target's files that changed, reusing the cached ASTs of the others, and emits the
// target's module when its interface changed since the module was last emitted. ifaces holds the
// interfaces of the targets loaded earlier in the run; load adds the target's own.
func (c cachedLoad) load(ifaces map[string]string, emitted map[string]bool) ([]ast.File, error) {
	t := c.t
	start := time.Now()
	derived := absPath(filepath.Join(c.outDir, "derived"))
	d, err := swiftmod.NewDumper(t, c.mods, derived, c.extra...)
	if err != nil {
		return nil, err
	}
	env := d.Key() + "\x00" + swiftmod.DependencyKey(t, c.targets, ifaces, c.searchDirs)
	cacheDir := absPath(filepath.Join(c.outDir, ".cache", t.Name))
	dump, err := swiftmod.DumpCached(cacheDir, env, d.Sources, d.Dump)
	if err != nil {
		return nil, err
	}
	ifaces[t.Name] = dump.Interface
	fmt.Fprintf(os.Stderr, "swift2go: %s: dumped %d of %d files in %s\n", t.Name, dump.Dumped, len(dump.Docs), time.Since(start).Round(time.Millisecond))
	if t.Type != "test" {
		stamp := filepath.Join(cacheDir, "module.stamp")
		want := env + "\x00" + dump.Interface
		if old, err := os.ReadFile(stamp); err != nil || string(old) != want || !fileExists(filepath.Join(c.mods, t.Name+".swiftmodule")) {
			start := time.Now()
			_ = os.Remove(stamp)
			if err := swiftmod.EmitModule(t, c.mods, c.extra...); err != nil {
				return nil, err
			}
			if err := os.WriteFile(stamp, []byte(want), 0o644); err != nil {
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "swift2go: %s: emitted its module in %s\n", t.Name, time.Since(start).Round(time.Millisecond))
		}
		emitted[t.Name] = true
	}
	files := make([][]ast.File, len(dump.Docs))
	errs := make([]error, len(dump.Docs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, doc := range dump.Docs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			files[i], errs[i] = ast.Load(doc)
		})
	}
	wg.Wait()
	var all []ast.File
	for i := range files {
		if errs[i] != nil {
			return nil, errs[i]
		}
		all = append(all, files[i]...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("swiftc -dump-ast produced no output for %s", t.Name)
	}
	return all, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
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
