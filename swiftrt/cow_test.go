package swiftrt

import (
	"reflect"
	"testing"
	"unsafe"
)

func sameStorage[T any](a, b []T) bool {
	return len(a) > 0 && len(b) > 0 && unsafe.SliceData(a) == unsafe.SliceData(b)
}

func TestCopySharesFrozenArrays(t *testing.T) {
	a := []int{1, 2, 3}
	b := Copy(a)
	if !sameStorage(a, b) {
		t.Fatal("a copy of an array with no spare capacity should share its storage")
	}
	Mut(&b)[0] = 9
	if sameStorage(a, b) || a[0] != 1 || b[0] != 9 {
		t.Fatalf("writing a shared copy must move it first: a=%v b=%v", a, b)
	}
	Mut(&b)[1] = 8
	if cap(b) == len(b) {
		t.Fatal("an array Mut moved should own spare capacity")
	}
	c := b
	Mut(&c)[2] = 7
	if !sameStorage(b, c) {
		t.Fatal("Mut must write an owned array in place")
	}
}

func TestCopyClonesOwnedArrays(t *testing.T) {
	a := make([]int, 3, 8)
	b := Copy(a)
	if sameStorage(a, b) || len(b) != cap(b) {
		t.Fatalf("a copy of an owned array should be new storage that copies may share: len %d cap %d", len(b), cap(b))
	}
	a = append(a, 4)
	if len(b) != 3 || len(a) != 4 {
		t.Fatal("appending to the original changed the copy")
	}
}

func TestCopyClonesWhatIsNotFrozen(t *testing.T) {
	inner := make([]int, 2, 4)
	outer := [][]int{{1}, inner}
	c := Copy(outer)
	if sameStorage(outer, c) {
		t.Fatal("an array holding an owned array must be copied")
	}
	if !sameStorage(outer[0], c[0]) || sameStorage(outer[1], c[1]) {
		t.Fatal("frozen elements should be shared and owned ones copied")
	}
	d := Copy(c)
	if !sameStorage(c, d) {
		t.Fatal("a copy should be frozen, so that copies of it share it")
	}
	Mut(&Mut(&d)[1])[0] = 5
	if c[1][0] != 0 || d[1][0] != 5 {
		t.Fatalf("nested write leaked: c=%v d=%v", c, d)
	}
}

type cowRow struct {
	Cells []int  `swift:"cells"`
	Tag   string `swift:"tag"`
}

type cowWithOptional struct {
	Cells []int `swift:"cells"`
	Opt   *int  `swift:"opt"`
}

type cowRuntimeOwned struct{ Items []int }

func TestShareableElements(t *testing.T) {
	for _, tc := range []struct {
		t         reflect.Type
		shareable bool
	}{
		{reflect.TypeFor[int](), true},
		{reflect.TypeFor[[]string](), true},
		{reflect.TypeFor[cowRow](), true},
		{reflect.TypeFor[[]cowRow](), true},
		{reflect.TypeFor[*copyClass](), true},
		{reflect.TypeFor[copyNode](), false},
		{reflect.TypeFor[cowWithOptional](), false},
		{reflect.TypeFor[map[string]int](), false},
		{reflect.TypeFor[any](), false},
		{reflect.TypeFor[cowRuntimeOwned](), false},
		{reflect.TypeFor[struct{ F0 []int }](), true},
	} {
		if got := shareableElems(tc.t); got != tc.shareable {
			t.Errorf("shareableElems(%v) = %v, want %v", tc.t, got, tc.shareable)
		}
	}
}

func TestUnshareableArraysAreAlwaysCopied(t *testing.T) {
	one := 1
	a := []cowWithOptional{{Cells: []int{1}, Opt: &one}}
	b := Copy(a)
	if sameStorage(a, b) || b[0].Opt == a[0].Opt {
		t.Fatal("an array of values holding optionals must be copied")
	}
	if cap(b) == len(b) {
		t.Fatal("an unshareable copy should own spare capacity, so Mut need not look it up")
	}
	exact := []cowWithOptional{{}}
	Mut(&exact)[0].Opt = &one
	if exact[0].Opt != &one {
		t.Fatal("Mut lost a write")
	}
}

func TestShrinkKeepsSharedStorageMarked(t *testing.T) {
	a := []int{1, 2, 3}
	b := Copy(a)
	SliceRemoveLast(&b)
	b = append(b, 30)
	if a[2] != 3 {
		t.Fatalf("append after removeLast wrote into a shared copy: %v", a)
	}
	owned := make([]int, 3, 6)
	SliceRemoveLast(&owned)
	if cap(owned) != 6 {
		t.Fatal("removeLast on an owned array should keep its capacity")
	}
	c := Copy(a)
	SliceRemoveAllKeepingCapacity(&c, true)
	c = append(c, 9)
	if a[0] != 1 {
		t.Fatalf("removeAll(keepingCapacity:) wrote into a shared copy: %v", a)
	}
}

func TestRuntimeMutatorsMoveSharedArrays(t *testing.T) {
	base := []int{3, 1, 2}
	for name, f := range map[string]func(*[]int){
		"sort":     SliceSort[int],
		"reverse":  SliceReverse[int],
		"swapAt":   func(xs *[]int) { SliceSwapAt(xs, 0, 2) },
		"remove":   func(xs *[]int) { SliceRemoveAt(xs, 0) },
		"where":    func(xs *[]int) { SliceRemoveAllWhere(xs, func(x int) bool { return x == 1 }) },
		"subrange": func(xs *[]int) { SliceRemoveSubrangeRange(xs, Range[int]{0, 2}) },
		"sortBy":   func(xs *[]int) { SliceSortBy(xs, func(a, b int) bool { return a > b }) },
	} {
		c := Copy(base)
		f(&c)
		if !reflect.DeepEqual(base, []int{3, 1, 2}) {
			t.Fatalf("%s wrote into a shared copy: %v", name, base)
		}
	}
}

func TestMutDoesNotAllocateForOwnedArrays(t *testing.T) {
	a := make([]int, 4, 5)
	if n := testing.AllocsPerRun(100, func() { Mut(&a)[1]++ }); n != 0 {
		t.Fatalf("Mut on an owned array allocated %v times", n)
	}
}

func TestCopySharesWithoutAllocating(t *testing.T) {
	v := struct {
		Rows  []cowRow   `swift:"rows"`
		Marks [][]string `swift:"marks"`
	}{Rows: []cowRow{{Cells: []int{1, 2}}}, Marks: [][]string{{"a"}}}
	v = Copy(v) // freeze whatever was built with spare capacity
	if n := testing.AllocsPerRun(100, func() { _ = Copy(v) }); n != 0 {
		t.Fatalf("copying a frozen value allocated %v times", n)
	}
}

// The read path is unchanged: an element read is a Go slice index whether the array shares its
// storage or not, and a write to an owned array costs one comparison.

var cowSink int

func BenchmarkReadSharedArray(b *testing.B) {
	a := make([]int, 1024)
	s := Copy(a)
	for b.Loop() {
		for i := range s {
			cowSink += s[i]
		}
	}
}

func BenchmarkReadOwnedArray(b *testing.B) {
	s := make([]int, 1024, 1025)
	for b.Loop() {
		for i := range s {
			cowSink += s[i]
		}
	}
}

func BenchmarkWriteOwnedArrayDirect(b *testing.B) {
	s := make([]int, 1024, 1025)
	for b.Loop() {
		for i := range s {
			s[i] = i
		}
	}
}

func BenchmarkWriteOwnedArrayMut(b *testing.B) {
	s := make([]int, 1024, 1025)
	for b.Loop() {
		for i := range s {
			Mut(&s)[i] = i
		}
	}
}

func BenchmarkCopySharedNested(b *testing.B) {
	v := make([][]int, 64)
	for i := range v {
		v[i] = []int{i, i}
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = Copy(v)
	}
}
