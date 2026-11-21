package swiftrt

import "reflect"

// Type is a Swift metatype value such as `Foo.self`.
type Type interface {
	Reflect() reflect.Type
	Description() string
}

// TypeT is the metatype of T; generic code infers T from it.
type TypeT[T any] struct{}

// Reflect returns the Go type.
func (TypeT[T]) Reflect() reflect.Type { return reflect.TypeFor[T]() }

// Description is the Swift spelling of the type's name.
func (TypeT[T]) Description() string { return typeName(reflect.TypeFor[T]()) }

// TypeOf is Swift `T.self`.
func TypeOf[T any]() TypeT[T] { return TypeT[T]{} }

type dynType struct{ t reflect.Type }

func (d dynType) Reflect() reflect.Type { return d.t }
func (d dynType) Description() string   { return typeName(d.t) }

// TypeOfValue is Swift `type(of:)`.
func TypeOfValue(v any) Type { return dynType{reflect.TypeOf(Dynamic(v))} }

func typeName(t reflect.Type) string {
	if t == nil {
		return "nil"
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}
