package swiftrt

import "testing"

type chainEntry struct{ Remaining int }

// TestOptionalChainWriteBack runs the shape the emitter gives `dict[k]?.remaining = v` on each
// dictionary family: the Get functions return a copy, so only the stored-back value survives.
func TestOptionalChainWriteBack(t *testing.T) {
	m := map[string]chainEntry{"a": {1}}
	if b := MapGet(m, "a"); b != nil {
		(*b).Remaining = 5
		m["a"] = *b
	}
	if m["a"].Remaining != 5 {
		t.Fatalf("map entry not written back: %v", m["a"])
	}

	var od OrderedDictionary[string, chainEntry]
	OrderedDictionarySet(&od, "a", &chainEntry{1})
	if b := OrderedDictionaryGet(od, "a"); b != nil {
		(*b).Remaining = 5
		if OrderedDictionaryGet(od, "a").Remaining != 1 {
			t.Fatal("OrderedDictionaryGet should return a copy")
		}
		OrderedDictionarySet(&od, "a", b)
	}
	if got := OrderedDictionaryGet(od, "a"); got.Remaining != 5 || OrderedDictionaryCount(od) != 1 {
		t.Fatalf("ordered dictionary entry not written back: %v", got)
	}

	var hm HMap[string, chainEntry]
	HMapSet(&hm, "a", &chainEntry{1})
	if b := HMapGet(hm, "a"); b != nil {
		(*b).Remaining = 5
		if HMapGet(hm, "a").Remaining != 1 {
			t.Fatal("HMapGet should return a copy")
		}
		HMapSet(&hm, "a", b)
	}
	if got := HMapGet(hm, "a"); got.Remaining != 5 || HMapCount(hm) != 1 {
		t.Fatalf("hashed map entry not written back: %v", got)
	}
}
