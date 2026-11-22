package swiftmod

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDescribeKeyFollowsManifestsAndFileNames(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "Sources", "Core")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "Package.swift"), "// v1", time.Now())
	write(t, filepath.Join(src, "A.swift"), "let a = 1", time.Now())
	targets := map[string]Target{"Core": {Name: "Core", Dir: root, Path: "Sources/Core"}}
	key := func() string { return describeKey(root, nil, targets) }

	k := key()
	write(t, filepath.Join(src, "A.swift"), "let a = 2", time.Now())
	if key() != k {
		t.Error("editing a source changed the key")
	}
	write(t, filepath.Join(src, ".hidden"), "", time.Now())
	if key() != k {
		t.Error("a hidden file changed the key")
	}
	write(t, filepath.Join(src, "B.swift"), "", time.Now())
	if key() == k {
		t.Error("adding a source kept the key")
	}
	k = key()
	write(t, filepath.Join(root, "Package.resolved"), "{}", time.Now())
	if key() == k {
		t.Error("a new Package.resolved kept the key")
	}
	k = key()
	write(t, filepath.Join(root, "Package.swift"), "// v2", time.Now())
	if key() == k {
		t.Error("a manifest edit kept the key")
	}
	k = key()
	local := filepath.Join(root, "Packages", "Local")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(local, "Package.swift"), "// local", time.Now())
	if key() == k {
		t.Error("a new local package kept the key")
	}
}
