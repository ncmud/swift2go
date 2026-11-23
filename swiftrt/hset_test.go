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
	c := Copy(s)
	HSetInsert(&c, []int{3})
	if HSetCount(s) != 2 || HSetCount(c) != 3 {
		t.Fatal("a copied set should not share storage")
	}
}

func TestHSetInsertCopiesMember(t *testing.T) {
	var s HSet[[]int]
	v := []int{1, 2}
	HSetInsert(&s, v)
	Mut(&v)[0] = 9
	if got := s.Elements()[0]; got[0] != 1 {
		t.Fatal("mutating the inserted value changed the stored member")
	}
	w := []int{3}
	lit := HSetOf(w)
	Mut(&w)[0] = 4
	if got := lit.Elements()[0]; got[0] != 3 {
		t.Fatal("mutating a literal's element changed the stored member")
	}
}
