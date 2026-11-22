// Package swiftmod reads a SwiftPM package's targets and dumps their typed ASTs.
package swiftmod

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Target is one SwiftPM target.
type Target struct {
	Name    string   `json:"name"`
	C99Name string   `json:"c99name"`
	Path    string   `json:"path"`
	Sources []string `json:"sources"`
	Deps    []string `json:"target_dependencies"`
	// ProductDeps are dependencies through a package's products, which share the target's name here.
	ProductDeps []string `json:"product_dependencies"`
	Type        string   `json:"type"`
	// ModuleType is SwiftTarget, ClangTarget, SystemLibraryTarget, ...
	ModuleType string     `json:"module_type"`
	Resources  []Resource `json:"resources"`
	// Dir is the root of the SwiftPM package that declares the target.
	Dir string `json:"-"`
	// Package is the name the package's manifest gives it, which names its resource bundles.
	Package string `json:"-"`
	// PublicHeaders is the public header directory of a C target, relative to Path.
	PublicHeaders string `json:"-"`
	// Plugins names the build-tool plugins the target uses.
	Plugins []string `json:"-"`
	// Generated are absolute paths of Swift sources compiled with the target that are not in
	// its directory: SwiftPM's resource accessor and build-tool plugin outputs. See [AddGenerated].
	Generated []string `json:"-"`
}

// Resource is a file SwiftPM bundles with a target.
type Resource struct {
	Path string `json:"path"`
}

// AllDeps are the target's dependencies through targets and through products.
func (t Target) AllDeps() []string {
	return append(append([]string{}, t.Deps...), t.ProductDeps...)
}

// SourcePaths are the absolute paths of every Swift file compiled into the target.
func (t Target) SourcePaths() []string {
	var paths []string
	for _, s := range t.Sources {
		paths = append(paths, filepath.Join(t.Dir, t.Path, s))
	}
	return append(paths, t.Generated...)
}

// Describe returns the targets of the package rooted at pkgDir and of every package in its
// dependency graph: local packages and remote checkouts alike. A target of the root package
// shadows a same-named target of a dependency. Remote dependencies are those checked out in buildDir,
// which are resolved there when it is not pkgDir/.build and holds none yet.
func Describe(pkgDir, buildDir string) (map[string]Target, error) {
	pkgDir, err := filepath.Abs(pkgDir)
	if err != nil {
		return nil, err
	}
	_, targets, err := describe(pkgDir, buildDir)
	return targets, err
}

// describe returns the directories of the packages in pkgDir's dependency graph, pkgDir first,
// and their targets.
func describe(pkgDir, buildDir string) ([]string, map[string]Target, error) {
	scratch, err := os.MkdirTemp("", "swift2go-describe-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(scratch)
	dirs := append([]string{pkgDir}, dependencyDirs(pkgDir, buildDir, scratch)...)
	subs := make([]map[string]Target, len(dirs))
	errs := make([]error, len(dirs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			subs[i], errs[i] = describeOne(dir, filepath.Join(scratch, fmt.Sprint(i)), i == 0)
		}()
	}
	wg.Wait()
	m := map[string]Target{}
	for i, sub := range subs {
		if errs[i] != nil {
			return nil, nil, fmt.Errorf("%s: %w", dirs[i], errs[i])
		}
		for name, t := range sub {
			if _, dup := m[name]; !dup {
				m[name] = t
			}
		}
	}
	return dirs, m, nil
}

// dependencyDirs lists the package directories in pkgDir's dependency graph, breadth first,
// followed by any packages under pkgDir/Packages that the graph does not reach.
func dependencyDirs(pkgDir, buildDir, scratch string) []string {
	seen := map[string]bool{pkgDir: true}
	var dirs []string
	add := func(dir string) {
		if dir != "" && !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	args := []string{"package", "--disable-automatic-resolution", "show-dependencies", "--format", "json"}
	switch {
	case buildDir != "" && exists(filepath.Join(buildDir, "checkouts")):
		args = append([]string{"package", "--scratch-path", buildDir}, args[1:]...)
	case buildDir != "" && buildDir != filepath.Join(pkgDir, ".build"):
		// A build directory of its own that is still empty: the checkouts resolved into it stay, for
		// the build of the dependencies and the targets described from them.
		args = []string{"package", "--scratch-path", buildDir, "show-dependencies", "--format", "json"}
	case !exists(filepath.Join(pkgDir, ".build")):
		args = append([]string{"package", "--scratch-path", filepath.Join(scratch, "graph")}, args[1:]...)
	}
	cmd := exec.Command("swift", args...)
	cmd.Dir = pkgDir
	out, err := cmd.Output()
	type node struct {
		Path         string `json:"path"`
		Dependencies []node `json:"dependencies"`
	}
	var root node
	if err == nil {
		err = json.Unmarshal(out, &root)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "swift2go: swift package show-dependencies failed in %s (%v); only packages under Packages/ are described\n", pkgDir, err)
	}
	for level := root.Dependencies; len(level) > 0; {
		var next []node
		for _, n := range level {
			add(n.Path)
			next = append(next, n.Dependencies...)
		}
		level = next
	}
	locals, _ := filepath.Glob(filepath.Join(pkgDir, "Packages", "*", "Package.swift"))
	for _, manifest := range locals {
		add(filepath.Dir(manifest))
	}
	return dirs
}

// describeOne describes one package, with SwiftPM's state in scratch rather than in the package.
// It reads the manifest as well for the root package and for packages with C targets.
func describeOne(pkgDir, scratch string, root bool) (map[string]Target, error) {
	global := []string{"--scratch-path", scratch}
	cmd := exec.Command("swift", append(append([]string{"package"}, global...), "describe", "--type", "json")...)
	cmd.Dir = pkgDir
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("swift package describe: %w\n%s", err, exit.Stderr)
		}
		return nil, fmt.Errorf("swift package describe: %w", err)
	}
	var d struct {
		Name    string   `json:"name"`
		Targets []Target `json:"targets"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}
	m := map[string]Target{}
	needManifest := root
	for _, t := range d.Targets {
		t.Dir = pkgDir
		t.Package = d.Name
		if len(t.Deps) == 0 {
			t.Deps = t.ProductDeps
		}
		if t.ModuleType == "ClangTarget" {
			t.PublicHeaders = "include"
			needManifest = true
		}
		m[t.Name] = t
	}
	if needManifest {
		if err := readManifest(pkgDir, global, m); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// readManifest fills in what `describe` leaves out: C targets' public header directories and
// the build-tool plugins each target uses.
func readManifest(pkgDir string, global []string, m map[string]Target) error {
	cmd := exec.Command("swift", append(append([]string{"package"}, global...), "dump-package")...)
	cmd.Dir = pkgDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("swift package dump-package: %w", err)
	}
	var d struct {
		Targets []struct {
			Name              string  `json:"name"`
			PublicHeadersPath *string `json:"publicHeadersPath"`
			PluginUsages      []struct {
				Plugin []any `json:"plugin"`
			} `json:"pluginUsages"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return err
	}
	for _, mt := range d.Targets {
		t, ok := m[mt.Name]
		if !ok {
			continue
		}
		if mt.PublicHeadersPath != nil && t.ModuleType == "ClangTarget" {
			t.PublicHeaders = *mt.PublicHeadersPath
		}
		for _, u := range mt.PluginUsages {
			if len(u.Plugin) > 0 {
				t.Plugins = append(t.Plugins, fmt.Sprint(u.Plugin[0]))
			}
		}
		m[mt.Name] = t
	}
	return nil
}

// BuildDir is the SwiftPM build directory holding pkgDir's built dependencies for d: pkgDir/.build,
// or, for a package checked out under another package's .build/checkouts, that package's build directory.
func BuildDir(pkgDir string, d Destination) string {
	pkgDir, _ = filepath.Abs(pkgDir)
	own := filepath.Join(pkgDir, ".build")
	if _, err := os.Stat(ProductsDir(own, d)); err == nil {
		return own
	}
	if parent := filepath.Dir(pkgDir); filepath.Base(parent) == "checkouts" {
		enclosing := filepath.Dir(parent)
		if _, err := os.Stat(ProductsDir(enclosing, d)); err == nil {
			return enclosing
		}
	}
	return own
}

// ProductsDir is the directory of buildDir that holds the products of a debug build for d. The
// host's are laid out by SwiftPM's default build system; another destination's by the native one,
// which [BuildDependencies] runs.
func ProductsDir(buildDir string, d Destination) string {
	if d.Host() {
		return filepath.Join(buildDir, "out", "Products", "Debug")
	}
	return filepath.Join(buildDir, d.Triple, "debug")
}

// ModulesDir is the directory of buildDir that holds the .swiftmodule files built for d.
func ModulesDir(buildDir string, d Destination) string {
	if d.Host() {
		return ProductsDir(buildDir, d)
	}
	return filepath.Join(ProductsDir(buildDir, d), "Modules")
}

// AddGenerated adds the Swift sources SwiftPM would generate for t to t.Generated: a
// `Bundle.module` accessor, written to derivedDir, when t has resources, and the outputs of the
// build-tool plugins t uses, read from buildDir. It fails when plugin outputs are missing or
// older than a non-Swift file in the target's directory, since `swift build` must regenerate them.
func AddGenerated(t *Target, buildDir, derivedDir string, d Destination) error {
	if len(t.Resources) > 0 {
		f, err := writeBundleAccessor(*t, derivedDir, d.ResourceBundleSuffix())
		if err != nil {
			return err
		}
		t.Generated = append(t.Generated, f)
	}
	outputs, err := pluginOutputs(*t, buildDir, ModulesDir(buildDir, d))
	if err != nil {
		return err
	}
	t.Generated = append(t.Generated, outputs...)
	return nil
}

// bundleName is the name SwiftPM gives a target's resource bundle: the package's name as its manifest
// spells it, then the target's.
func bundleName(t Target) string {
	pkg := t.Package
	if pkg == "" {
		pkg = strings.ToLower(filepath.Base(t.Dir))
	}
	return pkg + "_" + t.Name
}

func writeBundleAccessor(t Target, derivedDir, suffix string) (string, error) {
	dir := filepath.Join(derivedDir, t.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	src := fmt.Sprintf(`import class Foundation.Bundle
import struct Foundation.URL

extension Foundation.Bundle {
    /// Returns the resource bundle associated with the current Swift module.
    static nonisolated let module: Bundle = {
        let bundleName = %q
        let candidates = [Bundle.main.resourceURL, Bundle.main.bundleURL]
        for candidate in candidates {
            let bundlePath = candidate?.appendingPathComponent(bundleName + %q)
            if let bundle = bundlePath.flatMap(Bundle.init(url:)) {
                return bundle
            }
        }
        fatalError("unable to find bundle named \(bundleName)")
    }()
}
`, bundleName(t), suffix)
	path := filepath.Join(dir, "resource_bundle_accessor.swift")
	return path, os.WriteFile(path, []byte(src), 0o644)
}

// pluginOutputs lists the Swift files build-tool plugins generated for t under buildDir, whose
// built modules are in modulesDir.
func pluginOutputs(t Target, buildDir, modulesDir string) ([]string, error) {
	identity := strings.ToLower(filepath.Base(t.Dir))
	root := filepath.Join(buildDir, "plugins", "outputs", identity, t.Name, "destination")
	var files []string
	var newest time.Time
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".swift") {
			return nil
		}
		files = append(files, p)
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if len(t.Plugins) == 0 && len(files) == 0 {
		return nil, nil
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s uses build-tool plugins (%s) and %s holds none of their output; run `swift build --target %s` in %s first",
			t.Name, strings.Join(t.Plugins, ", "), root, t.Name, t.Dir)
	}
	if built := modTime(filepath.Join(modulesDir, t.Name+".swiftmodule")); built.After(newest) {
		newest = built
	}
	targetDir := filepath.Join(t.Dir, t.Path)
	var stale string
	_ = filepath.WalkDir(targetDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".swift") || stale != "" {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			stale = p
		}
		return nil
	})
	if stale != "" {
		return nil, fmt.Errorf("%s: the build-tool plugin outputs in %s are older than %s; run `swift build --target %s` in %s to regenerate them",
			t.Name, root, stale, t.Name, t.Dir)
	}
	sort.Strings(files)
	return files, nil
}

// modTime is the newest modification time of path or, for a directory, of any file in it.
func modTime(path string) time.Time {
	var newest time.Time
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// StaleModule reports a module that loading t would read from searchDirs although it is older
// than its own sources. It checks every target t depends on, directly or not, except those in
// fresh, which this run has already emitted. A dependency with no module in searchDirs is left to swiftc.
func StaleModule(t Target, targets map[string]Target, fresh map[string]bool, searchDirs []string) error {
	seen := map[string]bool{}
	var visit func(name string) error
	visit = func(name string) error {
		dep, ok := targets[name]
		if !ok || seen[name] {
			return nil
		}
		seen[name] = true
		for _, d := range dep.AllDeps() {
			if err := visit(d); err != nil {
				return err
			}
		}
		if fresh[name] || dep.ModuleType != "SwiftTarget" {
			return nil
		}
		modName := dep.C99Name
		if modName == "" {
			modName = name
		}
		for _, dir := range searchDirs {
			mod := filepath.Join(dir, modName+".swiftmodule")
			if _, err := os.Stat(mod); err != nil {
				continue
			}
			built := modTime(mod)
			for _, src := range dep.SourcePaths() {
				if modTime(src).After(built) {
					return fmt.Errorf("%s imports %s, and %s is older than %s; transpile %s in the same run, or rebuild it",
						t.Name, name, mod, src, name)
				}
			}
			return nil
		}
		return nil
	}
	for _, d := range t.AllDeps() {
		if err := visit(d); err != nil {
			return err
		}
	}
	return nil
}

// DumpAST runs `swiftc -dump-ast -dump-ast-format json` over the target and returns the JSON,
// one document per source file. Each file is dumped by its own frontend so a compiler crash on
// one file skips only that file; skipped files are reported on stderr.
// modulesDir holds the built .swiftmodule files of the target's dependencies.
//
// The dump leaves out the extensions that attached macros generate. A file with such a macro is
// dumped a second time from a copy, written under expandDir, that holds those extensions as
// source in place of the macro attributes; the JSON then names the copy as its file.
func DumpAST(t Target, modulesDir, expandDir string, extra ...string) ([]byte, error) {
	d, err := NewDumper(t, modulesDir, expandDir, extra...)
	if err != nil {
		return nil, err
	}
	all := make([]int, len(d.Sources))
	for i := range all {
		all[i] = i
	}
	dumps, err := d.Dump(all, nil)
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, f := range dumps {
		out = append(out, f.Doc...)
		out = append(out, '\n')
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("swiftc -dump-ast produced no output for %s", t.Name)
	}
	return out, nil
}

// A Dumper dumps the ASTs of a target's source files, all of them or a few at a time, with the
// frontend command the driver would run for the target.
type Dumper struct {
	// Sources are the target's source files, in the order the frontend reads them.
	Sources   []string
	dir       string
	job       []string
	expandDir string
}

// NewDumper asks the driver for the frontend command that dumps t's ASTs; see [DumpAST].
func NewDumper(t Target, modulesDir, expandDir string, extra ...string) (*Dumper, error) {
	args := []string{"-dump-ast", "-dump-ast-format", "json", "-module-name", t.Name, "-swift-version", "6", "-package-name", filepath.Base(t.Dir)}
	if modulesDir != "" {
		args = append(args, "-I", modulesDir)
	}
	args = append(args, extra...)
	sources := t.SourcePaths()
	job, err := frontendJob(t.Dir, append(append([]string{"-driver-print-jobs"}, args...), sources...), sources)
	if err != nil {
		return nil, err
	}
	return &Dumper{Sources: sources, dir: t.Dir, job: job, expandDir: filepath.Join(expandDir, t.Name, "macros")}, nil
}

// A FileDump is the dump of one source file.
type FileDump struct {
	// Doc is the file's JSON AST, nil when the compiler failed on the file.
	Doc []byte
	// Copy is the copy of the file that holds its macros' extensions, which the frontend read in
	// its place; "" when it needs none.
	Copy string
}

// Dump dumps the sources at the indices in primaries, type-checking each against all of the
// target's files, and returns their dumps in the same order. copies names the copies (see
// [FileDump.Copy]) of the files that are not dumped now: the frontend reads them in place of
// those files, as it would if they were dumped in the same run.
func (d *Dumper) Dump(primaries []int, copies map[int]string) ([]FileDump, error) {
	outs := make([][]byte, len(d.Sources))
	errs := make([]string, len(d.Sources))
	expansions := make([]string, len(d.Sources))
	run := func(sources []string, only []int, more ...string) error {
		list, err := os.CreateTemp("", "swift2go-sources-*")
		if err != nil {
			return err
		}
		defer os.Remove(list.Name())
		fmt.Fprintln(list, strings.Join(sources, "\n"))
		_ = list.Close()
		job := append(append(slices.Clone(d.job), "-filelist", list.Name()), more...)
		var wg sync.WaitGroup
		sem := make(chan struct{}, runtime.NumCPU())
		for _, i := range only {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				outs[i], expansions[i], errs[i] = dumpFile(job, d.dir, sources[i])
			})
		}
		wg.Wait()
		return nil
	}
	if err := run(d.Sources, primaries, "-dump-macro-expansions"); err != nil {
		return nil, err
	}
	expanded := slices.Clone(d.Sources)
	for i, c := range copies {
		expanded[i] = c
	}
	var redo []int
	for _, i := range primaries {
		if outs[i] == nil {
			continue
		}
		copied, err := expandedSource(d.Sources[i], d.expandDir, outs[i], expansions[i])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.Sources[i], err)
		}
		if copied != "" {
			expanded[i] = copied
			redo = append(redo, i)
		}
	}
	if len(redo) > 0 {
		if err := run(expanded, redo); err != nil {
			return nil, err
		}
	}
	dumps := make([]FileDump, len(primaries))
	for k, i := range primaries {
		if errs[i] != "" {
			fmt.Fprintln(os.Stderr, "swift2go: skipped", errs[i])
		}
		dumps[k] = FileDump{Doc: outs[i]}
		if expanded[i] != d.Sources[i] {
			dumps[k].Copy = expanded[i]
		}
	}
	return dumps, nil
}

// dumpFile runs one frontend job with src as its primary file. It returns the file's JSON AST and
// what the frontend wrote to stderr, or a description of the failure.
func dumpFile(job []string, dir, src string) (out []byte, stderrText, failure string) {
	var stdout, stderr bytes.Buffer
	run := func(more ...string) error {
		stdout.Reset()
		stderr.Reset()
		cmd := exec.Command(job[0], append(append(append([]string{}, job[1:]...), more...), "-primary-file", src)...)
		cmd.Dir = dir
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		return cmd.Run()
	}
	err := run()
	if err != nil && strings.Contains(stderr.String(), "Assertion failed") {
		err = run("-Xllvm", "-assert-continue")
	}
	out = stdout.Bytes()
	if !bytes.HasPrefix(bytes.TrimSpace(out), []byte("{")) {
		out = stderr.Bytes()
	}
	if j := bytes.Index(out, []byte(`{"_kind"`)); j >= 0 {
		out = out[j:]
	} else {
		out = nil
	}
	if err != nil {
		msg := stderr.String()
		if k := strings.Index(msg, "Assertion failed"); k >= 0 {
			msg = msg[k:]
		}
		if k := strings.Index(msg, "\n"); k >= 0 {
			msg = msg[:k]
		}
		return nil, "", fmt.Sprintf("%s: %v: %s", src, err, msg)
	}
	return out, stderr.String(), ""
}

// frontendJob asks the driver for its first frontend command line and strips the source inputs from it.
func frontendJob(dir string, driverArgs, sources []string) ([]string, error) {
	cmd := exec.Command("swiftc", driverArgs...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("swiftc -driver-print-jobs: %w", err)
	}
	line, _, _ := strings.Cut(string(out), "\n")
	toks := shellSplit(line)
	isSource := map[string]bool{}
	for _, s := range sources {
		isSource[s] = true
	}
	var job []string
	for i := 0; i < len(toks); i++ {
		switch {
		case toks[i] == "-primary-file":
			i++
		case toks[i] == "-filelist" || toks[i] == "-supplementary-output-file-map":
			i++
		case isSource[toks[i]]:
		default:
			job = append(job, toks[i])
		}
	}
	if len(job) == 0 {
		return nil, fmt.Errorf("swiftc -driver-print-jobs printed no job")
	}
	return job, nil
}

func shellSplit(s string) []string {
	var toks []string
	var cur strings.Builder
	in := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			in = true
		case r == ' ' || r == '\t':
			if in {
				toks = append(toks, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		toks = append(toks, cur.String())
	}
	return toks
}

// EmitModule writes TARGET.swiftmodule into dir so dependents (and @testable tests) can import it.
func EmitModule(t Target, dir string, extra ...string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	args := []string{"-emit-module", "-wmo", "-enable-testing", "-module-name", t.Name, "-swift-version", "6", "-package-name", filepath.Base(t.Dir),
		"-emit-module-path", filepath.Join(dir, t.Name+".swiftmodule"), "-I", dir}
	args = append(args, extra...)
	args = append(args, t.SourcePaths()...)
	for _, ext := range []string{".swiftmodule", ".swiftdoc", ".swiftsourceinfo", ".abi.json"} {
		_ = os.Remove(filepath.Join(dir, t.Name+ext))
	}
	cmd := exec.Command("swiftc", args...)
	cmd.Dir = t.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("swiftc -emit-module %s: %w\n%s", t.Name, err, out)
	}
	return nil
}

// TestingFlags are the swiftc flags that make the swift-testing module and its macros available
// when compiling for d. Another destination's SDK must provide the module itself.
func TestingFlags(d Destination) []string {
	dev := "/Applications/Xcode.app/Contents/Developer"
	flags := []string{"-plugin-path", dev + "/Toolchains/XcodeDefault.xctoolchain/usr/lib/swift/host/plugins/testing"}
	if d.Host() {
		flags = append([]string{"-F", dev + "/Platforms/MacOSX.platform/Developer/Library/Frameworks"}, flags...)
	}
	return flags
}

// PrebuiltFlags points swiftc at third-party modules, generated C module maps and macro plugins
// that SwiftPM already built for d under buildDir. Macro plugins run on the host, so a build for
// another destination holds them among the host's products.
func PrebuiltFlags(buildDir string, d Destination) []string {
	products := ProductsDir(buildDir, d)
	if _, err := os.Stat(products); err != nil {
		return nil
	}
	if !d.Host() {
		flags := []string{"-I", ModulesDir(buildDir, d)}
		hosts, _ := filepath.Glob(filepath.Join(buildDir, "*-apple-macosx*", "debug", "Macros-tool"))
		if len(hosts) > 0 {
			flags = append(flags, "-load-plugin-executable", hosts[0]+"#Macros")
		}
		return flags
	}
	maps := generatedMapsDir(buildDir)
	flags := []string{"-I", products, "-I", maps}
	generated, _ := filepath.Glob(filepath.Join(maps, "*.modulemap"))
	for _, f := range generated {
		flags = append(flags, "-Xcc", "-fmodule-map-file="+f)
	}
	if _, err := os.Stat(filepath.Join(products, "Macros")); err == nil {
		flags = append(flags, "-load-plugin-executable", filepath.Join(products, "Macros")+"#Macros")
	}
	return flags
}

func generatedMapsDir(buildDir string) string {
	return filepath.Join(buildDir, "out", "Intermediates.noindex", "GeneratedModuleMaps")
}

// CModuleFlags makes the C targets and system libraries in targets importable from Swift. Each
// C target's public headers are loaded through its own module map, or through one written to
// mapDir the way SwiftPM generates it: an umbrella header named after the module when there is
// one, the whole header directory otherwise. Modules that SwiftPM already generated a map for
// under buildDir are left to [PrebuiltFlags]. As in SwiftPM, a C target's header directory goes
// on the include path when another C target depends on it.
func CModuleFlags(targets map[string]Target, buildDir, mapDir string) ([]string, error) {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	includedByC := map[string]bool{}
	for _, t := range targets {
		if t.ModuleType == "ClangTarget" {
			for _, d := range t.AllDeps() {
				includedByC[d] = true
			}
		}
	}
	var flags []string
	for _, name := range names {
		t := targets[name]
		dir := filepath.Join(t.Dir, t.Path)
		switch t.ModuleType {
		case "SystemLibraryTarget":
			if exists(filepath.Join(dir, "module.modulemap")) {
				flags = append(flags, "-Xcc", "-fmodule-map-file="+filepath.Join(dir, "module.modulemap"))
			}
			continue
		case "ClangTarget":
		default:
			continue
		}
		if t.Type != "library" || exists(filepath.Join(generatedMapsDir(buildDir), t.C99Name+".modulemap")) {
			continue
		}
		headers := filepath.Clean(filepath.Join(dir, t.PublicHeaders))
		if info, err := os.Stat(headers); err != nil || !info.IsDir() {
			continue
		}
		modmap := filepath.Join(headers, "module.modulemap")
		if !exists(modmap) {
			umbrella := fmt.Sprintf("umbrella %q", headers)
			for _, h := range []string{filepath.Join(headers, t.C99Name+".h"), filepath.Join(headers, t.C99Name, t.C99Name+".h")} {
				if exists(h) {
					umbrella = fmt.Sprintf("umbrella header %q", h)
					break
				}
			}
			if err := os.MkdirAll(mapDir, 0o755); err != nil {
				return nil, err
			}
			modmap = filepath.Join(mapDir, t.C99Name+".modulemap")
			src := fmt.Sprintf("module %s {\n    %s\n    export *\n}\n", t.C99Name, umbrella)
			if err := os.WriteFile(modmap, []byte(src), 0o644); err != nil {
				return nil, err
			}
		}
		if includedByC[name] {
			flags = append(flags, "-Xcc", "-I"+headers)
		}
		flags = append(flags, "-Xcc", "-fmodule-map-file="+modmap)
	}
	return flags, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
