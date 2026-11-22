package swiftrt

import "testing"

type optKey struct{ zone, room int }

type optExit struct {
	description string
	destination optKey
}

//go:noinline
func optFind(xs []int, x int) Opt[int] {
	for i, v := range xs {
		if v == x {
			return OptValue(i)
		}
	}
	return Opt[int]{}
}

//go:noinline
func optExitFor(m map[int]optExit, d int) Opt[optExit] { return Copy(OptOf(MapGet(m, d))) }

// optDestination has the shape swift2go gives `exit(d)?.destination`.
//
//go:noinline
func optDestination(m map[int]optExit, d int) Opt[optKey] {
	return func() Opt[optKey] {
		e := optExitFor(m, d).Ptr()
		if e == nil {
			return Opt[optKey]{}
		}
		return OptValue[optKey]((*e).destination)
	}()
}

func TestOptRoundTrip(t *testing.T) {
	if p := OptOf[int](nil).Ptr(); p != nil {
		t.Fatalf("none.Ptr() = %v", *p)
	}
	if p := OptValue(7).Ptr(); p == nil || *p != 7 {
		t.Fatalf("OptValue(7).Ptr() = %v", p)
	}
	v := 3
	o := OptOf(&v)
	v = 4
	if !o.Ok || o.V != 3 {
		t.Fatalf("OptOf copied %v, want 3", o)
	}
}

func TestOptReturnDoesNotAllocate(t *testing.T) {
	xs := []int{1, 2, 3}
	exits := map[int]optExit{0: {description: "north", destination: optKey{30, 1}}}
	sum := 0
	allocs := testing.AllocsPerRun(100, func() {
		if p := optFind(xs, 2).Ptr(); p != nil {
			sum += *p
		}
		if p := optFind(xs, 9).Ptr(); p != nil {
			sum += *p
		}
		if p := optDestination(exits, 0).Ptr(); p != nil {
			sum += (*p).room
		}
		if p := optDestination(exits, 1).Ptr(); p != nil {
			sum += (*p).room
		}
	})
	if allocs != 0 {
		t.Fatalf("%v allocations per run, want 0", allocs)
	}
	if sum == 0 {
		t.Fatal("no values found")
	}
}

func BenchmarkOptReturn(b *testing.B) {
	xs := []int{1, 2, 3, 4, 5}
	exits := map[int]optExit{0: {description: "north", destination: optKey{30, 1}}}
	b.ReportAllocs()
	sum := 0
	for b.Loop() {
		if p := optFind(xs, 4).Ptr(); p != nil {
			sum += *p
		}
		if p := optDestination(exits, 0).Ptr(); p != nil {
			sum += (*p).room
		}
		if p := optDestination(exits, 1).Ptr(); p != nil {
			sum += (*p).room
		}
	}
	_ = sum
}
