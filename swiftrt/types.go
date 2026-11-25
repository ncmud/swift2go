package swiftrt

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
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

// MetaZero is a value of T that carries T's static requirements: T's zero value, or for a pointer type a
// pointer to a zero value, so that a value method of the type it points to can run.
func MetaZero[T any]() T {
	var z T
	if t := reflect.TypeFor[T](); t.Kind() == reflect.Pointer {
		return reflect.New(t.Elem()).Interface().(T) //nolint:forcetypeassert // reflect.New(Elem) is a T
	}
	return z
}

// witnesses holds, by Go type, the functions that wrap a value of the type in a value that carries a
// protocol conformance the type's Go form cannot: a Go type of another package, such as int or a slice,
// cannot gain methods, and an Optional is a pointer whose methods are those of the type it points to.
var (
	witnessesMu sync.RWMutex
	witnesses   = map[reflect.Type][]func(any) any{}
)

// RegisterWitness records wrap as a carrier of T's conformances: wrap returns a value whose methods are
// the requirements of the protocols T conforms to through it, its statics included.
func RegisterWitness[T any](wrap func(T) any) {
	witnessesMu.Lock()
	defer witnessesMu.Unlock()
	key := reflect.TypeFor[T]()
	witnesses[key] = append(witnesses[key], func(v any) any { return wrap(v.(T)) }) //nolint:forcetypeassert // keyed by T's type
}

// witnessAs wraps v in the registered carrier of its type that implements the interface P.
func witnessAs[P any](v any) (P, bool) {
	var zero P
	if v == nil || reflect.TypeFor[P]().Kind() != reflect.Interface {
		return zero, false
	}
	witnessesMu.RLock()
	ws := witnesses[reflect.TypeOf(v)]
	witnessesMu.RUnlock()
	for _, wrap := range ws {
		if p, ok := wrap(v).(P); ok {
			return p, true
		}
	}
	return zero, false
}

// Witnessed is a carrier of a conformance that RegisterWitness wraps a value in; it unwraps to the value.
type Witnessed interface{ SwiftWitnessed() any }

// Conforming is v as the protocol P: v itself, or the registered carrier of its conformance to P.
func Conforming[P any](v any) P {
	if p, ok := v.(P); ok {
		return p
	}
	if p, ok := witnessAs[P](v); ok {
		return p
	}
	Trap(fmt.Sprintf("value of type %T does not conform to %v", v, reflect.TypeFor[P]()))
	panic("unreachable")
}

// StaticWitness is a value of protocol P that carries T's static requirements: T's registered carrier that
// implements P, or else MetaZero[T]().
func StaticWitness[T any, P any]() P {
	z := MetaZero[T]()
	if p, ok := witnessAs[P](z); ok {
		return p
	}
	return any(z).(P) //nolint:forcetypeassert // generated code calls this for a T constrained by P
}

// MetatypeEqual is Swift's `==` on two optional metatypes, such as `type(of: a) == type(of: b)`:
// two types are equal when they are the same type, and both nil when both are nil.
func MetatypeEqual(a, b *Type) bool {
	if a == nil || b == nil || *a == nil || *b == nil {
		return (a == nil || *a == nil) == (b == nil || *b == nil)
	}
	return sameType((*a).Reflect(), (*b).Reflect())
}

// sameType reports whether Go types x and y are the same Swift type. Each case of an enum with
// payloads is its own Go type, and is the enum's Swift type, as the enum's Go interface is.
func sameType(x, y reflect.Type) bool {
	if x == y {
		return true
	}
	if x == nil || y == nil {
		return false
	}
	xi, yi := enumInterface(x), enumInterface(y)
	switch {
	case xi && yi:
		return false
	case xi:
		return isEnumCase(y) && y.Implements(x) && typeArgs(x) == typeArgs(y)
	case yi:
		return isEnumCase(x) && x.Implements(y) && typeArgs(x) == typeArgs(y)
	}
	if !isEnumCase(x) || !isEnumCase(y) || x.PkgPath() != y.PkgPath() || typeArgs(x) != typeArgs(y) {
		return false
	}
	xn, _ := reflect.Zero(x).Interface().(SwiftNamed)
	yn, _ := reflect.Zero(y).Interface().(SwiftNamed)
	return xn != nil && yn != nil && xn.SwiftTypeName() == yn.SwiftTypeName()
}

// enumInterface reports whether t is the Go interface of an enum with payloads, whose one method is
// the marker is<Enum>.
func enumInterface(t reflect.Type) bool {
	return t.Kind() == reflect.Interface && t.NumMethod() == 1 && t.Method(0).Name == "is"+strings.SplitN(t.Name(), "[", 2)[0]
}

// isEnumCase reports whether t is the Go type of one case of an enum with payloads.
func isEnumCase(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.Implements(reflect.TypeFor[SwiftCase]()) && t.Implements(reflect.TypeFor[SwiftNamed]())
}

// typeArgs is the bracketed type arguments of a generic Go type's name, or "".
func typeArgs(t reflect.Type) string {
	if i := strings.IndexByte(t.Name(), '['); i >= 0 {
		return t.Name()[i:]
	}
	return ""
}
