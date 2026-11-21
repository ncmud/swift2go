package swiftrt

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// CodingError is Swift `DecodingError` / `EncodingError`.
type CodingError struct{ Msg string }

func (e CodingError) Error() string { return e.Msg }

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
	return Str(k)
}

// KeyedEncodingContainer is Swift `KeyedEncodingContainer`.
type KeyedEncodingContainer struct{ M map[string]any }

// EncoderContainerKeyedBy is `encoder.container(keyedBy:)`.
func EncoderContainerKeyedBy(e *Encoder, _ Type) KeyedEncodingContainer {
	if m, ok := e.Value.(map[string]any); ok {
		return KeyedEncodingContainer{m}
	}
	m := map[string]any{}
	e.Value = m
	return KeyedEncodingContainer{m}
}

// KeyedEncodingContainerEncodeForKey is `container.encode(_:forKey:)`.
func KeyedEncodingContainerEncodeForKey[T any](c *KeyedEncodingContainer, v T, key any) {
	c.M[keyString(key)] = EncodeValue(v)
}

// KeyedEncodingContainerEncodeIfPresentForKey is `container.encodeIfPresent(_:forKey:)`.
func KeyedEncodingContainerEncodeIfPresentForKey[T any](c *KeyedEncodingContainer, v *T, key any) {
	if v != nil {
		c.M[keyString(key)] = EncodeValue(*v)
	}
}

// KeyedEncodingContainerEncodeNilForKey is `container.encodeNil(forKey:)`.
func KeyedEncodingContainerEncodeNilForKey(c *KeyedEncodingContainer, key any) {
	c.M[keyString(key)] = nil
}

// SingleValueEncodingContainer is Swift `SingleValueEncodingContainer`.
type SingleValueEncodingContainer struct{ e *Encoder }

// EncoderSingleValueContainer is `encoder.singleValueContainer()`.
func EncoderSingleValueContainer(e *Encoder) SingleValueEncodingContainer {
	return SingleValueEncodingContainer{e}
}

// SingleValueEncodingContainerEncode is `container.encode(_:)`.
func SingleValueEncodingContainerEncode[T any](c *SingleValueEncodingContainer, v T) {
	c.e.Value = EncodeValue(v)
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

// KeyedDecodingContainer is Swift `KeyedDecodingContainer`.
type KeyedDecodingContainer struct {
	M    map[string]any
	path []string
}

// DecoderContainerKeyedBy is `decoder.container(keyedBy:)`.
func DecoderContainerKeyedBy(d *Decoder, _ Type) KeyedDecodingContainer {
	m, ok := d.Value.(map[string]any)
	if !ok {
		codingFail("expected a keyed container at %s, found %T", strings.Join(d.CodingPath, "."), d.Value)
	}
	return KeyedDecodingContainer{m, d.CodingPath}
}

// KeyedDecodingContainerContains is `container.contains(_:)`.
func KeyedDecodingContainerContains(c KeyedDecodingContainer, key any) bool {
	_, ok := c.M[keyString(key)]
	return ok
}

// KeyedDecodingContainerAllKeys is `container.allKeys` as strings.
func KeyedDecodingContainerAllKeys(c KeyedDecodingContainer) []string { return MapKeys(c.M) }

// KeyedDecodingContainerDecodeForKey is `container.decode(_:forKey:)`.
func KeyedDecodingContainerDecodeForKey[T any](c KeyedDecodingContainer, _ TypeT[T], key any) T {
	k := keyString(key)
	raw, ok := c.M[k]
	if !ok {
		codingFail("key %q not found at %s", k, strings.Join(c.path, "."))
	}
	return DecodeValue[T](raw, append(c.path, k))
}

// KeyedDecodingContainerDecodeIfPresentForKey is `container.decodeIfPresent(_:forKey:)`.
func KeyedDecodingContainerDecodeIfPresentForKey[T any](c KeyedDecodingContainer, _ TypeT[T], key any) *T {
	k := keyString(key)
	raw, ok := c.M[k]
	if !ok || raw == nil {
		return nil
	}
	v := DecodeValue[T](raw, append(c.path, k))
	return &v
}

// KeyedDecodingContainerDecodeNilForKey is `container.decodeNil(forKey:)`.
func KeyedDecodingContainerDecodeNilForKey(c KeyedDecodingContainer, key any) bool {
	raw, ok := c.M[keyString(key)]
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

// EncodeValue turns a Swift value into a JSON-shaped tree.
func EncodeValue(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
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
	switch x := v.(type) {
	case interface{ RawValue() string }:
		return x.RawValue()
	case interface{ RawValue() int }:
		return float64(x.RawValue())
	case UUID:
		return UUIDUuidString(x)
	case Date:
		return float64(x) - 978307200
	}
	switch rv.Kind() {
	case reflect.Bool, reflect.String:
		return rv.Interface()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
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
		out := map[string]any{}
		for _, k := range rv.MapKeys() {
			out[keyString(k.Interface())] = EncodeValue(rv.MapIndex(k).Interface())
		}
		return out
	case reflect.Struct:
		out := map[string]any{}
		t := rv.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := f.Tag.Get("swift")
			if name == "" || name == "-" {
				continue
			}
			fv := rv.Field(i)
			if fv.Kind() == reflect.Pointer && fv.IsNil() {
				continue
			}
			out[name] = EncodeValue(fv.Interface())
		}
		return out
	}
	return Str(v)
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
		n, _ := raw.(float64)
		dst.Set(reflect.ValueOf(Date(n + 978307200)))
		return
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
			codingFail("expected Bool at %s", strings.Join(path, "."))
		}
		dst.SetBool(b)
	case reflect.String:
		s, ok := raw.(string)
		if !ok {
			codingFail("expected String at %s", strings.Join(path, "."))
		}
		dst.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := raw.(float64)
		if !ok {
			if s, isStr := raw.(string); isStr && decodeRawString(dst, s) {
				return
			}
			codingFail("expected a number at %s", strings.Join(path, "."))
		}
		if decodeRawInt(dst, int(n)) {
			return
		}
		dst.SetInt(int64(n))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, _ := raw.(float64)
		dst.SetUint(uint64(n))
	case reflect.Float32, reflect.Float64:
		n, _ := raw.(float64)
		dst.SetFloat(n)
	case reflect.Slice:
		xs, ok := raw.([]any)
		if !ok {
			codingFail("expected an array at %s", strings.Join(path, "."))
		}
		out := reflect.MakeSlice(t, len(xs), len(xs))
		for i, x := range xs {
			decodeInto(out.Index(i), x, append(path, fmt.Sprint(i)))
		}
		dst.Set(out)
	case reflect.Map:
		m, ok := raw.(map[string]any)
		if !ok {
			codingFail("expected a dictionary at %s", strings.Join(path, "."))
		}
		out := reflect.MakeMapWithSize(t, len(m))
		for k, x := range m {
			kv := reflect.New(t.Key()).Elem()
			decodeInto(kv, k, path)
			vv := reflect.New(t.Elem()).Elem()
			decodeInto(vv, x, append(path, k))
			out.SetMapIndex(kv, vv)
		}
		dst.Set(out)
	case reflect.Struct:
		m, ok := raw.(map[string]any)
		if !ok {
			codingFail("expected a keyed container at %s", strings.Join(path, "."))
		}
		for i := 0; i < t.NumField(); i++ {
			fd := t.Field(i)
			name := fd.Tag.Get("swift")
			if name == "" || name == "-" || !fd.IsExported() {
				continue
			}
			x, ok := m[name]
			if !ok {
				if fd.Type.Kind() == reflect.Pointer {
					continue
				}
				codingFail("key %q not found at %s", name, strings.Join(path, "."))
			}
			decodeInto(dst.Field(i), x, append(path, name))
		}
	default:
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, dst.Addr().Interface()); err != nil {
			codingFail("cannot decode %s at %s: %v", t, strings.Join(path, "."), err)
		}
	}
}

// decodeRawInt sets an Int-raw enum from its raw value when T has RawValue() int.
func decodeRawInt(dst reflect.Value, n int) bool {
	if _, ok := dst.Interface().(interface{ RawValue() int }); !ok {
		return false
	}
	dst.SetInt(int64(n))
	return true
}

// decodeRawString sets a String-raw enum by searching its cases for the raw value.
func decodeRawString(dst reflect.Value, s string) bool {
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
	codingFail("cannot initialize %s from invalid String value %q", dst.Type(), s)
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

// JSONEncoderEncode is `JSONEncoder.encode(_:)`.
func JSONEncoderEncode[T any](e *JSONEncoder, v T) []byte {
	tree := EncodeValue(v)
	var b []byte
	var err error
	if e.OutputFormatting.RawValue&1 != 0 {
		b, err = json.MarshalIndent(tree, "", "  ")
	} else {
		b, err = json.Marshal(tree)
	}
	if err != nil {
		codingFail("%v", err)
	}
	return b
}

// JSONDecoder is Foundation `JSONDecoder`.
type JSONDecoder struct{ UserInfo map[string]any }

// JSONDecoderInit is `JSONDecoder()`.
func JSONDecoderInit() *JSONDecoder { return &JSONDecoder{} }

// JSONDecoderDecodeFrom is `JSONDecoder.decode(_:from:)`.
func JSONDecoderDecodeFrom[T any](d *JSONDecoder, _ TypeT[T], data []byte) T {
	var tree any
	if err := json.Unmarshal(data, &tree); err != nil {
		codingFail("The data couldn't be read because it isn't in the correct format: %v", err)
	}
	return DecodeValue[T](tree, nil)
}
