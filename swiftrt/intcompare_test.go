package swiftrt

import (
	"math"
	"testing"
)

func TestIntCompare(t *testing.T) {
	if IntCompare(int32(5), 27) >= 0 || IntCompare(int32(30), 27) <= 0 || IntCompare(int8(-1), uint64(math.MaxUint64)) >= 0 {
		t.Fatal("ordering")
	}
	if IntCompare(uint64(math.MaxUint64), int64(-1)) <= 0 || !IntEqual(uint8(7), int64(7)) {
		t.Fatal("sign handling")
	}
}

func TestMaxOfMinOf(t *testing.T) {
	if MaxOf([]int{3, 9, 4}, []int{7}) != 9 || MinOf([]int{3, 9, 4}, nil) != 3 {
		t.Fatal("variadic max/min")
	}
}
