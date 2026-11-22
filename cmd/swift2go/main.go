// Command swift2go transpiles a SwiftPM target to Go.
package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
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
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: swift2go [flags] dump|transpile|stats <Target> [<TestTarget>]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), flag.Args()[1:], *pkgDir, *outDir, *modules); err != nil {
		fmt.Fprintln(os.Stderr, "swift2go:", err)
		os.Exit(1)
	}
}

func run(cmd string, names []string, pkgDir, outDir, modulesDir string) error {
	pkgDir = absPath(pkgDir)
	targets, err := swiftmod.Describe(pkgDir)
	if err != nil {
		return err
	}
	goModule, err := currentModule()
	if err != nil && cmd == "transpile" {
		return err
	}
	prebuilt := swiftmod.PrebuiltFlags(pkgDir)
	clang, err := swiftmod.ClangModuleFlags(targets, pkgDir, absPath(filepath.Join(outDir, "modules", "clang")))
	if err != nil {
		return err
	}
	prebuilt = append(prebuilt, clang...)
	load := func(name string) ([]ast.File, error) {
		t, ok := targets[name]
		if !ok {
			return nil, fmt.Errorf("no target %q", name)
		}
		cache := filepath.Join(outDir, "ast", name+".json")
		mods := filepath.Join(outDir, "modules")
		extra := append(swiftmod.TestingFlags(), prebuilt...)
		if t.Type != "test" {
			if err := swiftmod.EmitModule(t, absPath(mods), extra...); err != nil {
				return nil, err
			}
		}
		data, err := swiftmod.DumpAST(t, absPath(mods), extra...)
		if err != nil {
			return nil, err
		}
		_ = os.MkdirAll(filepath.Dir(cache), 0o755)
		_ = os.WriteFile(cache, data, 0o644)
		return ast.Load(data)
	}
	var done []*emit.Module
	byName := map[string]*emit.Module{}
	for _, name := range names {
		files, err := load(name)
		if err != nil {
			return err
		}
		if cmd == "dump" {
			fmt.Printf("%s: %d files\n", name, len(files))
			continue
		}
		m, err := emit.New(name, files, rtPath)
		if err != nil {
			return err
		}
		m.Deps = done
		t := targets[name]
		pkgOut := filepath.Join(outDir, strings.ToLower(name))
		if t.Type == "test" {
			if len(t.Deps) == 0 || byName[t.Deps[0]] == nil {
				return fmt.Errorf("%s: transpile its target %v first", name, t.Deps)
			}
			under := byName[t.Deps[0]]
			m.SetTarget(under)
			pkgOut = filepath.Join(outDir, under.Pkg)
		}
		m.ImportPath = path.Join(goModule, filepath.ToSlash(pkgOut))
		if _, err := m.WriteGo(pkgOut); err != nil {
			return err
		}
		fmt.Print(m.Report())
		if t.Type != "test" {
			done = append(done, m)
			byName[name] = m
		}
	}
	return nil
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
	for _, line := range strings.Split(string(data), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(mod), `"`), nil
		}
	}
	return "", fmt.Errorf("go.mod has no module line")
}
