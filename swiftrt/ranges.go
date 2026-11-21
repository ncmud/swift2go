package swiftrt

// Ordered is a Swift Comparable that Go can order with `<`.
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64 | ~string
}

// Range is Swift's half-open Range.
type Range[T Ordered] struct{ Lo, Hi T }

// ClosedRange is Swift's ClosedRange.
type ClosedRange[T Ordered] struct{ Lo, Hi T }

// PartialRangeFrom is `lo...`.
type PartialRangeFrom[T Ordered] struct{ Lo T }

// PartialRangeUpTo is `..<hi`.
type PartialRangeUpTo[T Ordered] struct{ Hi T }

// PartialRangeThrough is `...hi`.
type PartialRangeThrough[T Ordered] struct{ Hi T }

func (r Range[T]) Contains(v T) bool               { return r.Lo <= v && v < r.Hi }
func (r ClosedRange[T]) Contains(v T) bool         { return r.Lo <= v && v <= r.Hi }
func (r PartialRangeFrom[T]) Contains(v T) bool    { return r.Lo <= v }
func (r PartialRangeUpTo[T]) Contains(v T) bool    { return v < r.Hi }
func (r PartialRangeThrough[T]) Contains(v T) bool { return v <= r.Hi }

// RangeContains is Range.contains(_:).
func RangeContains[T Ordered](r Range[T], v T) bool { return r.Contains(v) }

// ClosedRangeContains is ClosedRange.contains(_:).
func ClosedRangeContains[T Ordered](r ClosedRange[T], v T) bool { return r.Contains(v) }

// RangeMap is (lo..<hi).map(f).
func RangeMap[U any](r Range[int], f func(int) U) []U {
	out := make([]U, 0, max(0, r.Hi-r.Lo))
	for i := r.Lo; i < r.Hi; i++ {
		out = append(out, f(i))
	}
	return out
}

// ClosedRangeMap is (lo...hi).map(f).
func ClosedRangeMap[U any](r ClosedRange[int], f func(int) U) []U {
	return RangeMap(Range[int]{r.Lo, r.Hi + 1}, f)
}

// ClosedRangeFilter is Swift `(lo...hi).filter(f)` over an integer range.
func ClosedRangeFilter[T Integer](r ClosedRange[T], f func(T) bool) []T {
	out := []T{}
	if r.Lo > r.Hi {
		return out
	}
	for i := r.Lo; ; i++ {
		if f(i) {
			out = append(out, i)
		}
		if i == r.Hi {
			return out
		}
	}
}

// RangeCount is Range.count.
func RangeCount(r Range[int]) int { return max(0, r.Hi-r.Lo) }

// RangeIsEmpty is Range.isEmpty.
func RangeIsEmpty[T Ordered](r Range[T]) bool { return !(r.Lo < r.Hi) }
