package swiftrt

// OptSome is Swift `Optional.some(_:)` used as a function: the value wrapped as a present optional.
func OptSome[T any](v T) *T { return &v }

// RefSome is Swift `Optional.some(_:)` around a value whose Go form is itself nil-able, such as an
// optional or a class reference: the outer optional is present even when the value inside is nil.
func RefSome[T any](v T) *T { return &v }

// OptMap is Swift `Optional.map`.
func OptMap[T, U any](p *T, f func(T) U) *U {
	if p == nil {
		return nil
	}
	v := f(*p)
	return &v
}

// OptFlatMap is Swift `Optional.flatMap`.
func OptFlatMap[T, U any](p *T, f func(T) *U) *U {
	if p == nil {
		return nil
	}
	return f(*p)
}

// RefMap is Swift `Optional.map` on an optional whose Go form is itself nil-able: a class
// reference (a bare pointer), a protocol existential, a closure, or a type parameter.
func RefMap[T any, U any](p T, f func(T) U) *U {
	if isNil(p) {
		return nil
	}
	v := f(p)
	return &v
}

// RefFlatMap is Swift `Optional.flatMap` on an optional whose Go form is itself nil-able.
func RefFlatMap[T any, U any](p T, f func(T) *U) *U {
	if isNil(p) {
		return nil
	}
	return f(p)
}

// RefMapAny is Swift `Optional.map` on a protocol existential.
func RefMapAny[T any, U any](p T, f func(T) U) *U {
	if isNil(p) {
		return nil
	}
	v := f(p)
	return &v
}

// OptDo is `optional.map(f)` with a Void closure: it runs f on the value, if any, and returns `()?`.
func OptDo[T any](p *T, f func(T)) *struct{} {
	if p == nil {
		return nil
	}
	f(*p)
	return &struct{}{}
}

// OptDoRef is OptDo for an Optional reference, which is the reference itself: it runs f on v
// unless v is nil.
func OptDoRef[T any](v T, f func(T)) *struct{} {
	if isNil(v) {
		return nil
	}
	f(v)
	return &struct{}{}
}

// OptionalExistential is the Optional existential P, whose Go form is the interface, of v, an Optional
// whose Go form is a pointer: a class reference, or a pointer to a value: nil when v is nil, where Go's conversion would make an
// interface that holds a nil pointer.
func OptionalExistential[P any, T any](v *T) P {
	var zero P
	if v == nil {
		return zero
	}
	if _, class := any(v).(interface{ SwiftClass() }); class {
		return any(v).(P) //nolint:forcetypeassert // generated code calls this for a v that implements P
	}
	// The Optional of a value: the existential holds the value.
	return any(*v).(P) //nolint:forcetypeassert // generated code calls this for a v that implements P
}
