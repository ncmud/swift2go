package swiftrt

import "testing"

type testFlags struct{ RawValue uint32 }

func TestAnyContains(t *testing.T) {
	set := testFlags{RawValue: 0b101}
	if !AnyContains(set, testFlags{RawValue: 0b100}) || AnyContains(set, testFlags{RawValue: 0b010}) {
		t.Fatal("AnyContains disagrees with the set's bits")
	}
}
