package swiftrt

import (
	"strconv"
	"strings"
	"testing"
)

// The fixtures in this file were produced on macOS 27 with Swift 6.4.2 by a small SwiftPM program
// that encodes and decodes the same values with Foundation's JSONEncoder, JSONDecoder and
// JSONSerialization, and with swift-yyjson b861a01 (the revision JSONCoding pins) configured as
// JSONCoding's GameJSONEncoder configures it. Its output was pasted here unchanged.

type fixtureOrdered struct {
	Z int            `swift:"z"`
	A string         `swift:"a"`
	M []int          `swift:"m"`
	E map[string]int `swift:"e"`
	O *string        `swift:"o"`
}

type fixtureP struct {
	B []int          `swift:"b"`
	A string         `swift:"a"`
	C map[string]int `swift:"c"`
}

type fixtureString struct {
	S string `swift:"s"`
}

type fixtureDouble struct {
	D float64 `swift:"d"`
}

type fixtureFloat struct {
	F float32 `swift:"f"`
}

type fixtureInts struct {
	Min int64  `swift:"min"`
	Max uint64 `swift:"max"`
}

// fixtureTwice writes "b" twice, the second time as nil.
type fixtureTwice struct{}

func (fixtureTwice) Encode(e *Encoder) {
	c := EncoderContainerKeyedBy(e, nil)
	KeyedEncodingContainerEncodeForKey(&c, 1, "b")
	KeyedEncodingContainerEncodeForKey(&c, 2, "a")
	KeyedEncodingContainerEncodeNilForKey(&c, "b")
	KeyedEncodingContainerEncodeForKey(&c, 4, "c")
}

func fixtureValue(t *testing.T, label string) any {
	switch {
	case label == "ordered":
		return fixtureOrdered{Z: 1, A: "x", M: []int{}, E: map[string]int{}}
	case label == "p":
		return fixtureP{B: []int{1, 2}, A: "x/y", C: map[string]int{}}
	case label == "strings":
		return fixtureString{"a/b <>&'\x01\x1f\x7f\t\n\r\b\f\u2028é😀\"\\"}
	case label == "ints":
		return fixtureInts{Min: -1 << 63, Max: 1<<64 - 1}
	case label == "twice":
		return fixtureTwice{}
	case label == "topString":
		return "a/b"
	case strings.HasPrefix(label, "double "):
		f, err := strconv.ParseFloat(strings.TrimPrefix(label, "double "), 64)
		if err != nil {
			t.Fatal(err)
		}
		return fixtureDouble{f}
	case strings.HasPrefix(label, "float "):
		f, err := strconv.ParseFloat(strings.TrimPrefix(label, "float "), 32)
		if err != nil {
			t.Fatal(err)
		}
		return fixtureFloat{float32(f)}
	}
	t.Fatalf("no value for %q", label)
	return nil
}

// fixtureEncode encodes v with Foundation's JSONEncoder ("foundation.*") or with swift-yyjson set
// up as JSONCoding's GameJSONEncoder sets it up ("yyjson.*").
func fixtureEncode(t *testing.T, variant string, v any) string {
	switch variant {
	case "foundation.default":
		return string(JSONEncoderEncode(JSONEncoderInit(), v))
	case "foundation.sorted":
		return string(JSONEncoderEncode(&JSONEncoder{OutputFormatting: OutputFormattingSortedKeys}, v))
	case "foundation.prettySorted":
		return string(JSONEncoderEncode(&JSONEncoder{OutputFormatting: OptionSetUnion(OutputFormattingPrettyPrinted, OutputFormattingSortedKeys)}, v))
	case "foundation.noSlashSorted":
		return string(JSONEncoderEncode(&JSONEncoder{OutputFormatting: OptionSetUnion(OutputFormattingWithoutEscapingSlashes, OutputFormattingSortedKeys)}, v))
	case "yyjson.default":
		return string(YYJSONEncoderEncode(YYJSONEncoder{WriteOptions: YYJSONWriteOptionsEscapeSlashes}, v))
	case "yyjson.sorted":
		return string(YYJSONEncoderEncode(YYJSONEncoder{WriteOptions: OptionSetUnion(YYJSONWriteOptionsEscapeSlashes, YYJSONWriteOptionsSortedKeys)}, v))
	case "yyjson.prettySorted":
		o := OptionSetUnion(OptionSetUnion(YYJSONWriteOptionsPrettyPrinted, YYJSONWriteOptionsIndentationTwoSpaces), OptionSetUnion(YYJSONWriteOptionsEscapeSlashes, YYJSONWriteOptionsSortedKeys))
		return string(YYJSONEncoderEncode(YYJSONEncoder{WriteOptions: o}, v))
	case "yyjson.noSlash":
		return string(YYJSONEncoderEncode(YYJSONEncoder{}, v))
	}
	t.Fatalf("no encoder for %q", variant)
	return ""
}

func TestJSONEncodeMatchesSwift(t *testing.T) {
	for _, c := range []struct{ label, variant, want string }{
		{"ordered", "foundation.sorted", "{\"a\":\"x\",\"e\":{},\"m\":[],\"z\":1}"},
		{"ordered", "foundation.prettySorted", "{\n  \"a\" : \"x\",\n  \"e\" : {\n\n  },\n  \"m\" : [\n\n  ],\n  \"z\" : 1\n}"},
		{"ordered", "foundation.noSlashSorted", "{\"a\":\"x\",\"e\":{},\"m\":[],\"z\":1}"},
		{"ordered", "yyjson.default", "{\"z\":1,\"a\":\"x\",\"m\":[],\"e\":{}}"},
		{"ordered", "yyjson.sorted", "{\"a\":\"x\",\"e\":{},\"m\":[],\"z\":1}"},
		{"ordered", "yyjson.prettySorted", "{\n  \"a\": \"x\",\n  \"e\": {},\n  \"m\": [],\n  \"z\": 1\n}"},
		{"ordered", "yyjson.noSlash", "{\"z\":1,\"a\":\"x\",\"m\":[],\"e\":{}}"},
		{"p", "foundation.sorted", "{\"a\":\"x\\/y\",\"b\":[1,2],\"c\":{}}"},
		{"p", "foundation.prettySorted", "{\n  \"a\" : \"x\\/y\",\n  \"b\" : [\n    1,\n    2\n  ],\n  \"c\" : {\n\n  }\n}"},
		{"p", "foundation.noSlashSorted", "{\"a\":\"x/y\",\"b\":[1,2],\"c\":{}}"},
		{"p", "yyjson.default", "{\"b\":[1,2],\"a\":\"x\\/y\",\"c\":{}}"},
		{"p", "yyjson.sorted", "{\"a\":\"x\\/y\",\"b\":[1,2],\"c\":{}}"},
		{"p", "yyjson.prettySorted", "{\n  \"a\": \"x\\/y\",\n  \"b\": [\n    1,\n    2\n  ],\n  \"c\": {}\n}"},
		{"p", "yyjson.noSlash", "{\"b\":[1,2],\"a\":\"x/y\",\"c\":{}}"},
		{"strings", "foundation.default", "{\"s\":\"a\\/b <>&'\\u0001\\u001f\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"}"},
		{"strings", "foundation.sorted", "{\"s\":\"a\\/b <>&'\\u0001\\u001f\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"}"},
		{"strings", "foundation.prettySorted", "{\n  \"s\" : \"a\\/b <>&'\\u0001\\u001f\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"\n}"},
		{"strings", "foundation.noSlashSorted", "{\"s\":\"a/b <>&'\\u0001\\u001f\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"}"},
		{"strings", "yyjson.default", "{\"s\":\"a\\/b <>&'\\u0001\\u001F\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"}"},
		{"strings", "yyjson.sorted", "{\"s\":\"a\\/b <>&'\\u0001\\u001F\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"}"},
		{"strings", "yyjson.prettySorted", "{\n  \"s\": \"a\\/b <>&'\\u0001\\u001F\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"\n}"},
		{"strings", "yyjson.noSlash", "{\"s\":\"a/b <>&'\\u0001\\u001F\x7f\\t\\n\\r\\b\\f\u2028é😀\\\"\\\\\"}"},
		{"double 1.0", "foundation.default", "{\"d\":1}"},
		{"double 1.0", "yyjson.default", "{\"d\":1.0}"},
		{"double 0.1", "foundation.default", "{\"d\":0.1}"},
		{"double 0.1", "yyjson.default", "{\"d\":0.1}"},
		{"double -0.0", "foundation.default", "{\"d\":-0}"},
		{"double -0.0", "yyjson.default", "{\"d\":-0.0}"},
		{"double 1000000000000000.0", "foundation.default", "{\"d\":1000000000000000}"},
		{"double 1000000000000000.0", "yyjson.default", "{\"d\":1000000000000000.0}"},
		{"double 1e+16", "foundation.default", "{\"d\":1e+16}"},
		{"double 1e+16", "yyjson.default", "{\"d\":10000000000000000.0}"},
		{"double 1e+20", "foundation.default", "{\"d\":1e+20}"},
		{"double 1e+20", "yyjson.default", "{\"d\":100000000000000000000.0}"},
		{"double 1e+21", "foundation.default", "{\"d\":1e+21}"},
		{"double 1e+21", "yyjson.default", "{\"d\":1e21}"},
		{"double 1.5e+300", "foundation.default", "{\"d\":1.5e+300}"},
		{"double 1.5e+300", "yyjson.default", "{\"d\":1.5e300}"},
		{"double 1e-05", "foundation.default", "{\"d\":1e-05}"},
		{"double 1e-05", "yyjson.default", "{\"d\":0.00001}"},
		{"double 1e-06", "foundation.default", "{\"d\":1e-06}"},
		{"double 1e-06", "yyjson.default", "{\"d\":0.000001}"},
		{"double 1e-07", "foundation.default", "{\"d\":1e-07}"},
		{"double 1e-07", "yyjson.default", "{\"d\":1e-7}"},
		{"double 5e-324", "foundation.default", "{\"d\":5e-324}"},
		{"double 5e-324", "yyjson.default", "{\"d\":5e-324}"},
		{"double 2.5e-08", "foundation.default", "{\"d\":2.5e-08}"},
		{"double 2.5e-08", "yyjson.default", "{\"d\":2.5e-8}"},
		{"double 9007199254740992.0", "foundation.default", "{\"d\":9007199254740992}"},
		{"double 9007199254740992.0", "yyjson.default", "{\"d\":9007199254740992.0}"},
		{"double 0.30000000000000004", "foundation.default", "{\"d\":0.30000000000000004}"},
		{"double 0.30000000000000004", "yyjson.default", "{\"d\":0.30000000000000004}"},
		{"float 0.1", "foundation.default", "{\"f\":0.1}"},
		{"float 0.1", "yyjson.default", "{\"f\":0.10000000149011612}"},
		{"float 1.0", "foundation.default", "{\"f\":1}"},
		{"float 1.0", "yyjson.default", "{\"f\":1.0}"},
		{"float 3.4e+38", "foundation.default", "{\"f\":3.4e+38}"},
		{"float 3.4e+38", "yyjson.default", "{\"f\":3.3999999521443642e38}"},
		{"float 1e-07", "foundation.default", "{\"f\":1e-07}"},
		{"float 1e-07", "yyjson.default", "{\"f\":1.0000000116860974e-7}"},
		{"ints", "foundation.sorted", "{\"max\":18446744073709551615,\"min\":-9223372036854775808}"},
		{"ints", "foundation.prettySorted", "{\n  \"max\" : 18446744073709551615,\n  \"min\" : -9223372036854775808\n}"},
		{"ints", "foundation.noSlashSorted", "{\"max\":18446744073709551615,\"min\":-9223372036854775808}"},
		{"ints", "yyjson.default", "{\"min\":-9223372036854775808,\"max\":18446744073709551615}"},
		{"ints", "yyjson.sorted", "{\"max\":18446744073709551615,\"min\":-9223372036854775808}"},
		{"ints", "yyjson.prettySorted", "{\n  \"max\": 18446744073709551615,\n  \"min\": -9223372036854775808\n}"},
		{"ints", "yyjson.noSlash", "{\"min\":-9223372036854775808,\"max\":18446744073709551615}"},
		{"twice", "foundation.sorted", "{\"a\":2,\"b\":null,\"c\":4}"},
		{"twice", "foundation.prettySorted", "{\n  \"a\" : 2,\n  \"b\" : null,\n  \"c\" : 4\n}"},
		{"twice", "foundation.noSlashSorted", "{\"a\":2,\"b\":null,\"c\":4}"},
		{"twice", "yyjson.default", "{\"b\":null,\"a\":2,\"c\":4}"},
		{"twice", "yyjson.sorted", "{\"a\":2,\"b\":null,\"c\":4}"},
		{"twice", "yyjson.prettySorted", "{\n  \"a\": 2,\n  \"b\": null,\n  \"c\": 4\n}"},
		{"twice", "yyjson.noSlash", "{\"b\":null,\"a\":2,\"c\":4}"},
		{"topString", "foundation.default", "\"a\\/b\""},
		{"topString", "foundation.sorted", "\"a\\/b\""},
		{"topString", "foundation.prettySorted", "\"a\\/b\""},
		{"topString", "foundation.noSlashSorted", "\"a/b\""},
		{"topString", "yyjson.default", "\"a\\/b\""},
		{"topString", "yyjson.sorted", "\"a\\/b\""},
		{"topString", "yyjson.prettySorted", "\"a\\/b\""},
		{"topString", "yyjson.noSlash", "\"a/b\""},
	} {
		if got := fixtureEncode(t, c.variant, fixtureValue(t, c.label)); got != c.want {
			t.Errorf("%s %s:\n got %q\nwant %q", c.label, c.variant, got, c.want)
		}
	}
	for _, v := range []any{fixtureDouble{FloatNan}, fixtureFloat{float32(FloatNan)}} {
		if Catch(func() { JSONEncoderEncode(JSONEncoderInit(), v) }) == nil {
			t.Errorf("JSONEncoder encoded %v", v)
		}
		if Catch(func() { YYJSONEncoderEncode(YYJSONEncoderInit(), v) }) == nil {
			t.Errorf("YYJSONEncoder encoded %v", v)
		}
	}
}

type fixtureA struct {
	A int `swift:"a"`
}

type fixtureAS struct {
	A string `swift:"a"`
}

type fixtureAD struct {
	A float64 `swift:"a"`
}

type fixtureAI64 struct {
	A int64 `swift:"a"`
}

type fixtureAU64 struct {
	A uint64 `swift:"a"`
}

var fixtureInputs = map[string]string{
	"duplicate key":      `{"a":1,"a":2}`,
	"key case":           `{"A":1}`,
	"invalid UTF-8":      "{\"a\":\"\xff\"}",
	"unpaired surrogate": `{"a":"\ud800"}`,
	"surrogate pair":     `{"a":"\ud83d\ude00"}`,
	"byte-order mark":    "\xef\xbb\xbf{\"a\":1}",
	"Int64 overflow":     `{"a":18446744073709551616}`,
	"UInt64 max":         `{"a":18446744073709551615}`,
	"Double overflow":    `{"a":1e400}`,
	"integral 1.0":       `{"a":1.0}`,
	"integral 1e2":       `{"a":1e2}`,
	"escaped slash":      `{"a":"\/"}`,
	"trailing content":   `{"a":1} x`,
	"control character":  "{\"a\":\"\x01\"}",
}

// fixtureDecode decodes data with decode into the type Swift decoded that case into, returning
// the field's description or "error".
func fixtureDecode(label string, data []byte, foundation bool) (out string) {
	err := Catch(func() {
		decode := func(t any) any {
			switch t.(type) {
			case fixtureAS:
				if foundation {
					return JSONDecoderDecodeFrom(JSONDecoderInit(), TypeT[fixtureAS]{}, data).A
				}
				return YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeT[fixtureAS]{}, data).A
			case fixtureAD:
				if foundation {
					return JSONDecoderDecodeFrom(JSONDecoderInit(), TypeT[fixtureAD]{}, data).A
				}
				return YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeT[fixtureAD]{}, data).A
			case fixtureAI64:
				if foundation {
					return JSONDecoderDecodeFrom(JSONDecoderInit(), TypeT[fixtureAI64]{}, data).A
				}
				return YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeT[fixtureAI64]{}, data).A
			case fixtureAU64:
				if foundation {
					return JSONDecoderDecodeFrom(JSONDecoderInit(), TypeT[fixtureAU64]{}, data).A
				}
				return YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeT[fixtureAU64]{}, data).A
			}
			if foundation {
				return JSONDecoderDecodeFrom(JSONDecoderInit(), TypeT[fixtureA]{}, data).A
			}
			return YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeT[fixtureA]{}, data).A
		}
		var t any = fixtureA{}
		switch label {
		case "invalid UTF-8", "unpaired surrogate", "surrogate pair", "escaped slash", "control character":
			t = fixtureAS{}
		case "Int64 overflow":
			t = fixtureAI64{}
		case "UInt64 max":
			t = fixtureAU64{}
		case "Double overflow":
			t = fixtureAD{}
		}
		out = Str(decode(t))
	})
	if err != nil {
		return "error"
	}
	return out
}

func TestJSONDecodeMatchesSwift(t *testing.T) {
	for _, c := range []struct{ label, foundation, yyjson string }{
		{"duplicate key", "1", "1"},
		{"key case", "error", "error"},
		{"invalid UTF-8", "error", "error"},
		{"unpaired surrogate", "error", "error"},
		{"surrogate pair", "😀", "😀"},
		{"byte-order mark", "1", "error"},
		{"Int64 overflow", "error", "error"},
		{"UInt64 max", "18446744073709551615", "18446744073709551615"},
		{"Double overflow", "error", "error"},
		{"integral 1.0", "1", "1"},
		{"integral 1e2", "100", "100"},
		{"escaped slash", "/", "/"},
		{"trailing content", "error", "error"},
		{"control character", "error", "error"},
	} {
		data := []byte(fixtureInputs[c.label])
		if got := fixtureDecode(c.label, data, true); got != c.foundation {
			t.Errorf("JSONDecoder %s: got %s, want %s", c.label, got, c.foundation)
		}
		if got := fixtureDecode(c.label, data, false); got != c.yyjson {
			t.Errorf("YYJSONDecoder %s: got %s, want %s", c.label, got, c.yyjson)
		}
	}
}

func TestJSONSerializationMatchesSwift(t *testing.T) {
	read := func(parse func([]byte) any, data []byte) (out string) {
		if err := Catch(func() { out = Str((*As[map[string]any](parse(data)))["a"]) }); err != nil {
			return "error"
		}
		return out
	}
	for _, c := range []struct{ label, foundation, yyjson string }{
		{"duplicate key", "1", "2"},
		{"byte-order mark", "1", "error"},
		{"invalid UTF-8", "error", "error"},
	} {
		data := []byte(fixtureInputs[c.label])
		if got := read(NSJSONSerializationJsonObjectWith, data); got != c.foundation {
			t.Errorf("JSONSerialization %s: got %s, want %s", c.label, got, c.foundation)
		}
		if got := read(YYJSONSerializationJsonObjectWith, data); got != c.yyjson {
			t.Errorf("YYJSONSerialization %s: got %s, want %s", c.label, got, c.yyjson)
		}
	}
	obj := map[string]any{"e": []any{}, "d": map[string]any{}, "n": map[string]any{"x": 1}}
	opts := NSJSONWritingOptions{NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingPrettyPrinted.RawValue | NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingSortedKeys.RawValue}
	want := "{\n  \"d\" : {\n\n  },\n  \"e\" : [\n\n  ],\n  \"n\" : {\n    \"x\" : 1\n  }\n}"
	if got := string(NSJSONSerializationDataWithJSONObjectOptions(obj, opts)); got != want {
		t.Errorf("pretty empty containers:\n got %q\nwant %q", got, want)
	}
}
