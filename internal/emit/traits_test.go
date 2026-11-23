package emit

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTraits checks swift-testing traits on suites and tests: a TestScoping trait wraps each test in its scope,
// a suite's outside a test's own, and .enabled(if:) and .disabled skip the test. The scope binds a concrete class
// to a task local whose type is a protocol.
func TestTraits(t *testing.T) {
	out := transpileFixture(t, "traits", "Lib", "Scopes", "LibTests")
	cmd := exec.Command("go", "test", "-count=1", "-v", ".")
	cmd.Dir = filepath.Join(out, "libtests")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, got)
	}
	for _, want := range []string{
		"--- PASS: TestLedgerTestsSuiteScope",
		"--- PASS: TestLedgerTestsTestScopeInsideSuiteScope",
		"--- SKIP: TestLedgerTestsDisabledByCondition",
		"--- SKIP: TestLedgerTestsDisabled",
		"--- PASS: TestLedgerTestsEnabledByCondition",
		"--- PASS: TestUnscoped",
		"--- PASS: TestOuterInnerOuterSuiteScopeReachesNestedSuites",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("go test output lacks %q:\n%s", want, got)
		}
	}
}
