package swiftrt

import (
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// CodingError is a Swift `DecodingError` or `EncodingError` that the runtime or a mapped package
// raises with a message alone, and the Go type of `EncodingError`.
type CodingError struct{ Msg string }

func (e CodingError) Error() string { return e.Msg }

// CodingContext is the error's context: its message as the debug description.
func (e CodingError) CodingContext() CodingContext { return CodingContext{DebugDescription: e.Msg} }

// DecodingError is Swift `DecodingError`: a CodingError, or a value of one of the case types
// DecodingErrorTypeMismatch, DecodingErrorValueNotFound, DecodingErrorKeyNotFound and
// DecodingErrorDataCorrupted, which `switch` patterns match.
type DecodingError interface {
	error
	// CodingContext is the context every case carries.
	CodingContext() CodingContext
}

// DecodingErrorTypeMismatch is `DecodingError.typeMismatch(_:_:)`.
type DecodingErrorTypeMismatch struct {
	F0 any
	F1 CodingContext
}

// DecodingErrorValueNotFound is `DecodingError.valueNotFound(_:_:)`.
type DecodingErrorValueNotFound struct {
	F0 any
	F1 CodingContext
}

// DecodingErrorKeyNotFound is `DecodingError.keyNotFound(_:_:)`.
type DecodingErrorKeyNotFound struct {
	F0 any
	F1 CodingContext
}

// DecodingErrorDataCorrupted is `DecodingError.dataCorrupted(_:)`.
type DecodingErrorDataCorrupted struct{ F0 CodingContext }

func (e DecodingErrorTypeMismatch) Error() string                 { return e.F1.render("typeMismatch") }
func (e DecodingErrorValueNotFound) Error() string                { return e.F1.render("valueNotFound") }
func (e DecodingErrorKeyNotFound) Error() string                  { return e.F1.render("keyNotFound") }
func (e DecodingErrorDataCorrupted) Error() string                { return e.F0.render("dataCorrupted") }
func (e DecodingErrorTypeMismatch) CodingContext() CodingContext  { return e.F1 }
func (e DecodingErrorValueNotFound) CodingContext() CodingContext { return e.F1 }
func (e DecodingErrorKeyNotFound) CodingContext() CodingContext   { return e.F1 }
func (e DecodingErrorDataCorrupted) CodingContext() CodingContext { return e.F0 }

func codingFail(format string, args ...any) {
	panic(Throw(CodingError{fmt.Sprintf(format, args...)}))
}

// Encoder is Swift `Encoder`, building a JSON-shaped tree.
type Encoder struct {
	Value    any
	UserInfo map[string]any
}

// Decoder is Swift `Decoder`, reading a JSON-shaped tree.
type Decoder struct {
	Value      any
	CodingPath []string
	UserInfo   map[string]any
}

// DecoderCodingPath is `Decoder.codingPath`.
func DecoderCodingPath(d *Decoder) []string { return d.CodingPath }

func keyString(k any) string {
	switch v := k.(type) {
	case string:
		return v
	case interface{ StringValue() string }:
		return v.StringValue()
	case interface{ RawValue() string }:
		return v.RawValue()
	}
	// A CodingKey struct whose stringValue is a stored property.
	if rv := reflect.ValueOf(k); rv.Kind() == reflect.Struct {
		if f := rv.FieldByName("StringValue"); f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
	}
	return Str(k)
}

// encodedObject is a JSON object built by encoding. Each key keeps the position of its first write
// and the value of its last, the order swift-yyjson writes an object in without `.sortedKeys`.
type encodedObject struct {
	keys []string
	m    map[string]any
}

func newEncodedObject(n int) *encodedObject {
	return &encodedObject{keys: make([]string, 0, n), m: make(map[string]any, n)}
}

func (o *encodedObject) set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

// treeObject returns the members of a JSON object in a parsed or encoded tree.
func treeObject(raw any) (map[string]any, bool) {
	switch o := raw.(type) {
	case map[string]any:
		return o, true
	case *encodedObject:
		return o.m, true
	}
	return nil, false
}

// KeyedEncodingContainer is Swift `KeyedEncodingContainer`.
type KeyedEncodingContainer struct{ obj *encodedObject }

// EncoderContainerKeyedBy is `encoder.container(keyedBy:)`.
func EncoderContainerKeyedBy(e *Encoder, _ Type) KeyedEncodingContainer {
	if o, ok := e.Value.(*encodedObject); ok {
		return KeyedEncodingContainer{o}
	}
	o := newEncodedObject(0)
	e.Value = o
	return KeyedEncodingContainer{o}
}

// KeyedEncodingContainerEncodeForKey is `container.encode(_:forKey:)`.
func KeyedEncodingContainerEncodeForKey[T any](c *KeyedEncodingContainer, v T, key any) {
	c.obj.set(keyString(key), EncodeValue(v))
}

// KeyedEncodingContainerEncodeIfPresentForKey is `container.encodeIfPresent(_:forKey:)`.
func KeyedEncodingContainerEncodeIfPresentForKey[T any](c *KeyedEncodingContainer, v *T, key any) {
	if v != nil {
		c.obj.set(keyString(key), EncodeValue(*v))
	}
}

// KeyedEncodingContainerEncodeNilForKey is `container.encodeNil(forKey:)`.
func KeyedEncodingContainerEncodeNilForKey(c *KeyedEncodingContainer, key any) {
	c.obj.set(keyString(key), nil)
}

// SingleValueEncodingContainer is Swift `SingleValueEncodingContainer`.
type SingleValueEncodingContainer struct{ e *Encoder }

// EncoderSingleValueContainer is `encoder.singleValueContainer()`.
func EncoderSingleValueContainer(e *Encoder) SingleValueEncodingContainer {
	return SingleValueEncodingContainer{e}
}

// SingleValueEncodingContainerEncode is `container.encode(_:)`.
func SingleValueEncodingContainerEncode[T any](c *SingleValueEncodingContainer, v T) struct{} {
	c.e.Value = EncodeValue(v)
	return struct{}{}
}

// UnkeyedEncodingContainer is Swift `UnkeyedEncodingContainer`.
type UnkeyedEncodingContainer struct{ e *Encoder }

// EncoderUnkeyedContainer is `encoder.unkeyedContainer()`.
func EncoderUnkeyedContainer(e *Encoder) UnkeyedEncodingContainer {
	e.Value = []any{}
	return UnkeyedEncodingContainer{e}
}

// UnkeyedEncodingContainerEncode is `container.encode(_:)`.
func UnkeyedEncodingContainerEncode[T any](c *UnkeyedEncodingContainer, v T) {
	c.e.Value = append(c.e.Value.([]any), EncodeValue(v))
}

// RawDecodable is a decoding source that decodes values itself instead of being read as a JSON-shaped
// tree. A package that decodes Codable types from its own storage passes such a value as the raw value of
// a [Decoder] or to [DecodeValue].
type RawDecodable interface {
	// SwiftDecodeInto sets dst, a settable value of the Go type being decoded, and reports whether it
	// did. On false the value is decoded by the default rules, which treat it as a JSON-shaped value.
	// It reports a failure by panicking with [Throw], as a Swift decoder throws.
	SwiftDecodeInto(dst reflect.Value, path []string) bool
}

// RawKeyed is a keyed decoding source other than a map[string]any: a keyed container and the fields of a
// synthesized decoder read their values through Lookup.
type RawKeyed interface {
	// Lookup returns the raw value for a key, and whether the key is present. A present nil decodes as nil.
	Lookup(key string) (any, bool)
	// Keys lists the keys present, for `allKeys`.
	Keys() []string
}

// KeyedDecodingContainer is Swift `KeyedDecodingContainer`.
type KeyedDecodingContainer struct {
	M    map[string]any
	src  RawKeyed
	path []string
}

func (c KeyedDecodingContainer) lookup(key string) (any, bool) {
	if c.src != nil {
		return c.src.Lookup(key)
	}
	v, ok := c.M[key]
	return v, ok
}

func (c KeyedDecodingContainer) keys() []string {
	if c.src != nil {
		keys := slices.Clone(c.src.Keys())
		slices.Sort(keys)
		return keys
	}
	return MapKeys(c.M)
}

// DecoderContainerKeyedBy is `decoder.container(keyedBy:)`.
func DecoderContainerKeyedBy(d *Decoder, _ Type) KeyedDecodingContainer {
	if src, ok := d.Value.(RawKeyed); ok {
		return KeyedDecodingContainer{src: src, path: d.CodingPath}
	}
	m, ok := treeObject(d.Value)
	if !ok {
		mismatch(reflect.TypeFor[map[string]any](), d.Value, d.CodingPath)
	}
	return KeyedDecodingContainer{M: m, path: d.CodingPath}
}

// KeyedDecodingContainerContains is `container.contains(_:)`.
func KeyedDecodingContainerContains(c KeyedDecodingContainer, key any) bool {
	_, ok := c.lookup(keyString(key))
	return ok
}

// KeyedDecodingContainerAllKeys is `container.allKeys`: the keys present that the CodingKey type's
// `init?(stringValue:)` accepts, in sorted order.
func KeyedDecodingContainerAllKeys[K any](c KeyedDecodingContainer, key func(string) *K) []K {
	out := []K{}
	for _, s := range c.keys() {
		if k := key(s); k != nil {
			out = append(out, *k)
		}
	}
	return out
}

// KeyedDecodingContainerDecodeForKey is `container.decode(_:forKey:)`.
func KeyedDecodingContainerDecodeForKey[T any](c KeyedDecodingContainer, _ TypeT[T], key any) T {
	k := keyString(key)
	raw, ok := c.lookup(k)
	if !ok {
		keyNotFound(k, c.path)
	}
	return DecodeValue[T](raw, append(c.path, k))
}

// KeyedDecodingContainerDecodeIfPresentForKey is `container.decodeIfPresent(_:forKey:)`.
func KeyedDecodingContainerDecodeIfPresentForKey[T any](c KeyedDecodingContainer, _ TypeT[T], key any) *T {
	k := keyString(key)
	raw, ok := c.lookup(k)
	if !ok || raw == nil {
		return nil
	}
	v := DecodeValue[T](raw, append(c.path, k))
	return &v
}

// KeyedDecodingContainerDecodeNilForKey is `container.decodeNil(forKey:)`.
func KeyedDecodingContainerDecodeNilForKey(c KeyedDecodingContainer, key any) bool {
	raw, ok := c.lookup(keyString(key))
	return !ok || raw == nil
}

// SingleValueDecodingContainer is Swift `SingleValueDecodingContainer`.
type SingleValueDecodingContainer struct{ d *Decoder }

// DecoderSingleValueContainer is `decoder.singleValueContainer()`.
func DecoderSingleValueContainer(d *Decoder) SingleValueDecodingContainer {
	return SingleValueDecodingContainer{d}
}

// SingleValueDecodingContainerDecode is `container.decode(_:)`.
func SingleValueDecodingContainerDecode[T any](c SingleValueDecodingContainer, _ TypeT[T]) T {
	return DecodeValue[T](c.d.Value, c.d.CodingPath)
}

// SingleValueDecodingContainerDecodeNil is `container.decodeNil()`.
func SingleValueDecodingContainerDecodeNil(c SingleValueDecodingContainer) bool {
	return c.d.Value == nil
}

var (
	decodersMu sync.RWMutex
	decoders   = map[reflect.Type]func(*Decoder) any{}
)

// RegisterDecoder records a type's hand-written `init(from:)`.
func RegisterDecoder[T any](f func(*Decoder) T) {
	decodersMu.Lock()
	defer decodersMu.Unlock()
	decoders[reflect.TypeFor[T]()] = func(d *Decoder) any { return f(d) }
}

type registeredEncoder struct {
	iface reflect.Type
	f     func(v any, e *Encoder)
}

var (
	encodersMu sync.RWMutex
	encoders   []registeredEncoder
)

// RegisterEncoder records the `encode(to:)` of a type emitted as a Go interface, such as an enum
// with associated values, whose encoder is a function rather than a method. EncodeValue calls it
// for every value whose dynamic type implements T.
func RegisterEncoder[T any](f func(T, *Encoder)) {
	encodersMu.Lock()
	defer encodersMu.Unlock()
	encoders = append(encoders, registeredEncoder{reflect.TypeFor[T](), func(v any, e *Encoder) { f(v.(T), e) }})
}

func registeredEncoderFor(t reflect.Type) func(any, *Encoder) {
	encodersMu.RLock()
	defer encodersMu.RUnlock()
	for _, r := range encoders {
		if t == r.iface || r.iface.Kind() == reflect.Interface && t.Implements(r.iface) {
			return r.f
		}
	}
	return nil
}

// rawCodable is an OptionSet whose Codable conformance is the one Swift gives every RawRepresentable
// type: it codes its RawValue field alone.
type rawCodable interface{ SwiftRawCodable() }

// EncodeValue turns a Swift value into a JSON-shaped tree.
func EncodeValue(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if f := registeredEncoderFor(rv.Type()); f != nil {
		e := &Encoder{}
		f(v, e)
		return e.Value
	}
	if m := rv.MethodByName("Encode"); m.IsValid() && m.Type().NumIn() == 1 && m.Type().In(0) == reflect.TypeFor[*Encoder]() {
		e := &Encoder{}
		m.Call([]reflect.Value{reflect.ValueOf(e)})
		return e.Value
	}
	if rv.Kind() != reflect.Pointer && !rv.CanAddr() {
		p := reflect.New(rv.Type())
		p.Elem().Set(rv)
		if m := p.MethodByName("Encode"); m.IsValid() && m.Type().NumIn() == 1 && m.Type().In(0) == reflect.TypeFor[*Encoder]() {
			e := &Encoder{}
			m.Call([]reflect.Value{reflect.ValueOf(e)})
			return e.Value
		}
	}
	if _, ok := v.(rawCodable); ok && rv.Kind() == reflect.Struct {
		return EncodeValue(rv.FieldByName("RawValue").Interface())
	}
	if c, ok := payloadCaseFor(rv.Type()); ok {
		return encodePayloadCase(c, rv)
	}
	if names, ok := caseNamesFor(rv.Type()); ok && rv.CanInt() && rv.Int() >= 0 && int(rv.Int()) < len(names) {
		outer := newEncodedObject(1)
		outer.set(names[rv.Int()], newEncodedObject(0))
		return outer
	}
	switch x := v.(type) {
	case interface{ RawValue() string }:
		return x.RawValue()
	case interface{ RawValue() int }:
		return jsonNumber(strconv.Itoa(x.RawValue()))
	case UUID:
		return UUIDUuidString(x)
	case Date:
		return float64(x) - 978307200
	case Decimal:
		return jsonNumber(x.Description())
	case []byte:
		return base64.StdEncoding.EncodeToString(x)
	}
	switch rv.Kind() {
	case reflect.Bool, reflect.String:
		return rv.Interface()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return jsonNumber(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return jsonNumber(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32:
		return float32(rv.Float())
	case reflect.Float64:
		return rv.Float()
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return EncodeValue(rv.Elem().Interface())
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = EncodeValue(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		if isSetType(rv.Type()) {
			out := make([]any, 0, rv.Len())
			for _, k := range rv.MapKeys() {
				out = append(out, EncodeValue(k.Interface()))
			}
			return out
		}
		if !keyedDictionary(rv.Type()) {
			out := make([]any, 0, 2*rv.Len())
			for it := rv.MapRange(); it.Next(); {
				out = append(out, EncodeValue(it.Key().Interface()), EncodeValue(it.Value().Interface()))
			}
			return out
		}
		out := map[string]any{}
		for _, k := range rv.MapKeys() {
			out[keyString(k.Interface())] = EncodeValue(rv.MapIndex(k).Interface())
		}
		return out
	case reflect.Struct:
		t := rv.Type()
		out := newEncodedObject(t.NumField())
		fields := make([]int, 0, t.NumField())
		names := map[string]int{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := codingKey(f)
			if name == "" || name == "-" {
				continue
			}
			fields = append(fields, i)
			names[name] = i
		}
		if o, ok := v.(interface{ SwiftCodingKeyOrder() []string }); ok {
			// The order of the type's CodingKeys cases, which a synthesized encode(to:) follows.
			fields = fields[:0]
			for _, k := range o.SwiftCodingKeyOrder() {
				if i, ok := names[k]; ok {
					fields = append(fields, i)
				}
			}
		}
		for _, i := range fields {
			fv := rv.Field(i)
			if fv.Kind() == reflect.Pointer && fv.IsNil() {
				continue
			}
			out.set(codingKey(t.Field(i)), EncodeValue(fv.Interface()))
		}
		return out
	}
	return Str(v)
}

// codingKey is the key a stored property encodes under: its CodingKeys case's string value (the `key`
// tag, "-" when CodingKeys leaves the property out), or else the property's name.
func codingKey(f reflect.StructField) string {
	if key, ok := f.Tag.Lookup("key"); ok {
		return key
	}
	return f.Tag.Get("swift")
}

// DecodeValue builds a T from a JSON-shaped tree.
func DecodeValue[T any](raw any, path []string) T {
	var out T
	decodeInto(reflect.ValueOf(&out).Elem(), raw, path)
	return out
}

func decodeInto(dst reflect.Value, raw any, path []string) {
	t := dst.Type()
	decodersMu.RLock()
	f := decoders[t]
	decodersMu.RUnlock()
	if f != nil {
		dst.Set(reflect.ValueOf(f(&Decoder{Value: raw, CodingPath: path})))
		return
	}
	if r, ok := raw.(RawDecodable); ok && r.SwiftDecodeInto(dst, path) {
		return
	}
	if t.Kind() == reflect.Struct && t.Implements(reflect.TypeFor[rawCodable]()) {
		decodeInto(dst.FieldByName("RawValue"), raw, path)
		return
	}
	if cases, ok := payloadCasesFor(t); ok {
		decodePayloadEnum(dst, cases, raw, path)
		return
	}
	if names, ok := caseNamesFor(t); ok {
		obj, ok := treeObject(raw)
		if ok && len(obj) == 1 {
			for i, name := range names {
				if _, ok := obj[name]; ok {
					dst.SetInt(int64(i))
					return
				}
			}
		}
		codingFail("typeMismatch: expected one case key at %s", strings.Join(path, "."))
	}
	if t == reflect.TypeFor[UUID]() {
		s, _ := raw.(string)
		u := UUIDOptInitUuidString(s)
		if u == nil {
			codingFail("invalid UUID at %s", strings.Join(path, "."))
		}
		dst.Set(reflect.ValueOf(*u))
		return
	}
	if t == reflect.TypeFor[Date]() {
		n, _ := rawFloat(raw)
		dst.Set(reflect.ValueOf(Date(n + 978307200)))
		return
	}
	if t == reflect.TypeFor[Decimal]() {
		text, ok := rawNumberText(raw)
		d := NSDecimalOptInitString(text)
		if !ok || d == nil {
			codingFail("Expected to decode Decimal but found %s instead at %s", rawKind(raw), strings.Join(path, "."))
		}
		dst.Set(reflect.ValueOf(*d))
		return
	}
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		if s, ok := raw.(string); ok {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				codingFail("Encountered Data is not valid Base64 at %s", strings.Join(path, "."))
			}
			dst.Set(reflect.ValueOf(b).Convert(t))
			return
		}
	}
	switch t.Kind() {
	case reflect.Pointer:
		if raw == nil {
			dst.Set(reflect.Zero(t))
			return
		}
		p := reflect.New(t.Elem())
		decodeInto(p.Elem(), raw, path)
		dst.Set(p)
	case reflect.Bool:
		b, ok := raw.(bool)
		if !ok {
			mismatch(t, raw, path)
		}
		dst.SetBool(b)
	case reflect.String:
		s, ok := raw.(string)
		if !ok {
			mismatch(t, raw, path)
		}
		dst.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if _, isNum := rawNumberText(raw); !isNum {
			if s, isStr := raw.(string); isStr && decodeRawString(dst, s, path) {
				return
			}
			mismatch(t, raw, path)
		}
		n, ok := rawInt(raw)
		if !ok || dst.OverflowInt(n) {
			notValidJSON()
		}
		if decodeRawInt(dst, n, path) {
			return
		}
		dst.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if _, isNum := rawNumberText(raw); !isNum {
			mismatch(t, raw, path)
		}
		n, ok := rawUint(raw)
		if !ok || dst.OverflowUint(n) {
			notValidJSON()
		}
		dst.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, ok := rawFloat(raw)
		if !ok {
			mismatch(t, raw, path)
		}
		dst.SetFloat(n)
	case reflect.Slice:
		xs, ok := raw.([]any)
		if !ok {
			mismatch(t, raw, path)
		}
		out := reflect.MakeSlice(t, len(xs), len(xs))
		for i, x := range xs {
			decodeInto(out.Index(i), x, append(path, "Index "+strconv.Itoa(i)))
		}
		dst.Set(out)
	case reflect.Map:
		if isSetType(t) {
			xs, ok := raw.([]any)
			if !ok {
				codingFail("expected an array at %s", strings.Join(path, "."))
			}
			out := reflect.MakeMapWithSize(t, len(xs))
			for i, x := range xs {
				kv := reflect.New(t.Key()).Elem()
				decodeInto(kv, x, append(path, "Index "+strconv.Itoa(i)))
				out.SetMapIndex(kv, reflect.Zero(t.Elem()))
			}
			dst.Set(out)
			return
		}
		if !keyedDictionary(t) {
			xs, ok := raw.([]any)
			if !ok || len(xs)%2 != 0 {
				codingFail("expected an array of key-value pairs at %s", strings.Join(path, "."))
			}
			out := reflect.MakeMapWithSize(t, len(xs)/2)
			for i := 0; i < len(xs); i += 2 {
				kv := reflect.New(t.Key()).Elem()
				decodeInto(kv, xs[i], append(path, "Index "+strconv.Itoa(i)))
				vv := reflect.New(t.Elem()).Elem()
				decodeInto(vv, xs[i+1], append(path, "Index "+strconv.Itoa(i+1)))
				out.SetMapIndex(kv, vv)
			}
			dst.Set(out)
			return
		}
		m, ok := treeObject(raw)
		if !ok {
			mismatch(t, raw, path)
		}
		out := reflect.MakeMapWithSize(t, len(m))
		for k, x := range m {
			kv := reflect.New(t.Key()).Elem()
			decodeMapKey(kv, k, path)
			vv := reflect.New(t.Elem()).Elem()
			decodeInto(vv, x, append(path, k))
			out.SetMapIndex(kv, vv)
		}
		dst.Set(out)
	case reflect.Struct:
		var c KeyedDecodingContainer
		if src, ok := raw.(RawKeyed); ok {
			c.src = src
		} else if m, ok := treeObject(raw); ok {
			c.M = m
		} else {
			mismatch(t, raw, path)
		}
		if dst.CanAddr() {
			if d, ok := dst.Addr().Interface().(interface{ SwiftDecodeDefaults() }); ok {
				d.SwiftDecodeDefaults()
			}
		}
		for i := 0; i < t.NumField(); i++ {
			fd := t.Field(i)
			name := codingKey(fd)
			if name == "" || name == "-" || !fd.IsExported() {
				continue
			}
			x, ok := c.lookup(name)
			if !ok {
				if fd.Type.Kind() == reflect.Pointer {
					continue
				}
				keyNotFound(name, path)
			}
			decodeInto(dst.Field(i), x, append(path, name))
		}
	default:
		b, err := json.Marshal(raw)
		if err == nil {
			err = json.Unmarshal(b, dst.Addr().Interface())
		}
		if err != nil {
			codingFail("cannot decode %s at %s: %v", t, strings.Join(path, "."), err)
		}
	}
}

// decodeRawInt sets an Int-raw enum from its raw value when T has RawValue() int, through the
// enum's init(rawValue:), since a case's raw value need not be its position.
func decodeRawInt(dst reflect.Value, n int64, path []string) bool {
	if _, ok := dst.Interface().(interface{ RawValue() int }); !ok {
		return false
	}
	f, ok := rawInitReg.Load(dst.Type())
	if !ok {
		dst.SetInt(n)
		return true
	}
	fn := reflect.ValueOf(f)
	out := fn.Call([]reflect.Value{reflect.ValueOf(n).Convert(fn.Type().In(0))})[0]
	if out.IsNil() {
		invalidRawValue(dst.Type(), "Int", strconv.FormatInt(n, 10), path)
	}
	dst.Set(out.Elem())
	return true
}

// decodeRawString sets a String-raw enum by searching its cases for the raw value.
func decodeRawString(dst reflect.Value, s string, path []string) bool {
	if _, ok := dst.Interface().(interface{ RawValue() string }); !ok {
		return false
	}
	for i := 0; i < 4096; i++ {
		dst.SetInt(int64(i))
		var rv string
		ok := func() (ok bool) {
			defer func() {
				if recover() != nil {
					ok = false
				}
			}()
			rv = dst.Interface().(interface{ RawValue() string }).RawValue()
			return true
		}()
		if !ok {
			break
		}
		if rv == s {
			return true
		}
	}
	invalidRawValue(dst.Type(), "String", s, path)
	return false
}

// JSONEncoder is Foundation `JSONEncoder`.
type JSONEncoder struct {
	OutputFormatting JSONEncoderOutputFormatting
	UserInfo         map[string]any
}

// JSONEncoderOutputFormatting is `JSONEncoder.OutputFormatting`.
type JSONEncoderOutputFormatting struct{ RawValue uint }

// OutputFormattingSortedKeys is `.sortedKeys`.
var OutputFormattingSortedKeys = JSONEncoderOutputFormatting{2}

// OutputFormattingPrettyPrinted is `.prettyPrinted`.
var OutputFormattingPrettyPrinted = JSONEncoderOutputFormatting{1}

// JSONEncoderInit is `JSONEncoder()`.
func JSONEncoderInit() *JSONEncoder { return &JSONEncoder{} }

// JSONEncoderEncode is `JSONEncoder.encode(_:)`: numbers as Swift's `description` without a
// trailing ".0", `/` escaped as `\/` unless `.withoutEscapingSlashes`, and for `.prettyPrinted`
// two-space indentation, `" : "` separators and empty containers written as `[\n\n]`. Keys are
// always sorted, one of the orders Foundation's unsorted output may take. NaN and infinity throw.
func JSONEncoderEncode[T any](e *JSONEncoder, v T) []byte {
	tree := EncodeValue(v)
	w := &yyjsonWriter{
		pretty:        e.OutputFormatting.RawValue&OutputFormattingPrettyPrinted.RawValue != 0,
		escapeSlashes: e.OutputFormatting.RawValue&OutputFormattingWithoutEscapingSlashes.RawValue == 0,
		sortKeys:      true,
		foundation:    true,
	}
	if err := Catch(func() { w.value(tree, 0) }); err != nil {
		codingFail("Unable to encode the given value to JSON: %v", err)
	}
	return []byte(w.b.String())
}

// JSONDecoder is Foundation `JSONDecoder`.
type JSONDecoder struct{ UserInfo map[string]any }

// JSONDecoderInit is `JSONDecoder()`.
func JSONDecoderInit() *JSONDecoder { return &JSONDecoder{} }

// JSONDecoderDecodeFrom is `JSONDecoder.decode(_:from:)`: strict UTF-8 JSON after an optional
// byte-order mark, where the first of duplicate keys wins.
func JSONDecoderDecodeFrom[T any](d *JSONDecoder, _ TypeT[T], data []byte) T {
	tree, err := parseJSON(data, jsonParseOptions{allowBOM: true})
	if err != nil {
		codingFail("The data couldn't be read because it isn't in the correct format: %v", err)
	}
	return DecodeValue[T](tree, nil)
}

// keyedDictionary reports whether Swift encodes a Dictionary of map type t as a JSON object: only a
// String or Int key does. Any other key, including an enum with a String raw value, makes an array
// that alternates keys and values.
func keyedDictionary(t reflect.Type) bool {
	return t.Key() == reflect.TypeFor[string]() || t.Key() == reflect.TypeFor[int]()
}

// isSetType reports whether t is the Go form of a Swift `Set`, a map to struct{}.
func isSetType(t reflect.Type) bool {
	return t.Kind() == reflect.Map && t.Elem().Kind() == reflect.Struct && t.Elem().NumField() == 0
}

// rawNumberText returns the literal text of a JSON number in a Codable tree.
func rawNumberText(raw any) (string, bool) {
	switch n := raw.(type) {
	case jsonNumber:
		return string(n), true
	case float64:
		return strconv.FormatFloat(n, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(n), 'g', -1, 32), true
	}
	return "", false
}

// foundKind is how Foundation's JSONDecoder names the JSON value it found instead of the one it
// expected.
func foundKind(raw any) string {
	switch raw.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case bool:
		return "bool"
	case []any:
		return "an array"
	case map[string]any, *encodedObject, RawKeyed:
		return "a dictionary"
	}
	return "number"
}

// decodeTypeName is how Foundation's JSONDecoder names the type it expected.
func decodeTypeName(t reflect.Type) string {
	if n, ok := reflect.Zero(t).Interface().(SwiftNamed); ok && t.Kind() != reflect.Struct {
		name := n.SwiftTypeName()
		return name[strings.LastIndexByte(name, '.')+1:]
	}
	switch t.Kind() {
	case reflect.String:
		return "String"
	case reflect.Bool:
		return "Bool"
	case reflect.Int:
		return "Int"
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "Int" + strconv.Itoa(t.Bits())
	case reflect.Uint:
		return "UInt"
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "UInt" + strconv.Itoa(t.Bits())
	case reflect.Float64:
		return "Double"
	case reflect.Float32:
		return "Float"
	case reflect.Slice:
		return "Array<Any>"
	}
	return "Dictionary<String, Any>"
}

func codingPathCopy(path []string) []string { return append([]string{}, path...) }

// mismatch throws the DecodingError Foundation's JSONDecoder throws for a value of the wrong kind:
// valueNotFound for null, typeMismatch otherwise.
func mismatch(t reflect.Type, raw any, path []string) {
	ctx := CodingContext{CodingPath: codingPathCopy(path)}
	if raw == nil {
		ctx.DebugDescription = "Cannot get value of type " + decodeTypeName(t) + " -- found null value instead"
		panic(Throw(DecodingErrorValueNotFound{F0: t, F1: ctx}))
	}
	ctx.DebugDescription = "Expected to decode " + decodeTypeName(t) + " but found " + foundKind(raw) + " instead."
	panic(Throw(DecodingErrorTypeMismatch{F0: t, F1: ctx}))
}

// keyNotFound throws the DecodingError for a key a container lacks.
func keyNotFound(key string, path []string) {
	panic(Throw(DecodingErrorKeyNotFound{F0: key, F1: CodingContext{
		CodingPath:       codingPathCopy(path),
		DebugDescription: fmt.Sprintf("No value associated with key CodingKeys(stringValue: %s, intValue: nil) (%s).", swiftQuote(key), swiftQuote(key)),
	}}))
}

// invalidRawValue throws the DecodingError for a raw value no case of an enum has.
func invalidRawValue(t reflect.Type, kind, value string, path []string) {
	panic(Throw(DecodingErrorDataCorrupted{F0: CodingContext{
		CodingPath:       codingPathCopy(path),
		DebugDescription: "Cannot initialize " + decodeTypeName(t) + " from invalid " + kind + " value " + value,
	}}))
}

// notValidJSON throws what Foundation's JSONDecoder throws for a number the expected integer type
// cannot hold: the data as a whole is not valid JSON.
func notValidJSON() {
	panic(Throw(DecodingErrorDataCorrupted{F0: CodingContext{DebugDescription: "The given data was not valid JSON."}}))
}

// rawKind is a JSON value's kind, for messages.
func rawKind(raw any) string {
	switch raw.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case bool:
		return "a bool"
	case []any:
		return "an array"
	case map[string]any, *encodedObject:
		return "a dictionary"
	}
	return "a number"
}

// rawFloat reads a JSON number as a Double. A number beyond Double's range fails, as in both
// Foundation and swift-yyjson; one that underflows reads as zero, as swift-yyjson reads it.
func rawFloat(raw any) (float64, bool) {
	text, ok := rawNumberText(raw)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil && (!errors.Is(err, strconv.ErrRange) || math.IsInf(f, 0)) {
		return 0, false
	}
	return f, true
}

// rawInt reads a JSON number as an Int64, accepting a fractional spelling of an integer such as
// `1.0` or `1e2`, as Foundation's JSONDecoder does.
func rawInt(raw any) (int64, bool) {
	text, ok := rawNumberText(raw)
	if !ok {
		return 0, false
	}
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		return i, true
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || f != math.Trunc(f) || f < -0x1p63 || f >= 0x1p63 {
		return 0, false
	}
	return int64(f), true
}

// rawUint reads a JSON number as a UInt64, as rawInt does for signed integers.
func rawUint(raw any) (uint64, bool) {
	text, ok := rawNumberText(raw)
	if !ok {
		return 0, false
	}
	if u, err := strconv.ParseUint(text, 10, 64); err == nil {
		return u, true
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || f != math.Trunc(f) || f < 0 || f >= 0x1p64 {
		return 0, false
	}
	return uint64(f), true
}

// decodeMapKey sets a dictionary key from its JSON object key: integer keys are parsed from their
// decimal text, as Swift does for `Int`-keyed dictionaries; other keys decode as strings.
func decodeMapKey(dst reflect.Value, k string, path []string) {
	if _, isRaw := dst.Interface().(interface{ RawValue() string }); !isRaw {
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n, err := strconv.ParseInt(k, 10, 64)
			if err != nil || dst.OverflowInt(n) {
				codingFail("Expected Int key but found %q at %s", k, strings.Join(path, "."))
			}
			dst.SetInt(n)
			return
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(k, 10, 64)
			if err != nil || dst.OverflowUint(n) {
				codingFail("Expected UInt key but found %q at %s", k, strings.Join(path, "."))
			}
			dst.SetUint(n)
			return
		}
	}
	decodeInto(dst, k, path)
}
