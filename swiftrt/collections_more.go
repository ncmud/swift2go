package swiftrt

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
)

// SliceRemoveLastInt is Swift `Array.removeLast(n)`.
func SliceRemoveLastInt[T any](xs *[]T, n int) {
	if n < 0 || n > len(*xs) {
		Trap("Can't remove more items from a collection than it contains")
	}
	*xs = (*xs)[:len(*xs)-n]
}

// SliceRemoveFirstInt is Swift `Array.removeFirst(n)`.
func SliceRemoveFirstInt[T any](xs *[]T, n int) {
	if n < 0 || n > len(*xs) {
		Trap("Can't remove more items from a collection than it contains")
	}
	*xs = append([]T{}, (*xs)[n:]...)
}

// SlicePopLast is Swift `Array.popLast()`.
func SlicePopLast[T any](xs *[]T) *T {
	if len(*xs) == 0 {
		return nil
	}
	v := (*xs)[len(*xs)-1]
	*xs = (*xs)[:len(*xs)-1]
	return &v
}

// SliceRemoveAllWhere is Swift `Array.removeAll(where:)`.
func SliceRemoveAllWhere[T any](xs *[]T, f func(T) bool) { *xs = slices.DeleteFunc(*xs, f) }

// SliceRemoveAllKeepingCapacity is Swift `Array.removeAll(keepingCapacity:)`.
func SliceRemoveAllKeepingCapacity[T any](xs *[]T, _ bool) { *xs = (*xs)[:0] }

// SliceReserveCapacityInt is Swift `Array.reserveCapacity(n)`.
func SliceReserveCapacityInt[T any](xs *[]T, n int) { *xs = slices.Grow(*xs, n-len(*xs)) }

// SliceFirstWhere is Swift `first(where:)`.
func SliceFirstWhere[T any](xs []T, f func(T) bool) *T {
	for i := range xs {
		if f(xs[i]) {
			v := xs[i]
			return &v
		}
	}
	return nil
}

// SliceLastWhere is Swift `last(where:)`.
func SliceLastWhere[T any](xs []T, f func(T) bool) *T {
	for _, v := range slices.Backward(xs) {
		if f(v) {
			return &v
		}
	}
	return nil
}

// SliceFirstIndexWhere is Swift `firstIndex(where:)`.
func SliceFirstIndexWhere[T any](xs []T, f func(T) bool) *int {
	if i := slices.IndexFunc(xs, f); i >= 0 {
		return &i
	}
	return nil
}

// SliceLastIndexWhere is Swift `lastIndex(where:)`.
func SliceLastIndexWhere[T any](xs []T, f func(T) bool) *int {
	for i := len(xs) - 1; i >= 0; i-- {
		if f(xs[i]) {
			return &i
		}
	}
	return nil
}

// SliceFirstIndexOf is Swift `firstIndex(of:)`.
func SliceFirstIndexOf[T any](xs []T, v T) *int {
	return SliceFirstIndexWhere(xs, func(x T) bool { return Equal(x, v) })
}

// SliceLastIndexOf is Swift `lastIndex(of:)`.
func SliceLastIndexOf[T any](xs []T, v T) *int {
	return SliceLastIndexWhere(xs, func(x T) bool { return Equal(x, v) })
}

// SliceStartIndex is Swift `Array.startIndex`.
func SliceStartIndex[T any](xs []T) int { return 0 }

// SliceEndIndex is Swift `Array.endIndex`.
func SliceEndIndex[T any](xs []T) int { return len(xs) }

// SliceIndexAfter is Swift `Array.index(after:)`.
func SliceIndexAfter[T any](xs []T, i int) int { return i + 1 }

// SliceIndexBefore is Swift `Array.index(before:)`.
func SliceIndexBefore[T any](xs []T, i int) int { return i - 1 }

// SliceSubscriptPartialRangeUpTo is Swift `xs[..<n]`.
func SliceSubscriptPartialRangeUpTo[T any](xs []T, r PartialRangeUpTo[int]) []T {
	return SliceSubscriptRange(xs, Range[int]{0, r.Hi})
}

// SliceSubscriptPartialRangeThrough is Swift `xs[...n]`.
func SliceSubscriptPartialRangeThrough[T any](xs []T, r PartialRangeThrough[int]) []T {
	return SliceSubscriptRange(xs, Range[int]{0, r.Hi + 1})
}

// SliceSubscriptAll is Swift `xs[...]`.
func SliceSubscriptAll[T any](xs []T) []T { return xs }

// SliceSplitSeparatorOmittingEmptySubsequences is Swift `split(separator:omittingEmptySubsequences:)` on an array.
func SliceSplitSeparatorOmittingEmptySubsequences[T comparable](xs []T, sep T, omit bool) [][]T {
	var out [][]T
	start := 0
	for i, x := range xs {
		if x == sep {
			if !(omit && start == i) {
				out = append(out, xs[start:i])
			}
			start = i + 1
		}
	}
	if !(omit && start == len(xs)) {
		out = append(out, xs[start:])
	}
	if out == nil {
		out = [][]T{}
	}
	return out
}

// SliceSplitSeparator is Swift `split(separator:)` on an array.
func SliceSplitSeparator[T comparable](xs []T, sep T) [][]T {
	return SliceSplitSeparatorOmittingEmptySubsequences(xs, sep, true)
}

// SliceStartsWith is Swift `starts(with:)`.
func SliceStartsWith[T comparable](xs, p []T) bool {
	return len(xs) >= len(p) && slices.Equal(xs[:len(p)], p)
}

// SliceDropWhile is Swift `drop(while:)`.
func SliceDropWhile[T any](xs []T, f func(T) bool) []T {
	i := 0
	for i < len(xs) && f(xs[i]) {
		i++
	}
	return xs[i:]
}

// SlicePrefixWhile is Swift `prefix(while:)`.
func SlicePrefixWhile[T any](xs []T, f func(T) bool) []T {
	i := 0
	for i < len(xs) && f(xs[i]) {
		i++
	}
	return xs[:i]
}

// SlicePrefixInt is Swift `prefix(n)`.
func SlicePrefixInt[T any](xs []T, n int) []T {
	if n < 0 {
		Trap("Can't take a prefix of negative length from a collection")
	}
	return xs[:min(n, len(xs))]
}

// SliceSuffixInt is Swift `suffix(n)`.
func SliceSuffixInt[T any](xs []T, n int) []T {
	if n < 0 {
		Trap("Can't take a suffix of negative length from a collection")
	}
	return xs[len(xs)-min(n, len(xs)):]
}

// ReversedCollectionMap is Swift `reversed().map`.
func ReversedCollectionMap[T, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// ReversedCollectionPrefixWhile is Swift `reversed().prefix(while:)`; the slice is already reversed.
func ReversedCollectionPrefixWhile[T any](xs []T, f func(T) bool) []T { return SlicePrefixWhile(xs, f) }

// MapReserveCapacityInt is Swift `Dictionary.reserveCapacity(_:)`. Go maps can't grow in place, so only an empty map is
// remade with room for n entries.
func MapReserveCapacityInt[M ~map[K]V, K comparable, V any](m *M, n int) {
	if len(*m) == 0 && n > 0 {
		*m = make(M, n)
	}
}

// SliceForEach is Swift `forEach`.
func SliceForEach[T any](xs []T, f func(T)) {
	for _, x := range xs {
		f(x)
	}
}

// SliceFlatMap is Swift `flatMap` returning an array.
func SliceFlatMap[T, U any](xs []T, f func(T) []U) []U {
	out := []U{}
	for _, x := range xs {
		out = append(out, f(x)...)
	}
	return out
}

// SliceSortedBy0 is Swift `sorted(by:)` with a strict-weak-order predicate.
func SliceSortBy[T any](xs *[]T, less func(T, T) bool) {
	slices.SortStableFunc(*xs, func(a, b T) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
}

// SliceSort is Swift `sort()`.
func SliceSort[T cmp.Ordered](xs *[]T) { slices.Sort(*xs) }

// SliceSwapAt is Swift `swapAt`.
func SliceSwapAt[T any](xs *[]T, i, j int) { (*xs)[i], (*xs)[j] = (*xs)[j], (*xs)[i] }

// SliceReverse is Swift `reverse()`.
func SliceReverse[T any](xs *[]T) { slices.Reverse(*xs) }

// SliceMinBy is Swift `min(by:)`.
func SliceMinBy[T any](xs []T, less func(T, T) bool) *T {
	if len(xs) == 0 {
		return nil
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if less(x, m) {
			m = x
		}
	}
	return &m
}

// SliceMaxBy is Swift `max(by:)`.
func SliceMaxBy[T any](xs []T, less func(T, T) bool) *T {
	if len(xs) == 0 {
		return nil
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if less(m, x) {
			m = x
		}
	}
	return &m
}

// SliceCountWhere is Swift `count(where:)`.
func SliceCountWhere[T any](xs []T, f func(T) bool) int {
	n := 0
	for _, x := range xs {
		if f(x) {
			n++
		}
	}
	return n
}

// SliceZip is Swift `zip(a, b)`.
func Zip[A, B any](a []A, b []B) []struct {
	F0 A
	F1 B
} {
	n := min(len(a), len(b))
	out := make([]struct {
		F0 A
		F1 B
	}, n)
	for i := range n {
		out[i].F0, out[i].F1 = a[i], b[i]
	}
	return out
}

// SetInit is Swift `Set()`.
func SetInit[S ~map[T]struct{}, T comparable]() S { return S{} }

// SetInitSlice is Swift `Set(array)`.
func SetInitSlice[S ~map[T]struct{}, T comparable](xs []T) S {
	s := S{}
	for _, x := range xs {
		s[x] = struct{}{}
	}
	return s
}

// SetInsert is Swift `Set.insert`, returning whether the element was new.
func SetInsert[T comparable](s *map[T]struct{}, v T) struct {
	F0 bool
	F1 T
} {
	if *s == nil {
		*s = map[T]struct{}{}
	}
	_, had := (*s)[v]
	(*s)[v] = struct{}{}
	return struct {
		F0 bool
		F1 T
	}{!had, v}
}

// SetRemove is Swift `Set.remove`.
func SetRemove[T comparable](s *map[T]struct{}, v T) *T {
	if _, ok := (*s)[v]; !ok {
		return nil
	}
	delete(*s, v)
	return &v
}

// SetContains is Swift `Set.contains`.
func SetContains[T comparable](s map[T]struct{}, v T) bool { _, ok := s[v]; return ok }

// SetIsEmpty is Swift `Set.isEmpty`.
func SetIsEmpty[T comparable](s map[T]struct{}) bool { return len(s) == 0 }

// SetCount is Swift `Set.count`.
func SetCount[T comparable](s map[T]struct{}) int { return len(s) }

// SetSorted is Swift `Set.sorted()`.
func SetSorted[T comparable](s map[T]struct{}) []T {
	out := make([]T, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	SortKeys(out)
	return out
}

// SetElements lists a set's members in a stable order for iteration.
func SetElements[T comparable](s map[T]struct{}) []T {
	out := make([]T, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	SortKeys(out)
	return out
}

// SetUnion is Swift `Set.union`.
func SetUnion[T comparable](a map[T]struct{}, b []T) map[T]struct{} {
	out := make(map[T]struct{}, len(a))
	for k := range a {
		out[k] = struct{}{}
	}
	for _, k := range b {
		out[k] = struct{}{}
	}
	return out
}

// SetFormUnion is Swift `Set.formUnion`.
func SetFormUnion[T comparable](a *map[T]struct{}, b any) { *a = SetUnion(*a, Iterate[T](b)) }

// SetIntersection is Swift `Set.intersection`.
func SetIntersection[T comparable](a map[T]struct{}, b []T) map[T]struct{} {
	out := map[T]struct{}{}
	for _, k := range b {
		if _, ok := a[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}

// SetSubtracting is Swift `Set.subtracting`.
func SetSubtracting[T comparable](a map[T]struct{}, b []T) map[T]struct{} {
	out := make(map[T]struct{}, len(a))
	for k := range a {
		out[k] = struct{}{}
	}
	for _, k := range b {
		delete(out, k)
	}
	return out
}

// SetIsSubsetOf is Swift `Set.isSubset(of:)`.
func SetIsSubsetOf[T comparable](a map[T]struct{}, b any) bool {
	other, ok := b.(map[T]struct{})
	if !ok {
		other = SetInitSlice[map[T]struct{}](Iterate[T](b))
	}
	for k := range a {
		if _, ok := other[k]; !ok {
			return false
		}
	}
	return true
}

// SetIsDisjointWith is Swift `Set.isDisjoint(with:)`, taking another set or any sequence of
// elements, passed as a map[T]struct{} or a []T.
func SetIsDisjointWith[T comparable](a map[T]struct{}, b any) bool {
	switch other := b.(type) {
	case map[T]struct{}:
		for k := range a {
			if _, ok := other[k]; ok {
				return false
			}
		}
	case []T:
		for _, k := range other {
			if _, ok := a[k]; ok {
				return false
			}
		}
	default:
		panic(fmt.Sprintf("swiftrt.SetIsDisjointWith: unsupported sequence %T", b))
	}
	return true
}

// MapKeys is Swift `Dictionary.keys`.
func MapKeys[K comparable, V any](m map[K]V) []K {
	out := slices.Collect(maps.Keys(m))
	SortKeys(out)
	return out
}

// MapValues is Swift `Dictionary.values`, in key order.
func MapValues[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, k := range MapKeys(m) {
		out = append(out, m[k])
	}
	return out
}

// MapRemoveValueForKey is Swift `Dictionary.removeValue(forKey:)`.
func MapRemoveValueForKey[K comparable, V any](m *map[K]V, k K) *V {
	v, ok := (*m)[k]
	if !ok {
		return nil
	}
	delete(*m, k)
	return &v
}

// MapUpdateValueForKey is Swift `Dictionary.updateValue(_:forKey:)`.
func MapUpdateValueForKey[K comparable, V any](m *map[K]V, v V, k K) *V {
	if *m == nil {
		*m = map[K]V{}
	}
	old, ok := (*m)[k]
	(*m)[k] = v
	if !ok {
		return nil
	}
	return &old
}

// MapMergeUniquingKeysWith is Swift `Dictionary.merge(_:uniquingKeysWith:)`.
func MapMergeUniquingKeysWith[K comparable, V any](m *map[K]V, other map[K]V, f func(V, V) V) {
	if *m == nil {
		*m = map[K]V{}
	}
	for k, v := range other {
		if old, ok := (*m)[k]; ok {
			(*m)[k] = f(old, v)
		} else {
			(*m)[k] = v
		}
	}
}

// MapFilter is Swift `Dictionary.filter`, which returns a dictionary.
func MapFilter[K comparable, V any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) bool) map[K]V {
	out := map[K]V{}
	for k, v := range m {
		if f(struct {
			F0 K
			F1 V
		}{k, v}) {
			out[k] = v
		}
	}
	return out
}

// MapMapValues is Swift `Dictionary.mapValues`.
func MapMapValues[K comparable, V, U any](m map[K]V, f func(V) U) map[K]U {
	out := make(map[K]U, len(m))
	for k, v := range m {
		out[k] = f(v)
	}
	return out
}

// MapInitUniqueKeysWithValues is Swift `Dictionary(uniqueKeysWithValues:)`.
func MapInitUniqueKeysWithValues[M ~map[K]V, K comparable, V any](pairs []struct {
	F0 K
	F1 V
}) M {
	out := M{}
	for _, p := range pairs {
		if _, dup := out[p.F0]; dup {
			Trap("Dictionary literal contains duplicate keys")
		}
		out[p.F0] = p.F1
	}
	return out
}

// MapInitSliceUniquingKeysWith is Swift `Dictionary(_:uniquingKeysWith:)`.
func MapInitSliceUniquingKeysWith[M ~map[K]V, K comparable, V any](pairs []struct {
	F0 K
	F1 V
}, f func(V, V) V) M {
	out := M{}
	for _, p := range pairs {
		if old, ok := out[p.F0]; ok {
			out[p.F0] = f(old, p.F1)
		} else {
			out[p.F0] = p.F1
		}
	}
	return out
}

// MapInitGroupingBy is Swift `Dictionary(grouping:by:)`.
func MapInitGroupingBy[M ~map[K][]V, K comparable, V any](xs []V, f func(V) K) M {
	out := M{}
	for _, x := range xs {
		k := f(x)
		out[k] = append(out[k], x)
	}
	return out
}

// MapRemoveAll is Swift `Dictionary.removeAll()`.
func MapRemoveAll[K comparable, V any](m *map[K]V) { *m = map[K]V{} }

// MapSubscriptDefault is Swift `dict[k, default: d]` as an rvalue.
func MapSubscriptDefault[K comparable, V any](m map[K]V, k K, d V) V {
	if v, ok := m[k]; ok {
		return v
	}
	return d
}

// MapInit is Swift `[K: V]()`.
func MapInit[M ~map[K]V, K comparable, V any]() M { return M{} }

// OffsetIndex shifts an index found in a re-based Go slice back into its Swift parent's index space.
func OffsetIndex(i *int, lo int) *int {
	if i == nil {
		return nil
	}
	v := *i + lo
	return &v
}

// SliceFlatMapSet is Swift `Array.flatMap(_:)` whose closure returns a Set; each set's elements
// are taken in Iterate's order.
func SliceFlatMapSet[T any, U comparable](xs []T, f func(T) map[U]struct{}) []U {
	out := []U{}
	for _, x := range xs {
		out = append(out, Iterate[U](f(x))...)
	}
	return out
}
