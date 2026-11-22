package swiftrt

import (
	"fmt"
	"reflect"
	"sort"
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
// collection, and -0.0 and 0.0, share a key. Pointers render by address, as class identity does.
func writeKey(b *strings.Builder, v reflect.Value) {
	if !v.IsValid() {
		b.WriteString("nil")
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
		sort.Strings(keys)
		b.WriteString("{" + strings.Join(keys, ",") + "}")
	case reflect.Struct:
		b.WriteString(v.Type().String() + "{")
		for i := 0; i < v.NumField(); i++ {
			writeKey(b, v.Field(i))
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
		s.M[hkey(x)] = x
	}
	return s
}

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
	s.M[k] = v
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
