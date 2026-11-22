package swiftrt

import (
	"cmp"
	"maps"
	"slices"
	"sort"
)

// OrderedDictionary is swift-collections `OrderedDictionary`: a map that iterates in insertion order.
// Clone it before mutating a copy; the emitter does this where Swift would copy.
type OrderedDictionary[K comparable, V any] struct {
	Keys_ []K
	M     map[K]V
}

// OrderedDictionaryInit is `OrderedDictionary()`.
func OrderedDictionaryInit[D ~struct {
	Keys_ []K
	M     map[K]V
}, K comparable, V any]() D {
	return D{M: map[K]V{}}
}

// OrderedDictionaryGet is `dict[key]`.
func OrderedDictionaryGet[K comparable, V any](d OrderedDictionary[K, V], k K) *V {
	v, ok := d.M[k]
	if !ok {
		return nil
	}
	return &v
}

// OrderedDictionarySubscriptInt is `dict[key]` on an Int-keyed dictionary.
func OrderedDictionarySubscriptInt[K comparable, V any](d OrderedDictionary[K, V], k K) *V {
	return OrderedDictionaryGet(d, k)
}

// OrderedDictionarySet is `dict[key] = value` (nil removes).
func OrderedDictionarySet[K comparable, V any](d *OrderedDictionary[K, V], k K, v *V) {
	if d.M == nil {
		d.M = map[K]V{}
	}
	if v == nil {
		OrderedDictionaryRemoveValueForKey(d, k)
		return
	}
	if _, ok := d.M[k]; !ok {
		d.Keys_ = append(d.Keys_, k)
	}
	d.M[k] = *v
}

// OrderedDictionaryRemoveValueForKey is `removeValue(forKey:)`.
func OrderedDictionaryRemoveValueForKey[K comparable, V any](d *OrderedDictionary[K, V], k K) *V {
	v, ok := d.M[k]
	if !ok {
		return nil
	}
	delete(d.M, k)
	d.Keys_ = slices.DeleteFunc(d.Keys_, func(x K) bool { return x == k })
	return &v
}

// OrderedDictionaryCount is `count`.
func OrderedDictionaryCount[K comparable, V any](d OrderedDictionary[K, V]) int { return len(d.Keys_) }

// OrderedDictionaryIsEmpty is `isEmpty`.
func OrderedDictionaryIsEmpty[K comparable, V any](d OrderedDictionary[K, V]) bool {
	return len(d.Keys_) == 0
}

// OrderedDictionaryKeys is `keys`.
func OrderedDictionaryKeys[K comparable, V any](d OrderedDictionary[K, V]) OrderedSet[K] {
	return OrderedSet[K]{Items: slices.Clone(d.Keys_)}
}

// OrderedDictionaryValues is `values`.
func OrderedDictionaryValues[K comparable, V any](d OrderedDictionary[K, V]) []V {
	out := make([]V, 0, len(d.Keys_))
	for _, k := range d.Keys_ {
		out = append(out, d.M[k])
	}
	return out
}

// OrderedDictionaryElements lists key/value pairs in order, for iteration.
func OrderedDictionaryElements[K comparable, V any](d OrderedDictionary[K, V]) []struct {
	F0 K
	F1 V
} {
	out := make([]struct {
		F0 K
		F1 V
	}, 0, len(d.Keys_))
	for _, k := range d.Keys_ {
		out = append(out, struct {
			F0 K
			F1 V
		}{k, d.M[k]})
	}
	return out
}

// OrderedDictionaryRemoveAll is `removeAll()`.
func OrderedDictionaryRemoveAll[K comparable, V any](d *OrderedDictionary[K, V]) {
	d.Keys_, d.M = nil, map[K]V{}
}

// Clone copies the dictionary so mutations do not alias.
func (d OrderedDictionary[K, V]) Clone() OrderedDictionary[K, V] {
	m := make(map[K]V, len(d.M))
	maps.Copy(m, d.M)
	return OrderedDictionary[K, V]{slices.Clone(d.Keys_), m}
}

// OrderedSet is swift-collections `OrderedSet`.
type OrderedSet[T comparable] struct{ Items []T }

// OrderedSetContains is `contains`.
func OrderedSetContains[T comparable](s OrderedSet[T], v T) bool { return slices.Contains(s.Items, v) }

// OrderedSetAppend is `append`, returning whether the element was new and its index.
func OrderedSetAppend[T comparable](s *OrderedSet[T], v T) struct {
	F0 bool
	F1 int
} {
	if i := slices.Index(s.Items, v); i >= 0 {
		return struct {
			F0 bool
			F1 int
		}{false, i}
	}
	s.Items = append(s.Items, v)
	return struct {
		F0 bool
		F1 int
	}{true, len(s.Items) - 1}
}

// OrderedSetInsertAt is `insert(_:at:)`.
func OrderedSetInsertAt[T comparable](s *OrderedSet[T], v T, i int) struct {
	F0 bool
	F1 int
} {
	if j := slices.Index(s.Items, v); j >= 0 {
		return struct {
			F0 bool
			F1 int
		}{false, j}
	}
	s.Items = slices.Insert(s.Items, i, v)
	return struct {
		F0 bool
		F1 int
	}{true, i}
}

// OrderedSetRemove is `remove`.
func OrderedSetRemove[T comparable](s *OrderedSet[T], v T) *T {
	i := slices.Index(s.Items, v)
	if i < 0 {
		return nil
	}
	s.Items = slices.Delete(s.Items, i, i+1)
	return &v
}

// OrderedSetCount is `count`.
func OrderedSetCount[T comparable](s OrderedSet[T]) int { return len(s.Items) }

// OrderedSetFirst is `first`.
func OrderedSetFirst[T comparable](s OrderedSet[T]) *T { return SliceFirst(s.Items) }

// OrderedSetElements lists members in order.
func OrderedSetElements[T comparable](s OrderedSet[T]) []T { return s.Items }

// OrderedSetFilter is `filter`.
func OrderedSetFilter[T comparable](s OrderedSet[T], f func(T) bool) []T {
	return SliceFilter(s.Items, f)
}

// OrderedSetSorted is `sorted()`.
func OrderedSetSorted[T cmp.Ordered](s OrderedSet[T]) []T { return SliceSorted(s.Items) }

// OrderedSetReversed is `reversed()`.
func OrderedSetReversed[T comparable](s OrderedSet[T]) []T { return SliceReversed(s.Items) }

// OrderedSetCompactMap is `compactMap`.
func OrderedSetCompactMap[T comparable, U any](s OrderedSet[T], f func(T) *U) []U {
	return SliceCompactMap(s.Items, f)
}

// OrderedSetFirstIndexOf is `firstIndex(of:)`.
func OrderedSetFirstIndexOf[T comparable](s OrderedSet[T], v T) *int {
	if i := slices.Index(s.Items, v); i >= 0 {
		return &i
	}
	return nil
}

// OrderedSetSubscriptInt is `set[i]`.
func OrderedSetSubscriptInt[T comparable](s OrderedSet[T], i int) T { return s.Items[i] }

// OrderedSetInitSlice is `OrderedSet(array)`.
func OrderedSetInitSlice[S ~struct{ Items []T }, T comparable](xs []T) S {
	var s OrderedSet[T]
	for _, x := range xs {
		OrderedSetAppend(&s, x)
	}
	return S(s)
}

// Heap is swift-collections `Heap` (a min-max heap; only min operations are provided). Less orders
// elements by the element type's own `<`; when nil, Go ordering is used.
type Heap[T any] struct {
	Items []T
	Less  func(a, b T) bool
}

// HeapInit is `Heap()` for a Go-ordered element type.
func HeapInit[H ~struct {
	Items []T
	Less  func(a, b T) bool
}, T any]() H {
	return H{}
}

// HeapInitLess is `Heap()` for an element type ordered by its own `<`.
func HeapInitLess[T any](less func(a, b T) bool) Heap[T] { return Heap[T]{Less: less} }

func (h *Heap[T]) less(a, b T) bool {
	if h.Less != nil {
		return h.Less(a, b)
	}
	return compareAny(a, b) < 0
}

// HeapInsert is `insert`.
func HeapInsert[T any](h *Heap[T], v T) {
	i := sort.Search(len(h.Items), func(i int) bool { return h.less(v, h.Items[i]) })
	h.Items = slices.Insert(h.Items, i, v)
}

// HeapPopMin is `popMin()`.
func HeapPopMin[T any](h *Heap[T]) *T {
	if len(h.Items) == 0 {
		return nil
	}
	v := h.Items[0]
	h.Items = h.Items[1:]
	return &v
}

// HeapMin is `min`.
func HeapMin[T any](h Heap[T]) *T { return SliceFirst(h.Items) }

// HeapCount is `count`.
func HeapCount[T any](h Heap[T]) int { return len(h.Items) }

// HeapIsEmpty is `isEmpty`.
func HeapIsEmpty[T any](h Heap[T]) bool { return len(h.Items) == 0 }
