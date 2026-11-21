package swiftrt

import (
	"math"
	"math/bits"
	"os"
	"reflect"
	"time"
)

// FloatInfinity is `Double.infinity`.
var FloatInfinity = math.Inf(1)

// FloatGreatestFiniteMagnitude is `Double.greatestFiniteMagnitude`.
const FloatGreatestFiniteMagnitude = math.MaxFloat64

// LogFloat is Foundation `log(Double)`.
func LogFloat(x float64) float64 { return math.Log(x) }

// Log2Float is Foundation `log2(Double)`.
func Log2Float(x float64) float64 { return math.Log2(x) }

// FloatingPointRoundingRule is Swift `FloatingPointRoundingRule`.
type FloatingPointRoundingRule int

// FloatingPointRoundingRule cases.
const (
	FloatingPointRoundingRuleToNearestOrAwayFromZero FloatingPointRoundingRule = iota
	FloatingPointRoundingRuleToNearestOrEven
	FloatingPointRoundingRuleUp
	FloatingPointRoundingRuleDown
	FloatingPointRoundingRuleTowardZero
	FloatingPointRoundingRuleAwayFromZero
)

// FloatRoundedFloatingPointRoundingRule is Swift `FloatingPoint.rounded(_:)`.
func FloatRoundedFloatingPointRoundingRule[F ~float32 | ~float64](x F, rule any) F {
	v := float64(x)
	switch rule.(FloatingPointRoundingRule) {
	case FloatingPointRoundingRuleToNearestOrAwayFromZero:
		return F(math.Round(v))
	case FloatingPointRoundingRuleToNearestOrEven:
		return F(math.RoundToEven(v))
	case FloatingPointRoundingRuleUp:
		return F(math.Ceil(v))
	case FloatingPointRoundingRuleDown:
		return F(math.Floor(v))
	case FloatingPointRoundingRuleTowardZero:
		return F(math.Trunc(v))
	default:
		if v < 0 {
			return F(math.Floor(v))
		}
		return F(math.Ceil(v))
	}
}

// IntOptInitExactly is Swift `IntN(exactly:)` from a generic BinaryInteger or BinaryFloatingPoint.
func IntOptInitExactly[T Integer](v any) *T {
	rv := reflect.ValueOf(v)
	lo, hi, unsigned, umax := bounds[T]()
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := rv.Int()
		if (unsigned && (n < 0 || uint64(n) > umax)) || (!unsigned && (n < lo || n > hi)) {
			return nil
		}
		t := T(n)
		return &t
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n := rv.Uint()
		if (unsigned && n > umax) || (!unsigned && n > uint64(hi)) {
			return nil
		}
		t := T(n)
		return &t
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if f != math.Trunc(f) || math.IsInf(f, 0) {
			return nil
		}
		if (unsigned && (f < 0 || f >= math.Ldexp(1, bitWidth[T]()))) || (!unsigned && (f < float64(lo) || f >= -float64(lo))) {
			return nil
		}
		t := T(f)
		return &t
	}
	panic("swiftrt.IntOptInitExactly: not a number: " + rv.Type().String())
}

// IntInitAny is Swift `IntN(x)` from a generic BinaryInteger; it traps when x does not fit.
func IntInitAny[T Integer](v any) T {
	p := IntOptInitExactly[T](v)
	if p == nil {
		Trap("integer conversion overflow")
	}
	return *p
}

// IntOptInitInt is Swift `Optional(x)`.
func IntOptInitInt[T any](v T) *T { return &v }

// IntSignum is Swift `BinaryInteger.signum()`.
func IntSignum[T Integer](v T) T {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return T(0) - 1
	}
	return 0
}

// IntIsMultipleOf is Swift `BinaryInteger.isMultiple(of:)`.
func IntIsMultipleOf[T Integer](v, of T) bool {
	if of == 0 {
		return v == 0
	}
	if _, _, unsigned, _ := bounds[T](); !unsigned && of == T(0)-1 {
		return true
	}
	return v%of == 0
}

// IntSubtractingReportingOverflowInt is Swift `subtractingReportingOverflow(_:)`.
func IntSubtractingReportingOverflowInt[T Integer](a, b T) struct {
	F0 T
	F1 bool
} {
	r := a - b
	_, _, unsigned, _ := bounds[T]()
	over := (unsigned && b > a) || (!unsigned && ((b > 0 && r > a) || (b < 0 && r < a)))
	return struct {
		F0 T
		F1 bool
	}{r, over}
}

// IntLeadingZeroBitCount is Swift `FixedWidthInteger.leadingZeroBitCount`.
func IntLeadingZeroBitCount[T Integer](v T) int {
	w := bitWidth[T]()
	return bits.LeadingZeros64(uint64(v)&(math.MaxUint64>>(64-w))) - (64 - w)
}

// IntMultipliedFullWidthBy is Swift `multipliedFullWidth(by:)` for 64-bit integers: (high, low).
func IntMultipliedFullWidthBy[T Integer](a, b T) struct {
	F0 T
	F1 uint64
} {
	if bitWidth[T]() != 64 {
		panic("swiftrt.IntMultipliedFullWidthBy: only 64-bit integers are supported")
	}
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if _, _, unsigned, _ := bounds[T](); !unsigned {
		if a < 0 {
			hi -= uint64(b)
		}
		if b < 0 {
			hi -= uint64(a)
		}
	}
	return struct {
		F0 T
		F1 uint64
	}{T(hi), lo}
}

// IntDividingFullWidth is Swift `dividingFullWidth(_:)` for 64-bit integers: (quotient, remainder).
func IntDividingFullWidth[T Integer](d T, n struct {
	F0 T
	F1 uint64
}) struct {
	F0 T
	F1 T
} {
	if bitWidth[T]() != 64 {
		panic("swiftrt.IntDividingFullWidth: only 64-bit integers are supported")
	}
	if d == 0 {
		Trap("Division by zero")
	}
	_, _, unsigned, _ := bounds[T]()
	if unsigned {
		if uint64(n.F0) >= uint64(d) {
			Trap("Quotient is not representable")
		}
		q, r := bits.Div64(uint64(n.F0), n.F1, uint64(d))
		return struct {
			F0 T
			F1 T
		}{T(q), T(r)}
	}
	negN, negD := n.F0 < 0, d < 0
	hi, lo := uint64(n.F0), n.F1
	if negN {
		lo = -lo
		hi = ^hi
		if lo == 0 {
			hi++
		}
	}
	ud := uint64(d)
	if negD {
		ud = -ud
	}
	if hi >= ud {
		Trap("Quotient is not representable")
	}
	q, r := bits.Div64(hi, lo, ud)
	negQ := negN != negD
	if (negQ && q > 1<<63) || (!negQ && q >= 1<<63) {
		Trap("Quotient is not representable")
	}
	if negQ {
		q = -q
	}
	if negN {
		r = -r
	}
	return struct {
		F0 T
		F1 T
	}{T(q), T(r)}
}

// DurationComponents is Swift `Duration.components`: (seconds, attoseconds).
func DurationComponents(d any) struct {
	F0 int64
	F1 int64
} {
	ns := int64(d.(Duration))
	return struct {
		F0 int64
		F1 int64
	}{ns / 1e9, (ns % 1e9) * 1e9}
}

// InstantDurationTo is Swift `ContinuousClock.Instant.duration(to:)`.
func InstantDurationTo(a, b ContinuousClockInstant) Duration {
	return Duration(time.Time(b).Sub(time.Time(a)))
}

// NSThreadCObjcForTimeInterval is Foundation `Thread.sleep(forTimeInterval:)`.
func NSThreadCObjcForTimeInterval(s float64) { time.Sleep(time.Duration(s * 1e9)) }

// CFTime is C `time(nil)`.
func CFTime(_ any) int { return int(time.Now().Unix()) }

// CFGetpid is C `getpid()`.
func CFGetpid() int32 { return int32(os.Getpid()) }
