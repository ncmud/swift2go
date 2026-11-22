package swiftrt

import (
	"os"
	"path/filepath"
	"strings"
)

// Bundle is Foundation `Bundle`: a directory of resources, named by its path.
type Bundle struct{ Path string }

// NSBundleMain is `Bundle.main` for a command-line executable: the directory that holds it.
var NSBundleMain = mainBundle()

func mainBundle() Bundle {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return Bundle{filepath.Dir(exe)}
}

func directoryURL(path string) URL { return URLInitFileURLWithPath(strings.TrimRight(path, "/") + "/") }

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// NSBundleOptInitURL is Foundation `Bundle(url:)`: the bundle at a file URL, or nil when no
// directory is there.
func NSBundleOptInitURL(u URL) *Bundle {
	path := URLPath(u)
	if splitURL(u.Raw).scheme == nil || *splitURL(u.Raw).scheme != "file" || !isDirectory(path) {
		return nil
	}
	return &Bundle{path}
}

// NSBundleBundleURL is Foundation `Bundle.bundleURL`: the bundle directory's file URL.
func NSBundleBundleURL(b Bundle) URL { return directoryURL(b.Path) }

// resourceDirectory is where the bundle keeps its resources: `Contents/Resources` in a macOS
// bundle, `Resources` in a framework-style one, and otherwise the bundle directory itself.
func (b Bundle) resourceDirectory() string {
	for _, sub := range []string{"Contents/Resources", "Resources"} {
		if dir := filepath.Join(b.Path, sub); isDirectory(dir) {
			return dir
		}
	}
	return b.Path
}

// NSBundleResourceURL is Foundation `Bundle.resourceURL`: the file URL of the directory that holds
// the bundle's resources.
func NSBundleResourceURL(b Bundle) *URL { return new(directoryURL(b.resourceDirectory())) }

// NSBundleUrlForResourceWithExtension is Foundation `Bundle.url(forResource:withExtension:)`: the
// file URL of the resource with that name and extension, or nil when the bundle has none. A nil
// or empty extension matches the name alone, and a nil or empty name matches the first resource
// with the extension. It looks in the resource directory only, not in localization directories.
func NSBundleUrlForResourceWithExtension(b Bundle, name, ext *string) *URL {
	dir := b.resourceDirectory()
	suffix := ""
	if ext != nil && *ext != "" {
		suffix = "." + strings.TrimPrefix(*ext, ".")
	}
	if name != nil && *name != "" {
		path := filepath.Join(dir, *name+suffix)
		if _, err := os.Stat(path); err != nil {
			return nil
		}
		return new(URLInitFileURLWithPath(path))
	}
	if suffix == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), suffix) && entry.Name() != suffix {
			return new(URLInitFileURLWithPath(filepath.Join(dir, entry.Name())))
		}
	}
	return nil
}
