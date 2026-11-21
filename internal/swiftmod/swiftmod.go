// Package swiftmod reads a SwiftPM package's targets and dumps their typed ASTs.
package swiftmod

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Target is one SwiftPM target.
type Target struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Sources []string `json:"sources"`
	Deps    []string `json:"target_dependencies"`
	// ProductDeps are dependencies through a package's products, which share the target's name here.
	ProductDeps []string `json:"product_dependencies"`
	Type        string   `json:"type"`
	// Dir is the root of the SwiftPM package that declares the target.
	Dir string `json:"-"`
}

// Describe returns the targets of the package rooted at pkgDir and of the local packages under pkgDir/Packages.
func Describe(pkgDir string) (map[string]Target, error) {
	m, err := describeOne(pkgDir)
	if err != nil {
		return nil, err
	}
	locals, _ := filepath.Glob(filepath.Join(pkgDir, "Packages", "*", "Package.swift"))
	for _, manifest := range locals {
		sub, err := describeOne(filepath.Dir(manifest))
		if err != nil {
			return nil, err
		}
		for name, t := range sub {
			if _, dup := m[name]; !dup {
				m[name] = t
			}
		}
	}
	return m, nil
}

func describeOne(pkgDir string) (map[string]Target, error) {
	cmd := exec.Command("swift", "package", "describe", "--type", "json")
	cmd.Dir = pkgDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("swift package describe: %w", err)
	}
	var d struct {
		Targets []Target `json:"targets"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}
	m := map[string]Target{}
	for _, t := range d.Targets {
		t.Dir = pkgDir
		if len(t.Deps) == 0 {
			t.Deps = t.ProductDeps
		}
		m[t.Name] = t
	}
	return m, nil
}

// DumpAST runs `swiftc -dump-ast -dump-ast-format json` over the target and returns the JSON,
// one document per source file. Each file is dumped by its own frontend so a compiler crash on
// one file skips only that file; skipped files are reported on stderr.
// modulesDir holds the built .swiftmodule files of the target's dependencies.
func DumpAST(t Target, modulesDir string, extra ...string) ([]byte, error) {
	args := []string{"-dump-ast", "-dump-ast-format", "json", "-module-name", t.Name, "-swift-version", "6", "-package-name", filepath.Base(t.Dir)}
	if modulesDir != "" {
		args = append(args, "-I", modulesDir)
	}
	args = append(args, extra...)
	var sources []string
	for _, s := range t.Sources {
		sources = append(sources, filepath.Join(t.Dir, t.Path, s))
	}
	job, err := frontendJob(t.Dir, append(append([]string{"-driver-print-jobs"}, args...), sources...), sources)
	if err != nil {
		return nil, err
	}
	list, err := os.CreateTemp("", "swift2go-sources-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(list.Name())
	fmt.Fprintln(list, strings.Join(sources, "\n"))
	_ = list.Close()
	job = append(job, "-filelist", list.Name())

	outs := make([][]byte, len(sources))
	errs := make([]string, len(sources))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, src := range sources {
		wg.Add(1)
		go func(i int, src string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var stdout, stderr bytes.Buffer
			run := func(more ...string) error {
				stdout.Reset()
				stderr.Reset()
				cmd := exec.Command(job[0], append(append(append([]string{}, job[1:]...), more...), "-primary-file", src)...)
				cmd.Dir = t.Dir
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				return cmd.Run()
			}
			err := run()
			if err != nil && strings.Contains(stderr.String(), "Assertion failed") {
				err = run("-Xllvm", "-assert-continue")
			}
			out := stdout.Bytes()
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
				errs[i] = fmt.Sprintf("%s: %v: %s", src, err, msg)
				out = nil
			}
			outs[i] = out
		}(i, src)
	}
	wg.Wait()
	var all []byte
	for i := range sources {
		if errs[i] != "" {
			fmt.Fprintln(os.Stderr, "swift2go: skipped", errs[i])
		}
		all = append(all, outs[i]...)
		all = append(all, '\n')
	}
	if len(bytes.TrimSpace(all)) == 0 {
		return nil, fmt.Errorf("swiftc -dump-ast produced no output for %s", t.Name)
	}
	return all, nil
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
	for _, s := range t.Sources {
		args = append(args, filepath.Join(t.Dir, t.Path, s))
	}
	cmd := exec.Command("swiftc", args...)
	cmd.Dir = t.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("swiftc -emit-module %s: %w\n%s", t.Name, err, out)
	}
	return nil
}

// TestingFlags are the swiftc flags that make the swift-testing module and its macros available.
func TestingFlags() []string {
	dev := "/Applications/Xcode.app/Contents/Developer"
	return []string{
		"-F", dev + "/Platforms/MacOSX.platform/Developer/Library/Frameworks",
		"-plugin-path", dev + "/Toolchains/XcodeDefault.xctoolchain/usr/lib/swift/host/plugins/testing",
	}
}

// PrebuiltFlags points swiftc at third-party modules, C module maps and macro plugins
// that SwiftPM already built under pkgDir/.build.
func PrebuiltFlags(pkgDir string) []string {
	products := filepath.Join(pkgDir, ".build", "out", "Products", "Debug")
	if _, err := os.Stat(products); err != nil {
		return nil
	}
	maps := filepath.Join(pkgDir, ".build", "out", "Intermediates.noindex", "GeneratedModuleMaps")
	flags := []string{"-I", products, "-I", maps}
	generated, _ := filepath.Glob(filepath.Join(maps, "*.modulemap"))
	checkouts, _ := filepath.Glob(filepath.Join(pkgDir, ".build", "checkouts", "*", "Sources", "*", "module.modulemap"))
	nested, _ := filepath.Glob(filepath.Join(pkgDir, ".build", "checkouts", "*", "Sources", "*", "include", "module.modulemap"))
	for _, f := range append(append(generated, checkouts...), nested...) {
		flags = append(flags, "-Xcc", "-fmodule-map-file="+f)
	}
	if _, err := os.Stat(filepath.Join(products, "Macros")); err == nil {
		flags = append(flags, "-load-plugin-executable", filepath.Join(products, "Macros")+"#Macros")
	}
	return flags
}
