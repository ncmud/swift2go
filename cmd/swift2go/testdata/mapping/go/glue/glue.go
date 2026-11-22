// Package glue stands in for the Swift module Glue, which the test does not transpile.
package glue

import "iter"

// LimitsGlue is Glue's `Limits.glue`.
var LimitsGlue = 40

// GlueAddIntInt is Glue's `glueAdd(_:_:)`.
func GlueAddIntInt(a, b int) int { return a + b }

// Tally is Glue's `Tally`.
type Tally struct{ Total int }

// TallyInitTotal is `Tally(total:)`.
func TallyInitTotal(total int) Tally { return Tally{Total: total} }

// TallyDoubled is `Tally.doubled()`.
func TallyDoubled(t Tally) int { return t.Total * 2 }

// Shade is Glue's `Shade`.
type Shade int

// The cases of Shade.
const (
	ShadeLight Shade = iota
	ShadeDark
)

// ShadeInt is `shade(_:)`.
func ShadeInt(n int) Shade {
	if n > 10 {
		return ShadeDark
	}
	return ShadeLight
}

// Box is Glue's generic `Box<T>`, so generated code names its type argument.
type Box[T any] struct{ Value T }

// BoxInitInt is `Box(_:)`.
func BoxInitInt[T any](v T) Box[T] { return Box[T]{Value: v} }

// Unbox is `unbox(_:)`. Its result type comes first, as for every generic function whose result Go cannot infer.
func Unbox[R, T any](b Box[T]) R { return any(b.Value).(R) }

// EvensUpTo is `evens(upTo:)`: a lazy sequence, which generated for-in loops range over directly.
func EvensUpTo(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; i < n; i += 2 {
			if !yield(i) {
				return
			}
		}
	}
}
