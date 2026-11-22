package swiftmod

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

type describeCache struct {
	Format  string
	Key     string
	Dirs    []string
	Targets map[string]Target
}

// DescribeCached is [Describe], reusing the result an earlier call saved in file while the
// toolchain, every package manifest and Package.resolved in the graph, and the file names under
// every target's directory are unchanged.
func DescribeCached(pkgDir, file string) (map[string]Target, error) {
	pkgDir, err := filepath.Abs(pkgDir)
	if err != nil {
		return nil, err
	}
	var c describeCache
	if f, err := os.Open(file); err == nil {
		_ = gob.NewDecoder(f).Decode(&c)
		_ = f.Close()
	}
	if c.Format == cacheFormat && c.Key != "" && c.Key == describeKey(pkgDir, c.Dirs, c.Targets) {
		return c.Targets, nil
	}
	dirs, targets, err := describe(pkgDir)
	if err != nil {
		return nil, err
	}
	c = describeCache{Format: cacheFormat, Key: describeKey(pkgDir, dirs, targets), Dirs: dirs, Targets: targets}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return targets, gob.NewEncoder(f).Encode(c)
}

// describeKey identifies what [Describe] reads: the toolchain, the manifests and resolved
// versions of the packages in dirs and of any local package under pkgDir/Packages, and the
// names of the files under each target's directory.
func describeKey(pkgDir string, dirs []string, targets map[string]Target) string {
	h := sha256.New()
	version, _ := exec.Command("swift", "--version").CombinedOutput()
	_, _ = h.Write(version)
	locals, _ := filepath.Glob(filepath.Join(pkgDir, "Packages", "*", "Package.swift"))
	for i, m := range locals {
		locals[i] = filepath.Dir(m)
	}
	for _, dir := range slices.Concat([]string{pkgDir}, dirs, locals) {
		manifests, _ := filepath.Glob(filepath.Join(dir, "Package*.swift"))
		for _, m := range append(manifests, filepath.Join(dir, "Package.resolved")) {
			fmt.Fprintf(h, "%s\x00", m)
			if f, err := os.Open(m); err == nil {
				_, _ = io.Copy(h, f)
				_ = f.Close()
			}
			_, _ = h.Write([]byte{0})
		}
	}
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		t := targets[name]
		root := filepath.Join(t.Dir, t.Path)
		fmt.Fprintf(h, "target %s %s\x00", name, root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if path != root && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			fmt.Fprintf(h, "%s %v\x00", path, d.IsDir())
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}
