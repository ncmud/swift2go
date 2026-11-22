package swiftmod

import (
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
)

// A Destination is the platform the Swift sources are compiled for, which decides the branches of
// `#if os(...)`, `#if canImport(...)` and `#if arch(...)` that the AST holds. The zero Destination
// is the host.
type Destination struct {
	// SDK is the artifact id of the Swift SDK that holds the destination's platform modules.
	SDK string
	// Triple is the target triple, such as x86_64-swift-linux-musl.
	Triple string
	// Root is the SDK's sysroot.
	Root string
	// Resources is the Swift resource directory, which holds the platform's standard library,
	// Foundation and C library modules.
	Resources string
	// Includes and Libraries are the SDK's extra header and library search paths.
	Includes, Libraries []string
}

// Host reports whether d is the machine swiftc runs on.
func (d Destination) Host() bool { return d.Triple == "" }

// OS is the destination's operating system as the triple names it ("linux"), "" for the host.
func (d Destination) OS() string {
	parts := strings.Split(d.Triple, "-")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// Flags are the swiftc flags that compile for d: none for the host.
func (d Destination) Flags() []string {
	if d.Host() {
		return nil
	}
	flags := []string{"-target", d.Triple}
	if d.Root != "" {
		flags = append(flags, "-sdk", d.Root)
	}
	if d.Resources != "" {
		flags = append(flags, "-resource-dir", d.Resources)
	}
	for _, dir := range d.Includes {
		flags = append(flags, "-I", dir, "-Xcc", "-I"+dir)
	}
	for _, dir := range d.Libraries {
		flags = append(flags, "-L", dir)
	}
	return flags
}

// SwiftBuildArgs are the `swift build` arguments that build for d: none for the host.
func (d Destination) SwiftBuildArgs() []string {
	if d.Host() {
		return nil
	}
	return []string{"--swift-sdk", d.SDK, "--triple", d.Triple}
}

// CorelibsFoundation reports whether d's Foundation is the Swift one of Linux and other platforms
// without Objective-C, rather than Darwin's.
func (d Destination) CorelibsFoundation() bool {
	return d.ResourceBundleSuffix() == ".resources"
}

// ResourceBundleSuffix is the extension SwiftPM gives a target's resource bundle on d.
func (d Destination) ResourceBundleSuffix() string {
	switch {
	case d.Host(), strings.HasPrefix(d.OS(), "macos"), strings.HasPrefix(d.OS(), "ios"):
		return ".bundle"
	}
	return ".resources"
}

// FindSwiftSDK finds an installed Swift SDK the way `swift build --swift-sdk` does. sdk is the
// SDK's artifact id or one of its target triples, and triple picks one of the SDK's triples; either
// may be "". An id whose SDK has more than one triple needs triple. Installed SDKs are those under
// dirs, or under SwiftPM's default directory when dirs is empty.
func FindSwiftSDK(sdk, triple string, dirs ...string) (Destination, error) {
	if sdk == "" && triple == "" {
		return Destination{}, nil
	}
	all, err := InstalledSwiftSDKs(dirs...)
	if err != nil {
		return Destination{}, err
	}
	var found []Destination
	for _, d := range all {
		switch {
		case sdk != "" && d.SDK != sdk && d.Triple != sdk:
		case triple != "" && d.Triple != triple:
		default:
			found = append(found, d)
		}
	}
	name := sdk
	if name == "" {
		name = triple
	}
	switch len(found) {
	case 0:
		if sdk != "" && triple != "" {
			return Destination{}, fmt.Errorf("the Swift SDK %s has no target triple %s; see `swift sdk list`", sdk, triple)
		}
		return Destination{}, fmt.Errorf("no installed Swift SDK matches %s; see `swift sdk list`", name)
	case 1:
		return found[0], nil
	}
	var triples []string
	for _, d := range found {
		triples = append(triples, d.SDK+" "+d.Triple)
	}
	return Destination{}, fmt.Errorf("the Swift SDK %s has more than one target triple (%s); pick one with -triple", name, strings.Join(triples, ", "))
}

func defaultSwiftSDKDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	if runtime.GOOS == "darwin" {
		return []string{filepath.Join(home, "Library", "org.swift.swiftpm", "swift-sdks")}
	}
	return []string{filepath.Join(home, ".swiftpm", "swift-sdks"), filepath.Join(home, ".config", "swiftpm", "swift-sdks")}
}

// InstalledSwiftSDKs lists every target triple of the Swift SDKs installed under dirs, or under
// SwiftPM's default directory when dirs is empty.
func InstalledSwiftSDKs(dirs ...string) ([]Destination, error) {
	if len(dirs) == 0 {
		dirs = defaultSwiftSDKDirs()
	}
	var out []Destination
	for _, dir := range dirs {
		bundles, _ := filepath.Glob(filepath.Join(dir, "*.artifactbundle"))
		sort.Strings(bundles)
		for _, bundle := range bundles {
			sdks, err := bundleSwiftSDKs(bundle)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", bundle, err)
			}
			out = append(out, sdks...)
		}
	}
	return out, nil
}

func bundleSwiftSDKs(bundle string) ([]Destination, error) {
	var info struct {
		Artifacts map[string]struct {
			Type     string `json:"type"`
			Variants []struct {
				Path string `json:"path"`
			} `json:"variants"`
		} `json:"artifacts"`
	}
	if err := readJSON(filepath.Join(bundle, "info.json"), &info); err != nil {
		return nil, err
	}
	var out []Destination
	for id, a := range info.Artifacts {
		if a.Type != "swiftSDK" {
			continue
		}
		for _, v := range a.Variants {
			// A variant names the directory of its swift-sdk.json or, in some bundles, the file itself.
			file := filepath.Join(bundle, v.Path)
			if filepath.Ext(file) != ".json" {
				file = filepath.Join(file, "swift-sdk.json")
			}
			dir := filepath.Dir(file)
			var meta struct {
				TargetTriples map[string]struct {
					SDKRootPath        string   `json:"sdkRootPath"`
					SwiftResourcesPath string   `json:"swiftResourcesPath"`
					IncludeSearchPaths []string `json:"includeSearchPaths"`
					LibrarySearchPaths []string `json:"librarySearchPaths"`
				} `json:"targetTriples"`
			}
			if err := readJSON(file, &meta); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					// SwiftPM skips a variant whose metadata is missing, with a warning.
					continue
				}
				return nil, err
			}
			for triple, m := range meta.TargetTriples {
				d := Destination{SDK: id, Triple: triple, Root: under(dir, m.SDKRootPath), Resources: under(dir, m.SwiftResourcesPath)}
				for _, p := range m.IncludeSearchPaths {
					d.Includes = append(d.Includes, under(dir, p))
				}
				for _, p := range m.LibrarySearchPaths {
					d.Libraries = append(d.Libraries, under(dir, p))
				}
				out = append(out, d)
			}
		}
	}
	slices.SortFunc(out, func(a, b Destination) int { return strings.Compare(a.SDK+" "+a.Triple, b.SDK+" "+b.Triple) })
	return out, nil
}

// under resolves a path in a Swift SDK's metadata, which is relative to the metadata's directory.
func under(dir, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// BuildDependencies runs `swift build` for d in pkgDir, with its products in buildDir, so that the
// targets named can be compiled against built modules: it builds each Swift dependency of those
// targets that is not itself named, and each named target that uses a build-tool plugin, whose
// outputs are read from the build. extra are more arguments for `swift build`. It builds nothing for the host.
func BuildDependencies(pkgDir, buildDir string, d Destination, targets map[string]Target, names []string, extra ...string) error {
	if d.Host() {
		return nil
	}
	named := map[string]bool{}
	for _, n := range names {
		named[n] = true
	}
	var build []string
	add := func(n string) {
		if !slices.Contains(build, n) {
			build = append(build, n)
		}
	}
	for _, n := range names {
		t, ok := targets[n]
		if !ok {
			continue
		}
		if len(t.Plugins) > 0 && t.Type != "test" {
			add(n)
		}
		for _, dep := range t.AllDeps() {
			if dt, ok := targets[dep]; ok && !named[dep] && dt.ModuleType == "SwiftTarget" {
				add(dep)
			}
		}
	}
	for _, n := range build {
		args := slices.Concat([]string{"build", "--build-system", "native", "--scratch-path", buildDir}, d.SwiftBuildArgs(), extra, []string{"--target", n})
		cmd := exec.Command("swift", args...)
		cmd.Dir = pkgDir
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		fmt.Fprintf(os.Stderr, "swift2go: swift %s\n", strings.Join(args, " "))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("swift build --target %s for %s: %w", n, d.Triple, err)
		}
	}
	return nil
}
