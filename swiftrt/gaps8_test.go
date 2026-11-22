package swiftrt

import (
	"strings"
	"testing"
)

func TestReversedCollectionPrefixWhile(t *testing.T) {
	got := ReversedCollectionPrefixWhile(SliceReversed([]uint8{1, 255, 255}), func(b uint8) bool { return b == 255 })
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestMapReserveCapacityInt(t *testing.T) {
	var m map[string]int
	MapReserveCapacityInt(&m, 4)
	m["a"] = 1
	MapReserveCapacityInt(&m, 8)
	if m["a"] != 1 {
		t.Fatalf("reserving capacity lost entries: %v", m)
	}
}

func TestRequireRefAndSourceLocation(t *testing.T) {
	defer BindT(t)()
	x := &struct{ n int }{1}
	if RequireRef(x, "") != x {
		t.Fatal("RequireRef changed its value")
	}
	if loc, _ := SourceLocationHere().(string); !strings.Contains(loc, "gaps8_test.go:") {
		t.Fatalf("SourceLocationHere() = %q", loc)
	}
}
