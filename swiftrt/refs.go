package swiftrt

import (
	"cmp"
	"reflect"
)

// compactRef appends the non-nil results of f, for closures returning an optional class reference.
func compactRef[T, U any](xs []T, f func(T) U) []U {
	out := []U{}
	for _, x := range xs {
		if v := f(x); !isNil(v) {
			out = append(out, v)
		}
	}
	return out
}

// SliceCompactMapRef is `compactMap` whose closure returns an optional class or protocol reference.
func SliceCompactMapRef[T, U any](xs []T, f func(T) U) []U { return compactRef(xs, f) }

// LazySequenceCompactMapRef is `lazy.compactMap` returning optional references.
func LazySequenceCompactMapRef[T, U any](xs []T, f func(T) U) []U { return compactRef(xs, f) }

// ValuesCompactMapRef is `dict.values.compactMap` returning optional references.
func ValuesCompactMapRef[T, U any](xs []T, f func(T) U) []U { return compactRef(xs, f) }

// KeysCompactMapRef is `dict.keys.compactMap` returning optional references.
func KeysCompactMapRef[T, U any](xs []T, f func(T) U) []U { return compactRef(xs, f) }

// SetCompactMapRef is `Set.compactMap` returning optional references.
func SetCompactMapRef[T comparable, U any](s map[T]struct{}, f func(T) U) []U {
	return compactRef(SetElements(s), f)
}

// MapCompactMapRef is `Dictionary.compactMap` returning optional references.
func MapCompactMapRef[K comparable, V, U any](m map[K]V, f func(struct {
	F0 K
	F1 V
}) U) []U {
	return compactRef(MapOrdered(m), f)
}

// RangeCompactMapRef is `range.compactMap` returning optional references.
func RangeCompactMapRef[U any](r Range[int], f func(int) U) []U {
	return compactRef(rangeInts(r.Lo, r.Hi), f)
}

// ToAnySlice widens a typed slice for a protocol requirement declared as `[Self]`.
func ToAnySlice[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// FromAnySlice narrows a `[Self]` requirement's result back to the conformer's type.
func FromAnySlice[T any](xs []any) []T {
	out := make([]T, len(xs))
	for i, x := range xs {
		out[i] = MustAs[T](x)
	}
	return out
}

// TupleCompare orders two tuples lexicographically, as Swift's tuple `<` does, comparing each element
// with its own comparator.
func TupleCompare(a, b any, cmps ...func(x, y any) int) int {
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < ra.NumField(); i++ {
		cmp := compareAny
		if i < len(cmps) {
			cmp = cmps[i]
		}
		if c := cmp(ra.Field(i).Interface(), rb.Field(i).Interface()); c != 0 {
			return c
		}
	}
	return 0
}

// CompareOrdered compares two values of a Go-ordered type.
func CompareOrdered[T cmp.Ordered](x, y any) int { return cmp.Compare(x.(T), y.(T)) }

// CompareBy adapts a type's own `<` to a three-way comparison.
func CompareBy[T any](less func(T, T) bool) func(x, y any) int {
	return func(x, y any) int {
		a, b := x.(T), y.(T)
		switch {
		case less(a, b):
			return -1
		case less(b, a):
			return 1
		}
		return 0
	}
}

// MaxLess is Swift `max(_:_:...)` for a type ordered by its own `<`; ties keep the later argument.
func MaxLess[T any](less func(T, T) bool, x T, ys ...T) T {
	m := x
	for _, y := range ys {
		if !less(y, m) {
			m = y
		}
	}
	return m
}

// MinLess is Swift `min(_:_:...)` for a type ordered by its own `<`; ties keep the earlier argument.
func MinLess[T any](less func(T, T) bool, x T, ys ...T) T {
	m := x
	for _, y := range ys {
		if less(y, m) {
			m = y
		}
	}
	return m
}

// SingleValueDecodingContainerDecodeAny is `container.decode(T.self)` where T is a generic parameter.
func SingleValueDecodingContainerDecodeAny[T any](c SingleValueDecodingContainer, t TypeT[T]) T {
	return SingleValueDecodingContainerDecode(c, t)
}

// KeyedDecodingContainerDecodeAnyForKey is `container.decode(T.self, forKey:)` where T is a generic parameter.
func KeyedDecodingContainerDecodeAnyForKey[T any](c KeyedDecodingContainer, t TypeT[T], key any) T {
	return KeyedDecodingContainerDecodeForKey(c, t, key)
}

// SomeRef boxes an optional class reference for runtime APIs that take *V; nil stays nil.
func SomeRef[T any](v T) *T {
	if isNil(v) {
		return nil
	}
	return &v
}

// SliceFlatten is `joined()` over an array of arrays.
func SliceFlatten[T any](xss [][]T) []T {
	out := []T{}
	for _, xs := range xss {
		out = append(out, xs...)
	}
	return out
}

// SliceFlattenMaps is `joined()` over an array of dictionaries: every key-value pair, in order.
func SliceFlattenMaps[K comparable, V any](ms []map[K]V) []struct {
	F0 K
	F1 V
} {
	out := []struct {
		F0 K
		F1 V
	}{}
	for _, m := range ms {
		out = append(out, MapOrdered(m)...)
	}
	return out
}

// WritableKeyPath is Swift `WritableKeyPath`/`ReferenceWritableKeyPath`: a getter and a setter on Root.
type WritableKeyPath[Root, Value any] struct {
	Get func(Root) Value
	Set func(*Root, Value)
}

// Foundation `ComparisonResult` cases.
const (
	NSComparisonResultOrderedAscending  = -1
	NSComparisonResultOrderedSame       = 0
	NSComparisonResultOrderedDescending = 1
)

// OrderedSetCompactMapRef is `OrderedSet.compactMap` returning optional references.
func OrderedSetCompactMapRef[T comparable, U any](s OrderedSet[T], f func(T) U) []U {
	return compactRef(s.Items, f)
}

// LazyFilterSequenceCompactMapRef is `lazy.filter(_:).compactMap(_:)` returning optional references.
func LazyFilterSequenceCompactMapRef[T, U any](xs []T, f func(T) U) []U { return compactRef(xs, f) }

// Zip2SequenceCompactMapRef is `zip(_:_:).compactMap` returning optional references.
func Zip2SequenceCompactMapRef[T, U any](xs []T, f func(T) U) []U { return compactRef(xs, f) }

// OrderedDictionarySubscriptDefault is `dict[key, default: d]` as an rvalue.
func OrderedDictionarySubscriptDefault[K comparable, V any](d OrderedDictionary[K, V], k K, def V) V {
	if v, ok := d.M[k]; ok {
		return v
	}
	return def
}
