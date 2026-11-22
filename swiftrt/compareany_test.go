package swiftrt

import "testing"

type unorderedKey struct{ N int }

func TestCompareAnyPanicsOnUnorderedKind(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("compareAny ordered two structs without their own <")
		}
	}()
	compareAny(unorderedKey{12}, unorderedKey{9})
}

func TestCompareAnyOrdersPrimitives(t *testing.T) {
	if compareAny(9, 12) >= 0 || compareAny("b", "a") <= 0 || compareAny(1.5, 1.5) != 0 {
		t.Fatal("compareAny misordered primitives")
	}
}

func TestSortKeysOrdersStructKeysByDescription(t *testing.T) {
	keys := []unorderedKey{{9}, {12}, {3}}
	SortKeys(keys)
	if keys[0] != (unorderedKey{12}) || keys[1] != (unorderedKey{3}) || keys[2] != (unorderedKey{9}) {
		t.Fatalf("SortKeys = %v", keys)
	}
}
