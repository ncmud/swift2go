package swiftrt

import (
	"math"
	"testing"
)

func TestHSet(t *testing.T) {
	var s HSet[[]int]
	if !HSetInsert(&s, []int{1, 2}).F0 || HSetInsert(&s, []int{1, 2}).F0 || !HSetInsert(&s, []int{2, 1}).F0 {
		t.Fatal("insert dedupe")
	}
	if HSetCount(s) != 2 || !HSetContains(s, []int{2, 1}) || HSetContains(s, []int{3}) {
		t.Fatal("contains/count")
	}
	if HSetCount(HSetOf([]int(nil), []int{})) != 1 || HSetCount(HSetOf([]float64{0}, []float64{math.Copysign(0, -1)})) != 1 {
		t.Fatal("nil and empty, and -0.0 and 0.0, are equal")
	}
	sets := HSetOf(map[int]struct{}{1: {}, 2: {}}, map[int]struct{}{2: {}, 1: {}})
	if HSetCount(sets) != 1 {
		t.Fatal("sets of sets compare by value")
	}
}
