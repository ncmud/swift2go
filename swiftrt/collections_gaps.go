package swiftrt

import (
	"hash/maphash"
	"reflect"
	"slices"
)

// RangeCompactMap is `(lo..<hi).compactMap`.
func RangeCompactMap[U any](r Range[int], f func(int) *U) []U {
	return SliceCompactMap(rangeInts(r.Lo, r.Hi), f)
}

// RangeFirstWhere is `(lo..<hi).first(where:)`.
func RangeFirstWhere(r Range[int], f func(int) bool) *int {
	for i := r.Lo; i < r.Hi; i++ {
		if f(i) {
			v := i
			return &v
		}
	}
	return nil
}

// RangeContainsWhere is `(lo..<hi).contains(where:)`.
func RangeContainsWhere(r Range[int], f func(int) bool) bool { return RangeFirstWhere(r, f) != nil }

// RangeReduce is `(lo..<hi).reduce(_:_:)`.
func RangeReduce[A any](r Range[int], init A, f func(A, int) A) A {
	for i := r.Lo; i < r.Hi; i++ {
		init = f(init, i)
	}
	return init
}

// RangeDropFirst is `(lo..<hi).dropFirst()`, whose SubSequence is itself a Range.
func RangeDropFirst(r Range[int]) Range[int] {
	if r.Lo < r.Hi {
		return Range[int]{r.Lo + 1, r.Hi}
	}
	return r
}

// RangeSortedBy is `(lo..<hi).sorted(by:)`.
func RangeSortedBy(r Range[int], less func(int, int) bool) []int {
	return SliceSortedBy(rangeInts(r.Lo, r.Hi), less)
}

// RangeMaxBy is `(lo..<hi).max(by:)`.
func RangeMaxBy(r Range[int], less func(int, int) bool) *int {
	return SliceMaxBy(rangeInts(r.Lo, r.Hi), less)
}

// RangeFlatMap is `(lo..<hi).flatMap`.
func RangeFlatMap[U any](r Range[int], f func(int) []U) []U {
	return SliceFlatMap(rangeInts(r.Lo, r.Hi), f)
}

// RangeLazy is `(lo..<hi).lazy`; Go evaluates eagerly.
func RangeLazy(r Range[int]) []int { return rangeInts(r.Lo, r.Hi) }

// eachClosed calls f for lo...hi in order until f returns false; it never computes hi+1, so hi may be Int.max.
func eachClosed(r ClosedRange[int], f func(int) bool) {
	if r.Hi < r.Lo {
		Trap("Range requires lowerBound <= upperBound")
	}
	for i := r.Lo; ; i++ {
		if !f(i) || i == r.Hi {
			return
		}
	}
}

// ClosedRangeReduce is `(lo...hi).reduce(_:_:)`.
func ClosedRangeReduce[A any](r ClosedRange[int], init A, f func(A, int) A) A {
	eachClosed(r, func(i int) bool { init = f(init, i); return true })
	return init
}

// ClosedRangeFirstWhere is `(lo...hi).first(where:)`.
func ClosedRangeFirstWhere(r ClosedRange[int], f func(int) bool) *int {
	var out *int
	eachClosed(r, func(i int) bool {
		if f(i) {
			out = &i
			return false
		}
		return true
	})
	return out
}

// ClosedRangeContainsWhere is `(lo...hi).contains(where:)`.
func ClosedRangeContainsWhere(r ClosedRange[int], f func(int) bool) bool {
	return ClosedRangeFirstWhere(r, f) != nil
}

// ClosedRangeCount is `(lo...hi).count`; it traps when the count overflows Int, like Swift.
func ClosedRangeCount(r ClosedRange[int]) int { return Add(Sub(r.Hi, r.Lo), 1) }

// ClosedRangePrefixInt is `(lo...hi).prefix(n)`.
func ClosedRangePrefixInt(r ClosedRange[int], n int) []int {
	if n < 0 {
		Trap("Can't take a prefix of negative length from a collection")
	}
	out := []int{}
	if n == 0 {
		return out
	}
	eachClosed(r, func(i int) bool { out = append(out, i); return len(out) < n })
	return out
}

// ClosedRangeCompactMap is `(lo...hi).compactMap`.
func ClosedRangeCompactMap[U any](r ClosedRange[int], f func(int) *U) []U {
	out := []U{}
	eachClosed(r, func(i int) bool {
		if v := f(i); v != nil {
			out = append(out, *v)
		}
		return true
	})
	return out
}

// PartialRangeFromFirstWhere is `(lo...).first(where:)`; it traps when the count overflows, like Swift.
func PartialRangeFromFirstWhere(r PartialRangeFrom[int], f func(int) bool) *int {
	for i := r.Lo; ; i = Add(i, 1) {
		if f(i) {
			v := i
			return &v
		}
	}
}

// StrideToCompactMap is `stride(from:to:by:).compactMap`.
func StrideToCompactMap[T, U any](xs []T, f func(T) *U) []U { return SliceCompactMap(xs, f) }

// SliceElementsEqual is `elementsEqual(_:)`.
func SliceElementsEqual[T any](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// SliceLexicographicallyPrecedesBy is `lexicographicallyPrecedes(_:by:)`.
func SliceLexicographicallyPrecedesBy[T any](a, b []T, less func(T, T) bool) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if less(a[i], b[i]) {
			return true
		}
		if less(b[i], a[i]) {
			return false
		}
	}
	return len(a) < len(b)
}

// SlicePopFirst is `popFirst()` on an ArraySlice.
func SlicePopFirst[T any](xs *[]T) *T {
	if len(*xs) == 0 {
		return nil
	}
	v := (*xs)[0]
	*xs = (*xs)[1:]
	return &v
}

// SliceIndexOffsetBy is Array `index(_:offsetBy:)`.
func SliceIndexOffsetBy[T any](_ []T, i, n int) int { return Add(i, n) }

// SliceFormIndexBefore is Array `formIndex(before:)`.
func SliceFormIndexBefore[T any](_ []T, i *int) { *i = Sub(*i, 1) }

// SliceInitOrderedSet is `Array(orderedSet)`; it also accepts `OrderedDictionary.keys`, which the runtime keeps as a slice.
func SliceInitOrderedSet[S ~[]T, T any](s any) S { return S(slices.Clone(Iterate[T](s))) }

// SliceInitReversedCollection is `Array(xs.reversed())`.
func SliceInitReversedCollection[S ~[]T, T any](xs []T) S { return S(slices.Clone(xs)) }

// SliceInitRange is `Array(lo..<hi)`.
func SliceInitRange[S ~[]int](r Range[int]) S { return S(rangeInts(r.Lo, r.Hi)) }

// SliceInitAny is `Array(sequence)`.
func SliceInitAny[S ~[]T, T any](xs []T) S { return S(slices.Clone(xs)) }

// SliceInitLazyMapSequence is `Array(xs.lazy.map(...))`.
func SliceInitLazyMapSequence[S ~[]T, T any](xs []T) S { return S(slices.Clone(xs)) }

// SliceInitLazyFilterSequence is `Array(xs.lazy.filter(...))`.
func SliceInitLazyFilterSequence[S ~[]T, T any](xs []T) S { return S(slices.Clone(xs)) }

// SetInitRange is `Set(lo..<hi)`.
func SetInitRange[S ~map[int]struct{}](r Range[int]) S { return SetInitSlice[S](rangeInts(r.Lo, r.Hi)) }

// SetInitValues is `Set(dict.values)`.
func SetInitValues[S ~map[T]struct{}, T comparable](xs []T) S { return SetInitSlice[S](xs) }

// SetInitSubstring is `Set(substring)`: its Characters.
func SetInitSubstring[S ~map[Character]struct{}](s Substring) S {
	return SetInitSlice[S](StringChars(s.String()))
}

// SetInitLazySequence is `Set(xs.lazy...)`.
func SetInitLazySequence[S ~map[T]struct{}, T comparable](xs []T) S { return SetInitSlice[S](xs) }

// SetInitLazyMapSequence is `Set(xs.lazy.map(...))`.
func SetInitLazyMapSequence[S ~map[T]struct{}, T comparable](xs []T) S { return SetInitSlice[S](xs) }

// SetSubtract is `Set.subtract(_:)` with a sequence argument.
func SetSubtract[T comparable](s *map[T]struct{}, xs []T) {
	for _, x := range xs {
		delete(*s, x)
	}
}

// SetRemoveAllKeepingCapacity is `Set.removeAll(keepingCapacity:)`.
func SetRemoveAllKeepingCapacity[T comparable](s *map[T]struct{}, _ bool) { clear(*s) }

// SetFirst is `Set.first`.
func SetFirst[T comparable](s map[T]struct{}) *T { return SliceFirst(SetElements(s)) }

// SetAllSatisfy is `Set.allSatisfy`.
func SetAllSatisfy[T comparable](s map[T]struct{}, f func(T) bool) bool {
	return SliceAllSatisfy(SetElements(s), f)
}

// SetFlatMap is `Set.flatMap`.
func SetFlatMap[T comparable, U any](s map[T]struct{}, f func(T) []U) []U {
	return SliceFlatMap(SetElements(s), f)
}

// SetReduceInto is `Set.reduce(into:_:)`; f receives the inout accumulator as a pointer.
func SetReduceInto[T comparable, A any](s map[T]struct{}, init A, f func(*A, T)) A {
	return SliceReduceInto(SetElements(s), init, f)
}

// SetSortedBy is `Set.sorted(by:)`.
func SetSortedBy[T comparable](s map[T]struct{}, less func(T, T) bool) []T {
	return SliceSortedBy(SetElements(s), less)
}

// MapReduceInto is `dict.reduce(into:_:)`; f receives the inout accumulator as a pointer.
func MapReduceInto[K comparable, V, A any](m map[K]V, init A, f func(*A, struct {
	F0 K
	F1 V
})) A {
	return SliceReduceInto(MapOrdered(m), init, f)
}

// MapReduce is `dict.reduce(_:_:)`.
func MapReduce[K comparable, V, A any](m map[K]V, init A, f func(A, struct {
	F0 K
	F1 V
}) A) A {
	return SliceReduce(MapOrdered(m), init, f)
}

// MapAllSatisfy is `dict.allSatisfy`.
func MapAllSatisfy[K comparable, V any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) bool) bool {
	return SliceAllSatisfy(MapOrdered(m), f)
}

// MapFlatMap is `dict.flatMap`.
func MapFlatMap[K comparable, V, U any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) []U) []U {
	return SliceFlatMap(MapOrdered(m), f)
}

// MapMinBy is `dict.min(by:)`.
func MapMinBy[K comparable, V any](m map[K]V, less func(struct {
	F0 K
	F1 V
}, struct {
	F0 K
	F1 V
}) bool) *struct {
	F0 K
	F1 V
} {
	return SliceMinBy(MapOrdered(m), less)
}

// MapFirst is `dict.first`.
func MapFirst[K comparable, V any](m map[K]V) *struct {
	F0 K
	F1 V
} {
	return SliceFirst(MapOrdered(m))
}

// MapCompactMapValues is `dict.compactMapValues`.
func MapCompactMapValues[K comparable, V, U any](m map[K]V, f func(V) *U) map[K]U {
	out := make(map[K]U, len(m))
	for _, kv := range MapOrdered(m) {
		if v := f(kv.F1); v != nil {
			out[kv.F0] = *v
		}
	}
	return out
}

// OrderedDictionaryCompactMap is `OrderedDictionary.compactMap`.
func OrderedDictionaryCompactMap[K comparable, V, U any](d OrderedDictionary[K, V], f func(struct {
	F0 K
	F1 V
}) *U) []U {
	return SliceCompactMap(OrderedDictionaryElements(d), f)
}

// OrderedDictionarySortedBy is `OrderedDictionary.sorted(by:)`.
func OrderedDictionarySortedBy[K comparable, V any](d OrderedDictionary[K, V], less func(struct {
	F0 K
	F1 V
}, struct {
	F0 K
	F1 V
}) bool) []struct {
	F0 K
	F1 V
} {
	return SliceSortedBy(OrderedDictionaryElements(d), less)
}

// KeysContainsWhere is `dict.keys.contains(where:)`.
func KeysContainsWhere[K any](ks []K, f func(K) bool) bool { return SliceContainsWhere(ks, f) }

// KeysCompactMap is `dict.keys.compactMap`.
func KeysCompactMap[K, U any](ks []K, f func(K) *U) []U { return SliceCompactMap(ks, f) }

// KeysFirstWhere is `dict.keys.first(where:)`.
func KeysFirstWhere[K any](ks []K, f func(K) bool) *K { return SliceFirstWhere(ks, f) }

// KeysAllSatisfy is `dict.keys.allSatisfy`.
func KeysAllSatisfy[K any](ks []K, f func(K) bool) bool { return SliceAllSatisfy(ks, f) }

// KeysMap is `dict.keys.map`.
func KeysMap[K, U any](ks []K, f func(K) U) []U { return SliceMap(ks, f) }

// KeysMax is `dict.keys.max()`.
func KeysMax[K Ordered](ks []K) *K { return SliceMaxBy(ks, func(a, b K) bool { return a < b }) }

// KeysMin is `dict.keys.min()`.
func KeysMin[K Ordered](ks []K) *K { return SliceMinBy(ks, func(a, b K) bool { return a < b }) }

// KeysLazy is `dict.keys.lazy`.
func KeysLazy[K any](ks []K) []K { return ks }

// ValuesCompactMap is `dict.values.compactMap`.
func ValuesCompactMap[V, U any](vs []V, f func(V) *U) []U { return SliceCompactMap(vs, f) }

// ValuesReduceInto is `dict.values.reduce(into:_:)`; f receives the inout accumulator as a pointer.
func ValuesReduceInto[V, A any](vs []V, init A, f func(*A, V)) A { return SliceReduceInto(vs, init, f) }

// ValuesFirstWhere is `dict.values.first(where:)`.
func ValuesFirstWhere[V any](vs []V, f func(V) bool) *V { return SliceFirstWhere(vs, f) }

// ValuesAllSatisfy is `dict.values.allSatisfy`.
func ValuesAllSatisfy[V any](vs []V, f func(V) bool) bool { return SliceAllSatisfy(vs, f) }

// ValuesCountWhere is `dict.values.count(where:)`.
func ValuesCountWhere[V any](vs []V, f func(V) bool) int { return SliceCountWhere(vs, f) }

// ValuesMax is `dict.values.max()`.
func ValuesMax[V Ordered](vs []V) *V { return SliceMaxBy(vs, func(a, b V) bool { return a < b }) }

// ValuesSorted is `dict.values.sorted()`.
func ValuesSorted[V Ordered](vs []V) []V {
	return SliceSortedBy(vs, func(a, b V) bool { return a < b })
}

// ValuesJoined is `dict.values.joined()` over array values.
func ValuesJoined[V any](vs [][]V) []V { return slices.Concat(vs...) }

// FlattenSequenceMap is `xs.joined().map`.
func FlattenSequenceMap[T, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// EnumeratedFlatMap is `xs.enumerated().flatMap`.
func EnumeratedFlatMap[T, U any](xs []struct {
	F0 int
	F1 T
}, f func(struct {
	F0 int
	F1 T
}) []U) []U {
	return SliceFlatMap(xs, f)
}

// EnumeratedCompactMap is `xs.enumerated().compactMap`.
func EnumeratedCompactMap[T, U any](xs []struct {
	F0 int
	F1 T
}, f func(struct {
	F0 int
	F1 T
}) *U) []U {
	return SliceCompactMap(xs, f)
}

// EnumeratedFilter is `xs.enumerated().filter`.
func EnumeratedFilter[T any](xs []struct {
	F0 int
	F1 T
}, f func(struct {
	F0 int
	F1 T
}) bool) []struct {
	F0 int
	F1 T
} {
	return SliceFilter(xs, f)
}

// EnumeratedSortedBy is `xs.enumerated().sorted(by:)`.
func EnumeratedSortedBy[T any](xs []struct {
	F0 int
	F1 T
}, less func(struct {
	F0 int
	F1 T
}, struct {
	F0 int
	F1 T
}) bool) []struct {
	F0 int
	F1 T
} {
	return SliceSortedBy(xs, less)
}

// EnumeratedAllSatisfy is `xs.enumerated().allSatisfy`.
func EnumeratedAllSatisfy[T any](xs []struct {
	F0 int
	F1 T
}, f func(struct {
	F0 int
	F1 T
}) bool) bool {
	return SliceAllSatisfy(xs, f)
}

// Zip2SequenceCompactMap is `zip(a, b).compactMap`.
func Zip2SequenceCompactMap[A, B, U any](xs []struct {
	F0 A
	F1 B
}, f func(struct {
	F0 A
	F1 B
}) *U) []U {
	return SliceCompactMap(xs, f)
}

// Zip2SequenceContainsWhere is `zip(a, b).contains(where:)`.
func Zip2SequenceContainsWhere[A, B any](xs []struct {
	F0 A
	F1 B
}, f func(struct {
	F0 A
	F1 B
}) bool) bool {
	return SliceContainsWhere(xs, f)
}

// Zip2SequenceAllSatisfy is `zip(a, b).allSatisfy`.
func Zip2SequenceAllSatisfy[A, B any](xs []struct {
	F0 A
	F1 B
}, f func(struct {
	F0 A
	F1 B
}) bool) bool {
	return SliceAllSatisfy(xs, f)
}

// LazyMapSequenceFilter is `lazy.map(...).filter`.
func LazyMapSequenceFilter[T any](xs []T, f func(T) bool) []T { return SliceFilter(xs, f) }

// LazyMapSequenceCompactMap is `lazy.map(...).compactMap`.
func LazyMapSequenceCompactMap[T, U any](xs []T, f func(T) *U) []U { return SliceCompactMap(xs, f) }

// LazyMapSequenceMinBy is `lazy.map(...).min(by:)`.
func LazyMapSequenceMinBy[T any](xs []T, less func(T, T) bool) *T { return SliceMinBy(xs, less) }

// LazyMapSequencePrefixInt is `lazy.map(...).prefix(n)`.
func LazyMapSequencePrefixInt[T any](xs []T, n int) []T { return SlicePrefixInt(xs, n) }

// LazyFilterSequenceCompactMap is `lazy.filter(...).compactMap`.
func LazyFilterSequenceCompactMap[T, U any](xs []T, f func(T) *U) []U { return SliceCompactMap(xs, f) }

// LazyFilterSequenceMap is `lazy.filter(...).map`.
func LazyFilterSequenceMap[T, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// LazyFilterSequencePrefixInt is `lazy.filter(...).prefix(n)`.
func LazyFilterSequencePrefixInt[T any](xs []T, n int) []T { return SlicePrefixInt(xs, n) }

// LazyFilterSequenceFlatMap is `lazy.filter(...).flatMap` whose closure returns a Set.
func LazyFilterSequenceFlatMap[T any, U comparable](xs []T, f func(T) map[U]struct{}) []U {
	return SliceFlatMap(xs, func(x T) []U { return SetElements(f(x)) })
}

// LazySequenceFlatMap is `lazy.flatMap`.
func LazySequenceFlatMap[T, U any](xs []T, f func(T) []U) []U { return SliceFlatMap(xs, f) }

// ReversedCollectionFlatMap is `xs.reversed().flatMap`.
func ReversedCollectionFlatMap[T, U any](xs []T, f func(T) []U) []U { return SliceFlatMap(xs, f) }

// ReversedCollectionIsEmpty is `reversed().isEmpty` on a value the emitter typed as `any`.
func ReversedCollectionIsEmpty(xs any) bool { return reflect.ValueOf(xs).Len() == 0 }

// PrefixSequenceMap is `prefix(n).map`.
func PrefixSequenceMap[T, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// PrefixSequenceReduce is `prefix(n).reduce(_:_:)`.
func PrefixSequenceReduce[T, A any](xs []T, init A, f func(A, T) A) A {
	return SliceReduce(xs, init, f)
}

// SubstringReversed is `Substring.reversed()`.
func SubstringReversed(s Substring) []Character { return SliceReversed(StringChars(s.String())) }

// StringFirstWhere is `String.first(where:)`.
func StringFirstWhere(s string, f func(Character) bool) *Character {
	return SliceFirstWhere(StringChars(s), f)
}

// UTF8ViewFirst is `utf8.first`.
func UTF8ViewFirst(b []byte) *byte { return SliceFirst(b) }

// UTF8ViewAllSatisfy is `utf8.allSatisfy`.
func UTF8ViewAllSatisfy(b []byte, f func(byte) bool) bool { return SliceAllSatisfy(b, f) }

// SubstringUtf8 is `Substring.utf8`.
func SubstringUtf8(s Substring) []byte { return []byte(s.String()) }

// CollectionCount is `Collection.count` reached through a key path.
func CollectionCount(c any) int {
	switch v := c.(type) {
	case Substring:
		return SubstringCount(v)
	case string:
		return StringCount(v)
	}
	return reflect.ValueOf(c).Len()
}

// AnyIsEmpty is `Collection.isEmpty` on a value the emitter could not type more precisely.
func AnyIsEmpty[T any](c T) bool {
	switch v := any(c).(type) {
	case Substring:
		return v.Lo == v.Hi
	case string:
		return v == ""
	}
	return reflect.ValueOf(c).Len() == 0
}

type stringIterator struct {
	chars []Character
	i     int
}

// StringMakeIterator is `String.makeIterator()`.
func StringMakeIterator(s string) any { return &stringIterator{chars: StringChars(s)} }

// IteratorNext is `String.Iterator.next()`.
func IteratorNext(it *any) *Character {
	s := (*it).(*stringIterator)
	if s.i >= len(s.chars) {
		return nil
	}
	c := s.chars[s.i]
	s.i++
	return &c
}

// Swap is Swift `swap(_:_:)`.
func Swap[T any](a, b *T) { *a, *b = *b, *a }

// BoolToggle is `Bool.toggle()`.
func BoolToggle(b *bool) { *b = !*b }

// ObjectIdentifierInit is `ObjectIdentifier(metatype)`.
func ObjectIdentifierInit(t Type) any { return t.Reflect() }

// AssertionFailure is Swift `assertionFailure`, which does nothing in optimized (-O) builds.
func AssertionFailure(msg ...string) {}

// IndexingIterator is Swift `IndexingIterator` over an Array.
type IndexingIterator[T any] struct {
	Elements []T
	Position int
}

// SliceMakeIterator is `Array.makeIterator()`; the iterator holds its own copy of the array.
func SliceMakeIterator[T any](xs []T) IndexingIterator[T] {
	return IndexingIterator[T]{Elements: slices.Clone(xs)}
}

// IndexingIteratorNext is `IndexingIterator.next()`.
func IndexingIteratorNext[T any](it *IndexingIterator[T]) *T {
	if it.Position >= len(it.Elements) {
		return nil
	}
	v := it.Elements[it.Position]
	it.Position++
	return &v
}

var hasherSeed = maphash.MakeSeed()

// Hasher is Swift `Hasher`, seeded once per process.
type Hasher struct{ h maphash.Hash }

func hasherIn(h *any) *Hasher {
	if hh, ok := (*h).(*Hasher); ok {
		return hh
	}
	hh := &Hasher{}
	hh.h.SetSeed(hasherSeed)
	*h = hh
	return hh
}

// HasherCombine is `Hasher.combine(_:)`: a value with a `Hash(*any)` method feeds itself, as Swift's
// `hash(into:)` does; any other value is hashed by Go equality.
func HasherCombine[T comparable](h *any, v T) {
	if hv, ok := any(v).(interface{ Hash(*any) }); ok {
		hv.Hash(h)
		return
	}
	maphash.WriteComparable(&hasherIn(h).h, v)
}

// HasherFinalize is `Hasher.finalize()`.
func HasherFinalize(h any) int { return int(hasherIn(&h).h.Sum64()) }
