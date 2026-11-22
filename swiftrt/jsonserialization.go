package swiftrt

import (
	"math"
	"reflect"
)

// Dictionary is Foundation `NSDictionary` as JSONSerialization produces it: string keys and
// Foundation object values. `==` compares entries the way `NSDictionary.isEqual(to:)` does.
type Dictionary = map[string]any

// FloatNan is `Double.nan`.
var FloatNan = math.NaN()

// NSJSONWritingOptions is Foundation `JSONSerialization.WritingOptions`.
type NSJSONWritingOptions struct{ RawValue uint }

// JSONSerialization.WritingOptions members.
var (
	NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingPrettyPrinted          = NSJSONWritingOptions{1 << 0}
	NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingSortedKeys             = NSJSONWritingOptions{1 << 1}
	NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingFragmentsAllowed       = NSJSONWritingOptions{1 << 2}
	NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingWithoutEscapingSlashes = NSJSONWritingOptions{1 << 3}
)

func jsonSerializationThrow(code int, msg string) {
	panic(Throw(NSError{Domain: "NSCocoaErrorDomain", Code: code, Msg: msg}))
}

// NSJSONSerializationJsonObjectWith is Foundation `JSONSerialization.jsonObject(with:)`: it parses JSON
// whose top level is an array or object into `[Any]`, `[String: Any]`, String, NSNumber (Bool,
// Int64, UInt64, or Double for fractional and out-of-range values) and NSNull, throwing
// NSCocoaErrorDomain 3840 on invalid input or a top-level fragment. A leading byte-order mark is
// skipped, and the first of duplicate keys wins.
func NSJSONSerializationJsonObjectWith(data []byte) any {
	v, err := parseJSON(data, jsonParseOptions{allowBOM: true})
	if err != nil {
		jsonSerializationThrow(3840, "The data couldn't be read because it isn't in the correct format.")
	}
	switch v.(type) {
	case []any, map[string]any:
	default:
		jsonSerializationThrow(3840, "JSON text did not start with array or object and option to allow fragments not set.")
	}
	return foundationObject(v)
}

// NSJSONSerializationDataWithJSONObjectOptions is Foundation
// `JSONSerialization.data(withJSONObject:options:)` as swift-corelibs-foundation writes it: `/`
// escaped as `\/` unless `.withoutEscapingSlashes`, doubles by their Swift description with a
// trailing ".0" dropped, and two-space indentation with `" : "` separators and a blank line inside
// empty containers for `.prettyPrinted`.
// Keys are always written sorted, one of the orders an unsorted dictionary may produce. A top-level
// value other than an array or dictionary needs `.fragmentsAllowed`; it, NaN, infinity, non-string
// keys and non-JSON values throw NSCocoaErrorDomain 3851.
func NSJSONSerializationDataWithJSONObjectOptions(obj any, options NSJSONWritingOptions) []byte {
	return jsonSerializationData(obj, options)
}

// NSJSONSerializationDataWithJSONObject is `JSONSerialization.data(withJSONObject:)`, with no options.
func NSJSONSerializationDataWithJSONObject(obj any) []byte {
	return jsonSerializationData(obj, NSJSONWritingOptions{})
}

func jsonSerializationData(obj any, options NSJSONWritingOptions) []byte {
	has := func(o NSJSONWritingOptions) bool { return options.RawValue&o.RawValue != 0 }
	rv := reflect.ValueOf(obj)
	for rv.IsValid() && (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) && !rv.IsNil() {
		rv = rv.Elem()
	}
	switch {
	case rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array || rv.Kind() == reflect.Map):
	case rv.IsValid() && isJSONFragment(rv.Interface()) && has(NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingFragmentsAllowed):
	default:
		jsonSerializationThrow(3851, "Invalid top-level type in JSON write")
	}
	w := &yyjsonWriter{
		pretty:        has(NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingPrettyPrinted),
		escapeSlashes: !has(NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingWithoutEscapingSlashes),
		foundation:    true,
	}
	if err := Catch(func() { w.value(rv.Interface(), 0) }); err != nil {
		jsonSerializationThrow(3851, "Invalid value in JSON write: "+err.Error())
	}
	return []byte(w.b.String())
}

// JSONWritingOptions is Foundation `JSONSerialization.WritingOptions` under the name the emitter
// gives it in option-set array literals.
type JSONWritingOptions = NSJSONWritingOptions
