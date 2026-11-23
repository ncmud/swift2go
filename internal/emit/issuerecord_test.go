package emit

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIssueRecord checks the test helpers a test target imports from a library, as ncmud's TestExpectations
// replaces #expect: a generic Equatable or Comparable parameter compares arrays and structs with their own <,
// which Go's == and < cannot, and a mismatch is reported with Issue.record at the caller's source location.
// It also checks a parameterized test whose arguments come through a key path.
func TestIssueRecord(t *testing.T) {
	out := transpileFixture(t, "issuerecord", "Lib", "Checks", "LibTests")
	dir := filepath.Join(out, "lib")
	cmd := exec.Command("go", "test", "-count=1", "-v", ".")
	cmd.Dir = dir
	got, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("go test passed; want unequalArraysRecordAnIssue to fail:\n%s", got)
	}
	text := string(got)
	for _, want := range []string{
		"--- PASS: TestEqualArraysPass",
		"--- PASS: TestComparableUsesTheTypesOwnLess",
		"--- PASS: TestNamedRowsKeyPathArguments",
		"--- PASS: TestOptionalReferencesThroughGenerics",
		"--- PASS: TestZippedArguments",
		"--- PASS: TestZippedRanges",
		"--- FAIL: TestUnequalArraysRecordAnIssue",
		"Tests/LibTests/LibTests.swift:18:15: checkEqual failed: [\"a\"] != [\"b\"] - arrays differ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("go test output lacks %q:\n%s", want, text)
		}
	}
}
