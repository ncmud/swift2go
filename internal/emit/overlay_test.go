package emit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ncmud/swift2go/internal/ast"
)

func TestWriteGoLeavesOverlaidFilesToTheOverlay(t *testing.T) {
	overlay, out := t.TempDir(), t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(overlay, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("socket_lifecycle.go", "package server\n\n// hand-written\n")
	write("helpers.go", "package server\n\n// extra\n")
	write("socket_lifecycle_test.go", "package server\n")
	write("notes.txt", "not Go")

	m, err := New("Server", []ast.File{{Filename: "/src/SocketLifecycle.swift"}, {Filename: "/src/GameLoop.swift"}}, "example.com/swiftrt")
	if err != nil {
		t.Fatal(err)
	}
	m.Overlay = overlay
	paths, err := m.WriteGo(out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	slices.Sort(names)
	if want := []string{"game_loop.go", "helpers.go", "socket_lifecycle.go"}; !slices.Equal(names, want) {
		t.Fatalf("wrote %v, want %v", names, want)
	}
	got, err := os.ReadFile(filepath.Join(out, "socket_lifecycle.go"))
	if err != nil || !strings.Contains(string(got), "hand-written") {
		t.Fatalf("socket_lifecycle.go = %q, %v", got, err)
	}
	if !slices.Equal(m.Replaced, []string{"SocketLifecycle.swift"}) {
		t.Fatalf("Replaced = %v", m.Replaced)
	}
	if report := m.Report(); !strings.Contains(report, "replaced by overlay: SocketLifecycle.swift") {
		t.Fatalf("report does not name the replaced file:\n%s", report)
	}

	// A test target takes only the overlay's test files.
	m.Test, m.Replaced = true, nil
	paths, err = m.WriteGo(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	names = nil
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	slices.Sort(names)
	if want := []string{"game_loop_test.go", "socket_lifecycle_test.go"}; !slices.Equal(names, want) {
		t.Fatalf("test target wrote %v, want %v", names, want)
	}
}

func TestWriteGoReportsAMissingOverlayDirectory(t *testing.T) {
	m, err := New("Server", nil, "example.com/swiftrt")
	if err != nil {
		t.Fatal(err)
	}
	m.Overlay = filepath.Join(t.TempDir(), "absent")
	if _, err := m.WriteGo(t.TempDir()); err == nil {
		t.Fatal("WriteGo succeeded with a missing overlay directory")
	}
}
