package swiftrt

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// NSNumber is a Foundation `NSNumber` produced by JSON parsing: a Bool, a signed or unsigned 64-bit
// integer, or a Double. `as?` casts to Swift numeric types succeed when the value is exactly
// representable in the target type, as Foundation's bridging does.
type NSNumber struct {
	kind byte // 'b' Bool, 'i' Int64, 'u' UInt64, 'd' Double
	i    int64
	u    uint64
	f    float64
}

// NSNumberBool is `NSNumber(value: Bool)`.
func NSNumberBool(b bool) NSNumber {
	n := NSNumber{kind: 'b'}
	if b {
		n.i = 1
	}
	return n
}

// NSNumberInt64 is `NSNumber(value: Int64)`.
func NSNumberInt64(v int64) NSNumber { return NSNumber{kind: 'i', i: v} }

// NSNumberUInt64 is `NSNumber(value: UInt64)`.
func NSNumberUInt64(v uint64) NSNumber { return NSNumber{kind: 'u', u: v} }

// NSNumberDouble is `NSNumber(value: Double)`.
func NSNumberDouble(v float64) NSNumber { return NSNumber{kind: 'd', f: v} }

// Description is `NSNumber.description`: Booleans print as 1 or 0, doubles as C's `%.16g`.
func (n NSNumber) Description() string {
	switch n.kind {
	case 'u':
		return strconv.FormatUint(n.u, 10)
	case 'd':
		return strconv.FormatFloat(n.f, 'g', 16, 64)
	}
	return strconv.FormatInt(n.i, 10)
}

// bridge converts n to a value of type t when Foundation's `as?` would succeed.
func (n NSNumber) bridge(t reflect.Type) (reflect.Value, bool) {
	out := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.Bool:
		switch {
		case n.kind == 'd' && (n.f == 0 || n.f == 1):
			out.SetBool(n.f == 1)
		case n.kind == 'u' && n.u <= 1:
			out.SetBool(n.u == 1)
		case (n.kind == 'i' || n.kind == 'b') && (n.i == 0 || n.i == 1):
			out.SetBool(n.i == 1)
		default:
			return out, false
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		switch n.kind {
		case 'u':
			if n.u > math.MaxInt64 {
				return out, false
			}
			v = int64(n.u)
		case 'd':
			if n.f != math.Trunc(n.f) || n.f < -0x1p63 || n.f >= 0x1p63 {
				return out, false
			}
			v = int64(n.f)
		default:
			v = n.i
		}
		if out.OverflowInt(v) {
			return out, false
		}
		out.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		var v uint64
		switch n.kind {
		case 'u':
			v = n.u
		case 'd':
			if n.f != math.Trunc(n.f) || n.f < 0 || n.f >= 0x1p64 {
				return out, false
			}
			v = uint64(n.f)
		default:
			if n.i < 0 {
				return out, false
			}
			v = uint64(n.i)
		}
		if out.OverflowUint(v) {
			return out, false
		}
		out.SetUint(v)
	case reflect.Float32, reflect.Float64:
		var v float64
		switch n.kind {
		case 'u':
			v = float64(n.u)
			if v >= 0x1p64 || uint64(v) != n.u {
				return out, false
			}
		case 'd':
			v = n.f
		default:
			v = float64(n.i)
			if v >= 0x1p63 || int64(v) != n.i {
				return out, false
			}
		}
		if t.Kind() == reflect.Float32 && !math.IsInf(v, 0) && !math.IsNaN(v) && float64(float32(v)) != v {
			return out, false
		}
		out.SetFloat(v)
	default:
		return out, false
	}
	return out, true
}

// NSNull is Foundation `NSNull`, the JSON `null` value.
type NSNull struct{}

// Description is `NSNull.description`.
func (NSNull) Description() string { return "<null>" }

// bridgeValue converts v to type t the way Swift's `as?` does across Foundation bridging: NSNumber to
// numeric types and element-wise casts of `[Any]` and `[String: Any]` to typed collections.
func bridgeValue(t reflect.Type, v any) (reflect.Value, bool) {
	if v == nil {
		return reflect.Value{}, false
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(t) {
		out := reflect.New(t).Elem()
		out.Set(rv)
		return out, true
	}
	if n, ok := v.(NSNumber); ok {
		return n.bridge(t)
	}
	switch {
	case rv.Kind() == reflect.Slice && t.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Interface:
		out := reflect.MakeSlice(t, rv.Len(), rv.Len())
		for i := 0; i < rv.Len(); i++ {
			e, ok := bridgeValue(t.Elem(), rv.Index(i).Interface())
			if !ok {
				return reflect.Value{}, false
			}
			out.Index(i).Set(e)
		}
		return out, true
	case rv.Kind() == reflect.Map && t.Kind() == reflect.Map && rv.Type().Elem().Kind() == reflect.Interface && rv.Type().Key() == t.Key():
		out := reflect.MakeMapWithSize(t, rv.Len())
		for _, k := range rv.MapKeys() {
			e, ok := bridgeValue(t.Elem(), rv.MapIndex(k).Interface())
			if !ok {
				return reflect.Value{}, false
			}
			out.SetMapIndex(k, e)
		}
		return out, true
	}
	return reflect.Value{}, false
}

func bridgeAs[T any](v any) (T, bool) {
	var zero T
	switch v.(type) {
	case NSNumber, []any, map[string]any:
	default:
		return zero, false
	}
	out, ok := bridgeValue(reflect.TypeFor[T](), v)
	if !ok {
		return zero, false
	}
	return out.Interface().(T), true
}

// YYJSONError is swift-yyjson's `YYJSONError`.
type YYJSONError struct{ Msg string }

func (e YYJSONError) Error() string { return e.Msg }

// YYJSONErrorDescription is swift-yyjson `YYJSONError.description`: the message, followed by
// " (at path)" when the error has a coding path. The runtime's errors carry none.
func YYJSONErrorDescription(e YYJSONError) string { return e.Msg }

func yyjsonThrow(msg string) { panic(Throw(YYJSONError{msg})) }

// parseJSONStrict parses one RFC 8259 JSON document, numbers kept as text.
func parseJSONStrict(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected content at the end of the document")
	}
	return v, nil
}

func jsonNumberObject(n json.Number) NSNumber {
	s := string(n)
	if !strings.ContainsAny(s, ".eE") {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return NSNumberInt64(i)
		}
		if u, err := strconv.ParseUint(s, 10, 64); err == nil {
			return NSNumberUInt64(u)
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return NSNumberDouble(f)
}

func foundationObject(v any) any {
	switch x := v.(type) {
	case nil:
		return NSNull{}
	case json.Number:
		return jsonNumberObject(x)
	case bool:
		return NSNumberBool(x)
	case []any:
		for i := range x {
			x[i] = foundationObject(x[i])
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = foundationObject(e)
		}
		return x
	}
	return v
}

// YYJSONSerializationJsonObjectWith is swift-yyjson `YYJSONSerialization.jsonObject(with:)`: it parses
// strict JSON whose top level is an array or object into `[Any]`, `[String: Any]`, String, NSNumber
// (Bool, Int64, UInt64, or Double for fractional and out-of-range values) and NSNull, throwing
// YYJSONError on invalid input.
func YYJSONSerializationJsonObjectWith(data []byte) any {
	v, err := parseJSONStrict(data)
	if err != nil {
		yyjsonThrow(err.Error())
	}
	switch v.(type) {
	case []any, map[string]any:
	default:
		yyjsonThrow("Top-level JSON value must be an array or dictionary")
	}
	return foundationObject(v)
}

// yyjsonDouble formats a finite double as yyjson writes it: the shortest round-tripping digits, in
// fixed notation with at least one fractional digit when the decimal point falls within 21 digits
// of the first digit and after the sixth fractional zero, otherwise as `1.5e300` or `1e-7`.
func yyjsonDouble(f float64) string {
	if f == 0 {
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, expText, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expText)
	dot := exp + 1
	switch {
	case dot > -6 && dot <= 0:
		return sign + "0." + strings.Repeat("0", -dot) + digits
	case dot > 0 && dot <= 21 && dot >= len(digits):
		return sign + digits + strings.Repeat("0", dot-len(digits)) + ".0"
	case dot > 0 && dot <= 21:
		return sign + digits[:dot] + "." + digits[dot:]
	}
	out := sign + digits[:1]
	if len(digits) > 1 {
		out += "." + digits[1:]
	}
	return out + "e" + strconv.Itoa(exp)
}

type yyjsonWriter struct {
	b             strings.Builder
	pretty        bool
	escapeSlashes bool
	// integralDoubles writes doubles with no fractional part as integers, for trees in which
	// Codable integers have already become float64.
	integralDoubles bool
	// fourSpaces indents pretty output by four spaces per level instead of two.
	fourSpaces bool
}

func (w *yyjsonWriter) str(s string) {
	w.b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			w.b.WriteString(`\"`)
		case '\\':
			w.b.WriteString(`\\`)
		case '/':
			if w.escapeSlashes {
				w.b.WriteString(`\/`)
			} else {
				w.b.WriteByte('/')
			}
		case '\b':
			w.b.WriteString(`\b`)
		case '\t':
			w.b.WriteString(`\t`)
		case '\n':
			w.b.WriteString(`\n`)
		case '\f':
			w.b.WriteString(`\f`)
		case '\r':
			w.b.WriteString(`\r`)
		default:
			if r < 0x20 {
				w.b.WriteString(`\u00`)
				w.b.WriteString(strings.ToUpper(strconv.FormatInt(int64(r)>>4, 16) + strconv.FormatInt(int64(r)&15, 16)))
			} else {
				w.b.WriteRune(r)
			}
		}
	}
	w.b.WriteByte('"')
}

func (w *yyjsonWriter) indent(level int) {
	if w.pretty {
		w.b.WriteByte('\n')
		if w.fourSpaces {
			level *= 2
		}
		w.b.WriteString(strings.Repeat("  ", level))
	}
}

func (w *yyjsonWriter) double(f float64) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		yyjsonThrow("NaN or Infinity not allowed in JSON")
	}
	if w.integralDoubles && f == math.Trunc(f) && f >= -0x1p63 && f < 0x1p63 {
		w.b.WriteString(strconv.FormatInt(int64(f), 10))
		return
	}
	w.b.WriteString(yyjsonDouble(f))
}

func (w *yyjsonWriter) value(v any, level int) {
	switch x := v.(type) {
	case nil, NSNull:
		w.b.WriteString("null")
		return
	case string:
		w.str(x)
		return
	case Substring:
		w.str(x.String())
		return
	case bool:
		w.b.WriteString(strconv.FormatBool(x))
		return
	case NSNumber:
		switch x.kind {
		case 'b':
			w.b.WriteString(strconv.FormatBool(x.i == 1))
		case 'i':
			w.b.WriteString(strconv.FormatInt(x.i, 10))
		case 'u':
			w.b.WriteString(strconv.FormatUint(x.u, 10))
		default:
			w.double(x.f)
		}
		return
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.b.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		w.b.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		w.double(rv.Float())
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			w.b.WriteString("null")
		} else {
			w.value(rv.Elem().Interface(), level)
		}
	case reflect.Slice, reflect.Array:
		if rv.Len() == 0 {
			w.b.WriteString("[]")
			return
		}
		w.b.WriteByte('[')
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				w.b.WriteByte(',')
			}
			w.indent(level + 1)
			w.value(rv.Index(i).Interface(), level+1)
		}
		w.indent(level)
		w.b.WriteByte(']')
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			yyjsonThrow("Dictionary keys must be strings")
		}
		if rv.Len() == 0 {
			w.b.WriteString("{}")
			return
		}
		keys := rv.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		w.b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				w.b.WriteByte(',')
			}
			w.indent(level + 1)
			w.str(k.String())
			w.b.WriteByte(':')
			if w.pretty {
				w.b.WriteByte(' ')
			}
			w.value(rv.MapIndex(k).Interface(), level+1)
		}
		w.indent(level)
		w.b.WriteByte('}')
	default:
		yyjsonThrow("Invalid JSON object")
	}
}

// YYJSONSerializationDataWithJSONObject is swift-yyjson `YYJSONSerialization.data(withJSONObject:)`:
// compact JSON with `/` escaped as `\/`, throwing YYJSONError unless the top level is an array or
// dictionary, or when a value is NaN, infinite or not a JSON type. Dictionary keys are written in
// sorted order, one of the orders NSDictionary may produce.
func YYJSONSerializationDataWithJSONObject(obj any) []byte {
	rv := reflect.ValueOf(obj)
	for rv.IsValid() && (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) && !rv.IsNil() {
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array && rv.Kind() != reflect.Map {
		yyjsonThrow("Top-level JSON value must be an array or dictionary")
	}
	w := &yyjsonWriter{escapeSlashes: true}
	w.value(rv.Interface(), 0)
	return []byte(w.b.String())
}

// GameJSONEncoderOutputFormatting is JSONCoding `GameJSONEncoder.OutputFormatting`.
type GameJSONEncoderOutputFormatting struct{ RawValue int }

// GameJSONEncoderOutputFormatting options.
var (
	GameJSONEncoderOutputFormattingPrettyPrinted          = GameJSONEncoderOutputFormatting{1}
	GameJSONEncoderOutputFormattingSortedKeys             = GameJSONEncoderOutputFormatting{2}
	GameJSONEncoderOutputFormattingWithoutEscapingSlashes = GameJSONEncoderOutputFormatting{4}
)

// GameJSONEncoder is JSONCoding `GameJSONEncoder`.
type GameJSONEncoder struct {
	OutputFormatting GameJSONEncoderOutputFormatting
}

// GameJSONEncoderInit is JSONCoding `GameJSONEncoder()`.
func GameJSONEncoderInit() GameJSONEncoder { return GameJSONEncoder{} }

// GameJSONEncoderEncode is JSONCoding `GameJSONEncoder.encode(_:)`: yyjson output with two-space
// indentation for `.prettyPrinted` and `/` escaped unless `.withoutEscapingSlashes`. Object keys are
// always sorted, and doubles without a fractional part are written as integers, because the Codable
// tree no longer distinguishes them from Int. NaN and infinity throw EncodingError.invalidValue.
func GameJSONEncoderEncode[T any](e GameJSONEncoder, value T) []byte {
	tree := EncodeValue(value)
	w := &yyjsonWriter{
		pretty:          e.OutputFormatting.RawValue&1 != 0,
		escapeSlashes:   e.OutputFormatting.RawValue&4 == 0,
		integralDoubles: true,
	}
	if err := Catch(func() { w.value(tree, 0) }); err != nil {
		panic(Throw(CodingError{"invalidValue: " + err.Error()}))
	}
	return []byte(w.b.String())
}

// GameJSONDecoder is JSONCoding `GameJSONDecoder`.
type GameJSONDecoder struct{}

// GameJSONDecoderInit is JSONCoding `GameJSONDecoder()`.
func GameJSONDecoderInit() GameJSONDecoder { return GameJSONDecoder{} }

// GameJSONDecoderDecodeFrom is JSONCoding `GameJSONDecoder.decode(_:from:)`: strict JSON decoded with
// Codable semantics, throwing DecodingError.dataCorrupted on malformed input.
func GameJSONDecoderDecodeFrom[T any](_ GameJSONDecoder, t TypeT[T], data []byte) T {
	if _, err := parseJSONStrict(data); err != nil {
		panic(Throw(CodingError{"dataCorrupted: " + err.Error()}))
	}
	return JSONDecoderDecodeFrom(JSONDecoderInit(), t, data)
}

// YYJSONSerializationReadingOptions is swift-yyjson `YYJSONSerialization.ReadingOptions`.
type YYJSONSerializationReadingOptions struct{ RawValue uint }

// YYJSONSerialization.ReadingOptions members. The mutability options change nothing here, since Go
// containers are always mutable.
var (
	ReadingOptionsMutableContainers = YYJSONSerializationReadingOptions{1 << 0}
	ReadingOptionsMutableLeaves     = YYJSONSerializationReadingOptions{1 << 1}
	ReadingOptionsFragmentsAllowed  = YYJSONSerializationReadingOptions{1 << 2}
)

// YYJSONSerializationJsonObjectWithOptions is swift-yyjson
// `YYJSONSerialization.jsonObject(with:options:)`: YYJSONSerializationJsonObjectWith, except that
// with `.fragmentsAllowed` a top-level string, number, Boolean or null is returned instead of
// thrown on.
func YYJSONSerializationJsonObjectWithOptions(data []byte, options YYJSONSerializationReadingOptions) any {
	if options.RawValue&ReadingOptionsFragmentsAllowed.RawValue == 0 {
		return YYJSONSerializationJsonObjectWith(data)
	}
	v, err := parseJSONStrict(data)
	if err != nil {
		yyjsonThrow(err.Error())
	}
	return foundationObject(v)
}

// YYJSONSerializationWritingOptions is swift-yyjson `YYJSONSerialization.WritingOptions`.
type YYJSONSerializationWritingOptions struct{ RawValue uint }

// YYJSONSerialization.WritingOptions members.
var (
	WritingOptionsFragmentsAllowed       = YYJSONSerializationWritingOptions{1 << 0}
	WritingOptionsPrettyPrinted          = YYJSONSerializationWritingOptions{1 << 1}
	WritingOptionsSortedKeys             = YYJSONSerializationWritingOptions{1 << 2}
	WritingOptionsWithoutEscapingSlashes = YYJSONSerializationWritingOptions{1 << 3}
	WritingOptionsIndentationTwoSpaces   = YYJSONSerializationWritingOptions{1 << 4}
	WritingOptionsNewlineAtEnd           = YYJSONSerializationWritingOptions{1 << 6}
)

// isJSONFragment reports whether v is a value `data(withJSONObject:)` treats as a fragment: an
// NSString, NSNumber (Bool or number) or NSNull.
func isJSONFragment(v any) bool {
	switch v.(type) {
	case string, Substring, bool, NSNumber, NSNull:
		return true
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// YYJSONSerializationDataWithJSONObjectOptions is swift-yyjson
// `YYJSONSerialization.data(withJSONObject:options:)`. `.indentationTwoSpaces` pretty-prints with two
// spaces and overrides `.prettyPrinted`, which indents by four; `/` is escaped unless
// `.withoutEscapingSlashes`; `.newlineAtEnd` appends a newline; and `.fragmentsAllowed` accepts a
// top-level string, number, Boolean or null. Keys are written sorted with or without `.sortedKeys`,
// as in YYJSONSerializationDataWithJSONObject.
func YYJSONSerializationDataWithJSONObjectOptions(obj any, options YYJSONSerializationWritingOptions) []byte {
	has := func(o YYJSONSerializationWritingOptions) bool { return options.RawValue&o.RawValue != 0 }
	rv := reflect.ValueOf(obj)
	for rv.IsValid() && (rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface) && !rv.IsNil() {
		rv = rv.Elem()
	}
	switch {
	case rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array || rv.Kind() == reflect.Map):
	case rv.IsValid() && isJSONFragment(rv.Interface()):
		if !has(WritingOptionsFragmentsAllowed) {
			yyjsonThrow("Top-level JSON value must be an array or dictionary")
		}
	default:
		yyjsonThrow("Invalid JSON object")
	}
	w := &yyjsonWriter{
		pretty:        has(WritingOptionsIndentationTwoSpaces) || has(WritingOptionsPrettyPrinted),
		fourSpaces:    !has(WritingOptionsIndentationTwoSpaces),
		escapeSlashes: !has(WritingOptionsWithoutEscapingSlashes),
	}
	w.value(rv.Interface(), 0)
	if has(WritingOptionsNewlineAtEnd) {
		w.b.WriteByte('\n')
	}
	return []byte(w.b.String())
}

// YYJSONWriteOptions is swift-yyjson `YYJSONWriteOptions`, the flags YYJSONEncoder writes with.
type YYJSONWriteOptions struct{ RawValue uint32 }

// YYJSONWriteOptions members.
var (
	YYJSONWriteOptionsDefault              = YYJSONWriteOptions{0}
	YYJSONWriteOptionsPrettyPrinted        = YYJSONWriteOptions{1 << 0}
	YYJSONWriteOptionsEscapeSlashes        = YYJSONWriteOptions{1 << 2}
	YYJSONWriteOptionsIndentationTwoSpaces = YYJSONWriteOptions{1 << 6}
	YYJSONWriteOptionsNewlineAtEnd         = YYJSONWriteOptions{1 << 7}
	YYJSONWriteOptionsSortedKeys           = YYJSONWriteOptions{1 << 16}
)

// YYJSONWriteOptionsInsert is OptionSet `insert(_:)` on YYJSONWriteOptions.
func YYJSONWriteOptionsInsert(s *YYJSONWriteOptions, m YYJSONWriteOptions) struct {
	F0 bool
	F1 YYJSONWriteOptions
} {
	return OptionSetInsert(s, m)
}

// YYJSONWriteOptionsFormUnion is OptionSet `formUnion(_:)` on YYJSONWriteOptions.
func YYJSONWriteOptionsFormUnion(s *YYJSONWriteOptions, o YYJSONWriteOptions) {
	*s = OptionSetUnion(*s, o)
}

// YYJSONEncoder is swift-yyjson `YYJSONEncoder`.
type YYJSONEncoder struct {
	WriteOptions YYJSONWriteOptions
}

// YYJSONEncoderInit is swift-yyjson `YYJSONEncoder()`: an encoder with the default write options.
func YYJSONEncoderInit() YYJSONEncoder { return YYJSONEncoder{} }

// YYJSONEncoderEncode is swift-yyjson `YYJSONEncoder.encode(_:)`. `.prettyPrinted` indents by four
// spaces, or two with `.indentationTwoSpaces` (which alone also pretty-prints); `/` is escaped only
// with `.escapeSlashes`; `.newlineAtEnd` appends a newline. Object keys are always sorted, and doubles
// without a fractional part are written as integers, because the Codable tree keeps neither the
// encoding order nor the Int/Double distinction. NaN and infinity throw YYJSONError.
func YYJSONEncoderEncode[T any](e YYJSONEncoder, value T) []byte {
	has := func(o YYJSONWriteOptions) bool { return e.WriteOptions.RawValue&o.RawValue != 0 }
	tree := EncodeValue(value)
	w := &yyjsonWriter{
		pretty:          has(YYJSONWriteOptionsPrettyPrinted) || has(YYJSONWriteOptionsIndentationTwoSpaces),
		fourSpaces:      !has(YYJSONWriteOptionsIndentationTwoSpaces),
		escapeSlashes:   has(YYJSONWriteOptionsEscapeSlashes),
		integralDoubles: true,
	}
	w.value(tree, 0)
	if has(YYJSONWriteOptionsNewlineAtEnd) {
		w.b.WriteByte('\n')
	}
	return []byte(w.b.String())
}

// YYJSONDecoder is swift-yyjson `YYJSONDecoder`.
type YYJSONDecoder struct{}

// YYJSONDecoderInit is swift-yyjson `YYJSONDecoder()`.
func YYJSONDecoderInit() YYJSONDecoder { return YYJSONDecoder{} }

// YYJSONDecoderDecodeFrom is swift-yyjson `YYJSONDecoder.decode(_:from:)`: strict JSON decoded with
// Codable semantics, throwing YYJSONError on malformed input.
func YYJSONDecoderDecodeFrom[T any](_ YYJSONDecoder, t TypeT[T], data []byte) T {
	if len(data) == 0 {
		yyjsonThrow("Empty content")
	}
	if _, err := parseJSONStrict(data); err != nil {
		yyjsonThrow(err.Error())
	}
	return JSONDecoderDecodeFrom(JSONDecoderInit(), t, data)
}
