package swiftrt

import (
	"strconv"
	"testing"
)

func TestSetFormUnion(t *testing.T) {
	var s map[string]struct{}
	SetFormUnion(&s, []string{"a", "b"})
	SetFormUnion(&s, []string{"b", "c"})
	SetFormUnionSet(&s, map[string]struct{}{"d": {}})
	if len(s) != 4 {
		t.Fatalf("got %v", s)
	}
	for _, k := range []string{"a", "b", "c", "d"} {
		if _, ok := s[k]; !ok {
			t.Errorf("missing %q in %v", k, s)
		}
	}
}

// BenchmarkSetFormUnionLoop grows a set by merging small sets into it, as a keyword index does.
// formUnion must cost the size of its argument, not of the set it grows.
func BenchmarkSetFormUnionLoop(b *testing.B) {
	parts := make([][]string, 2000)
	for i := range parts {
		parts[i] = []string{strconv.Itoa(i), strconv.Itoa(i) + "x"}
	}
	for b.Loop() {
		var s map[string]struct{}
		for _, p := range parts {
			SetFormUnion(&s, p)
		}
	}
}
