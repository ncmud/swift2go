// Command swift2go transpiles a SwiftPM target to Go.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
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
	strict := flag.Bool("strict", os.Getenv("SWIFT2GO_STRICT") != "", "fail a target, printing the compiler's errors, when the compiler fails on one of its files, instead of transpiling the target without the file (default from SWIFT2GO_STRICT)")
	swiftSDK := flag.String("swift-sdk", "", "compile for an installed Swift SDK, named by its id or a target triple as for `swift build --swift-sdk`, and build the package's dependencies for it")
	triple := flag.String("triple", "", "the target triple of the Swift SDK to compile for, when it has more than one")
	var xcc listFlag
	flag.Var(&xcc, "Xcc", "pass `flag` to the C compiler that imports C modules, here and in the `swift build` that -swift-sdk runs (repeatable)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: swift2go [flags] dump|transpile|stats <Target> [<TestTarget>]")
		flag.PrintDefaults()
	}
	cpuprofile := flag.String("cpuprofile", os.Getenv("SWIFT2GO_CPUPROFILE"), "write a CPU profile of the run to `file` (default from SWIFT2GO_CPUPROFILE)")
	flag.Parse()
	if os.Getenv("GOGC") == "" {
		// The run keeps every target's AST live; collecting less often saves a fifth of a run that emits little.
		debug.SetGCPercent(400)
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile) //nolint:gosec // the file the user named
		if err != nil {
			fmt.Fprintln(os.Stderr, "swift2go:", err)
			os.Exit(2)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(os.Stderr, "swift2go:", err)
			os.Exit(2)
		}
		defer pprof.StopCPUProfile()
	}
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}
	dest, err := swiftmod.FindSwiftSDK(*swiftSDK, *triple)
	if err != nil {
		fmt.Fprintln(os.Stderr, "swift2go:", err)
		os.Exit(2)
	}
	if *buildDir == "" {
		*buildDir = swiftmod.BuildDir(*pkgDir, dest)
	}
	start := time.Now()
	if err := run(flag.Arg(0), flag.Args()[1:], *pkgDir, *outDir, absPath(*buildDir), *modules, maps, overlays, *incremental, *strict, dest, xcc...); err != nil {
		fmt.Fprintln(os.Stderr, "swift2go:", err)
		pprof.StopCPUProfile()
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "swift2go: ran in %s\n", time.Since(start).Round(time.Millisecond))
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

// listFlag collects the values of a repeatable flag.
type listFlag []string

func (f *listFlag) String() string { return strings.Join(*f, " ") }

func (f *listFlag) Set(v string) error {
	*f = append(*f, v)
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

func run(cmd string, names []string, pkgDir, outDir, buildDir, modulesDir string, maps mapFlag, overlays overlayFlag, incremental, strict bool, dest swiftmod.Destination, xcc ...string) error {
	pkgDir = absPath(pkgDir)
	if !dest.Host() {
		// A Swift SDK's modules come from a swift.org build of the compiler, which Xcode's compiler
		// of the same version refuses by its revision unless told otherwise.
		if err := os.Setenv("SWIFT_IGNORE_SWIFTMODULE_REVISION", "1"); err != nil {
			return err
		}
	}
	describe := func(dir string) (map[string]swiftmod.Target, error) { return swiftmod.Describe(dir, buildDir) }
	if incremental {
		describe = func(dir string) (map[string]swiftmod.Target, error) {
			return swiftmod.DescribeCached(dir, buildDir, absPath(filepath.Join(outDir, ".cache", "describe.gob")))
		}
	}
	targets, err := describe(pkgDir)
	if err != nil {
		return err
	}
	// swiftc and swift build spell C compiler flags alike.
	var ccFlags []string
	for _, f := range xcc {
		ccFlags = append(ccFlags, "-Xcc", f)
	}
	if err := swiftmod.BuildDependencies(pkgDir, buildDir, dest, targets, names, ccFlags...); err != nil {
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
	extra := slices.Concat(dest.Flags(), ccFlags, swiftmod.TestingFlags(dest), swiftmod.PrebuiltFlags(buildDir, dest), cflags)
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
	searchDirs := []string{mods, swiftmod.ModulesDir(buildDir, dest)}
	modules := &moduleJobs{jobs: map[string]*moduleJob{}}
	emitted := map[string]bool{}
	ifaces := map[string]string{}
	load := func(t swiftmod.Target) ([]ast.File, error) {
		if err := swiftmod.AddGenerated(&t, buildDir, absPath(filepath.Join(outDir, "derived")), dest); err != nil {
			return nil, err
		}
		if err := swiftmod.StaleModule(t, targets, emitted, searchDirs); err != nil {
			return nil, err
		}
		if incremental {
			c := cachedLoad{modules: modules, t: t, targets: targets, outDir: outDir, mods: mods, searchDirs: searchDirs, extra: extra, strict: strict}
			return c.load(ifaces, emitted)
		}
		if t.Type != "test" {
			if err := swiftmod.EmitModule(t, mods, extra...); err != nil {
				return nil, err
			}
			emitted[t.Name] = true
		}
		data, err := swiftmod.DumpAST(t, mods, absPath(filepath.Join(outDir, "derived")), strict, extra...)
		if err != nil {
			return nil, err
		}
		cache := filepath.Join(outDir, "ast", t.Name+".json")
		_ = os.MkdirAll(filepath.Dir(cache), 0o755)
		_ = os.WriteFile(cache, data, 0o644) //nolint:gosec // under the output directory it was given, as G304 allows for reads
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
		loadStart := time.Now()
		files, err := load(t)
		loaded := time.Since(loadStart)
		if err != nil {
			fail(name, err)
			continue
		}
		if cmd == "dump" {
			fmt.Printf("%s: %d files\n", name, len(files))
			continue
		}
		// Generated sources lie outside the package; the Go names them apart from where the build put them.
		// The copies of sources that macros extend keep the path from Sources or Tests down.
		roots := map[string]string{
			absPath(filepath.Join(outDir, "derived")):                   "derived",
			absPath(filepath.Join(outDir, "derived", t.Name, "macros")): "",
			absPath(swiftmod.PluginOutputDir(t, buildDir)):              path.Join("plugins", t.Name),
		}
		opts := []emit.Option{emit.Dependencies(done), emit.SourceRoots(roots)}
		if dest.CorelibsFoundation() {
			opts = append(opts, emit.CorelibsFoundation())
		}
		newStart := time.Now()
		m, err := emit.New(name, files, rtPath, opts...)
		if err != nil {
			fail(name, err)
			continue
		}
		if incremental {
			m.Interface = ifaces[name]
			m.EmitCache = absPath(filepath.Join(outDir, ".cache", name, "emit.gob"))
			m.EmitKey = transpilerKey()
			m.CheckEmitCache = os.Getenv("SWIFT2GO_CHECK_INCREMENTAL") != ""
		}
		m.Overlay = overlays[name]
		m.Mappings = mappings
		if importPath, ok := maps[name]; ok {
			// A mapped target is read for its declarations only: dependents import the hand-written package.
			if t.Type == "test" {
				fail(name, fmt.Errorf("a test target cannot be mapped"))
				continue
			}
			m.ImportPath = importPath
			m.Mapped = true
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
			if t.TestCycles(targets) {
				m.SetTestPackage()
			} else {
				m.SetTarget(under)
				m.Overlay = overlays[t.Deps[0]]
				pkgOut = filepath.Join(outDir, under.Pkg)
			}
		}
		m.ImportPath = path.Join(goModule, filepath.ToSlash(pkgOut))
		indexed := time.Since(newStart)
		writeStart := time.Now()
		if _, err := m.WriteGo(pkgOut); err != nil {
			fail(name, err)
			continue
		}
		if incremental {
			fmt.Fprintf(os.Stderr, "swift2go: %s: kept %d of %d Go files (loaded in %s, indexed in %s, wrote in %s)\n", name, m.Reused, len(m.Files), loaded.Round(time.Millisecond), indexed.Round(time.Millisecond), time.Since(writeStart).Round(time.Millisecond))
		}
		if len(m.Stale) > 0 {
			fail(name, fmt.Errorf("the incremental run would have kept Go files that emitting changes: %s", strings.Join(m.Stale, ", ")))
			continue
		}
		fmt.Print(m.Report())
		if t.Type != "test" {
			done = append(done, m)
			byName[name] = m
		}
	}
	if err := modules.waitAll(); err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d targets not transpiled: %s", len(failures), len(names), strings.Join(failures, ", "))
	}
	return nil
}

// moduleJobs are the module emissions an incremental run started, by target.
type moduleJobs struct {
	mu   sync.Mutex
	jobs map[string]*moduleJob
}

type moduleJob struct {
	done chan struct{}
	err  error
}

// start emits a target's module in the background.
func (m *moduleJobs) start(target string, emit func() error) {
	j := &moduleJob{done: make(chan struct{})}
	m.mu.Lock()
	m.jobs[target] = j
	m.mu.Unlock()
	go func() {
		defer close(j.done)
		if err := emit(); err != nil {
			j.err = fmt.Errorf("%s's module: %w", target, err)
		}
	}()
}

// waitAll waits for every module emission and returns the first error.
func (m *moduleJobs) waitAll() error {
	m.mu.Lock()
	targets := slices.Sorted(maps.Keys(m.jobs))
	m.mu.Unlock()
	return m.wait(targets)
}

// wait waits for the modules of the targets being emitted and returns the first error.
func (m *moduleJobs) wait(targets []string) error {
	for _, t := range targets {
		m.mu.Lock()
		j := m.jobs[t]
		m.mu.Unlock()
		if j == nil {
			continue
		}
		<-j.done
		if j.err != nil {
			return j.err
		}
	}
	return nil
}

// cachedLoad loads a target's AST through the cache under OUT/.cache/<Target>.
type cachedLoad struct {
	modules    *moduleJobs
	t          swiftmod.Target
	targets    map[string]swiftmod.Target
	outDir     string
	mods       string
	searchDirs []string
	extra      []string
	strict     bool
}

// load dumps the target's files that changed, reusing the cached ASTs of the others, and emits the
// target's module when its interface changed since the module was last emitted. ifaces holds the
// interfaces of the targets loaded earlier in the run; load adds the target's own.
func (c cachedLoad) load(ifaces map[string]string, emitted map[string]bool) ([]ast.File, error) {
	t := c.t
	start := time.Now()
	derived := absPath(filepath.Join(c.outDir, "derived"))
	cacheDir := absPath(filepath.Join(c.outDir, ".cache", t.Name))
	d, err := swiftmod.NewCachedDumper(cacheDir+".job", t, c.mods, derived, c.extra...)
	if err != nil {
		return nil, err
	}
	d.Strict = c.strict
	env := d.Key() + "\x00" + swiftmod.DependencyKey(t, c.targets, ifaces, c.searchDirs)
	// A strict run never caches a failed file, so it must not reuse a cache that has one.
	if c.strict {
		env += "\x00strict"
	}
	// The frontend reads the modules of the target's dependencies, which may still be being emitted.
	var waited sync.Once
	var waitErr error
	dumpFiles := func(primaries []int, copies map[int]string) ([]swiftmod.FileDump, error) {
		waited.Do(func() { waitErr = c.modules.wait(transitiveDeps(t, c.targets)) })
		if waitErr != nil {
			return nil, waitErr
		}
		return d.Dump(primaries, copies)
	}
	dump, err := swiftmod.DumpCached(cacheDir, env, d.Sources, dumpFiles)
	if err != nil {
		return nil, err
	}
	ifaces[t.Name] = dump.Interface
	fmt.Fprintf(os.Stderr, "swift2go: %s: dumped %d of %d files in %s\n", t.Name, dump.Dumped, len(dump.Docs), time.Since(start).Round(time.Millisecond))
	if t.Type != "test" {
		stamp := filepath.Join(cacheDir, "module.stamp")
		want := env + "\x00" + dump.Interface
		if old, err := os.ReadFile(stamp); err != nil || string(old) != want || !fileExists(filepath.Join(c.mods, t.Name+".swiftmodule")) {
			_ = os.Remove(stamp)
			deps := transitiveDeps(t, c.targets)
			// The target's Go is emitted meanwhile; only the targets that import it wait for its module.
			c.modules.start(t.Name, func() error {
				if err := c.modules.wait(deps); err != nil {
					return err
				}
				start := time.Now()
				if err := swiftmod.EmitModule(t, c.mods, c.extra...); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "swift2go: %s: emitted its module in %s\n", t.Name, time.Since(start).Round(time.Millisecond))
				return os.WriteFile(stamp, []byte(want), 0o644)
			})
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
			sum := sha256.Sum256(doc)
			for j := range files[i] {
				files[i][j].Hash = hex.EncodeToString(sum[:])
			}
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

// transpilerKey identifies the running swift2go binary, whose emitter decides the Go of every file.
var transpilerKey = sync.OnceValue(func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(exe) //nolint:gosec // the running executable
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
})

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

// transitiveDeps lists the targets t depends on, directly or not.
func transitiveDeps(t swiftmod.Target, targets map[string]swiftmod.Target) []string {
	seen := map[string]bool{}
	var out []string
	var visit func(names []string)
	visit = func(names []string) {
		for _, n := range names {
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
			if d, ok := targets[n]; ok {
				visit(d.AllDeps())
			}
		}
	}
	visit(t.AllDeps())
	return out
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
