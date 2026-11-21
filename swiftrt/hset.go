package swiftrt

import "fmt"

// HSet is a Swift `Set` whose element type cannot be a Go map key (an array, a set, or a struct
// holding one). Elements are keyed by a canonical rendering of their value, so equal values
// collapse as Swift's Hashable equality would.
type HSet[T any] struct{ m map[string]T }

func hkey(v any) string { return fmt.Sprintf("%#v", v) }

// HSetOf is a set literal.
func HSetOf[T any](xs ...T) HSet[T] {
	s := HSet[T]{m: map[string]T{}}
	for _, x := range xs {
		s.m[hkey(x)] = x
	}
	return s
}

// HSetInsert is `Set.insert(_:)`: whether v was new, and the member equal to v afterwards.
func HSetInsert[T any](s *HSet[T], v T) struct {
	F0 bool
	F1 T
} {
	if s.m == nil {
		s.m = map[string]T{}
	}
	k := hkey(v)
	if old, ok := s.m[k]; ok {
		return struct {
			F0 bool
			F1 T
		}{false, old}
	}
	s.m[k] = v
	return struct {
		F0 bool
		F1 T
	}{true, v}
}

// HSetContains is `Set.contains(_:)`.
func HSetContains[T any](s HSet[T], v T) bool {
	_, ok := s.m[hkey(v)]
	return ok
}

// HSetRemove is `Set.remove(_:)`.
func HSetRemove[T any](s *HSet[T], v T) *T {
	k := hkey(v)
	old, ok := s.m[k]
	if !ok {
		return nil
	}
	delete(s.m, k)
	return &old
}

// HSetCount is `Set.count`.
func HSetCount[T any](s HSet[T]) int { return len(s.m) }

// HSetIsEmpty is `Set.isEmpty`.
func HSetIsEmpty[T any](s HSet[T]) bool { return len(s.m) == 0 }

// HSetRemoveAll is `Set.removeAll()`.
func HSetRemoveAll[T any](s *HSet[T]) { s.m = nil }

// HSetFormUnion is `Set.formUnion(_:)`.
func HSetFormUnion[T any](s *HSet[T], other any) {
	for _, x := range Iterate[T](other) {
		HSetInsert(s, x)
	}
}

// Elements lists the members, for iteration.
func (s HSet[T]) Elements() []T {
	out := make([]T, 0, len(s.m))
	for _, v := range s.m {
		out = append(out, v)
	}
	return out
}
