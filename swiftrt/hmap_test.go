package swiftrt

import "testing"

func TestHMap(t *testing.T) {
	var d HMap[map[int]struct{}, string]
	a := map[int]struct{}{1: {}, 2: {}}
	b := map[int]struct{}{2: {}, 1: {}}
	if HMapGet(d, a) != nil || !HMapIsEmpty(d) {
		t.Fatal("empty HMap should hold nothing")
	}
	v := "x"
	HMapSet(&d, a, &v)
	if got := HMapGet(d, b); got == nil || *got != "x" {
		t.Fatalf("a set with the same members should find the entry, got %v", got)
	}
	w := "y"
	if old := HMapUpdateValueForKey(&d, w, b); old == nil || *old != "x" || HMapCount(d) != 1 {
		t.Fatal("updating through an equal key should replace one entry")
	}
	if HMapSubscriptDefault(d, map[int]struct{}{3: {}}, "z") != "z" {
		t.Fatal("a missing key should give the default")
	}

	delete(a, 1)
	if len(HMapKeys(d)[0]) != 2 {
		t.Fatal("mutating the caller's key should not change the stored one")
	}
	a[1] = struct{}{}

	c := Copy(d)
	HMapSet(&c, map[int]struct{}{3: {}}, &v)
	if HMapCount(d) != 1 || HMapCount(c) != 2 {
		t.Fatal("a copied HMap should not share storage")
	}
	if ks := HMapKeys(c); len(ks) != 2 || len(HMapValues(c)) != 2 || len(Iterate[struct {
		F0 map[int]struct{}
		F1 string
	}](c)) != 2 {
		t.Fatalf("keys, values and iteration should list every entry, got %v", ks)
	}
	if old := HMapRemoveValueForKey(&c, a); old == nil || *old != "y" || HMapCount(c) != 1 {
		t.Fatal("removeValue should drop the equal key")
	}
	HMapSet(&c, map[int]struct{}{3: {}}, nil)
	if !HMapIsEmpty(c) {
		t.Fatal("assigning nil should remove the entry")
	}

	lit := HMapOf([][]int{{1}, {1}, nil, {}}, []int{1, 2, 3, 4})
	if HMapCount(lit) != 2 || *HMapGet(lit, []int{1}) != 2 || *HMapGet(lit, nil) != 4 {
		t.Fatal("a literal should collapse equal keys, the last value winning")
	}
}

func TestHMapRemoveAllKeepingCapacityLeavesCopies(t *testing.T) {
	var d HMap[[]int, string]
	v := "a"
	HMapSet(&d, []int{1, 2}, &v)
	c := Copy(d)
	HMapRemoveAllKeepingCapacity(&d, true)
	if HMapCount(d) != 0 || HMapCount(c) != 1 {
		t.Fatalf("after removeAll: original %d entries, copy %d; want 0 and 1", HMapCount(d), HMapCount(c))
	}
}
