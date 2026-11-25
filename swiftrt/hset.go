package swiftrt

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// HSet is a Swift `Set` whose element type cannot be a Go map key (an array, a set, or a struct
// holding one). Elements are keyed by a canonical rendering of their value, so equal values
// collapse as Swift's Hashable equality would.
type HSet[T any] struct{ M map[string]T }

func hkey(v any) string {
	var b strings.Builder
	writeKey(&b, reflect.ValueOf(v))
	return b.String()
}

// writeKey renders v canonically: equal Swift values render alike, so a nil and an empty
// collection, and -0.0 and 0.0, share a key. A class reference renders by address, as class identity does,
// and an Optional of a value type by its value.
func writeKey(b *strings.Builder, v reflect.Value) {
	if !v.IsValid() {
		b.WriteString("nil")
		return
	}
	if v.Type() == substringType {
		fmt.Fprintf(b, "%q", v.Interface().(Substring).String())
		return
	}
	if k, ok := customHashKey(v); ok {
		b.WriteString(k)
		return
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		b.WriteString("[")
		for i := 0; i < v.Len(); i++ {
			writeKey(b, v.Index(i))
			b.WriteString(",")
		}
		b.WriteString("]")
	case reflect.Map:
		keys := make([]string, 0, v.Len())
		for _, k := range v.MapKeys() {
			var kb strings.Builder
			writeKey(&kb, k)
			kb.WriteString(":")
			writeKey(&kb, v.MapIndex(k))
			keys = append(keys, kb.String())
		}
		slices.Sort(keys)
		b.WriteString("{" + strings.Join(keys, ",") + "}")
	case reflect.Struct:
		b.WriteString(v.Type().String() + "{")
		for _, field := range v.Fields() {
			writeKey(b, field)
			b.WriteString(",")
		}
		b.WriteString("}")
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if f == 0 {
			f = 0
		}
		fmt.Fprintf(b, "%v", f)
	case reflect.Interface:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		writeKey(b, v.Elem())
	case reflect.Pointer:
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		if !v.Type().Implements(swiftClassType) {
			// An Optional of a value type: its key is its value's.
			b.WriteString("?")
			writeKey(b, v.Elem())
			return
		}
		fmt.Fprintf(b, "0x%x", v.Pointer())
	case reflect.String:
		fmt.Fprintf(b, "%q", v.String())
	default:
		fmt.Fprintf(b, "%v", v)
	}
}

// HSetOf is a set literal.
func HSetOf[T any](xs ...T) HSet[T] {
	s := HSet[T]{M: map[string]T{}}
	for _, x := range xs {
		s.M[hkey(x)] = Copy(x)
	}
	return s
}

// HSetInitSlice is `Set(_:)` from an array.
func HSetInitSlice[T any](xs []T) HSet[T] { return HSetOf(xs...) }

// HSetInsert is `Set.insert(_:)`: whether v was new, and the member equal to v afterwards.
func HSetInsert[T any](s *HSet[T], v T) struct {
	F0 bool
	F1 T
} {
	if s.M == nil {
		s.M = map[string]T{}
	}
	k := hkey(v)
	if old, ok := s.M[k]; ok {
		return struct {
			F0 bool
			F1 T
		}{false, old}
	}
	s.M[k] = Copy(v)
	return struct {
		F0 bool
		F1 T
	}{true, v}
}

// HSetContains is `Set.contains(_:)`.
func HSetContains[T any](s HSet[T], v T) bool {
	_, ok := s.M[hkey(v)]
	return ok
}

// HSetRemove is `Set.remove(_:)`.
func HSetRemove[T any](s *HSet[T], v T) *T {
	k := hkey(v)
	old, ok := s.M[k]
	if !ok {
		return nil
	}
	delete(s.M, k)
	return &old
}

// HSetCount is `Set.count`.
func HSetCount[T any](s HSet[T]) int { return len(s.M) }

// HSetIsEmpty is `Set.isEmpty`.
func HSetIsEmpty[T any](s HSet[T]) bool { return len(s.M) == 0 }

// HSetRemoveAll is `Set.removeAll()`.
func HSetRemoveAll[T any](s *HSet[T]) { s.M = nil }

// HSetFormUnion is `Set.formUnion(_:)`.
func HSetFormUnion[T any](s *HSet[T], other any) {
	for _, x := range Iterate[T](other) {
		HSetInsert(s, x)
	}
}

// Elements lists the members, for iteration.
func (s HSet[T]) Elements() []T {
	out := make([]T, 0, len(s.M))
	for _, v := range s.M {
		out = append(out, v)
	}
	return out
}

// SetUpcast is a Set converted to a Set of a wider element type (Set<Race> as Set<AnyHashable>).
func SetUpcast[D ~map[K2]struct{}, K2, K1 comparable](s map[K1]struct{}) D {
	out := D{}
	for k := range s {
		out[any(k).(K2)] = struct{}{}
	}
	return out
}

// customHashKey renders a value whose type declares hash(into:) from what that method combines, so
// values its own == calls equal share a key.
func customHashKey(v reflect.Value) (string, bool) {
	if !v.CanInterface() || v.Kind() == reflect.Pointer && v.IsNil() {
		return "", false
	}
	if v.Kind() == reflect.Pointer {
		if _, class := v.Interface().(SwiftClass); !class {
			return "", false
		}
	}
	hv, ok := v.Interface().(interface{ Hash(*any) })
	if !ok {
		return "", false
	}
	hh := &Hasher{key: &strings.Builder{}}
	var h any = hh
	hv.Hash(&h)
	return v.Type().String() + "#" + hh.key.String(), true
}

// HSetFirst is `Set.first`: some element, or nil for an empty set. It takes the element whose key sorts first,
// so a run picks the same one each time.
func HSetFirst[T any](s HSet[T]) *T {
	var first *string
	for k := range s.M {
		if first == nil || k < *first {
			first = &k
		}
	}
	if first == nil {
		return nil
	}
	v := s.M[*first]
	return &v
}

// HSetInit is `Set()`: an empty set.
func HSetInit[T any]() HSet[T] { return HSet[T]{M: map[string]T{}} }

// HSetContainsWhere is `contains(where:)` on a hashed Set.
func HSetContainsWhere[T any](s HSet[T], f func(T) bool) bool {
	for _, v := range s.M {
		if f(v) {
			return true
		}
	}
	return false
}

// hsetKeys is the set of keys of other, a hashed Set or any sequence of T.
func hsetKeys[T any](other any) map[string]struct{} {
	if o, ok := other.(HSet[T]); ok {
		keys := make(map[string]struct{}, len(o.M))
		for k := range o.M {
			keys[k] = struct{}{}
		}
		return keys
	}
	keys := map[string]struct{}{}
	for _, x := range Iterate[T](other) {
		keys[hkey(x)] = struct{}{}
	}
	return keys
}

// HSetUnion is `Set.union(_:)`: a new set of the elements of s and other. Where both hold equal
// elements, the result holds s's.
func HSetUnion[T any](s HSet[T], other any) HSet[T] {
	out := Copy(s)
	if out.M == nil {
		out.M = map[string]T{}
	}
	HSetFormUnion(&out, other)
	return out
}

// HSetIntersection is `Set.intersection(_:)`: a new set of s's elements that other also holds.
func HSetIntersection[T any](s HSet[T], other any) HSet[T] {
	keys := hsetKeys[T](other)
	out := HSet[T]{M: map[string]T{}}
	for k, v := range s.M {
		if _, ok := keys[k]; ok {
			out.M[k] = Copy(v)
		}
	}
	return out
}

// HSetIntersectionHSet is `Set.intersection(_:)` with a Set argument.
func HSetIntersectionHSet[T any](s, other HSet[T]) HSet[T] { return HSetIntersection(s, other) }

// HSetFormIntersection is `Set.formIntersection(_:)`: s keeps only the elements other also holds.
func HSetFormIntersection[T any](s *HSet[T], other any) {
	keys := hsetKeys[T](other)
	for k := range s.M {
		if _, ok := keys[k]; !ok {
			delete(s.M, k)
		}
	}
}

// HSetSubtracting is `Set.subtracting(_:)`: a new set of s's elements that other does not hold.
func HSetSubtracting[T any](s HSet[T], other any) HSet[T] {
	out := Copy(s)
	HSetSubtract(&out, other)
	if out.M == nil {
		out.M = map[string]T{}
	}
	return out
}

// HSetSubtractingHSet is `Set.subtracting(_:)` with a Set argument.
func HSetSubtractingHSet[T any](s, other HSet[T]) HSet[T] { return HSetSubtracting(s, other) }

// HSetSubtract is `Set.subtract(_:)`: it removes other's elements from s.
func HSetSubtract[T any](s *HSet[T], other any) {
	for k := range hsetKeys[T](other) {
		delete(s.M, k)
	}
}

// HSetSubtractHSet is `Set.subtract(_:)` with a Set argument.
func HSetSubtractHSet[T any](s *HSet[T], other HSet[T]) { HSetSubtract(s, other) }

// HSetSymmetricDifference is `Set.symmetricDifference(_:)`: a new set of the elements in exactly one
// of s and other. An element other repeats counts once.
func HSetSymmetricDifference[T any](s HSet[T], other any) HSet[T] {
	out := Copy(s)
	HSetFormSymmetricDifference(&out, other)
	return out
}

// HSetFormSymmetricDifference is `Set.formSymmetricDifference(_:)`: s keeps the elements in exactly
// one of s and other.
func HSetFormSymmetricDifference[T any](s *HSet[T], other any) {
	o, ok := other.(HSet[T])
	if !ok {
		o = HSetInitSlice(Iterate[T](other))
	}
	if s.M == nil {
		s.M = map[string]T{}
	}
	for k, v := range o.M {
		if _, both := s.M[k]; both {
			delete(s.M, k)
		} else {
			s.M[k] = Copy(v)
		}
	}
}

// HSetFormSymmetricDifferenceHSet is `Set.formSymmetricDifference(_:)` with a Set argument.
func HSetFormSymmetricDifferenceHSet[T any](s *HSet[T], other HSet[T]) {
	HSetFormSymmetricDifference(s, other)
}

// HSetIsSubsetOf is `Set.isSubset(of:)`: whether other holds every element of s.
func HSetIsSubsetOf[T any](s HSet[T], other any) bool {
	keys := hsetKeys[T](other)
	for k := range s.M {
		if _, ok := keys[k]; !ok {
			return false
		}
	}
	return true
}

// HSetIsStrictSubsetOf is `Set.isStrictSubset(of:)`: whether s is a subset of other and other holds
// an element s does not.
func HSetIsStrictSubsetOf[T any](s HSet[T], other any) bool {
	return HSetIsSubsetOf(s, other) && len(hsetKeys[T](other)) > len(s.M)
}

// HSetIsSupersetOf is `Set.isSuperset(of:)`: whether s holds every element of other.
func HSetIsSupersetOf[T any](s HSet[T], other any) bool {
	for k := range hsetKeys[T](other) {
		if _, ok := s.M[k]; !ok {
			return false
		}
	}
	return true
}

// HSetIsStrictSupersetOf is `Set.isStrictSuperset(of:)`: whether s is a superset of other and holds
// an element other does not.
func HSetIsStrictSupersetOf[T any](s HSet[T], other any) bool {
	return HSetIsSupersetOf(s, other) && len(s.M) > len(hsetKeys[T](other))
}

// HSetIsDisjointWith is `Set.isDisjoint(with:)`: whether s and other share no element.
func HSetIsDisjointWith[T any](s HSet[T], other any) bool {
	for k := range hsetKeys[T](other) {
		if _, ok := s.M[k]; ok {
			return false
		}
	}
	return true
}

// HSetMap is `map` on a hashed Set: an array of f applied to each element.
func HSetMap[T, U any](s HSet[T], f func(T) U) []U { return SliceMap(s.Elements(), f) }

// HSetSorted is `Set.sorted()`, ordered by the element type's own `<`.
func HSetSorted[T any](s HSet[T]) []T { return SliceSortedBy(s.Elements(), Less[T]) }

// HSetSortedBy is `Set.sorted(by:)`.
func HSetSortedBy[T any](s HSet[T], less func(T, T) bool) []T {
	return SliceSortedBy(s.Elements(), less)
}
