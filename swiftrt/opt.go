package swiftrt

// Opt is a Swift Optional of a value type held inline, so returning one does not allocate. V is
// the zero value when Ok is false.
type Opt[T any] struct {
	V  T
	Ok bool
}

// OptOf is the Opt holding *p, or none when p is nil.
func OptOf[T any](p *T) Opt[T] {
	if p == nil {
		return Opt[T]{}
	}
	return Opt[T]{V: *p, Ok: true}
}

// OptValue is the present Opt holding v.
func OptValue[T any](v T) Opt[T] { return Opt[T]{V: v, Ok: true} }

// Ptr is o as a pointer optional: nil when o is none, else a pointer to a copy of its value.
func (o Opt[T]) Ptr() *T {
	if !o.Ok {
		return nil
	}
	return &o.V
}

// OptEqual is Swift == on two Optionals of a type whose Go == is Swift's.
func OptEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
