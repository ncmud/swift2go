package swiftrt

import (
	"math"
	"testing"
)

func TestInt128(t *testing.T) {
	max := Int128InitInt(math.MaxInt64)
	sum := Int128Add(Int128Mul(max, Int128Of(3)), Int128Of(7))
	if Int128Compare(sum, max) <= 0 || Int128ToIntClamping[int](sum) != math.MaxInt64 {
		t.Fatal("clamping above")
	}
	if Int128ToIntClamping[int](Int128Neg(sum)) != math.MinInt64 || Int128ToIntClamping[uint8](Int128Of(-4)) != 0 {
		t.Fatal("clamping below")
	}
	if Int128Div(Int128Of(-7), Int128Of(2)).String() != "-3" || Int128Rem(Int128Of(-7), Int128Of(2)).String() != "-1" {
		t.Fatal("truncating division")
	}
	if Int128Compare(Int128InitInt(uint64(math.MaxUint64)), Int128Parse("18446744073709551615")) != 0 {
		t.Fatal("unsigned init")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected overflow trap")
		}
	}()
	Int128Mul(Int128Parse("170141183460469231731687303715884105727"), Int128Of(2))
}
