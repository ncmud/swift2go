package swiftrt

import "reflect"

// A Swift array is a Go slice, and copies of an array may share its storage (see docs/COPY_ON_WRITE.md).
// A slice whose length equals its capacity may be sharing its storage, so nothing writes into it in
// place: Mut first gives it storage of its own. A slice with spare capacity holds its storage alone
// and is written in place.

// Mut returns *s ready to have its elements written in place, first moving *s to storage of its own if
// it may be sharing storage with copies of it. Every in-place write to a Swift array goes through it:
// an element assignment is `swiftrt.Mut(&xs)[i] = v`.
func Mut[S ~[]E, E any](s *S) S {
	if len(*s) == cap(*s) {
		unshare(s)
	}
	return *s
}

// unshare gives *s storage of its own with a spare element of capacity, which marks it as owned. An
// array of elements that are never shared (see [copyPlan.shareable]) is left where it is.
//
//go:noinline
func unshare[S ~[]E, E any](s *S) {
	n := len(*s)
	if n == 0 || !shareableElems(reflect.TypeFor[E]()) {
		return
	}
	own := make(S, n, n+1)
	copy(own, *s)
	*s = own
}

// part is s[lo:hi], a slice of an array that shares its storage. It is marked as sharing storage when s
// is, and otherwise keeps s's capacity past its end, which marks it as not sharing: copying it copies
// it, and appending to it reallocates. A part never claims storage another array holds as its own.
func part[S ~[]E, E any](s S, lo, hi int) S {
	if len(s) == cap(s) {
		return s[lo:hi:hi]
	}
	return s[lo:hi]
}

// shrink is s[:n] for an array that drops its tail. Storage that may be shared stays marked as such, so
// that a later append cannot write into elements that copies still hold.
func shrink[S ~[]E, E any](s S, n int) S {
	if len(s) == cap(s) {
		return s[:n:n]
	}
	return s[:n]
}

// MutPtr returns *p ready to be written through, after moving the value it points at to storage of its
// own. A value type's Optional is a pointer, and copies of an Optional share what it points at (Copy),
// so every write through one goes through MutPtr: `swiftrt.MutPtr(&opt).Field = v`. A pointer carries no
// mark of being owned, so each write moves the value; the returned pointer can be written repeatedly.
func MutPtr[T any](p **T) *T {
	if *p == nil {
		return nil
	}
	own := new(T)
	*own = **p
	*p = own
	return own
}

// OwnOptional is a copy of the Optional p that is its own to write through, for a write through an
// Optional that is not a place, such as a getter's result.
func OwnOptional[T any](p *T) *T {
	return MutPtr(&p)
}
