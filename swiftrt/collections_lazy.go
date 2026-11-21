package swiftrt

import (
	"math"
	"slices"
)

// SliceLazy is Swift `.lazy`; Go evaluates eagerly.
func SliceLazy[T any](xs []T) []T { return xs }

// SetLazy is Swift `Set.lazy`.
func SetLazy[T comparable](s map[T]struct{}) []T { return SetElements(s) }

// LazySequenceCompactMap is `lazy.compactMap`.
func LazySequenceCompactMap[T, U any](xs []T, f func(T) *U) []U { return SliceCompactMap(xs, f) }

// LazySequenceFilter is `lazy.filter`.
func LazySequenceFilter[T any](xs []T, f func(T) bool) []T { return SliceFilter(xs, f) }

// LazySequenceMap is `lazy.map`.
func LazySequenceMap[T, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// LazyMapSequenceFirst is `lazy...first`.
func LazyMapSequenceFirst[T any](xs []T) *T { return SliceFirst(xs) }

// LazyMapSequenceFirstWhere is `lazy...first(where:)`.
func LazyMapSequenceFirstWhere[T any](xs []T, f func(T) bool) *T { return SliceFirstWhere(xs, f) }

// LazyFilterSequenceCount is `lazy.filter(...).count`.
func LazyFilterSequenceCount[T any](xs []T) int { return len(xs) }

// KeysSorted is `dict.keys.sorted()`.
func KeysSorted[K comparable](keys []K) []K {
	out := slices.Clone(keys)
	SortKeys(out)
	return out
}

// KeysFilter is `dict.keys.filter`.
func KeysFilter[K any](keys []K, f func(K) bool) []K { return SliceFilter(keys, f) }

// KeysSortedBy is `dict.keys.sorted(by:)`.
func KeysSortedBy[K any](keys []K, less func(K, K) bool) []K { return SliceSortedBy(keys, less) }

// ValuesContainsWhere is `dict.values.contains(where:)`.
func ValuesContainsWhere[V any](vs []V, f func(V) bool) bool { return SliceContainsWhere(vs, f) }

// ValuesFilter is `dict.values.filter`.
func ValuesFilter[V any](vs []V, f func(V) bool) []V { return SliceFilter(vs, f) }

// ValuesMap is `dict.values.map`.
func ValuesMap[V, U any](vs []V, f func(V) U) []U { return SliceMap(vs, f) }

// ValuesSortedBy is `dict.values.sorted(by:)`.
func ValuesSortedBy[V any](vs []V, less func(V, V) bool) []V { return SliceSortedBy(vs, less) }

// ValuesFlatMap is `dict.values.flatMap`.
func ValuesFlatMap[V, U any](vs []V, f func(V) []U) []U { return SliceFlatMap(vs, f) }

// ValuesReduce is `dict.values.reduce`.
func ValuesReduce[V, A any](vs []V, init A, f func(A, V) A) A { return SliceReduce(vs, init, f) }

// ValuesContains is `dict.values.contains`.
func ValuesContains[V any](vs []V, v V) bool {
	return SliceContainsWhere(vs, func(x V) bool { return Equal(x, v) })
}

// EnumeratedMap is `xs.enumerated().map`.
func EnumeratedMap[T, U any](xs []struct {
	F0 int
	F1 T
}, f func(struct {
	F0 int
	F1 T
}) U) []U {
	return SliceMap(xs, f)
}

// MapSortedBy is `dict.sorted(by:)`.
func MapSortedBy[K comparable, V any](m map[K]V, less func(struct {
	F0 K
	F1 V
}, struct {
	F0 K
	F1 V
}) bool) []struct {
	F0 K
	F1 V
} {
	return SliceSortedBy(MapOrdered(m), less)
}

// MapMap is `dict.map`.
func MapMap[K comparable, V, U any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) U) []U {
	return SliceMap(MapOrdered(m), f)
}

// MapCompactMap is `dict.compactMap`.
func MapCompactMap[K comparable, V, U any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) *U) []U {
	return SliceCompactMap(MapOrdered(m), f)
}

// MapFirstWhere is `dict.first(where:)`.
func MapFirstWhere[K comparable, V any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) bool) *struct {
	F0 K
	F1 V
} {
	return SliceFirstWhere(MapOrdered(m), f)
}

// MapContainsWhere is `dict.contains(where:)`.
func MapContainsWhere[K comparable, V any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) bool) bool {
	return SliceContainsWhere(MapOrdered(m), f)
}

// MapRemoveAllKeepingCapacity is `dict.removeAll(keepingCapacity:)`.
func MapRemoveAllKeepingCapacity[K comparable, V any](m *map[K]V, _ bool) { clear(*m) }

// SetFilter is `Set.filter`, which returns a Set.
func SetFilter[T comparable](s map[T]struct{}, f func(T) bool) map[T]struct{} {
	out := map[T]struct{}{}
	for k := range s {
		if f(k) {
			out[k] = struct{}{}
		}
	}
	return out
}

// SetContainsWhere is `Set.contains(where:)`.
func SetContainsWhere[T comparable](s map[T]struct{}, f func(T) bool) bool {
	for _, k := range SetElements(s) {
		if f(k) {
			return true
		}
	}
	return false
}

// SetMap is `Set.map`.
func SetMap[T comparable, U any](s map[T]struct{}, f func(T) U) []U {
	return SliceMap(SetElements(s), f)
}

// SetCompactMap is `Set.compactMap`.
func SetCompactMap[T comparable, U any](s map[T]struct{}, f func(T) *U) []U {
	return SliceCompactMap(SetElements(s), f)
}

// SetReduce is `Set.reduce`.
func SetReduce[T comparable, A any](s map[T]struct{}, init A, f func(A, T) A) A {
	return SliceReduce(SetElements(s), init, f)
}

// SetMin is `Set.min()`.
func SetMin[T comparable](s map[T]struct{}) *T { return SliceFirst(SetSorted(s)) }

// SetMax is `Set.max()`.
func SetMax[T comparable](s map[T]struct{}) *T { return SliceLast(SetSorted(s)) }

// SetSubtractingSet is `Set.subtracting(_:)` with a Set argument.
func SetSubtractingSet[T comparable](a, b map[T]struct{}) map[T]struct{} {
	return SetSubtracting(a, SetElements(b))
}

// SetSubtractSet is `Set.subtract(_:)` with a Set argument.
func SetSubtractSet[T comparable](a *map[T]struct{}, b map[T]struct{}) {
	for k := range b {
		delete(*a, k)
	}
}

// SetUnionSet is `Set.union(_:)` with a Set argument.
func SetUnionSet[T comparable](a, b map[T]struct{}) map[T]struct{} {
	return SetUnion(a, SetElements(b))
}

// SetFormUnionSet is `Set.formUnion(_:)` with a Set argument.
func SetFormUnionSet[T comparable](a *map[T]struct{}, b map[T]struct{}) {
	SetFormUnion(a, SetElements(b))
}

// SetIntersectionSet is `Set.intersection(_:)` with a Set argument.
func SetIntersectionSet[T comparable](a, b map[T]struct{}) map[T]struct{} {
	return SetIntersection(a, SetElements(b))
}

// SetRemoveAll is `Set.removeAll()`.
func SetRemoveAll[T comparable](s *map[T]struct{}) { *s = map[T]struct{}{} }

// SetInitKeys is `Set(dict.keys)`.
func SetInitKeys[S ~map[T]struct{}, T comparable](keys []T) S { return SetInitSlice[S](keys) }

// SliceInitKeys is `Array(dict.keys)`.
func SliceInitKeys[S ~[]T, T any](keys []T) S { return S(slices.Clone(keys)) }

// SliceInitValues is `Array(dict.values)`.
func SliceInitValues[S ~[]T, T any](vals []T) S { return S(slices.Clone(vals)) }

// SliceInitSet is `Array(set)`.
func SliceInitSet[S ~[]T, T comparable](s map[T]struct{}) S { return S(SetElements(s)) }

// SliceReduceInto is `reduce(into:_:)`; the accumulator is a reference type in the emitted code.
func SliceReduceInto[T, A any](xs []T, init A, f func(A, T)) A {
	for _, x := range xs {
		f(init, x)
	}
	return init
}

// Zip2SequenceMap is `zip(a, b).map`.
func Zip2SequenceMap[A, B, U any](xs []struct {
	F0 A
	F1 B
}, f func(struct {
	F0 A
	F1 B
}) U) []U {
	return SliceMap(xs, f)
}

// StrideFromThroughBy is `stride(from:through:by:)`.
func StrideFromThroughBy[T Number](from, through, by T) []T {
	var out []T
	if by > 0 {
		for v := from; v <= through; v += by {
			out = append(out, v)
		}
	} else if by < 0 {
		for v := from; v >= through; v += by {
			out = append(out, v)
		}
	}
	return out
}

// StrideFromToBy is `stride(from:to:by:)`.
func StrideFromToBy[T Number](from, to, by T) []T {
	var out []T
	if by > 0 {
		for v := from; v < to; v += by {
			out = append(out, v)
		}
	} else if by < 0 {
		for v := from; v > to; v += by {
			out = append(out, v)
		}
	}
	return out
}

// RangeFilter is `range.filter`.
func RangeFilter(r Range[int], f func(int) bool) []int { return SliceFilter(rangeInts(r.Lo, r.Hi), f) }

// RangePrefixInt is `range.prefix(n)`.
func RangePrefixInt(r Range[int], n int) Range[int] {
	return Range[int]{r.Lo, min(r.Hi, r.Lo+max(n, 0))}
}

// RangeReversed is `range.reversed()`.
func RangeReversed(r Range[int]) []int { return SliceReversed(rangeInts(r.Lo, r.Hi)) }

// ClosedRangeReversed is `closedRange.reversed()`.
func ClosedRangeReversed(r ClosedRange[int]) []int { return SliceReversed(rangeInts(r.Lo, r.Hi+1)) }

// Abs is Swift `abs`; it traps on Int.min like Swift.
func Abs[T Number](v T) T {
	if v < 0 {
		var zero T
		r := zero - v
		if r < 0 {
			overflow()
		}
		return r
	}
	return v
}

// FloatRounded is Swift `Double.rounded()` (to nearest, ties away from zero).
func FloatRounded[F ~float32 | ~float64](v F) F { return F(math.Round(float64(v))) }

// FloatSquareRoot is Swift `Double.squareRoot()`.
func FloatSquareRoot[F ~float32 | ~float64](v F) F { return F(math.Sqrt(float64(v))) }

// FloatIsFinite is Swift `Double.isFinite`.
func FloatIsFinite[F ~float32 | ~float64](v F) bool {
	return !math.IsInf(float64(v), 0) && !math.IsNaN(float64(v))
}

// FloatIsInfinite is Swift `Double.isInfinite`.
func FloatIsInfinite[F ~float32 | ~float64](v F) bool { return math.IsInf(float64(v), 0) }

// IntInitAscii is Swift `UInt8(ascii:)`.
func IntInitAscii[T Integer](r rune) T {
	if r >= 0x80 {
		Trap("code point value is not ASCII")
	}
	return T(r)
}

// IntInitBitPattern is Swift `IntN(bitPattern:)`.
func IntInitBitPattern[T Integer, S Integer](v S) T { return T(v) }

// IntMagnitude is Swift `Int.magnitude`.
func IntMagnitude[T Integer](v T) uint {
	if v < 0 {
		return uint(-int64(v))
	}
	return uint(v)
}

// IntAddingReportingOverflowInt is Swift `addingReportingOverflow`.
func IntAddingReportingOverflowInt[T Integer](a, b T) struct {
	F0 T
	F1 bool
} {
	r := a + b
	over := (b > 0 && r < a) || (b < 0 && r > a)
	return struct {
		F0 T
		F1 bool
	}{r, over}
}

// IntMultipliedReportingOverflowBy is Swift `multipliedReportingOverflow(by:)`.
func IntMultipliedReportingOverflowBy[T Integer](a, b T) struct {
	F0 T
	F1 bool
} {
	r := a * b
	over := a != 0 && r/a != b
	return struct {
		F0 T
		F1 bool
	}{r, over}
}

// IntTrailingZeroBitCount is Swift `trailingZeroBitCount`.
func IntTrailingZeroBitCount[T Integer](v T) int {
	if v == 0 {
		return 64
	}
	n := 0
	for v&1 == 0 {
		v >>= 1
		n++
	}
	return n
}

// PreconditionFailure is Swift `preconditionFailure`.
func PreconditionFailure(msg ...string) { Trap("precondition failure " + Str(msg)) }

// ObjectIdentifierInitAnyObject is Swift `ObjectIdentifier(obj)`: the object's identity.
func ObjectIdentifierInitAnyObject(v any) any { return Dynamic(v) }

// TaskInitOperation is Swift `Task { ... }`, run on a goroutine.
func TaskInitOperation(f func()) struct{} {
	go f()
	return struct{}{}
}

// MutexInitState is Synchronization `Mutex(_:)` with a struct state.
func MutexInitState[T any](v T) *Mutex[T] { return MutexInit(v) }

// MutexInitMap is Synchronization `Mutex(_:)` with a dictionary state.
func MutexInitMap[T any](v T) *Mutex[T] { return MutexInit(v) }

// MutexInitSet is Synchronization `Mutex(_:)` with a set state.
func MutexInitSet[T any](v T) *Mutex[T] { return MutexInit(v) }

// MutexInitSlice is Synchronization `Mutex(_:)` with an array state.
func MutexInitSlice[T any](v T) *Mutex[T] { return MutexInit(v) }

// MutexInitInt is Synchronization `Mutex(_:)` with an integer state.
func MutexInitInt[T any](v T) *Mutex[T] { return MutexInit(v) }

// MutexInitBool is Synchronization `Mutex(_:)` with a Bool state.
func MutexInitBool[T any](v T) *Mutex[T] { return MutexInit(v) }

// StringEncoding is Foundation `String.Encoding`.
type StringEncoding int

// EncodingUtf8 is `.utf8`.
const EncodingUtf8 StringEncoding = 4

// StringOptInitDataEncoding is Foundation `String(data:encoding:)`.
func StringOptInitDataEncoding(b []byte, _ StringEncoding) *string {
	s := string(b)
	return &s
}

// StringDataUsing is Foundation `String.data(using:)`.
func StringDataUsing(s string, _ StringEncoding) *[]byte {
	b := []byte(s)
	return &b
}

// DataInitSlice is Foundation `Data(bytes)`.
func DataInitSlice(b []byte) []byte { return slices.Clone(b) }

// StringSplitSeparatorMaxSplits is `split(separator:maxSplits:)`.
func StringSplitSeparatorMaxSplits(s string, sep Character, maxSplits int) []Substring {
	return StringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s, sep, maxSplits, true)
}

// SubstringDropWhile is `Substring.drop(while:)`.
func SubstringDropWhile(s Substring, f func(Character) bool) Substring {
	p := SubstringPrefixWhile(s, f)
	return Substring{s.Base, p.Hi, s.Hi}
}

// OptionSetInitSlice is `Flags(array)`.
func OptionSetInitSlice[T ~struct{ RawValue R }, R Integer](ms []T) T { return OptionSetOf(ms...) }

// RangeMinBy is `range.min(by:)`.
func RangeMinBy(r Range[int], less func(int, int) bool) *int {
	return SliceMinBy(rangeInts(r.Lo, r.Hi), less)
}

// Zip2SequenceFilter is `zip(a, b).filter`.
func Zip2SequenceFilter[A, B any](xs []struct {
	F0 A
	F1 B
}, f func(struct {
	F0 A
	F1 B
}) bool) []struct {
	F0 A
	F1 B
} {
	return SliceFilter(xs, f)
}

// Tupled2 adapts a two-parameter closure to Swift's single tuple parameter.
func Tupled2[A, B, R any](f func(A, B) R) func(struct {
	F0 A
	F1 B
}) R {
	return func(t struct {
		F0 A
		F1 B
	}) R {
		return f(t.F0, t.F1)
	}
}

// Tupled3 adapts a three-parameter closure to Swift's single tuple parameter.
func Tupled3[A, B, C, R any](f func(A, B, C) R) func(struct {
	F0 A
	F1 B
	F2 C
}) R {
	return func(t struct {
		F0 A
		F1 B
		F2 C
	}) R {
		return f(t.F0, t.F1, t.F2)
	}
}

// Tupled2V adapts a two-parameter Void closure to Swift's single tuple parameter.
func Tupled2V[A, B any](f func(A, B)) func(struct {
	F0 A
	F1 B
}) {
	return func(t struct {
		F0 A
		F1 B
	}) {
		f(t.F0, t.F1)
	}
}
