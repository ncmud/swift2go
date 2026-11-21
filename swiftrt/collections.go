package swiftrt

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
)

// SliceCount is Array.count.
func SliceCount[T any](xs []T) int { return len(xs) }

// SliceIsEmpty is Array.isEmpty.
func SliceIsEmpty[T any](xs []T) bool { return len(xs) == 0 }

// SliceFirst is Array.first.
func SliceFirst[T any](xs []T) *T {
	if len(xs) == 0 {
		return nil
	}
	v := xs[0]
	return &v
}

// SliceLast is Array.last.
func SliceLast[T any](xs []T) *T {
	if len(xs) == 0 {
		return nil
	}
	v := xs[len(xs)-1]
	return &v
}

// SliceIndices is Array.indices.
func SliceIndices[T any](xs []T) Range[int] { return Range[int]{0, len(xs)} }

// SliceMap is Sequence.map(_:).
func SliceMap[T, U any](xs []T, f func(T) U) []U {
	out := make([]U, len(xs))
	for i, x := range xs {
		out[i] = f(x)
	}
	return out
}

// SliceFilter is Sequence.filter(_:).
func SliceFilter[T any](xs []T, f func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	if out == nil {
		out = []T{}
	}
	return out
}

// SliceCompactMap is Sequence.compactMap(_:).
func SliceCompactMap[T, U any](xs []T, f func(T) *U) []U {
	out := []U{}
	for _, x := range xs {
		if v := f(x); v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// SliceReduce is Sequence.reduce(_:_:).
func SliceReduce[T, A any](xs []T, init A, f func(A, T) A) A {
	acc := init
	for _, x := range xs {
		acc = f(acc, x)
	}
	return acc
}

// SliceAllSatisfy is Sequence.allSatisfy(_:).
func SliceAllSatisfy[T any](xs []T, f func(T) bool) bool {
	for _, x := range xs {
		if !f(x) {
			return false
		}
	}
	return true
}

// SliceContains is Sequence.contains(_:).
func SliceContains[T comparable](xs []T, v T) bool { return slices.Contains(xs, v) }

// SliceContainsWhere is Sequence.contains(where:).
func SliceContainsWhere[T any](xs []T, f func(T) bool) bool { return slices.ContainsFunc(xs, f) }

// SliceMax is Sequence.max().
func SliceMax[T cmp.Ordered](xs []T) *T {
	if len(xs) == 0 {
		return nil
	}
	v := slices.Max(xs)
	return &v
}

// SliceMin is Sequence.min().
func SliceMin[T cmp.Ordered](xs []T) *T {
	if len(xs) == 0 {
		return nil
	}
	v := slices.Min(xs)
	return &v
}

// SliceJoinedSeparator is [String].joined(separator:).
func SliceJoinedSeparator[T ~string](xs []T, sep string) string {
	ss := make([]string, len(xs))
	for i, x := range xs {
		ss[i] = string(x)
	}
	return strings.Join(ss, sep)
}

// SliceJoined is [String].joined().
func SliceJoined[T ~string](xs []T) string { return SliceJoinedSeparator(xs, "") }

// SliceDropLast is Array.dropLast().
func SliceDropLast[T any](xs []T) []T { return SliceDropLastInt(xs, 1) }

// SliceDropFirst is Array.dropFirst().
func SliceDropFirst[T any](xs []T) []T { return SliceDropFirstInt(xs, 1) }

// SliceDropLastInt is Array.dropLast(_:).
func SliceDropLastInt[T any](xs []T, n int) []T {
	if n >= len(xs) {
		return []T{}
	}
	return Clone(xs[:len(xs)-n])
}

// SliceDropFirstInt is Array.dropFirst(_:).
func SliceDropFirstInt[T any](xs []T, n int) []T {
	if n >= len(xs) {
		return []T{}
	}
	return Clone(xs[n:])
}

// SliceSubscriptRange is Array[a..<b].
func SliceSubscriptRange[T any](xs []T, r Range[int]) []T {
	if r.Lo < 0 || r.Hi > len(xs) || r.Lo > r.Hi {
		Trap("Array index is out of range")
	}
	return xs[r.Lo:r.Hi:r.Hi]
}

// SliceSubscriptClosedRange is Array[a...b].
func SliceSubscriptClosedRange[T any](xs []T, r ClosedRange[int]) []T {
	return SliceSubscriptRange(xs, Range[int]{r.Lo, r.Hi + 1})
}

// SliceSubscriptPartialRangeFrom is Array[a...].
func SliceSubscriptPartialRangeFrom[T any](xs []T, r PartialRangeFrom[int]) []T {
	return SliceSubscriptRange(xs, Range[int]{r.Lo, len(xs)})
}

// SliceInitSlice is Array(_:) from another sequence.
func SliceInitSlice[S ~[]T, T any](xs []T) S { return S(Clone(xs)) }

// SliceInitString is Array(_: String): its Characters.
func SliceInitString[S ~[]Character](s string) S { return S(StringChars(s)) }

// SliceInitRepeatingCount is Array(repeating:count:).
func SliceInitRepeatingCount[S ~[]T, T any](v T, n int) S {
	out := make(S, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// SliceInit is Array().
func SliceInit[S ~[]T, T any]() S { return S{} }

// SliceRemoveLast is Array.removeLast().
func SliceRemoveLast[T any](xs *[]T) T {
	if len(*xs) == 0 {
		Trap("Can't remove last element from an empty collection")
	}
	v := (*xs)[len(*xs)-1]
	*xs = (*xs)[:len(*xs)-1]
	return v
}

// SliceRemoveFirst is Array.removeFirst().
func SliceRemoveFirst[T any](xs *[]T) T {
	if len(*xs) == 0 {
		Trap("Can't remove first element from an empty collection")
	}
	v := (*xs)[0]
	*xs = (*xs)[1:]
	return v
}

// SliceRemoveAt is Array.remove(at:).
func SliceRemoveAt[T any](xs *[]T, i int) T {
	v := (*xs)[i]
	*xs = slices.Delete(*xs, i, i+1)
	return v
}

// SliceInsertAt is Array.insert(_:at:).
func SliceInsertAt[T any](xs *[]T, v T, i int) { *xs = slices.Insert(*xs, i, v) }

// SliceRemoveAll is Array.removeAll().
func SliceRemoveAll[T any](xs *[]T) { *xs = []T{} }

// SliceReserveCapacity is Array.reserveCapacity(_:); a no-op.
func SliceReserveCapacity[T any](xs *[]T, n int) {}

// SliceAppend is Array.append(_:).
func SliceAppend[T any](xs *[]T, v T) { *xs = append(*xs, v) }

// SliceAppendContentsOf is Array.append(contentsOf:).
func SliceAppendContentsOf[T any](xs *[]T, vs []T) { *xs = append(*xs, vs...) }

// SliceReversed is Sequence.reversed().
func SliceReversed[T any](xs []T) []T {
	out := Clone(xs)
	slices.Reverse(out)
	return out
}

// SliceSorted is Sequence.sorted().
func SliceSorted[T cmp.Ordered](xs []T) []T {
	out := Clone(xs)
	slices.Sort(out)
	return out
}

// SliceSortedBy is Sequence.sorted(by:).
func SliceSortedBy[T any](xs []T, less func(T, T) bool) []T {
	out := Clone(xs)
	slices.SortStableFunc(out, func(a, b T) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
	return out
}

// SliceEnumerated is Sequence.enumerated().
func SliceEnumerated[T any](xs []T) []struct {
	F0 int
	F1 T
} {
	out := make([]struct {
		F0 int
		F1 T
	}, len(xs))
	for i, x := range xs {
		out[i].F0, out[i].F1 = i, x
	}
	return out
}

// IntQuotientAndRemainderDividingBy is BinaryInteger.quotientAndRemainder(dividingBy:).
func IntQuotientAndRemainderDividingBy[T Integer](a, b T) struct{ F0, F1 T } {
	return struct{ F0, F1 T }{a / b, a % b}
}

// MapGet is Dictionary[key].
func MapGet[K comparable, V any](m map[K]V, k K) *V {
	v, ok := m[k]
	if !ok {
		return nil
	}
	return &v
}

// MapSet is Dictionary[key] = optional.
func MapSet[K comparable, V any](m map[K]V, k K, v *V) {
	if v == nil {
		delete(m, k)
		return
	}
	m[k] = *v
}

// MapCount is Dictionary.count.
func MapCount[K comparable, V any](m map[K]V) int { return len(m) }

// MapIsEmpty is Dictionary.isEmpty.
func MapIsEmpty[K comparable, V any](m map[K]V) bool { return len(m) == 0 }

// MapOrdered iterates a map in a deterministic key order (Swift's order is unspecified).
func MapOrdered[K comparable, V any](m map[K]V) []struct {
	F0 K
	F1 V
} {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	SortKeys(keys)
	out := make([]struct {
		F0 K
		F1 V
	}, len(keys))
	for i, k := range keys {
		out[i].F0, out[i].F1 = k, m[k]
	}
	return out
}

// SortKeys orders map keys deterministically: numerically or lexically when the key is ordered, else by description.
func SortKeys[K comparable](keys []K) {
	slices.SortFunc(keys, func(a, b K) int { return compareAny(a, b) })
}

func compareAny(a, b any) int {
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	switch ra.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return cmp.Compare(ra.Int(), rb.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return cmp.Compare(ra.Uint(), rb.Uint())
	case reflect.Float32, reflect.Float64:
		return cmp.Compare(ra.Float(), rb.Float())
	case reflect.String:
		return cmp.Compare(ra.String(), rb.String())
	}
	return cmp.Compare(Str(a), Str(b))
}
