package swiftrt

import (
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
			out[i] = Str(k)
		}
		return out
	}
	return Iterate[string](path)
}

func (c CodingContext) error(kind string) CodingError {
	msg := kind + ": " + c.DebugDescription
	if len(c.CodingPath) > 0 {
		msg += " (at " + strings.Join(c.CodingPath, ".") + ")"
	}
	return CodingError{msg}
}

// DecodingErrorDataCorruptedContext is `DecodingError.dataCorrupted(_:)`.
func DecodingErrorDataCorruptedContext(c CodingContext) error { return c.error("dataCorrupted") }

// DecodingErrorDataCorruptedErrorForKeyInDebugDescription is `DecodingError.dataCorruptedError(forKey:in:debugDescription:)`.
func DecodingErrorDataCorruptedErrorForKeyInDebugDescription(key any, c KeyedDecodingContainer, desc string) error {
	path := append(append([]string{}, c.path...), keyString(key))
	return CodingContext{CodingPath: path, DebugDescription: desc}.error("dataCorrupted")
}

// EncodingErrorInvalidValueAny is `EncodingError.invalidValue(_:_:)`.
func EncodingErrorInvalidValueAny(v any, c CodingContext) error {
	return c.error("invalidValue " + Str(v))
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
