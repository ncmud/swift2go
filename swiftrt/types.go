package swiftrt

import (
	"reflect"
	"strings"
)

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

// dynType is the dynamic type of a value. A payload enum's value is a Go type of its case, so it
// carries the enum's Swift name for its description.
type dynType struct {
	t    reflect.Type
	name string
}

func (d dynType) Reflect() reflect.Type { return d.t }

func (d dynType) Description() string {
	if d.name != "" {
		return d.name
	}
	return typeName(d.t)
}

// TypeOfValue is Swift `type(of:)`.
func TypeOfValue(v any) Type {
	v = Dynamic(v)
	if _, ok := v.(SwiftCase); ok {
		if n, ok := v.(SwiftNamed); ok {
			name := n.SwiftTypeName()
			return dynType{reflect.TypeOf(v), name[strings.LastIndexByte(name, '.')+1:]}
		}
	}
	return dynType{t: reflect.TypeOf(v)}
}

func typeName(t reflect.Type) string {
	if t == nil {
		return "nil"
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	// A generated type knows its Swift name; a nested type's Go name is its owners' and its own run together.
	if t.Kind() != reflect.Interface && t.Name() != "" {
		if n, ok := reflect.New(t).Interface().(SwiftNamed); ok {
			name := n.SwiftTypeName()
			return name[strings.LastIndexByte(name, '.')+1:]
		}
	}
	return t.Name()
}
