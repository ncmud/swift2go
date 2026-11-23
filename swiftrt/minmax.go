package swiftrt

import (
	"cmp"
	"reflect"
	"time"
)

// MaxOf is Swift's variadic `max(_:_:_:_:)`: the greatest of xs and rest, the last one on ties.
func MaxOf[T cmp.Ordered](xs []T, rest []T) T {
	all := append(append([]T(nil), xs...), rest...)
	m := all[0]
	for _, x := range all[1:] {
		if x >= m {
			m = x
		}
	}
	return m
}

// MinOf is Swift's variadic `min(_:_:_:_:)`: the least of xs and rest, the first one on ties.
func MinOf[T cmp.Ordered](xs []T, rest []T) T {
	all := append(append([]T(nil), xs...), rest...)
	m := all[0]
	for _, x := range all[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

// InstantSub is `ContinuousClock.Instant - ContinuousClock.Instant`.
func InstantSub(a, b ContinuousClockInstant) Duration {
	return Duration(time.Time(a).Sub(time.Time(b)).Nanoseconds())
}

// SliceInitClosedRange is `Array(lo...hi)`.
func SliceInitClosedRange[S ~[]T, T Integer](r ClosedRange[T]) S {
	out := S{}
	if r.Hi < r.Lo {
		return out
	}
	for v := r.Lo; ; v++ {
		out = append(out, v)
		if v == r.Hi {
			return out
		}
	}
}

// MapSetRef is `dict[k] = value` where the value is an optional class reference: nil removes the entry.
func MapSetRef[K comparable, V any](m map[K]V, k K, v V) {
	if isNilRef(v) {
		delete(m, k)
		return
	}
	m[k] = v
}

func isNilRef[R any](r R) bool {
	v := reflect.ValueOf(&r).Elem()
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// LiftOpt0 adapts a closure returning an optional class reference (nil-able R) to a generic
// `() -> T?` with T the class: nil stays nil, anything else is wrapped.
func LiftOpt0[R any](f func() R) func() *R {
	return func() *R {
		r := f()
		if isNilRef(r) {
			return nil
		}
		return &r
	}
}

// LiftOpt1 is LiftOpt0 for one parameter.
func LiftOpt1[A, R any](f func(A) R) func(A) *R {
	return func(a A) *R {
		r := f(a)
		if isNilRef(r) {
			return nil
		}
		return &r
	}
}

// LiftOpt2 is LiftOpt0 for two parameters.
func LiftOpt2[A, B, R any](f func(A, B) R) func(A, B) *R {
	return func(a A, b B) *R {
		r := f(a, b)
		if isNilRef(r) {
			return nil
		}
		return &r
	}
}

// DurationRatio is `Duration / Duration`, a Double.
func DurationRatio(a, b Duration) float64 { return float64(a) / float64(b) }

// RangeAllSatisfy is `Range.allSatisfy(_:)`: whether f holds for every integer in [Lo, Hi).
func RangeAllSatisfy[T Integer](r Range[T], f func(T) bool) bool {
	for v := r.Lo; v < r.Hi; v++ {
		if !f(v) {
			return false
		}
	}
	return true
}
