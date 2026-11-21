package swiftrt

import (
	"fmt"
	"reflect"
	"sync"
)

// Generic Swift code can call a protocol's static requirements on a type parameter
// (`T.allCases`, `T(rawValue:)`). Go generics can't, so each conforming type registers its
// implementation at init and generic code looks it up by type.

var (
	allCasesReg sync.Map // reflect.Type -> func() []T
	rawInitReg  sync.Map // reflect.Type -> func(R) *T
)

// RegisterAllCases records T's `CaseIterable.allCases`.
func RegisterAllCases[T any](f func() []T) { allCasesReg.Store(reflect.TypeFor[T](), f) }

// AllCasesOf is `T.allCases` for a type parameter T.
func AllCasesOf[T any]() []T {
	f, ok := allCasesReg.Load(reflect.TypeFor[T]())
	if !ok {
		panic(fmt.Sprintf("swiftrt: %v has no registered allCases", reflect.TypeFor[T]()))
	}
	return f.(func() []T)()
}

// RegisterRawInit records T's `RawRepresentable.init(rawValue:)`.
func RegisterRawInit[T, R any](f func(R) *T) { rawInitReg.Store(reflect.TypeFor[T](), f) }

// RawInitOf is `T(rawValue:)` for a type parameter T.
func RawInitOf[T, R any](raw R) *T {
	f, ok := rawInitReg.Load(reflect.TypeFor[T]())
	if !ok {
		panic(fmt.Sprintf("swiftrt: %v has no registered init(rawValue:)", reflect.TypeFor[T]()))
	}
	g, ok := f.(func(R) *T)
	if !ok {
		panic(fmt.Sprintf("swiftrt: %v's init(rawValue:) doesn't take %v", reflect.TypeFor[T](), reflect.TypeFor[R]()))
	}
	return g(raw)
}

// DecodeAs is `T(from: decoder)` for a type parameter T.
func DecodeAs[T any](d *Decoder) T { return DecodeValue[T](d.Value, d.CodingPath) }

// EncodeInto is `value.encode(to: encoder)` for a value of a type parameter.
func EncodeInto[T any](v T, e *Encoder) { e.Value = EncodeValue(v) }
