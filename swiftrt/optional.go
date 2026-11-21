package swiftrt

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

// RefMap is Swift `Optional.map` on a class reference, which Go represents as a bare pointer.
func RefMap[T any, U any](p *T, f func(*T) U) *U {
	if p == nil {
		return nil
	}
	v := f(p)
	return &v
}

// RefFlatMap is Swift `Optional.flatMap` on a class reference.
func RefFlatMap[T any, U any](p *T, f func(*T) *U) *U {
	if p == nil {
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
