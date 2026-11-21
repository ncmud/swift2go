// Command swift2go transpiles a SwiftPM target to Go.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/emit"
	"github.com/ncmud/swift2go/internal/swiftmod"
)

const rtPath = "github.com/ncmud/swift2go/swiftrt"

func main() {
	pkgDir := flag.String("pkg", os.ExpandEnv("$HOME/dev/nc/ncmud"), "SwiftPM package directory")
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
	targets, err := swiftmod.Describe(pkgDir)
	if err != nil {
		return err
	}
	prebuilt := swiftmod.PrebuiltFlags(pkgDir)
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
			m.SetTarget(under.Pkg)
			pkgOut = filepath.Join(outDir, under.Pkg)
		}
		m.ImportPath = "github.com/ncmud/swift2go/" + filepath.ToSlash(pkgOut)
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
