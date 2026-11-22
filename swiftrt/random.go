package swiftrt

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"math/bits"
	"reflect"
)

// RandomNumberGenerator is Swift `RandomNumberGenerator`.
type RandomNumberGenerator interface{ Next() uint64 }

// SystemRandomNumberGenerator is Swift `SystemRandomNumberGenerator`.
type SystemRandomNumberGenerator struct{}

// Next is `SystemRandomNumberGenerator.next()`.
func (*SystemRandomNumberGenerator) Next() uint64 {
	var b [8]byte
	rand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// rng accepts a generator the emitter passes as an unconstrained generic `inout some RandomNumberGenerator`.
func rng(g any) RandomNumberGenerator {
	if p, ok := g.(*any); ok {
		g = *p
	}
	return g.(RandomNumberGenerator)
}

// SystemRandomNumberGeneratorInit is Swift `SystemRandomNumberGenerator()`.
func SystemRandomNumberGeneratorInit() *SystemRandomNumberGenerator {
	return &SystemRandomNumberGenerator{}
}

type interval[T any] interface {
	interval() (lo, hi T, closed bool)
}

func (r Range[T]) interval() (T, T, bool)       { return r.Lo, r.Hi, false }
func (r ClosedRange[T]) interval() (T, T, bool) { return r.Lo, r.Hi, true }

func bitWidth[T Integer]() int {
	var z T
	return int(reflect.TypeOf(z).Size()) * 8
}

// nextUpTo is `RandomNumberGenerator.next(upperBound:)` for a w-bit unsigned type: Lemire's nearly divisionless method.
func nextUpTo(g RandomNumberGenerator, w int, upper uint64) uint64 {
	if upper == 0 {
		Trap("upperBound cannot be zero.")
	}
	mask := uint64(math.MaxUint64)
	if w < 64 {
		mask = 1<<w - 1
	}
	mul := func(r uint64) (hi, lo uint64) {
		if w == 64 {
			return bits.Mul64(r, upper)
		}
		p := r * upper
		return p >> w, p & mask
	}
	random := g.Next() & mask
	hi, lo := mul(random)
	if lo < upper {
		t := ((0 - upper) & mask) % upper
		for lo < t {
			random = g.Next() & mask
			hi, lo = mul(random)
		}
	}
	return hi
}

func intRandom[T Integer](r interval[T], g RandomNumberGenerator) T {
	lo, hi, closed := r.interval()
	w := bitWidth[T]()
	mask := uint64(math.MaxUint64)
	if w < 64 {
		mask = 1<<w - 1
	}
	delta := uint64(hi-lo) & mask
	if !closed {
		if !(lo < hi) {
			Trap("Can't get random value with an empty range")
		}
		return T(uint64(lo) + nextUpTo(g, w, delta))
	}
	if hi < lo {
		Trap("Range requires lowerBound <= upperBound")
	}
	if delta == mask {
		return T(g.Next() & mask)
	}
	return T(uint64(lo) + nextUpTo(g, w, delta+1))
}

// IntRandomInUsing is Swift `FixedWidthInteger.random(in:using:)` for a Range or ClosedRange.
func IntRandomInUsing[T Integer](r interval[T], g any) T { return intRandom(r, rng(g)) }

// IntRandomIn is Swift `FixedWidthInteger.random(in:)` for a Range or ClosedRange.
func IntRandomIn[T Integer](r interval[T]) T { return intRandom(r, &SystemRandomNumberGenerator{}) }

func floatRandom[F ~float32 | ~float64](r interval[F], g RandomNumberGenerator) F {
	lo, hi, closed := r.interval()
	if !closed && !(lo < hi) {
		Trap("Can't get random value with an empty range")
	}
	if closed && hi < lo {
		Trap("Range requires lowerBound <= upperBound")
	}
	delta := hi - lo
	if math.IsInf(float64(delta), 0) || math.IsNaN(float64(delta)) {
		Trap("There is no uniform distribution on an infinite range")
	}
	var z F
	is32 := reflect.TypeOf(z).Kind() == reflect.Float32
	sigBits, rawWidth := 53, 64
	if is32 {
		sigBits, rawWidth = 24, 32
	}
	maxSig := uint64(1) << sigBits
	half := F(math.Ldexp(1, -sigBits))
	for {
		var rand uint64
		if closed {
			rand = nextUpTo(g, rawWidth, maxSig+1)
			if rand == maxSig {
				return hi
			}
		} else {
			rand = g.Next() & (maxSig - 1)
		}
		unit := F(rand) * half
		v := delta*unit + lo
		if closed || v != hi {
			return v
		}
	}
}

// FloatRandomInUsing is Swift `BinaryFloatingPoint.random(in:using:)` for a Range or ClosedRange.
func FloatRandomInUsing[F ~float32 | ~float64](r interval[F], g any) F {
	return floatRandom(r, rng(g))
}

// FloatRandomIn is Swift `BinaryFloatingPoint.random(in:)` for a Range or ClosedRange.
func FloatRandomIn[F ~float32 | ~float64](r interval[F]) F {
	return floatRandom(r, &SystemRandomNumberGenerator{})
}

// BoolRandomUsing is Swift `Bool.random(using:)`.
func BoolRandomUsing(g any) bool { return (rng(g).Next()>>17)&1 == 0 }

// BoolRandom is Swift `Bool.random()`.
func BoolRandom() bool { return BoolRandomUsing(&SystemRandomNumberGenerator{}) }

// SliceRandomElementUsing is Swift `Collection.randomElement(using:)`.
func SliceRandomElementUsing[T any](xs []T, g any) *T {
	if len(xs) == 0 {
		return nil
	}
	v := xs[intRandom[int](Range[int]{0, len(xs)}, rng(g))]
	return &v
}

// SliceRandomElement is Swift `Collection.randomElement()`.
func SliceRandomElement[T any](xs []T) *T {
	return SliceRandomElementUsing(xs, &SystemRandomNumberGenerator{})
}

// SliceShuffleUsing is Swift `MutableCollection.shuffle(using:)`.
func SliceShuffleUsing[T any](xs *[]T, g any) {
	s := *xs
	if len(s) <= 1 {
		return
	}
	amount := len(s)
	cur := 0
	for amount > 1 {
		r := intRandom[int](Range[int]{0, amount}, rng(g))
		amount--
		s[cur], s[cur+r] = s[cur+r], s[cur]
		cur++
	}
}

// SliceShuffledUsing is Swift `Sequence.shuffled(using:)`.
func SliceShuffledUsing[T any](xs []T, g any) []T {
	out := Clone(xs)
	SliceShuffleUsing(&out, g)
	return out
}

// SliceShuffled is Swift `Sequence.shuffled()`.
func SliceShuffled[T any](xs []T) []T { return SliceShuffledUsing(xs, &SystemRandomNumberGenerator{}) }
