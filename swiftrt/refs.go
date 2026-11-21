package swiftrt

import "reflect"

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

// TupleCompare orders two tuples lexicographically, as Swift's tuple `<` does.
func TupleCompare(a, b any) int {
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < ra.NumField(); i++ {
		if c := compareAny(ra.Field(i).Interface(), rb.Field(i).Interface()); c != 0 {
			return c
		}
	}
	return 0
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
