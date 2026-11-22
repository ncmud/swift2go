package swiftrt

import (
	"maps"
	"slices"
)

// HMap is a Swift `Dictionary` whose key type cannot be a Go map key (a set, an array, or a struct
// holding one). Entries are keyed by the canonical rendering HSet uses, so keys that Swift's
// Hashable equality calls equal, such as two sets with the same members, share one entry.
type HMap[K, V any] struct{ M map[string]HEntry[K, V] }

// HEntry is one HMap entry: the key as first stored, and its value.
type HEntry[K, V any] struct {
	Key   K
	Value V
}

// HMapOf is a dictionary literal: keys[i] maps to values[i].
func HMapOf[K, V any](keys []K, values []V) HMap[K, V] {
	d := HMap[K, V]{M: map[string]HEntry[K, V]{}}
	for i, k := range keys {
		d.M[hkey(k)] = HEntry[K, V]{Copy(k), values[i]}
	}
	return d
}

// HMapGet is `dict[key]`.
func HMapGet[K, V any](d HMap[K, V], k K) *V {
	e, ok := d.M[hkey(k)]
	if !ok {
		return nil
	}
	return &e.Value
}

// HMapSubscriptDefault is `dict[key, default: d]` as an rvalue.
func HMapSubscriptDefault[K, V any](d HMap[K, V], k K, def V) V {
	if e, ok := d.M[hkey(k)]; ok {
		return e.Value
	}
	return def
}

// HMapSet is `dict[key] = value`; nil removes the entry. A replaced entry keeps its original key,
// as Swift's does.
func HMapSet[K, V any](d *HMap[K, V], k K, v *V) {
	hk := hkey(k)
	if v == nil {
		delete(d.M, hk)
		return
	}
	if d.M == nil {
		d.M = map[string]HEntry[K, V]{}
	}
	if old, ok := d.M[hk]; ok {
		k = old.Key
	} else {
		k = Copy(k)
	}
	d.M[hk] = HEntry[K, V]{k, *v}
}

// HMapUpdateValueForKey is `updateValue(_:forKey:)`.
func HMapUpdateValueForKey[K, V any](d *HMap[K, V], v V, k K) *V {
	old := HMapGet(*d, k)
	HMapSet(d, k, &v)
	return old
}

// HMapRemoveValueForKey is `removeValue(forKey:)`.
func HMapRemoveValueForKey[K, V any](d *HMap[K, V], k K) *V {
	old := HMapGet(*d, k)
	delete(d.M, hkey(k))
	return old
}

// HMapRemoveAll is `removeAll()`.
func HMapRemoveAll[K, V any](d *HMap[K, V]) { d.M = nil }

// HMapCount is `count`.
func HMapCount[K, V any](d HMap[K, V]) int { return len(d.M) }

// HMapIsEmpty is `isEmpty`.
func HMapIsEmpty[K, V any](d HMap[K, V]) bool { return len(d.M) == 0 }

// HMapKeys is `keys`.
func HMapKeys[K, V any](d HMap[K, V]) []K {
	out := make([]K, 0, len(d.M))
	for _, e := range HMapElements(d) {
		out = append(out, e.F0)
	}
	return out
}

// HMapValues is `values`.
func HMapValues[K, V any](d HMap[K, V]) []V {
	out := make([]V, 0, len(d.M))
	for _, e := range HMapElements(d) {
		out = append(out, e.F1)
	}
	return out
}

// HMapElements lists the key/value pairs in a deterministic order (Swift's order is unspecified).
func HMapElements[K, V any](d HMap[K, V]) []struct {
	F0 K
	F1 V
} {
	hks := slices.Sorted(maps.Keys(d.M))
	out := make([]struct {
		F0 K
		F1 V
	}, len(hks))
	for i, hk := range hks {
		out[i].F0, out[i].F1 = d.M[hk].Key, d.M[hk].Value
	}
	return out
}

// Elements lists the key/value pairs, for iteration.
func (d HMap[K, V]) Elements() []struct {
	F0 K
	F1 V
} {
	return HMapElements(d)
}
