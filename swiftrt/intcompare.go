package swiftrt

// IntCompare is the heterogeneous `BinaryInteger` comparison (`Int32(x) > someInt`): it orders
// two integers of different types by mathematical value, with no truncation or sign wrap.
func IntCompare[A, B Integer](a A, b B) int {
	aNeg, bNeg := a < 0, b < 0
	switch {
	case aNeg && !bNeg:
		return -1
	case !aNeg && bNeg:
		return 1
	case aNeg:
		x, y := int64(a), int64(b)
		return cmpOrdered(x, y)
	}
	x, y := uint64(a), uint64(b)
	return cmpOrdered(x, y)
}

// IntEqual is the heterogeneous `BinaryInteger` `==`.
func IntEqual[A, B Integer](a A, b B) bool { return IntCompare(a, b) == 0 }

func cmpOrdered[T int64 | uint64](x, y T) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}
