package swiftmod

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncmud/swift2go/internal/swifttest"
)

// TestStrictDumpFailsWithTheCompilersErrors checks that a strict dump of a target with a type error fails with the
// compiler's error, where a plain dump leaves the file without a document.
func TestStrictDumpFailsWithTheCompilersErrors(t *testing.T) {
	swifttest.Require(t, "xcrun", "swiftc")
	pkg := t.TempDir()
	write(t, filepath.Join(pkg, "Sources/Core/Good.swift"), "public let x = 1\n", time.Now())
	write(t, filepath.Join(pkg, "Sources/Core/Bad.swift"), "public let y: Int = \"one\"\n", time.Now())
	tg := Target{Name: "Core", Path: "Sources/Core", Sources: []string{"Bad.swift", "Good.swift"}, Dir: pkg}
	d, err := NewDumper(tg, "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	all := []int{0, 1}
	dumps, err := d.Dump(all, nil)
	if err != nil {
		t.Fatalf("a plain dump failed: %v", err)
	}
	if dumps[0].Doc != nil || dumps[1].Doc == nil {
		t.Fatalf("a plain dump has documents %v, %v; want only Good.swift's", dumps[0].Doc != nil, dumps[1].Doc != nil)
	}
	d.Strict = true
	_, err = d.Dump(all, nil)
	if err == nil {
		t.Fatal("a strict dump of a file with a type error succeeded")
	}
	for _, want := range []string{"failed on 1 files", "Bad.swift:1:", "error: cannot convert value of type 'String'"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the strict dump's error lacks %q:\n%v", want, err)
		}
	}
}

// TestFailuresListsEachDiagnosticOnce checks that files failing on the same broken declaration report its errors
// once, and that a dump with no failures reports none.
func TestFailuresListsEachDiagnosticOnce(t *testing.T) {
	errs := []string{"/p/A.swift: exit status 1: x", "", "/p/C.swift: exit status 1: x"}
	diags := []string{"/p/D.swift:3:1: error: broken\n", "", "/p/D.swift:3:1: error: broken\n"}
	err := failures([]int{0, 1, 2}, errs, diags)
	if err == nil {
		t.Fatal("no error for two failed files")
	}
	msg := err.Error()
	if !strings.Contains(msg, "failed on 2 files (/p/A.swift, /p/C.swift)") || strings.Count(msg, "error: broken") != 1 {
		t.Errorf("failures = %q", msg)
	}
	if err := failures([]int{1}, errs, diags); err != nil {
		t.Errorf("failures with no failed file = %v", err)
	}
}
