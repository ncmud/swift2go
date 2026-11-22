package swiftmod

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ncmud/swift2go/internal/swifttest"
)

func write(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestCModuleFlagsWritesSwiftPMModuleMaps(t *testing.T) {
	pkg := t.TempDir()
	now := time.Now()
	write(t, filepath.Join(pkg, "Sources/CZlib/include/CZlib.h"), "#include <zlib.h>\n", now)
	write(t, filepath.Join(pkg, "Sources/cmth/mth.h"), "", now)
	write(t, filepath.Join(pkg, "src/include/module.modulemap"), "module cmark { header \"cmark.h\" }\n", now)
	targets := map[string]Target{
		"CZlib":     {Name: "CZlib", C99Name: "CZlib", Path: "Sources/CZlib", ModuleType: "ClangTarget", Type: "library", PublicHeaders: "include", Dir: pkg},
		"Cmth":      {Name: "Cmth", C99Name: "Cmth", Path: "Sources/cmth", ModuleType: "ClangTarget", Type: "library", PublicHeaders: "./", Dir: pkg},
		"cmark":     {Name: "cmark", C99Name: "cmark", Path: "src", ModuleType: "ClangTarget", Type: "library", PublicHeaders: "include", Dir: pkg},
		"cmark-ext": {Name: "cmark-ext", C99Name: "cmark_ext", Path: "ext", ModuleType: "ClangTarget", Type: "library", PublicHeaders: "include", Deps: []string{"cmark"}, Dir: pkg},
	}
	maps := filepath.Join(t.TempDir(), "maps")
	flags, err := CModuleFlags(targets, filepath.Join(pkg, ".build"), maps)
	if err != nil {
		t.Fatal(err)
	}
	zlib, _ := os.ReadFile(filepath.Join(maps, "CZlib.modulemap"))
	if want := `umbrella header "` + filepath.Join(pkg, "Sources/CZlib/include/CZlib.h") + `"`; !strings.Contains(string(zlib), want) {
		t.Errorf("CZlib map = %q, want %s", zlib, want)
	}
	mth, _ := os.ReadFile(filepath.Join(maps, "Cmth.modulemap"))
	if want := `umbrella "` + filepath.Join(pkg, "Sources/cmth") + `"`; !strings.Contains(string(mth), want) {
		t.Errorf("Cmth map = %q, want %s", mth, want)
	}
	if !slices.Contains(flags, "-fmodule-map-file="+filepath.Join(pkg, "src/include/module.modulemap")) {
		t.Errorf("flags %v do not load cmark's own module map", flags)
	}
	if !slices.Contains(flags, "-I"+filepath.Join(pkg, "src/include")) {
		t.Errorf("flags %v do not put cmark's headers on the include path for cmark-ext", flags)
	}
	if slices.Contains(flags, "-I"+filepath.Join(pkg, "Sources/CZlib/include")) {
		t.Errorf("flags %v put CZlib's headers on the include path though no C target includes them", flags)
	}
}

func TestStaleModuleReportsDependencyOlderThanSources(t *testing.T) {
	pkg := t.TempDir()
	mods := t.TempDir()
	old := time.Now().Add(-time.Hour)
	write(t, filepath.Join(pkg, "Sources/Core/Core.swift"), "", time.Now())
	write(t, filepath.Join(mods, "Core.swiftmodule"), "", old)
	targets := map[string]Target{
		"Core": {Name: "Core", C99Name: "Core", Path: "Sources/Core", Sources: []string{"Core.swift"}, ModuleType: "SwiftTarget", Dir: pkg},
		"Mid":  {Name: "Mid", C99Name: "Mid", ModuleType: "SwiftTarget", Deps: []string{"Core"}, Dir: pkg},
		"App":  {Name: "App", C99Name: "App", ModuleType: "SwiftTarget", Deps: []string{"Mid"}, Dir: pkg},
	}
	err := StaleModule(targets["App"], targets, map[string]bool{"Mid": true}, []string{mods})
	if err == nil || !strings.Contains(err.Error(), "App imports Core") {
		t.Fatalf("StaleModule = %v, want the stale Core module reported", err)
	}
	if err := StaleModule(targets["App"], targets, map[string]bool{"Mid": true, "Core": true}, []string{mods}); err != nil {
		t.Errorf("StaleModule with Core emitted in the run = %v", err)
	}
}

func TestAddGeneratedReadsPluginOutputsAndRejectsStaleOnes(t *testing.T) {
	pkg := filepath.Join(t.TempDir(), "MyPkg")
	build := filepath.Join(pkg, ".build")
	old := time.Now().Add(-time.Hour)
	out := filepath.Join(build, "plugins/outputs/mypkg/Spec/destination/Gen/GeneratedSources/Types.swift")
	write(t, out, "", old)
	write(t, filepath.Join(pkg, "Sources/Spec/openapi.yaml"), "", old.Add(-time.Minute))
	spec := Target{Name: "Spec", C99Name: "Spec", Path: "Sources/Spec", Plugins: []string{"Gen"}, Dir: pkg,
		Resources: []Resource{{Path: filepath.Join(pkg, "Sources/Spec/openapi.yaml")}}}
	derived := t.TempDir()
	got := spec
	if err := AddGenerated(&got, build, derived, Destination{}); err != nil {
		t.Fatal(err)
	}
	accessor := filepath.Join(derived, "Spec", "resource_bundle_accessor.swift")
	if want := []string{accessor, out}; !slices.Equal(got.Generated, want) {
		t.Errorf("Generated = %v, want %v", got.Generated, want)
	}
	if src, _ := os.ReadFile(accessor); !strings.Contains(string(src), `"mypkg_Spec"`) {
		t.Errorf("accessor does not name the mypkg_Spec bundle:\n%s", src)
	}

	write(t, filepath.Join(pkg, "Sources/Spec/openapi.yaml"), "", time.Now())
	stale := spec
	if err := AddGenerated(&stale, build, derived, Destination{}); err == nil || !strings.Contains(err.Error(), "swift build --target Spec") {
		t.Errorf("AddGenerated with an edited plugin input = %v, want an error naming swift build", err)
	}

	missing := spec
	if err := AddGenerated(&missing, filepath.Join(t.TempDir(), ".build"), derived, Destination{}); err == nil {
		t.Error("AddGenerated without plugin outputs succeeded")
	}
}

// TestDescribeResolvesIntoAnEmptyBuildDir checks that a remote dependency described for a build directory
// of its own is checked out there, so its targets' directories outlive the description.
func TestDescribeResolvesIntoAnEmptyBuildDir(t *testing.T) {
	if testing.Short() {
		t.Skip("resolves a SwiftPM package")
	}
	swifttest.Require(t, "swift", "git")
	now := time.Now()
	dep := filepath.Join(t.TempDir(), "dep")
	write(t, filepath.Join(dep, "Package.swift"), "// swift-tools-version:5.9\nimport PackageDescription\nlet package = Package(name: \"dep\", products: [.library(name: \"Dep\", targets: [\"Dep\"])], targets: [.target(name: \"Dep\")])\n", now)
	write(t, filepath.Join(dep, "Sources/Dep/Dep.swift"), "public let dep = 1\n", now)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "dep"}, {"tag", "1.0.0"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dep
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	pkg := t.TempDir()
	write(t, filepath.Join(pkg, "Package.swift"), "// swift-tools-version:5.9\nimport PackageDescription\nlet package = Package(name: \"app\", dependencies: [.package(url: \"file://"+dep+"\", from: \"1.0.0\")], targets: [.target(name: \"App\", dependencies: [.product(name: \"Dep\", package: \"dep\")])])\n", now)
	write(t, filepath.Join(pkg, "Sources/App/App.swift"), "import Dep\n", now)
	build := filepath.Join(t.TempDir(), "linux-build")
	targets, err := Describe(pkg, build)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := targets["Dep"]
	if !ok {
		t.Fatalf("no Dep target among %v", targets)
	}
	if !strings.HasPrefix(d.Dir, build) || !exists(filepath.Join(d.Dir, d.Path, "Dep.swift")) {
		t.Errorf("Dep is described at %s, not in a checkout under %s that exists", d.Dir, build)
	}
}
