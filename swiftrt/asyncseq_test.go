package swiftrt

import "testing"

func TestAsyncIterStream(t *testing.T) {
	ms := AsyncStreamMakeStreamOf(TypeT[int]{})
	ContinuationYield(ms.F1, 1)
	ContinuationYield(ms.F1, 2)
	ContinuationFinish(ms.F1)
	next := AsyncIter[int](ms.F0)
	var got []int
	for p := next(); p != nil; p = next() {
		got = append(got, *p)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestAsyncIterSlice(t *testing.T) {
	next := AsyncIter[string]([]string{"a"})
	if p := next(); p == nil || *p != "a" || next() != nil {
		t.Fatal("slice walk")
	}
}
