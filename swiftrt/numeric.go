package swiftrt

import (
	"math"
	"strconv"
	"strings"
)

// Number is any Swift integer or floating-point type.
type Number interface {
	Integer | ~float32 | ~float64
}

// IntInitInt is Swift `IntN(x)` from another integer; it traps when x does not fit.
func IntInitInt[T Integer, S Integer](v S) T {
	t := T(v)
	if S(t) != v || (t < 0) != (v < 0) {
		Trap("integer conversion overflow")
	}
	return t
}

// IntInitFloat is Swift `IntN(x)` from a floating-point value; it truncates and traps on NaN or overflow.
func IntInitFloat[T Integer, F ~float32 | ~float64](v F) T {
	f := math.Trunc(float64(v))
	lo, hi, unsigned, umax := bounds[T]()
	if math.IsNaN(f) || (unsigned && (f < 0 || f > float64(umax))) || (!unsigned && (f < float64(lo) || f >= -float64(lo))) {
		Trap("float to integer conversion overflow")
	}
	_ = hi
	return T(f)
}

// IntInitClamping is Swift `IntN(clamping:)`.
func IntInitClamping[T Integer, S Integer](v S) T {
	lo, hi, unsigned, umax := bounds[T]()
	if v < 0 {
		if unsigned {
			return 0
		}
		if int64(v) < lo {
			return T(lo)
		}
		return T(v)
	}
	if unsigned {
		if uint64(v) > umax {
			return T(umax)
		}
		return T(v)
	}
	if uint64(v) > uint64(hi) {
		return T(hi)
	}
	return T(v)
}

// IntInitTruncatingIfNeeded is Swift `IntN(truncatingIfNeeded:)`.
func IntInitTruncatingIfNeeded[T Integer, S Integer](v S) T { return T(v) }

// IntInitExactly is Swift `IntN(exactly:)` from an integer.
func IntInitExactly[T Integer, S Number](v S) *T {
	t := T(v)
	if S(t) != v {
		return nil
	}
	return &t
}

// FloatInitInt is Swift `Double(x)` from an integer.
func FloatInitInt[T ~float32 | ~float64, S Integer](v S) T { return T(v) }

// FloatInitFloat is Swift `Double(x)` from another floating-point value.
func FloatInitFloat[T ~float32 | ~float64, S ~float32 | ~float64](v S) T { return T(v) }

// IntOptInitString is Swift `Int(String)`.
func IntOptInitString[T Integer](s string) *T { return parseInt[T](s) }

// IntOptInitSubstring is Swift `Int(Substring)`.
func IntOptInitSubstring[T Integer](s Substring) *T { return parseInt[T](s.String()) }

// IntOptInitStringRadix is Swift `Int(String, radix:)`.
func IntOptInitStringRadix[T Integer](s string, radix int) *T { return parseIntRadix[T](s, radix) }

// IntOptInitSubstringRadix is Swift `Int(Substring, radix:)`.
func IntOptInitSubstringRadix[T Integer](s Substring, radix int) *T {
	return parseIntRadix[T](s.String(), radix)
}

func parseInt[T Integer](s string) *T { return parseIntRadix[T](s, 10) }

func parseIntRadix[T Integer](s string, radix int) *T {
	if s == "" || strings.ContainsAny(s, "_ \t\n") || (len(s) > 1 && s[0] == '+' && (s[1] == '+' || s[1] == '-')) {
		return nil
	}
	_, _, unsigned, _ := bounds[T]()
	if unsigned {
		u, err := strconv.ParseUint(strings.TrimPrefix(s, "+"), radix, 64)
		if err != nil {
			return nil
		}
		return IntInitExactly[T](u)
	}
	i, err := strconv.ParseInt(s, radix, 64)
	if err != nil {
		return nil
	}
	return IntInitExactly[T](i)
}

// FloatOptInitString is Swift `Double(String)`.
func FloatOptInitString[T ~float32 | ~float64](s string) *T {
	if s == "" || strings.ContainsAny(s, " _") {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return nil
		}
	}
	t := T(f)
	return &t
}

// FloatOptInitSubstring is Swift `Double(Substring)`.
func FloatOptInitSubstring[T ~float32 | ~float64](s Substring) *T {
	return FloatOptInitString[T](s.String())
}

// IntMax is Swift `Int.max`.
const IntMax = math.MaxInt

// IntMin is Swift `Int.min`.
const IntMin = math.MinInt

// Div is Swift integer `/`; it traps on division by zero and on overflow.
func Div[T Integer](a, b T) T {
	if b == 0 {
		Trap("division by zero")
	}
	lo, _, unsigned, _ := bounds[T]()
	if !unsigned && int64(a) == lo && int64(b) == -1 {
		overflow()
	}
	return a / b
}

// Rem is Swift integer `%`; it traps on division by zero.
func Rem[T Integer](a, b T) T {
	if b == 0 {
		Trap("division by zero in remainder")
	}
	lo, _, unsigned, _ := bounds[T]()
	if !unsigned && int64(a) == lo && int64(b) == -1 {
		overflow()
	}
	return a % b
}

// Neg is Swift integer prefix `-`; it traps on overflow.
func Neg[T Integer](a T) T { return Sub(T(0), a) }

// IntAbs is Swift `abs` on an integer.
func IntAbs[T Integer](a T) T {
	if a < 0 {
		return Neg(a)
	}
	return a
}

// Precondition is Swift `precondition`.
func Precondition(ok bool, msg ...string) {
	if !ok {
		Trap("precondition failed " + strings.Join(msg, ""))
	}
}

// Assert is Swift `assert`; debug builds trap.
func Assert(ok bool, msg ...string) { Precondition(ok, msg...) }

// FatalError is Swift `fatalError`.
func FatalError[T any](msg ...string) T {
	panic("swift fatalError: " + strings.Join(msg, ""))
}

// RangeUpperBound is Swift `Range.upperBound`.
func RangeUpperBound[T Ordered](r Range[T]) T { return r.Hi }

// RangeLowerBound is Swift `Range.lowerBound`.
func RangeLowerBound[T Ordered](r Range[T]) T { return r.Lo }

// ClosedRangeUpperBound is Swift `ClosedRange.upperBound`.
func ClosedRangeUpperBound[T Ordered](r ClosedRange[T]) T { return r.Hi }

// ClosedRangeLowerBound is Swift `ClosedRange.lowerBound`.
func ClosedRangeLowerBound[T Ordered](r ClosedRange[T]) T { return r.Lo }
