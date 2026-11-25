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

func TestHSetAlgebra(t *testing.T) {
	a, b, c := []int{1}, []int{2}, []int{3}
	left := HSetOf(a, b)
	right := HSetOf(b, c)
	if got := HSetUnion(left, right); HSetCount(got) != 3 || HSetCount(left) != 2 {
		t.Fatal("union should hold both sets' elements and leave its receiver unchanged")
	}
	if got := HSetUnion(HSet[[]int]{}, [][]int{c, c}); HSetCount(got) != 1 || !HSetContains(got, c) {
		t.Fatal("union of an empty set and a sequence")
	}
	if got := HSetIntersection(left, [][]int{b, c}); HSetCount(got) != 1 || !HSetContains(got, b) {
		t.Fatal("intersection")
	}
	if got := HSetSubtractingHSet(left, right); HSetCount(got) != 1 || !HSetContains(got, a) || HSetCount(left) != 2 {
		t.Fatal("subtracting")
	}
	if got := HSetSymmetricDifference(left, [][]int{b, c, c}); HSetCount(got) != 2 || HSetContains(got, b) {
		t.Fatal("symmetric difference")
	}
	if !HSetIsSubsetOf(HSetOf(a), left) || HSetIsSubsetOf(left, right) || !HSetIsStrictSubsetOf(HSetOf(a), left) || HSetIsStrictSubsetOf(left, left) {
		t.Fatal("subset")
	}
	if !HSetIsSupersetOf(left, [][]int{a, a}) || !HSetIsStrictSupersetOf(left, [][]int{a, a}) || HSetIsStrictSupersetOf(left, [][]int{a, b}) {
		t.Fatal("superset")
	}
	if !HSetIsDisjointWith(left, [][]int{c}) || HSetIsDisjointWith(left, right) {
		t.Fatal("disjoint")
	}
}

type hsetUnionTag struct {
	Name string
	Note []string
}

func (h hsetUnionTag) Hash(hh *any) { HasherCombine(hh, h.Name) }

func TestHSetUnionKeepsReceiverElement(t *testing.T) {
	mine := HSetOf(hsetUnionTag{"a", []string{"mine"}})
	got := HSetUnion(mine, []hsetUnionTag{{"a", []string{"theirs"}}, {"b", nil}})
	if HSetCount(got) != 2 {
		t.Fatalf("count %d", HSetCount(got))
	}
	for _, v := range got.Elements() {
		if v.Name == "a" && v.Note[0] != "mine" {
			t.Fatal("union replaced the receiver's element with the argument's")
		}
	}
}
