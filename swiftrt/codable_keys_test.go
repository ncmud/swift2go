package swiftrt

import (
	"reflect"
	"testing"
)

type fieldKey struct{ StringValue string }

func TestKeyedDecodingContainerAllKeys(t *testing.T) {
	c := DecoderContainerKeyedBy(&Decoder{Value: map[string]any{"b": 1, "a": 2, "skip": 3}}, nil)
	keys := KeyedDecodingContainerAllKeys(c, func(s string) *fieldKey {
		if s == "skip" {
			return nil
		}
		return &fieldKey{s}
	})
	if want := []fieldKey{{"a"}, {"b"}}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("allKeys = %v, want %v", keys, want)
	}
}

func TestMapInitGroupingByDictionaryElements(t *testing.T) {
	type entry = struct {
		F0 string
		F1 int
	}
	byValue := MapInitGroupingBy[map[int][]entry](MapOrdered(map[string]int{"z": 1, "x": 1, "y": 2}), func(e entry) int { return e.F1 })
	want := map[int][]entry{1: {{"x", 1}, {"z", 1}}, 2: {{"y", 2}}}
	if !reflect.DeepEqual(byValue, want) {
		t.Fatalf("grouping = %v, want %v", byValue, want)
	}
}
