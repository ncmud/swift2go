package swiftrt

import (
	"fmt"
	"reflect"
	"sort"
)

// Iterate turns any Swift sequence the emitter has no direct loop for into a slice of its elements.
func Iterate[T any](seq any) []T {
	switch s := seq.(type) {
	case []T:
		return s
	case Range[int]:
		return any(rangeInts(s.Lo, s.Hi)).([]T)
	case ClosedRange[int]:
		return any(rangeInts(s.Lo, s.Hi+1)).([]T)
	case interface{ Elements() []T }:
		return s.Elements()
	}
	rv := reflect.ValueOf(seq)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]T, rv.Len())
		for i := range out {
			out[i] = rv.Index(i).Interface().(T)
		}
		return out
	case reflect.Map:
		keys := rv.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
		out := make([]T, 0, len(keys))
		for _, k := range keys {
			if v, ok := k.Interface().(T); ok {
				out = append(out, v)
			}
		}
		return out
	case reflect.Struct:
		if f := rv.FieldByName("Items"); f.IsValid() {
			return Iterate[T](f.Interface())
		}
	}
	panic(fmt.Sprintf("swiftrt.Iterate: cannot iterate %T", seq))
}

func rangeInts(lo, hi int) []int {
	out := make([]int, 0, max(0, hi-lo))
	for i := lo; i < hi; i++ {
		out = append(out, i)
	}
	return out
}
