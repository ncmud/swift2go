package swiftrt

import (
	"slices"
	"testing"
)

func TestStringScalarViewUsesStringIndices(t *testing.T) {
	s := "é{x}" // é is two bytes, so String indices and scalar counts differ
	from := StringIndexAfter(s, StringStartIndex(s))
	if got, want := StringScalarIndicesFrom(s, PartialRangeFrom[int]{Lo: from}), []int{2, 3, 4}; !slices.Equal(got, want) {
		t.Errorf("indices from %d = %v, want %v", from, got, want)
	}
	if got := StringScalarAt(s, 4); got != '}' {
		t.Errorf("scalar at 4 = %q, want }", got)
	}
	if got := StringScalarIndexAfter(s, 0); got != 2 {
		t.Errorf("index after 0 = %d, want 2", got)
	}
	if got := string(StringScalarsFrom(s, PartialRangeFrom[int]{Lo: 2})); got != "{x}" {
		t.Errorf("scalars from 2 = %q, want {x}", got)
	}
}
