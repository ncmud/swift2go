package swiftrt

import (
	"reflect"
	"testing"
)

type copyClass struct{ n int }

func (*copyClass) SwiftClass() {}

type copyInner struct {
	Opt  *int
	List []int
}

type copyOuter struct {
	Name    string
	Inner   copyInner
	Nested  [][]int
	ByKey   map[string]copyInner
	Any     any
	Ref     *copyClass
	OptBox  *copyInner
	Pair    [2]copyInner
	private []int
}

type copyNode struct {
	Value    int
	Children []copyNode
	Next     *copyNode
}

type copyPlain struct {
	A int
	B string
	C [3]float64
	D *copyClass
}

func newCopyOuter() copyOuter {
	one, two := 1, 2
	return copyOuter{
		Name:    "a",
		Inner:   copyInner{Opt: &one, List: []int{1, 2, 3}},
		Nested:  [][]int{{1}, {2, 3}},
		ByKey:   map[string]copyInner{"k": {Opt: &two, List: []int{4}}},
		Any:     copyInner{List: []int{5}},
		Ref:     &copyClass{n: 1},
		OptBox:  &copyInner{List: []int{6}},
		Pair:    [2]copyInner{{List: []int{7}}, {List: []int{8}}},
		private: []int{9},
	}
}

func TestCopyUnsharesEveryValuePart(t *testing.T) {
	orig := newCopyOuter()
	want := newCopyOuter()
	c := Copy(orig)
	if !reflect.DeepEqual(c, orig) {
		t.Fatalf("copy differs from original: %+v", c)
	}

	*c.Inner.Opt = 100
	Mut(&c.Inner.List)[0] = 100
	Mut(&Mut(&c.Nested)[1])[0] = 100
	byKey := c.ByKey["k"]
	Mut(&byKey.List)[0] = 100
	*byKey.Opt = 100
	c.ByKey["k"] = byKey
	c.ByKey["new"] = copyInner{}
	inAny := c.Any.(copyInner)
	Mut(&inAny.List)[0] = 100
	c.Any = inAny
	Mut(&c.OptBox.List)[0] = 100
	Mut(&c.Pair[1].List)[0] = 100

	want.Ref, want.private = orig.Ref, orig.private
	if !reflect.DeepEqual(orig, want) {
		t.Fatalf("mutating the copy changed the original: %+v", orig)
	}
	if c.Ref != orig.Ref {
		t.Fatal("class reference lost its identity")
	}
}

func TestCopyOriginalMutationsStayOutOfTheCopy(t *testing.T) {
	orig := newCopyOuter()
	c := Copy(orig)
	want := newCopyOuter()
	want.Ref, want.private = c.Ref, c.private

	*orig.Inner.Opt = 100
	Mut(&orig.Inner.List)[0] = 100
	Mut(&Mut(&orig.Nested)[0])[0] = 100
	byKey := orig.ByKey["k"]
	Mut(&byKey.List)[0] = 100
	orig.ByKey["k"] = byKey
	inAny := orig.Any.(copyInner)
	Mut(&inAny.List)[0] = 100
	orig.Any = inAny
	Mut(&orig.OptBox.List)[0] = 100
	Mut(&orig.Pair[0].List)[0] = 100

	if !reflect.DeepEqual(c, want) {
		t.Fatalf("mutating the original changed the copy: %+v", c)
	}
}

func TestCopyKeepsNilAndEmpty(t *testing.T) {
	var zero copyOuter
	c := Copy(zero)
	if c.Inner.List != nil || c.ByKey != nil || c.Any != nil || c.OptBox != nil || c.Nested != nil {
		t.Fatalf("nil parts became non-nil: %+v", c)
	}
	empty := []int{}
	if got := Copy(empty); got == nil || len(got) != 0 {
		t.Fatalf("empty slice became %v", got)
	}
}

func TestCopyRecursiveTypes(t *testing.T) {
	orig := copyNode{Value: 1, Children: []copyNode{{Value: 2, Children: []copyNode{{Value: 3}}}}, Next: &copyNode{Value: 4, Next: &copyNode{Value: 5}}}
	c := Copy(orig)
	c.Children[0].Children[0].Value = 30
	c.Next.Next.Value = 50
	if orig.Children[0].Children[0].Value != 3 || orig.Next.Next.Value != 5 {
		t.Fatalf("recursive copy shares storage: %+v", orig)
	}
	if c.Children[0].Value != 2 || c.Next.Value != 4 {
		t.Fatalf("recursive copy lost values: %+v", c)
	}
}

func TestCopyTopLevelContainers(t *testing.T) {
	s := []copyInner{{List: []int{1}}}
	cs := Copy(s)
	Mut(&Mut(&cs)[0].List)[0] = 9
	if s[0].List[0] != 1 {
		t.Fatal("slice of structs shares element storage")
	}

	m := map[int][]int{1: {1}}
	cm := Copy(m)
	first := cm[1]
	Mut(&first)[0] = 9
	cm[1] = first
	cm[2] = nil
	if m[1][0] != 1 || len(m) != 1 {
		t.Fatal("map shares storage")
	}

	one := 1
	p := Copy(&one)
	*p = 9
	if one != 1 {
		t.Fatal("optional pointer shares its target")
	}

	ref := &copyClass{n: 1}
	if Copy(ref) != ref {
		t.Fatal("class reference was copied")
	}

	var a any = []int{1}
	ca := Copy(a)
	inAny := ca.([]int)
	Mut(&inAny)[0] = 9
	if a.([]int)[0] != 1 {
		t.Fatal("interface holding a slice shares storage")
	}
}

func TestNeedsDeepCopy(t *testing.T) {
	for _, tc := range []struct {
		t    reflect.Type
		deep bool
	}{
		{reflect.TypeFor[copyPlain](), false},
		{reflect.TypeFor[[4]copyPlain](), false},
		{reflect.TypeFor[*copyClass](), false},
		{reflect.TypeFor[copyInner](), true},
		{reflect.TypeFor[copyNode](), true},
		{reflect.TypeFor[[2]copyInner](), true},
		{reflect.TypeFor[any](), true},
	} {
		if got := needsDeepCopy(tc.t); got != tc.deep {
			t.Errorf("needsDeepCopy(%v) = %v, want %v", tc.t, got, tc.deep)
		}
	}
}

func TestCopyPlainTypeDoesNotAllocate(t *testing.T) {
	v := copyPlain{A: 1, B: "b", D: &copyClass{}}
	if n := testing.AllocsPerRun(100, func() { v = Copy(v) }); n != 0 {
		t.Fatalf("plain copy allocated %v times", n)
	}
}

func TestCopyAllocatesOnlyForSharedParts(t *testing.T) {
	one := 1
	v := struct {
		Big [32]int64
		Opt *int
	}{Opt: &one}
	// One allocation for the pointer target; the struct itself must not be boxed.
	if n := testing.AllocsPerRun(100, func() { v = Copy(v) }); n > 2 {
		t.Fatalf("deep copy allocated %v times", n)
	}
}

func BenchmarkCopyStructWithOptional(b *testing.B) {
	one := 1
	v := struct {
		Big [32]int64
		Opt *int
	}{Opt: &one}
	b.ReportAllocs()
	for b.Loop() {
		v = Copy(v)
	}
}

func TestSliceInitRepeatingCountCopiesEachElement(t *testing.T) {
	rows := SliceInitRepeatingCount[[][]int]([]int{0, 0}, 3)
	Mut(&Mut(&rows)[1])[1] = 5
	if want := [][]int{{0, 0}, {0, 5}, {0, 0}}; !reflect.DeepEqual(rows, want) {
		t.Errorf("rows share storage: got %v, want %v", rows, want)
	}
	ref := &copyClass{n: 1}
	refs := SliceInitRepeatingCount[[]*copyClass](ref, 2)
	if refs[0] != ref || refs[1] != ref {
		t.Error("a repeated class reference must keep its identity")
	}
}

type copyBacking struct {
	_values []int `swift:"_values"`
	cache   []int
}

func TestCopyUnexportedSwiftProperty(t *testing.T) {
	orig := copyBacking{_values: []int{1, 2}, cache: []int{3}}
	c := Copy(orig)
	Mut(&c._values)[0] = 9
	c.cache[0] = 9
	if orig._values[0] != 1 {
		t.Error("a Swift property with an unexported Go name must be copied")
	}
	if orig.cache[0] != 9 {
		t.Error("an unexported runtime field stays shared")
	}
}

type copyKey interface{ isCopyKey() }

type copyKeyRoom struct{ vnum int }

func (copyKeyRoom) isCopyKey() {}

type copyExit struct {
	Description string   `swift:"description"`
	Keywords    []string `swift:"keywords"`
	Destination copyKey  `swift:"destination"`
}

func TestCopyAllocatesOnlyWhatItUnshares(t *testing.T) {
	bare := copyExit{Description: "north", Destination: copyKeyRoom{3001}}
	withKeywords := copyExit{Description: "south", Keywords: []string{"door"}, Destination: copyKeyRoom{3002}}
	var out copyExit
	if n := testing.AllocsPerRun(100, func() { out = Copy(bare) }); n != 0 {
		t.Errorf("copying a value with nothing to unshare made %v allocations, want 0", n)
	}
	keywords := withKeywords.Keywords
	slice := testing.AllocsPerRun(100, func() { keywords = Copy(keywords) })
	if n := testing.AllocsPerRun(100, func() { out = Copy(withKeywords) }); n != slice {
		t.Errorf("copying a value with one slice made %v allocations, want the slice's own %v", n, slice)
	}
	Mut(&out.Keywords)[0] = "gate"
	if withKeywords.Keywords[0] != "door" || out.Destination != withKeywords.Destination {
		t.Errorf("copy shares storage or lost a field: %+v from %+v", out, withKeywords)
	}
}

func BenchmarkCopyStruct(b *testing.B) {
	v := copyExit{Description: "south", Keywords: []string{"door"}, Destination: copyKeyRoom{3002}}
	b.ReportAllocs()
	for b.Loop() {
		v = Copy(v)
	}
}

// deepNode nests n nodes, each the only child of the one before.
func deepNode(n int) copyNode {
	root := copyNode{Value: 0}
	cur := &root
	for i := 1; i < n; i++ {
		cur.Children = []copyNode{{Value: i}}
		cur = &cur.Children[0]
	}
	return root
}

// TestCopyWhileTheStackGrows copies a value whose plan recurses deep enough to grow a fresh
// goroutine's stack while the copy is being fixed in place on that stack.
func TestCopyWhileTheStackGrows(t *testing.T) {
	const depth = 2_000
	orig := deepNode(depth)
	done := make(chan copyNode)
	go func() { done <- Copy(orig) }()
	c := <-done
	cur, other := &c, &orig
	for i := range depth {
		if cur.Value != i {
			t.Fatalf("node %d holds %d", i, cur.Value)
		}
		if i < depth-1 {
			if &cur.Children[0] == &other.Children[0] {
				t.Fatalf("node %d shares its children with the original", i)
			}
			cur, other = &cur.Children[0], &other.Children[0]
		}
	}
}

// BenchmarkCopyNested copies a struct like the gear optimizer's candidates: nested structs holding
// slices of structs and optionals.
func BenchmarkCopyNested(b *testing.B) {
	v := newCopyOuter()
	b.ReportAllocs()
	for b.Loop() {
		v = Copy(v)
	}
}
