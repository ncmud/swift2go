package swiftrt

import (
	"math/big"
	"math/bits"
	"strings"
)

// Decimal is Foundation `Decimal`: a sign, a 128-bit mantissa and a base-10 exponent in
// -128...127. Values are kept compacted, trailing mantissa zeros moved into the exponent and zero
// held with exponent 0, so two Decimals are `==` exactly when their fields are equal.
type Decimal struct {
	neg    bool
	hi, lo uint64
	exp    int32
	nan    bool
}

// DecimalNan is Foundation `Decimal.nan`.
var DecimalNan = Decimal{nan: true}

// mulAdd10 returns m*10+d, or ok false when the result does not fit in 128 bits.
func decimalMulAdd10(hi, lo uint64, d uint64) (uint64, uint64, bool) {
	h1, l := bits.Mul64(lo, 10)
	h2, h := bits.Mul64(hi, 10)
	if h2 != 0 {
		return 0, 0, false
	}
	h, c := bits.Add64(h, h1, 0)
	if c != 0 {
		return 0, 0, false
	}
	l, c = bits.Add64(l, d, 0)
	h, c = bits.Add64(h, 0, c)
	if c != 0 {
		return 0, 0, false
	}
	return h, l, true
}

// compact moves trailing mantissa zeros into the exponent while it stays at most 127, and gives zero
// exponent 0 and a positive sign.
func (d Decimal) compact() Decimal {
	if d.nan {
		return d
	}
	if d.hi == 0 && d.lo == 0 {
		return Decimal{}
	}
	for d.exp < 127 {
		q1, r1 := bits.Div64(0, d.hi, 10)
		q0, r := bits.Div64(r1, d.lo, 10)
		if r != 0 {
			break
		}
		d.hi, d.lo = q1, q0
		d.exp++
	}
	return d
}

func isDecimalSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// NSDecimalOptInitString is Foundation `Decimal(string:)`. Like Foundation it reads the longest
// prefix it can: leading ASCII whitespace, a sign, digits with an optional `.` fraction and an
// optional `e` exponent, so `"12abc"` is 12 and `"-"` is 0. It returns nil when no character is
// consumed or the exponent falls outside -128...127. Digits beyond the 128-bit mantissa are dropped,
// raising the exponent for integer digits.
func NSDecimalOptInitString(s string) *Decimal {
	i := 0
	for i < len(s) && isDecimalSpace(s[i]) {
		i++
	}
	var d Decimal
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		d.neg = s[i] == '-'
		i++
	}
	exp := 0
	full := false
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		if !full {
			if h, l, ok := decimalMulAdd10(d.hi, d.lo, uint64(s[i]-'0')); ok {
				d.hi, d.lo = h, l
				continue
			}
			full = true
		}
		exp++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
			if full {
				continue
			}
			if h, l, ok := decimalMulAdd10(d.hi, d.lo, uint64(s[i]-'0')); ok {
				d.hi, d.lo = h, l
				exp--
			} else {
				full = true
			}
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		eneg := false
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			eneg = s[i] == '-'
			i++
		}
		e := 0
		for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
			if e < 1<<20 {
				e = e*10 + int(s[i]-'0')
			}
		}
		if eneg {
			e = -e
		}
		exp += e
	}
	if i == 0 {
		return nil
	}
	if exp < -128 || exp > 127 {
		return nil
	}
	d.exp = int32(exp)
	d = d.compact()
	return &d
}

// DecimalOptInitString is Foundation `Decimal(string:)`; see NSDecimalOptInitString.
func DecimalOptInitString(s string) *Decimal { return NSDecimalOptInitString(s) }

func (d Decimal) mantissa() *big.Int {
	m := new(big.Int).SetUint64(d.hi)
	m.Lsh(m, 64)
	return m.Or(m, new(big.Int).SetUint64(d.lo))
}

// Description is Foundation `Decimal.description`: plain notation with no exponent, a leading `-`
// for negative values, and `NaN` for NaN.
func (d Decimal) Description() string {
	if d.nan {
		return "NaN"
	}
	digits := d.mantissa().String()
	var out string
	switch {
	case d.hi == 0 && d.lo == 0:
		out = "0"
	case d.exp >= 0:
		out = digits + strings.Repeat("0", int(d.exp))
	default:
		point := len(digits) + int(d.exp)
		if point <= 0 {
			out = "0." + strings.Repeat("0", -point) + digits
		} else {
			out = digits[:point] + "." + digits[point:]
		}
	}
	if d.neg {
		return "-" + out
	}
	return out
}

// DecimalIsNaN is Foundation `Decimal.isNaN`.
func DecimalIsNaN(d Decimal) bool { return d.nan }

// DecimalExponent is Foundation `Decimal.exponent`.
func DecimalExponent(d Decimal) int { return int(d.exp) }

// DecimalInitInt is Foundation `Decimal(_: Int)`.
func DecimalInitInt[T Integer](v T) Decimal {
	var d Decimal
	if v < 0 {
		d.neg = true
		d.lo = uint64(-int64(v))
	} else {
		d.lo = uint64(v)
	}
	return d.compact()
}

// DecimalLess is Foundation `Decimal <`. NaN compares less than every number.
func DecimalLess(a, b Decimal) bool {
	if a.nan || b.nan {
		return a.nan && !b.nan
	}
	return a.rat().Cmp(b.rat()) < 0
}

func (d Decimal) rat() *big.Rat {
	r := new(big.Rat).SetInt(d.mantissa())
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs32(d.exp))), nil)
	if d.exp >= 0 {
		r.Mul(r, new(big.Rat).SetInt(scale))
	} else {
		r.Quo(r, new(big.Rat).SetInt(scale))
	}
	if d.neg {
		r.Neg(r)
	}
	return r
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
