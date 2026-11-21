package swiftrt

// OptionSetOf is an OptionSet array literal.
func OptionSetOf[T ~struct{ RawValue R }, R Integer](ms ...T) T {
	var r R
	for _, m := range ms {
		r |= struct{ RawValue R }(m).RawValue
	}
	return T(struct{ RawValue R }{r})
}

func osRaw[T ~struct{ RawValue R }, R Integer](t T) R { return struct{ RawValue R }(t).RawValue }

func osMake[T ~struct{ RawValue R }, R Integer](r R) T { return T(struct{ RawValue R }{r}) }

// OptionSetInit is `Flags()`.
func OptionSetInit[T ~struct{ RawValue R }, R Integer]() T { return osMake[T](R(0)) }

// OptionSetContains is OptionSet `contains`.
func OptionSetContains[T ~struct{ RawValue R }, R Integer](s, m T) bool {
	return osRaw(s)&osRaw(m) == osRaw(m)
}

// OptionSetIsEmpty is OptionSet `isEmpty`.
func OptionSetIsEmpty[T ~struct{ RawValue R }, R Integer](s T) bool { return osRaw(s) == 0 }

// OptionSetUnion is OptionSet `union`.
func OptionSetUnion[T ~struct{ RawValue R }, R Integer](s, o T) T {
	return osMake[T](osRaw(s) | osRaw(o))
}

// OptionSetIntersection is OptionSet `intersection`.
func OptionSetIntersection[T ~struct{ RawValue R }, R Integer](s, o T) T {
	return osMake[T](osRaw(s) & osRaw(o))
}

// OptionSetSymmetricDifference is OptionSet `symmetricDifference`.
func OptionSetSymmetricDifference[T ~struct{ RawValue R }, R Integer](s, o T) T {
	return osMake[T](osRaw(s) ^ osRaw(o))
}

// OptionSetSubtracting is OptionSet `subtracting`.
func OptionSetSubtracting[T ~struct{ RawValue R }, R Integer](s, o T) T {
	return osMake[T](osRaw(s) &^ osRaw(o))
}

// OptionSetIsDisjointWith is OptionSet `isDisjoint(with:)`.
func OptionSetIsDisjointWith[T ~struct{ RawValue R }, R Integer](s, o T) bool {
	return osRaw(s)&osRaw(o) == 0
}

// OptionSetIsSubsetOf is OptionSet `isSubset(of:)`.
func OptionSetIsSubsetOf[T ~struct{ RawValue R }, R Integer](s, o T) bool {
	return osRaw(s)&^osRaw(o) == 0
}

// OptionSetIsSupersetOf is OptionSet `isSuperset(of:)`.
func OptionSetIsSupersetOf[T ~struct{ RawValue R }, R Integer](s, o T) bool {
	return osRaw(o)&^osRaw(s) == 0
}

// OptionSetInsert is OptionSet `insert`.
func OptionSetInsert[T ~struct{ RawValue R }, R Integer](s *T, m T) struct {
	F0 bool
	F1 T
} {
	had := OptionSetContains(*s, m)
	*s = OptionSetUnion(*s, m)
	return struct {
		F0 bool
		F1 T
	}{!had, m}
}

// OptionSetRemove is OptionSet `remove`.
func OptionSetRemove[T ~struct{ RawValue R }, R Integer](s *T, m T) *T {
	in := OptionSetIntersection(*s, m)
	*s = OptionSetSubtracting(*s, m)
	if osRaw(in) == 0 {
		return nil
	}
	return &in
}

// OptionSetUpdateWith is OptionSet `update(with:)`.
func OptionSetUpdateWith[T ~struct{ RawValue R }, R Integer](s *T, m T) *T {
	in := OptionSetIntersection(*s, m)
	*s = OptionSetUnion(*s, m)
	if osRaw(in) == 0 {
		return nil
	}
	return &in
}

// OptionSetFormUnion is OptionSet `formUnion`.
func OptionSetFormUnion[T ~struct{ RawValue R }, R Integer](s *T, o T) { *s = OptionSetUnion(*s, o) }

// OptionSetFormIntersection is OptionSet `formIntersection`.
func OptionSetFormIntersection[T ~struct{ RawValue R }, R Integer](s *T, o T) {
	*s = OptionSetIntersection(*s, o)
}

// OptionSetFormSymmetricDifference is OptionSet `formSymmetricDifference`.
func OptionSetFormSymmetricDifference[T ~struct{ RawValue R }, R Integer](s *T, o T) {
	*s = OptionSetSymmetricDifference(*s, o)
}

// OptionSetSubtract is OptionSet `subtract`.
func OptionSetSubtract[T ~struct{ RawValue R }, R Integer](s *T, o T) {
	*s = OptionSetSubtracting(*s, o)
}
