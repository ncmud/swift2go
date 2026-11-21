package swiftrt

import (
	"cmp"
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
func SliceInitClosedRange[S ~[]int](r ClosedRange[int]) S {
	if r.Hi < r.Lo {
		return S{}
	}
	return S(rangeInts(r.Lo, r.Hi+1))
}
