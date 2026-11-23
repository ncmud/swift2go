package swiftmod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildDependenciesSkipsUnchangedBuilds checks, with a stand-in swift that logs its runs, that a second run
// with nothing changed builds nothing, and that a change to a dependency's file or to Package.resolved builds again.
func TestBuildDependenciesSkipsUnchangedBuilds(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "runs")
	script := "#!/bin/sh\necho \"$*\" >>" + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "swift"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	pkg := t.TempDir()
	write(t, filepath.Join(pkg, "Package.resolved"), "{}\n", time.Now())
	write(t, filepath.Join(pkg, "Sources/Dep/Dep.swift"), "public let x = 1\n", time.Now())
	write(t, filepath.Join(pkg, "Sources/Core/Core.swift"), "import Dep\n", time.Now())
	targets := map[string]Target{
		"Dep":  {Name: "Dep", Path: "Sources/Dep", ModuleType: "SwiftTarget", Dir: pkg},
		"Core": {Name: "Core", Path: "Sources/Core", ModuleType: "SwiftTarget", Deps: []string{"Dep"}, Dir: pkg},
	}
	d := Destination{SDK: "sdk", Triple: "aarch64-swift-linux-musl"}
	build := t.TempDir()
	runs := func() int {
		t.Helper()
		if err := BuildDependencies(pkg, build, d, targets, []string{"Core"}); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(log)
		return strings.Count(string(data), "\n")
	}
	if n := runs(); n != 1 {
		t.Fatalf("the first run built %d targets, want Dep", n)
	}
	if n := runs(); n != 1 {
		t.Fatalf("an unchanged run built again (%d builds in all)", n)
	}
	write(t, filepath.Join(pkg, "Sources/Dep/Dep.swift"), "public let x = 2\n", time.Now().Add(time.Second))
	if n := runs(); n != 2 {
		t.Fatalf("a changed dependency was not built again (%d builds in all)", n)
	}
	write(t, filepath.Join(pkg, "Package.resolved"), "{\"pins\":[]}\n", time.Now())
	if n := runs(); n != 3 {
		t.Fatalf("a changed Package.resolved was not built again (%d builds in all)", n)
	}
	if err := BuildDependencies(pkg, build, Destination{SDK: "sdk", Triple: "x86_64-swift-linux-musl"}, targets, []string{"Core"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(log); strings.Count(string(data), "\n") != 4 {
		t.Fatal("another triple reused the stamp")
	}
}
