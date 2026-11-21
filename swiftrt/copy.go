package swiftrt

import "reflect"

// SwiftClass marks Go types that came from Swift classes; their pointers are references, not optionals.
type SwiftClass interface{ SwiftClass() }

var swiftClassType = reflect.TypeFor[SwiftClass]()

// Copy gives v Swift value semantics: structs, arrays, dictionaries and optional structs are copied
// deeply, while class references keep their identity.
func Copy[T any](v T) T {
	rv := reflect.ValueOf(&v).Elem()
	out := deepCopy(rv)
	return out.Interface().(T)
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
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if f := out.Field(i); f.CanSet() {
				f.Set(deepCopy(v.Field(i)))
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		if isPlain(v.Type().Elem()) {
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
		plain := isPlain(v.Type().Elem())
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
