package swiftrt

import (
	"math/big"
	"reflect"
)

// Int128 is Swift `Int128`. It is an immutable value: every operation returns a new one.
type Int128 struct{ v *big.Int }

var (
	int128Min = new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 127))
	int128Max = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
)

func (a Int128) big() *big.Int {
	if a.v == nil {
		return new(big.Int)
	}
	return a.v
}

func int128Checked(v *big.Int, op string) Int128 {
	if v.Cmp(int128Min) < 0 || v.Cmp(int128Max) > 0 {
		panic("Fatal error: arithmetic overflow in Int128 " + op)
	}
	return Int128{v}
}

// String is Int128's decimal description.
func (a Int128) String() string { return a.big().String() }

// Int128Of is an Int128 integer literal.
func Int128Of(v int64) Int128 { return Int128{big.NewInt(v)} }

// Int128Parse is an Int128 integer literal too large for int64.
func Int128Parse(s string) Int128 {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("swift2go: bad Int128 literal " + s)
	}
	return int128Checked(v, "literal")
}

// Int128InitInt is `Int128(_:)` from a fixed-width integer.
func Int128InitInt[T Integer](v T) Int128 {
	if reflect.ValueOf(v).CanInt() {
		return Int128{big.NewInt(reflect.ValueOf(v).Int())}
	}
	return Int128{new(big.Int).SetUint64(reflect.ValueOf(v).Uint())}
}

// Int128Add is Int128 `+`, which traps on overflow.
func Int128Add(a, b Int128) Int128 { return int128Checked(new(big.Int).Add(a.big(), b.big()), "+") }

// Int128Sub is Int128 `-`, which traps on overflow.
func Int128Sub(a, b Int128) Int128 { return int128Checked(new(big.Int).Sub(a.big(), b.big()), "-") }

// Int128Mul is Int128 `*`, which traps on overflow.
func Int128Mul(a, b Int128) Int128 { return int128Checked(new(big.Int).Mul(a.big(), b.big()), "*") }

// Int128Div is Int128 `/`, truncating toward zero; dividing by zero traps.
func Int128Div(a, b Int128) Int128 {
	if b.big().Sign() == 0 {
		panic("Fatal error: Division by zero")
	}
	return int128Checked(new(big.Int).Quo(a.big(), b.big()), "/")
}

// Int128Rem is Int128 `%`, with the sign of the dividend; dividing by zero traps.
func Int128Rem(a, b Int128) Int128 {
	if b.big().Sign() == 0 {
		panic("Fatal error: Division by zero in remainder operation")
	}
	return Int128{new(big.Int).Rem(a.big(), b.big())}
}

// Int128Neg is Int128 prefix `-`.
func Int128Neg(a Int128) Int128 { return int128Checked(new(big.Int).Neg(a.big()), "negation") }

// Int128Compare orders two Int128 values: -1, 0 or 1.
func Int128Compare(a, b Int128) int { return a.big().Cmp(b.big()) }

func intBounds[R Integer]() (*big.Int, *big.Int) {
	var r R
	t := reflect.TypeOf(r)
	bits := uint(t.Bits())
	if reflect.ValueOf(r).CanInt() {
		lo := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), bits-1))
		hi := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), bits-1), big.NewInt(1))
		return lo, hi
	}
	return new(big.Int), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), bits), big.NewInt(1))
}

func bigTo[R Integer](v *big.Int) R {
	var r R
	if reflect.ValueOf(r).CanInt() {
		return R(v.Int64())
	}
	return R(v.Uint64())
}

// Int128ToIntClamping is `Int(clamping:)` (or another width) from an Int128.
func Int128ToIntClamping[R Integer](a Int128) R {
	lo, hi := intBounds[R]()
	v := a.big()
	switch {
	case v.Cmp(lo) < 0:
		return bigTo[R](lo)
	case v.Cmp(hi) > 0:
		return bigTo[R](hi)
	}
	return bigTo[R](v)
}

// Int128ToInt is `Int(_:)` (or another width) from an Int128, which traps when the value does not fit.
func Int128ToInt[R Integer](a Int128) R {
	lo, hi := intBounds[R]()
	if v := a.big(); v.Cmp(lo) < 0 || v.Cmp(hi) > 0 {
		panic("Fatal error: Not enough bits to represent the passed value")
	}
	return bigTo[R](a.big())
}

// Int128Less is Int128 `<`.
func Int128Less(a, b Int128) bool { return Int128Compare(a, b) < 0 }
