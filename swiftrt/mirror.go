package swiftrt

import "reflect"

// Mirror is Swift's `Mirror`: here, the children of a reflected value.
type Mirror struct {
	children []struct {
		F0 *string
		F1 any
	}
}

// MirrorInitReflecting is `Mirror(reflecting:)`. A value's children are its stored properties, or,
// for an enum case, its payload; a value that implements SwiftChildren lists its own. A thrown
// error is reflected as the Swift value it carries.
func MirrorInitReflecting(v any) Mirror {
	if d, ok := v.(interface{ SwiftDynamic() any }); ok {
		v = d.SwiftDynamic()
	}
	var m Mirror
	add := func(label *string, value any) {
		m.children = append(m.children, struct {
			F0 *string
			F1 any
		}{label, value})
	}
	if c, ok := v.(interface{ SwiftChildren() []any }); ok {
		for _, x := range c.SwiftChildren() {
			add(nil, x)
		}
		return m
	}
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Pointer && !rv.IsNil() && !rv.Type().Implements(swiftClassType) {
		rv = rv.Elem()
	}
	if rv.IsValid() && rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return m
	}
	t := rv.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous {
			continue
		}
		var label *string
		if tag := f.Tag.Get("swift"); tag != "" {
			l := tag
			label = &l
		}
		add(label, rv.Field(i).Interface())
	}
	return m
}

// MirrorChildren is `Mirror.children`: each child's label, nil for none, and value.
func MirrorChildren(m Mirror) []struct {
	F0 *string
	F1 any
} {
	return m.children
}
