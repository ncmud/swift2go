package swiftrt

import (
	"reflect"
	"strconv"
	"strings"
)

// SingleValueEncodingContainerEncodeInt is `container.encode(_: Int)`.
func SingleValueEncodingContainerEncodeInt[T Integer](c *SingleValueEncodingContainer, v T) struct{} {
	return SingleValueEncodingContainerEncode(c, v)
}

// SingleValueEncodingContainerEncodeFloat is `container.encode(_: Double)`.
func SingleValueEncodingContainerEncodeFloat[F ~float32 | ~float64](c *SingleValueEncodingContainer, v F) struct{} {
	return SingleValueEncodingContainerEncode(c, v)
}

// SingleValueEncodingContainerEncodeString is `container.encode(_: String)`.
func SingleValueEncodingContainerEncodeString(c *SingleValueEncodingContainer, v string) struct{} {
	return SingleValueEncodingContainerEncode(c, v)
}

// SingleValueEncodingContainerEncodeBool is `container.encode(_: Bool)`.
func SingleValueEncodingContainerEncodeBool(c *SingleValueEncodingContainer, v bool) struct{} {
	return SingleValueEncodingContainerEncode(c, v)
}

// SingleValueEncodingContainerEncodeNil is `container.encodeNil()`.
func SingleValueEncodingContainerEncodeNil(c *SingleValueEncodingContainer) struct{} {
	c.e.Value = nil
	return struct{}{}
}

// EncoderCodingPath is `Encoder.codingPath`; the runtime encoder does not track one, so it is empty.
func EncoderCodingPath(e *Encoder) []string { return []string{} }

// CodingContext is Swift `DecodingError.Context` / `EncodingError.Context`.
type CodingContext struct {
	CodingPath       []string
	DebugDescription string
	UnderlyingError  error
}

// ContextInitCodingPathDebugDescription is `Context(codingPath:debugDescription:)`.
func ContextInitCodingPathDebugDescription(path any, desc string) CodingContext {
	return CodingContext{CodingPath: codingPathStrings(path), DebugDescription: desc}
}

// ContextInitCodingPathDebugDescriptionUnderlyingError is `Context(codingPath:debugDescription:underlyingError:)`.
func ContextInitCodingPathDebugDescriptionUnderlyingError(path any, desc string, underlying error) CodingContext {
	return CodingContext{CodingPath: codingPathStrings(path), DebugDescription: desc, UnderlyingError: underlying}
}

// codingPathStrings renders a coding path, given as key strings or CodingKey values, as strings.
func codingPathStrings(path any) []string {
	switch p := path.(type) {
	case nil:
		return nil
	case []string:
		return p
	case []any:
		out := make([]string, len(p))
		for i, k := range p {
			out[i] = keyString(k)
		}
		return out
	}
	return Iterate[string](path)
}

// describe is Swift's description of a coding error with this context: the case and a summary,
// then the coding path, the debug description and the underlying error, each left out when empty,
// as in "DecodingError.typeMismatch: Expected value of type Int. Path: items[0].n. Debug
// description: Expected to decode Int but found a string instead.".
func (c CodingContext) describe(kind, summary string) string {
	parts := []string{summary}
	if path := codingPathDescription(c.CodingPath); path != "" {
		parts = append(parts, "Path: "+path)
	}
	if c.DebugDescription != "" {
		parts = append(parts, "Debug description: "+c.DebugDescription)
	}
	if c.UnderlyingError != nil {
		parts = append(parts, "Underlying error: "+Str(c.UnderlyingError))
	}
	return kind + ": " + strings.Join(parts, ". ")
}

// codingPathDescription writes a coding path as Swift's coding error descriptions do: keys joined
// with dots, and an array index as "[n]" after the key before it.
func codingPathDescription(path []string) string {
	var b strings.Builder
	for i, k := range path {
		if n := CodingKeyIntValue(k); n != nil {
			b.WriteString("[" + strconv.Itoa(*n) + "]")
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(k)
	}
	return b.String()
}

// codingTypeName is the name a coding error's summary gives the type it carries.
func codingTypeName(t any) string {
	switch x := t.(type) {
	case reflect.Type:
		return decodeTypeName(x)
	case Type:
		return decodeTypeName(x.Reflect())
	}
	return Str(t)
}

// ContextCodingPath is `DecodingError.Context.codingPath`, as coding keys.
func ContextCodingPath(c any) []any {
	path := c.(CodingContext).CodingPath
	out := make([]any, len(path))
	for i, k := range path {
		out[i] = k
	}
	return out
}

// ContextUnderlyingError is `DecodingError.Context.underlyingError`.
func ContextUnderlyingError(c any) *error {
	if err := c.(CodingContext).UnderlyingError; err != nil {
		return &err
	}
	return nil
}

// ContextDebugDescription is `DecodingError.Context.debugDescription`.
func ContextDebugDescription(c any) string { return c.(CodingContext).DebugDescription }

// CodingKeyStringValue is `CodingKey.stringValue` for a key of a coding path.
func CodingKeyStringValue(k any) string { return keyString(k) }

// CodingKeyIntValue is `CodingKey.intValue` for a key of a coding path: the index of an array
// element, which a path records as "Index N".
func CodingKeyIntValue(k any) *int {
	if s, ok := k.(string); ok {
		if rest, ok := strings.CutPrefix(s, "Index "); ok {
			if n, err := strconv.Atoi(rest); err == nil {
				return &n
			}
		}
	}
	return nil
}

// DecodingErrorDataCorruptedContext is `DecodingError.dataCorrupted(_:)`.
func DecodingErrorDataCorruptedContext(c CodingContext) error {
	return DecodingErrorDataCorrupted{F0: c}
}

// DecodingErrorDataCorruptedErrorForKeyInDebugDescription is `DecodingError.dataCorruptedError(forKey:in:debugDescription:)`.
func DecodingErrorDataCorruptedErrorForKeyInDebugDescription(key any, c KeyedDecodingContainer, desc string) error {
	path := append(append([]string{}, c.path...), keyString(key))
	return DecodingErrorDataCorrupted{F0: CodingContext{CodingPath: path, DebugDescription: desc}}
}

// EncodingErrorInvalidValueAny is `EncodingError.invalidValue(_:_:)`.
func EncodingErrorInvalidValueAny(v any, c CodingContext) error {
	return CodingError{c.describe("EncodingError.invalidValue", StringInitReflecting(v)+" ("+decodeTypeName(reflect.TypeOf(v))+")")}
}

// UnkeyedDecodingContainer is Swift `UnkeyedDecodingContainer`.
type UnkeyedDecodingContainer struct {
	items []any
	path  []string
	index int
}

// DecoderUnkeyedContainer is `decoder.unkeyedContainer()`.
func DecoderUnkeyedContainer(d *Decoder) *UnkeyedDecodingContainer {
	items, ok := d.Value.([]any)
	if !ok {
		codingFail("expected an unkeyed container at %s, found %T", strings.Join(d.CodingPath, "."), d.Value)
	}
	return &UnkeyedDecodingContainer{items: items, path: d.CodingPath}
}

// KeyedDecodingContainerNestedUnkeyedContainerForKey is `container.nestedUnkeyedContainer(forKey:)`.
func KeyedDecodingContainerNestedUnkeyedContainerForKey(c KeyedDecodingContainer, key any) *UnkeyedDecodingContainer {
	k := keyString(key)
	path := append(append([]string{}, c.path...), k)
	raw, ok := c.lookup(k)
	if !ok {
		codingFail("key %q not found at %s", k, strings.Join(c.path, "."))
	}
	items, ok := raw.([]any)
	if !ok {
		codingFail("expected an unkeyed container at %s, found %T", strings.Join(path, "."), raw)
	}
	return &UnkeyedDecodingContainer{items: items, path: path}
}

// UnkeyedDecodingContainerSuperDecoder is `UnkeyedDecodingContainer.superDecoder()`: a decoder for the
// next element, after which the container moves past it.
func UnkeyedDecodingContainerSuperDecoder(c *any) *Decoder {
	u := (*c).(*UnkeyedDecodingContainer)
	if u.index >= len(u.items) {
		codingFail("unkeyed container is at end at %s", strings.Join(u.path, "."))
	}
	d := &Decoder{Value: u.items[u.index], CodingPath: append(append([]string{}, u.path...), "Index "+strconv.Itoa(u.index))}
	u.index++
	return d
}

// UnkeyedDecodingContainerIsAtEnd is `UnkeyedDecodingContainer.isAtEnd`.
func UnkeyedDecodingContainerIsAtEnd(c any) bool {
	u := c.(*UnkeyedDecodingContainer)
	return u.index >= len(u.items)
}

// UnkeyedDecodingContainerDecode is `UnkeyedDecodingContainer.decode(_:)`; the index advances only on success.
func UnkeyedDecodingContainerDecode[T any](c *any, _ TypeT[T]) T {
	u := (*c).(*UnkeyedDecodingContainer)
	if u.index >= len(u.items) {
		codingFail("unkeyed container is at end at %s", strings.Join(u.path, "."))
	}
	v := DecodeValue[T](u.items[u.index], append(append([]string{}, u.path...), "Index "+strconv.Itoa(u.index)))
	u.index++
	return v
}

// OutputFormattingWithoutEscapingSlashes is `JSONEncoder.OutputFormatting.withoutEscapingSlashes`.
var OutputFormattingWithoutEscapingSlashes = JSONEncoderOutputFormatting{8}
