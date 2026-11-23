package swiftrt

import (
	"reflect"
	"strconv"
)

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

// NewRange is Swift's `lo..<hi`, which traps when hi is below lo.
func NewRange[T Ordered](lo, hi T) Range[T] {
	if hi < lo {
		Trap("Range requires lowerBound <= upperBound")
	}
	return Range[T]{lo, hi}
}

// NewClosedRange is Swift's `lo...hi`, which traps when hi is below lo.
func NewClosedRange[T Ordered](lo, hi T) ClosedRange[T] {
	if hi < lo {
		Trap("Range requires lowerBound <= upperBound")
	}
	return ClosedRange[T]{lo, hi}
}

func (r Range[T]) Contains(v T) bool               { return r.Lo <= v && v < r.Hi }
func (r ClosedRange[T]) Contains(v T) bool         { return r.Lo <= v && v <= r.Hi }
func (r PartialRangeFrom[T]) Contains(v T) bool    { return r.Lo <= v }
func (r PartialRangeUpTo[T]) Contains(v T) bool    { return v < r.Hi }
func (r PartialRangeThrough[T]) Contains(v T) bool { return v <= r.Hi }

// RangeContains is Range.contains(_:).
func RangeContains[T Ordered](r Range[T], v T) bool { return r.Contains(v) }

// ClosedRangeContains is ClosedRange.contains(_:).
func ClosedRangeContains[T Ordered](r ClosedRange[T], v T) bool { return r.Contains(v) }

// PartialRangeFromContains is PartialRangeFrom.contains(_:).
func PartialRangeFromContains[T Ordered](r PartialRangeFrom[T], v T) bool { return r.Contains(v) }

// PartialRangeUpToContains is PartialRangeUpTo.contains(_:).
func PartialRangeUpToContains[T Ordered](r PartialRangeUpTo[T], v T) bool { return r.Contains(v) }

// PartialRangeThroughContains is PartialRangeThrough.contains(_:).
func PartialRangeThroughContains[T Ordered](r PartialRangeThrough[T], v T) bool { return r.Contains(v) }

// RangeMap is (lo..<hi).map(f).
func RangeMap[U any, T Integer](r Range[T], f func(T) U) []U {
	out := make([]U, 0, max(0, int(r.Hi)-int(r.Lo)))
	for i := r.Lo; i < r.Hi; i++ {
		out = append(out, f(i))
	}
	return out
}

// ClosedRangeMap is (lo...hi).map(f).
func ClosedRangeMap[U any, T Integer](r ClosedRange[T], f func(T) U) []U {
	out := []U{}
	eachClosed(r, func(i T) bool { out = append(out, f(i)); return true })
	return out
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

// Encode is Range's Encodable conformance: the array [lowerBound, upperBound].
func (r Range[T]) Encode(e *Encoder) { e.Value = []any{EncodeValue(r.Lo), EncodeValue(r.Hi)} }

// Encode is ClosedRange's Encodable conformance: the array [lowerBound, upperBound].
func (r ClosedRange[T]) Encode(e *Encoder) { e.Value = []any{EncodeValue(r.Lo), EncodeValue(r.Hi)} }

// boundsDecodable is a Range or ClosedRange, which Swift decodes from [lowerBound, upperBound].
type boundsDecodable interface{ decodeBounds(raw any, path []string) }

func (r *Range[T]) decodeBounds(raw any, path []string) {
	r.Lo, r.Hi = decodeBounds[T](raw, path)
	if r.Hi < r.Lo {
		boundsOutOfOrder("Range", r.Lo, r.Hi, path)
	}
}

func (r *ClosedRange[T]) decodeBounds(raw any, path []string) {
	r.Lo, r.Hi = decodeBounds[T](raw, path)
	if r.Hi < r.Lo {
		boundsOutOfOrder("ClosedRange", r.Lo, r.Hi, path)
	}
}

// decodeBounds reads the first two elements of an unkeyed container, as a range's init(from:)
// does; elements after them are ignored.
func decodeBounds[T Ordered](raw any, path []string) (T, T) {
	xs, ok := raw.([]any)
	if !ok {
		mismatch(reflect.TypeFor[[]any](), raw, path)
	}
	var bounds [2]T
	for i := range bounds {
		elementPath := append(codingPathCopy(path), "Index "+strconv.Itoa(i))
		if i >= len(xs) {
			panic(Throw(DecodingErrorValueNotFound{F0: reflect.TypeFor[T](), F1: CodingContext{
				CodingPath: elementPath, DebugDescription: "Unkeyed container is at end.",
			}}))
		}
		bounds[i] = DecodeValue[T](xs[i], elementPath)
	}
	return bounds[0], bounds[1]
}

func boundsOutOfOrder[T Ordered](kind string, lo, hi T, path []string) {
	panic(Throw(DecodingErrorDataCorrupted{F0: CodingContext{
		CodingPath: codingPathCopy(path),
		DebugDescription: "Cannot initialize " + kind + "<" + decodeTypeName(reflect.TypeFor[T]()) + "> with a lowerBound (" +
			Str(lo) + ") greater than upperBound (" + Str(hi) + ")",
	}}))
}

// Description is Range's description, "lo..<hi".
func (r Range[T]) Description() string { return Str(r.Lo) + "..<" + Str(r.Hi) }

// DebugDescription is Range's debugDescription, "Range(lo..<hi)".
func (r Range[T]) DebugDescription() string {
	return "Range(" + debugStr(r.Lo) + "..<" + debugStr(r.Hi) + ")"
}

// Description is ClosedRange's description, "lo...hi".
func (r ClosedRange[T]) Description() string { return Str(r.Lo) + "..." + Str(r.Hi) }

// DebugDescription is ClosedRange's debugDescription, "ClosedRange(lo...hi)".
func (r ClosedRange[T]) DebugDescription() string {
	return "ClosedRange(" + debugStr(r.Lo) + "..." + debugStr(r.Hi) + ")"
}
