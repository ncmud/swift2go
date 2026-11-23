package swiftrt

import (
	"math/bits"
	"runtime"
	"slices"
	"time"
)

// ContinuousClock is Swift's `ContinuousClock`.
type ContinuousClock struct{}

// ContinuousClockInit is `ContinuousClock()`.
func ContinuousClockInit() ContinuousClock { return ContinuousClock{} }

// ContinuousClockMeasure is `ContinuousClock.measure(_:)`: how long body takes.
func ContinuousClockMeasure(_ ContinuousClock, body func()) Duration {
	start := time.Now()
	body()
	return Duration(time.Since(start).Nanoseconds())
}

// InstantLess is `<` on two `ContinuousClock.Instant`s.
func InstantLess(a, b ContinuousClockInstant) bool { return time.Time(a).Before(time.Time(b)) }

// InstantAdvanced is `ContinuousClock.Instant ± Duration`.
func InstantAdvanced(a ContinuousClockInstant, d Duration) ContinuousClockInstant {
	return ContinuousClockInstant(time.Time(a).Add(time.Duration(d)))
}

// TaskYield is `Task.yield()`: it lets other goroutines run.
func TaskYield() { runtime.Gosched() }

// EnumeratedDropFirst is `xs.enumerated().dropFirst()`.
func EnumeratedDropFirst[T any](xs []struct {
	F0 int
	F1 T
}) []struct {
	F0 int
	F1 T
} {
	if len(xs) == 0 {
		return xs
	}
	return xs[1:]
}

// FlattenSequenceCount is `joined().count` on an array of arrays, which the runtime has already flattened.
func FlattenSequenceCount[T any](xs []T) int { return len(xs) }

// StrideThroughMap is `stride(from:through:by:).map`.
func StrideThroughMap[T Number, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// StrideToMap is `stride(from:to:by:).map`.
func StrideToMap[T Number, U any](xs []T, f func(T) U) []U { return SliceMap(xs, f) }

// SliceInitStrideTo is `Array(stride(from:to:by:))`.
func SliceInitStrideTo[S ~[]T, T Number](xs []T) S { return S(slices.Clone(xs)) }

// IntNonzeroBitCount is `nonzeroBitCount` on an integer.
func IntNonzeroBitCount[T Integer](v T) int { return bits.OnesCount64(uint64(v)) }

// HSetAllSatisfy is `allSatisfy` on a Set whose elements Go cannot use as map keys.
func HSetAllSatisfy[T any](s HSet[T], f func(T) bool) bool {
	for _, v := range s.M {
		if !f(v) {
			return false
		}
	}
	return true
}

// AnyCollectionCompactMap is `compactMap` on an `AnyCollection`, such as `Mirror.children`.
func AnyCollectionCompactMap[T, U any](xs []T, f func(T) *U) []U { return SliceCompactMap(xs, f) }

// DateDistanceTo is `Date.distance(to:)`.
func DateDistanceTo(a, b Date) float64 { return float64(b - a) }

// SliceInitUTF8View is `Array(s.utf8)`.
func SliceInitUTF8View[S ~[]uint8](b []uint8) S { return S(slices.Clone(b)) }

// CollectionFirst is `first` on a collection the runtime keeps as a slice, read through a key path.
func CollectionFirst[T any](xs []T) *T { return SliceFirst(xs) }

// Zip2SequenceFirstWhere is `zip(a, b).first(where:)`.
func Zip2SequenceFirstWhere[T any](xs []T, f func(T) bool) *T { return SliceFirstWhere(xs, f) }

// RangeForEach is `forEach` on a Range of integers.
func RangeForEach[T Integer](r Range[T], f func(T)) {
	for i := r.Lo; i < r.Hi; i++ {
		f(i)
	}
}

// ReversedCollectionStartsWith is `xs.reversed().starts(with: ys.reversed())`.
func ReversedCollectionStartsWith[T comparable](xs, prefix []T) bool {
	return len(xs) >= len(prefix) && slices.Equal(xs[:len(prefix)], prefix)
}

// SliceContainsSubsequence is `contains(_ other: some Collection)`: whether other occurs in xs as a run of
// consecutive elements.
func SliceContainsSubsequence[T any](xs, other []T) bool {
	if len(other) == 0 {
		return true
	}
	for i := 0; i+len(other) <= len(xs); i++ {
		if Equal(xs[i:i+len(other)], other) {
			return true
		}
	}
	return false
}

// SliceContainsEqual is `contains(_:)` by Swift's ==, for elements Go's == would compare by address, such as
// Optionals.
func SliceContainsEqual[T any](xs []T, v T) bool {
	for _, x := range xs {
		if Equal(x, v) {
			return true
		}
	}
	return false
}
