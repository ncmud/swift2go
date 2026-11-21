package swiftrt

import (
	"reflect"
	"sync"
)

// SwiftClass marks Go types that came from Swift classes; their pointers are references, not optionals.
type SwiftClass interface{ SwiftClass() }

var swiftClassType = reflect.TypeFor[SwiftClass]()

// Copy gives v Swift value semantics: structs, arrays, dictionaries and optional structs are copied
// deeply, while class references keep their identity.
func Copy[T any](v T) T {
	if !needsDeepCopy(reflect.TypeFor[T]()) {
		// Go's assignment already copied every part Swift would.
		return v
	}
	rv := reflect.ValueOf(&v).Elem()
	out := deepCopy(rv)
	return out.Interface().(T)
}

var deepCache sync.Map // reflect.Type -> bool

// needsDeepCopy reports whether a value of type t shares storage that Swift would copy: a slice,
// map, interface or non-class pointer anywhere inside it.
func needsDeepCopy(t reflect.Type) bool {
	if v, ok := deepCache.Load(t); ok {
		return v.(bool)
	}
	deepCache.Store(t, true) // recursive types: assume deep while computing
	deep := computeNeedsDeep(t)
	deepCache.Store(t, deep)
	return deep
}

func computeNeedsDeep(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Map, reflect.Interface:
		return true
	case reflect.Pointer:
		return !t.Implements(swiftClassType)
	case reflect.Array:
		return needsDeepCopy(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if needsDeepCopy(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

func deepCopy(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || v.Type().Implements(swiftClassType) {
			return v
		}
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(deepCopy(v.Elem()))
		return p
	case reflect.Struct:
		if !needsDeepCopy(v.Type()) {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if f := out.Field(i); f.CanSet() && needsDeepCopy(f.Type()) {
				f.Set(deepCopy(v.Field(i)))
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		if isPlain(v.Type().Elem()) || !needsDeepCopy(v.Type().Elem()) {
			reflect.Copy(out, v)
			return out
		}
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(deepCopy(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		plain := isPlain(v.Type().Elem()) || !needsDeepCopy(v.Type().Elem())
		it := v.MapRange()
		for it.Next() {
			if plain {
				out.SetMapIndex(it.Key(), it.Value())
			} else {
				out.SetMapIndex(it.Key(), deepCopy(it.Value()))
			}
		}
		return out
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		inner := deepCopy(v.Elem())
		out := reflect.New(v.Type()).Elem()
		out.Set(inner)
		return out
	}
	return v
}

func isPlain(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	}
	return false
}
