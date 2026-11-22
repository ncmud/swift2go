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
