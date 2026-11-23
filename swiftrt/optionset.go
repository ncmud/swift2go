package swiftrt

import (
	"fmt"
	"reflect"
)

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

// AnyContains is `OptionSet.contains(_:)` on an existential option set: whether every bit of
// member is set in set. Both must hold the same option set type.
func AnyContains(set, member any) bool {
	a, b := reflect.ValueOf(set), reflect.ValueOf(member)
	if a.Kind() == reflect.Struct {
		a, b = a.FieldByName("RawValue"), b.FieldByName("RawValue")
	}
	switch a.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int()&b.Int() == b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint()&b.Uint() == b.Uint()
	}
	panic(fmt.Sprintf("swiftrt.AnyContains: %T is not an option set", set))
}

// AnyInsert is `OptionSet.insert(_:)` on a type parameter's option set: it sets member's bits in set and
// reports whether any of them was clear.
func AnyInsert[T any](set *T, member T) struct {
	F0 bool
	F1 T
} {
	had := AnyContains(*set, member)
	osBits(set, member, func(s, m uint64) uint64 { return s | m })
	return struct {
		F0 bool
		F1 T
	}{!had, member}
}

// AnyRemove is `OptionSet.remove(_:)` on a type parameter's option set: it clears member's bits in set
// and returns the bits of member that were set, or nil when none was.
func AnyRemove[T any](set *T, member T) *T {
	in := *set
	osBits(&in, member, func(s, m uint64) uint64 { return s & m })
	osBits(set, member, func(s, m uint64) uint64 { return s &^ m })
	if reflect.ValueOf(AnyRawValue(in)).IsZero() {
		return nil
	}
	return &in
}

// AnyRawValue is `rawValue` of a type parameter's value: an option set's raw bits or a raw-value enum's raw value.
func AnyRawValue(v any) any {
	if r, ok := v.(interface{ RawValue() any }); ok {
		return r.RawValue()
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct {
		if f := rv.FieldByName("RawValue"); f.IsValid() {
			return f.Interface()
		}
	}
	if m := rv.MethodByName("RawValue"); m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
		return m.Call(nil)[0].Interface()
	}
	panic(fmt.Sprintf("swiftrt.AnyRawValue: %T has no raw value", v))
}

// osBits replaces the raw bits of the option set *set with op of them and member's.
func osBits[T any](set *T, member T, op func(s, m uint64) uint64) {
	s, m := reflect.ValueOf(set).Elem().FieldByName("RawValue"), reflect.ValueOf(member).FieldByName("RawValue")
	if !s.IsValid() || !m.IsValid() {
		panic(fmt.Sprintf("swiftrt: %T is not an option set", member))
	}
	switch s.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s.SetInt(int64(op(uint64(s.Int()), uint64(m.Int()))))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s.SetUint(op(s.Uint(), m.Uint()))
	default:
		panic(fmt.Sprintf("swiftrt: %T is not an option set", member))
	}
}
