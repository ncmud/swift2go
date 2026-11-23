package swiftrt

import (
	"fmt"
	"reflect"
	"strings"
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
	case string:
		return any(StringChars(s)).([]T)
	case Substring:
		return any(StringChars(s.String())).([]T)
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
		// A Set, in the order SetElements gives its elements.
		out := make([]T, 0, rv.Len())
		for it := rv.MapRange(); it.Next(); {
			if v, ok := it.Key().Interface().(T); ok {
				out = append(out, v)
			}
		}
		sortDeterministic(out)
		return out
	case reflect.Struct:
		if out, ok := iterateIntegerRange[T](rv); ok {
			return out
		}
		if f := rv.FieldByName("Items"); f.IsValid() {
			return Iterate[T](f.Interface())
		}
	}
	panic(fmt.Sprintf("swiftrt.Iterate: cannot iterate %T", seq))
}

// rangeValues is lo..<hi as a slice.
func rangeValues[T Integer](lo, hi T) []T {
	out := []T{}
	for i := lo; i < hi; i++ {
		out = append(out, i)
	}
	return out
}

func rangeInts(lo, hi int) []int {
	out := make([]int, 0, max(0, hi-lo))
	for i := lo; i < hi; i++ {
		out = append(out, i)
	}
	return out
}

// iterateIntegerRange lists a Range or ClosedRange of any integer type, such as `0..<n` of UInt64.
func iterateIntegerRange[T any](rv reflect.Value) ([]T, bool) {
	name := rv.Type().Name()
	closed := strings.HasPrefix(name, "ClosedRange[")
	if !closed && !strings.HasPrefix(name, "Range[") || rv.Type().PkgPath() != reflect.TypeFor[Range[int]]().PkgPath() {
		return nil, false
	}
	lo, hi := rv.FieldByName("Lo"), rv.FieldByName("Hi")
	et := reflect.TypeFor[T]()
	var out []T
	switch {
	case lo.CanInt():
		end := hi.Int()
		for i := lo.Int(); i < end || closed && i == end; i++ {
			out = append(out, reflect.ValueOf(i).Convert(et).Interface().(T)) //nolint:forcetypeassert // converted to T
			if closed && i == end {
				break
			}
		}
	case lo.CanUint():
		end := hi.Uint()
		for i := lo.Uint(); i < end || closed && i == end; i++ {
			out = append(out, reflect.ValueOf(i).Convert(et).Interface().(T)) //nolint:forcetypeassert // converted to T
			if closed && i == end {
				break
			}
		}
	default:
		return nil, false
	}
	return out, true
}
