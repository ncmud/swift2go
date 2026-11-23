package swiftrt

import "testing"

type metaZeroWord struct{ _ string }

func (metaZeroWord) StaticName() string { return "word" }

type metaZeroNamer interface{ StaticName() string }

func metaZeroName[T metaZeroNamer]() string { return MetaZero[T]().StaticName() }

func TestMetaZeroCallsAValueMethodThroughAPointerType(t *testing.T) {
	if got := metaZeroName[*metaZeroWord](); got != "word" {
		t.Fatalf("pointer type argument: %q", got)
	}
	if got := metaZeroName[metaZeroWord](); got != "word" {
		t.Fatalf("value type argument: %q", got)
	}
	if MetaZero[int]() != 0 || MetaZero[*int]() == nil {
		t.Fatal("MetaZero of int or *int")
	}
}
