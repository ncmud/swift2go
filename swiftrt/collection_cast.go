package swiftrt

import (
	"reflect"
	"strings"
)

// collectionAs is Swift's conditional cast between collection types: an array, dictionary or set
// casts to another of the same kind when every element (and every key) casts to the target's
// element type, as `[String: Int]` held in `Any` casts to `[String: Any]`. An empty collection
// casts to any element type.
func collectionAs[T any](v any) (T, bool) {
	var zero T
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return zero, false
	}
	out, ok := castCollection(reflect.TypeFor[T](), rv)
	if !ok {
		return zero, false
	}
	return out.Interface().(T), true
}

type collectionKind int

const (
	notCollection collectionKind = iota
	arrayKind
	dictKind
	setKind
	hashedDictKind
	hashedSetKind
)

var swiftrtPkgPath = reflect.TypeFor[HSet[int]]().PkgPath()

// kindOf tells which Swift collection a Go type represents.
func kindOf(t reflect.Type) collectionKind {
	switch t.Kind() {
	case reflect.Slice:
		return arrayKind
	case reflect.Map:
		if e := t.Elem(); e.Kind() == reflect.Struct && e.NumField() == 0 {
			return setKind
		}
		return dictKind
	case reflect.Struct:
		if t.PkgPath() != swiftrtPkgPath {
			return notCollection
		}
		switch {
		case strings.HasPrefix(t.Name(), "HMap["):
			return hashedDictKind
		case strings.HasPrefix(t.Name(), "HSet["):
			return hashedSetKind
		}
	}
	return notCollection
}

// castCollection converts rv, a collection, to the collection type t element by element.
func castCollection(t reflect.Type, rv reflect.Value) (reflect.Value, bool) {
	from, to := kindOf(rv.Type()), kindOf(t)
	if from == notCollection || to == notCollection {
		return reflect.Value{}, false
	}
	dict := func(k collectionKind) bool { return k == dictKind || k == hashedDictKind }
	set := func(k collectionKind) bool { return k == setKind || k == hashedSetKind }
	switch {
	case from == arrayKind && to == arrayKind:
		out := reflect.MakeSlice(t, rv.Len(), rv.Len())
		for i := range rv.Len() {
			e, ok := castValue(t.Elem(), rv.Index(i).Interface())
			if !ok {
				return reflect.Value{}, false
			}
			out.Index(i).Set(e)
		}
		return out, true
	case dict(from) && dict(to):
		keyT, valT := dictTypes(t, to)
		out := newCollection(t, to)
		for k, v := range dictEntries(rv, from) {
			ck, ok := castValue(keyT, k.Interface())
			if !ok {
				return reflect.Value{}, false
			}
			cv, ok := castValue(valT, v.Interface())
			if !ok {
				return reflect.Value{}, false
			}
			if to == dictKind && !ck.Comparable() {
				// An AnyHashable key whose Go value cannot key a Go map.
				return reflect.Value{}, false
			}
			insert(out, to, ck, cv)
		}
		return out, true
	case set(from) && set(to):
		var elemT reflect.Type
		if to == hashedSetKind {
			elemT = t.Field(0).Type.Elem()
		} else {
			elemT = t.Key()
		}
		out := newCollection(t, to)
		for e := range setElements(rv, from) {
			ce, ok := castValue(elemT, e.Interface())
			if !ok {
				return reflect.Value{}, false
			}
			if to == setKind && !ce.Comparable() {
				return reflect.Value{}, false
			}
			insert(out, to, ce, reflect.Value{})
		}
		return out, true
	}
	return reflect.Value{}, false
}

// dictTypes are the key and value types of a dictionary type.
func dictTypes(t reflect.Type, k collectionKind) (key, value reflect.Type) {
	if k == hashedDictKind {
		entry := t.Field(0).Type.Elem()
		return entry.Field(0).Type, entry.Field(1).Type
	}
	return t.Key(), t.Elem()
}

func newCollection(t reflect.Type, k collectionKind) reflect.Value {
	out := reflect.New(t).Elem()
	switch k {
	case dictKind, setKind:
		out.Set(reflect.MakeMap(t))
	case hashedDictKind, hashedSetKind:
		out.Field(0).Set(reflect.MakeMap(t.Field(0).Type))
	}
	return out
}

// insert adds a key and value, or a set element (value invalid), to a collection newCollection made.
func insert(c reflect.Value, k collectionKind, key, value reflect.Value) {
	switch k {
	case dictKind:
		c.SetMapIndex(key, value)
	case setKind:
		c.SetMapIndex(key, reflect.Zero(c.Type().Elem()))
	case hashedDictKind:
		entry := reflect.New(c.Field(0).Type().Elem()).Elem()
		entry.Field(0).Set(key)
		entry.Field(1).Set(value)
		c.Field(0).SetMapIndex(reflect.ValueOf(hkey(key.Interface())), entry)
	case hashedSetKind:
		c.Field(0).SetMapIndex(reflect.ValueOf(hkey(key.Interface())), key)
	}
}

func dictEntries(rv reflect.Value, k collectionKind) func(yield func(key, value reflect.Value) bool) {
	return func(yield func(key, value reflect.Value) bool) {
		if k == dictKind {
			for it := rv.MapRange(); it.Next(); {
				if !yield(it.Key(), it.Value()) {
					return
				}
			}
			return
		}
		for it := rv.Field(0).MapRange(); it.Next(); {
			if !yield(it.Value().Field(0), it.Value().Field(1)) {
				return
			}
		}
	}
}

func setElements(rv reflect.Value, k collectionKind) func(yield func(reflect.Value) bool) {
	return func(yield func(reflect.Value) bool) {
		if k == setKind {
			for it := rv.MapRange(); it.Next(); {
				if !yield(it.Key()) {
					return
				}
			}
			return
		}
		for it := rv.Field(0).MapRange(); it.Next(); {
			if !yield(it.Value()) {
				return
			}
		}
	}
}

var nsNumberType = reflect.TypeFor[NSNumber]()

// castValue is Swift's `v as? T` for one element of a collection cast, with T given as a Go type.
func castValue(t reflect.Type, v any) (reflect.Value, bool) {
	if v == nil {
		if t.Kind() == reflect.Interface || t.Kind() == reflect.Pointer {
			return reflect.Zero(t), true
		}
		return reflect.Value{}, false
	}
	as := func(x any) (reflect.Value, bool) {
		rv := reflect.ValueOf(x)
		if rv.Type().AssignableTo(t) {
			out := reflect.New(t).Elem()
			out.Set(rv)
			return out, true
		}
		if t.Kind() == reflect.Interface && rv.Kind() != reflect.Pointer && reflect.PointerTo(rv.Type()).Implements(t) {
			// A struct whose methods take a pointer receiver is boxed when it becomes an existential.
			box := reflect.New(rv.Type())
			box.Elem().Set(rv)
			out := reflect.New(t).Elem()
			out.Set(box)
			return out, true
		}
		return reflect.Value{}, false
	}
	if out, ok := as(v); ok {
		return out, true
	}
	if _, ok := v.(interface{ SwiftDynamic() any }); ok {
		if out, ok := as(Dynamic(v)); ok {
			return out, true
		}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		// An Optional holding a value casts as the value does.
		if out, ok := castValue(t, rv.Elem().Interface()); ok {
			return out, true
		}
	}
	if n, ok := v.(NSNumber); ok {
		return n.bridge(t)
	}
	if t == nsNumberType {
		if n, ok := nsNumberOf(v); ok {
			return reflect.ValueOf(n), true
		}
	}
	if out, ok := castCollection(t, rv); ok {
		return out, true
	}
	if t.Kind() == reflect.Pointer {
		// A value cast to an Optional of a type it casts to.
		if inner, ok := castValue(t.Elem(), v); ok {
			p := reflect.New(t.Elem())
			p.Elem().Set(inner)
			return p, true
		}
	}
	return reflect.Value{}, false
}
