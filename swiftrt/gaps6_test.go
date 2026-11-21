package swiftrt

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDecimalInitString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"9999999999999999.99", "9999999999999999.99"},
		{"12abc", "12"},
		{"abc", "nil"},
		{"", "nil"},
		{"x", "nil"},
		{"-", "0"},
		{" abc", "0"},
		{"e5", "0"},
		{"1.50", "1.5"},
		{"1e3", "1000"},
		{"1E-3", "0.001"},
		{".5", "0.5"},
		{"+3", "3"},
		{"-1.25e2x", "-125"},
		{"\t5", "5"},
		{" 5", "nil"},
		{"1e1e1", "10"},
		{"1e127", "1" + strings.Repeat("0", 127)},
		{"1e128", "nil"},
		{"1e-128", "0." + strings.Repeat("0", 127) + "1"},
		{"1e-129", "nil"},
		{"10e-129", "nil"},
		{"0e999", "nil"},
		{"99999999999999999999999999999999999999999", strings.Repeat("9", 38) + "000"},
		{"340282366920938463463374607431768211456", "340282366920938463463374607431768211450"},
		{"1." + strings.Repeat("9", 41), "1." + strings.Repeat("9", 38)},
		{"-0.0", "0"},
		{"-0.5", "-0.5"},
	}
	for _, c := range cases {
		got := "nil"
		if d := NSDecimalOptInitString(c.in); d != nil {
			got = d.Description()
		}
		if got != c.want {
			t.Errorf("Decimal(string: %q) = %s, want %s", c.in, got, c.want)
		}
	}
	if !Equal(*NSDecimalOptInitString("1.0"), *NSDecimalOptInitString("1")) {
		t.Error("1.0 != 1")
	}
	if e := DecimalExponent(*NSDecimalOptInitString("100e126")); e != 127 {
		t.Errorf("100e126 exponent = %d, want 127", e)
	}
	if !DecimalLess(*NSDecimalOptInitString("-5"), *NSDecimalOptInitString("0.2")) || DecimalLess(DecimalInitInt(3), DecimalInitInt(-3)) {
		t.Error("Decimal ordering")
	}
	if got := DecimalInitInt(int64(-9223372036854775808)).Description(); got != "-9223372036854775808" {
		t.Errorf("Decimal(Int64.min) = %s", got)
	}
}

type codableExactRecord struct {
	ID       int64               `swift:"id"`
	Unsigned uint64              `swift:"unsigned"`
	Number   Decimal             `swift:"number"`
	Data     []byte              `swift:"data"`
	Tags     map[string]struct{} `swift:"tags"`
	Counts   map[int]int         `swift:"counts"`
	Ratio    float64             `swift:"ratio"`
}

func TestCodableKeepsIntegersDecimalsDataAndSets(t *testing.T) {
	r := codableExactRecord{
		ID: -9223372036854775808, Unsigned: 18446744073709551615,
		Number: *NSDecimalOptInitString("-0.5e3"), Data: []byte{0, 255, 42},
		Tags: map[string]struct{}{"a": {}}, Counts: map[int]int{1: 2, -3: 4}, Ratio: 3,
	}
	enc := JSONEncoderInit()
	enc.OutputFormatting = OutputFormattingSortedKeys
	want := `{"counts":{"-3":4,"1":2},"data":"AP8q","id":-9223372036854775808,"number":-500,"ratio":3,"tags":["a"],"unsigned":18446744073709551615}`
	if got := string(JSONEncoderEncode(enc, r)); got != want {
		t.Errorf("JSONEncoder = %s\nwant %s", got, want)
	}
	ge := YYJSONEncoderInit()
	ge.WriteOptions = YYJSONWriteOptionsSortedKeys
	wantYY := `{"counts":{"-3":4,"1":2},"data":"AP8q","id":-9223372036854775808,"number":-500,"ratio":3.0,"tags":["a"],"unsigned":18446744073709551615}`
	yy := YYJSONEncoderEncode(ge, r)
	if string(yy) != wantYY {
		t.Errorf("YYJSONEncoder = %s\nwant %s", yy, wantYY)
	}
	back := JSONDecoderDecodeFrom(JSONDecoderInit(), TypeOf[codableExactRecord](), yy)
	if !Equal(back, r) {
		t.Errorf("round trip = %+v", back)
	}
	for _, c := range []struct {
		in string
		ok bool
	}{{`{"a":1.0}`, true}, {`{"a":1e2}`, true}, {`{"a":1.5}`, false}, {`{"a":"1"}`, false}, {`{"a":9223372036854775808}`, false}} {
		err := Catch(func() { JSONDecoderDecodeFrom(JSONDecoderInit(), TypeOf[map[string]int](), []byte(c.in)) })
		if (err == nil) != c.ok {
			t.Errorf("decode [String: Int] from %s: err = %v", c.in, err)
		}
	}
	if err := Catch(func() { JSONDecoderDecodeFrom(JSONDecoderInit(), TypeOf[map[int]int](), []byte(`{"x":1}`)) }); err == nil {
		t.Error("[Int: Int] decoded a non-integer key")
	}
}

func TestJSONSerializationFoundationValues(t *testing.T) {
	text := []byte(`{"true":true,"zero":0,"large":9223372036854775807,"null":null,"array":[1,true,"a"]}`)
	a := MustAs[Dictionary](NSJSONSerializationJsonObjectWith(text))
	b := MustAs[Dictionary](YYJSONSerializationJsonObjectWith(text))
	if !Equal(a, b) {
		t.Errorf("Foundation %v != yyjson %v", a, b)
	}
	var ns NSError
	if err := Catch(func() { NSJSONSerializationJsonObjectWith([]byte("5")) }); !errors.As(err, &ns) || ns.Code != 3840 {
		t.Errorf("top-level fragment: %v", err)
	}
	obj := map[string]any{"a": 1.0, "b": 0.1, "c": 1e21, "e": "x/y", "f": true, "h": []int{1, 2}, "m": "\u0001é"}
	got := string(NSJSONSerializationDataWithJSONObjectOptions(obj, NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingSortedKeys))
	want := `{"a":1,"b":0.1,"c":1e+21,"e":"x\/y","f":true,"h":[1,2],"m":"\u0001é"}`
	if got != want {
		t.Errorf("data(withJSONObject:) = %s\nwant %s", got, want)
	}
	pretty := string(NSJSONSerializationDataWithJSONObjectOptions(map[string]any{"a": []any{1}, "b": 2},
		NSJSONWritingOptions{NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingSortedKeys.RawValue | NSJSONWritingOptionsCENSJSONWritingOptionsNSJSONWritingPrettyPrinted.RawValue}))
	if pretty != "{\n  \"a\" : [\n    1\n  ],\n  \"b\" : 2\n}" {
		t.Errorf("pretty = %q", pretty)
	}
	if err := Catch(func() {
		NSJSONSerializationDataWithJSONObjectOptions(map[string]any{"a": FloatNan}, NSJSONWritingOptions{})
	}); !errors.As(err, &ns) {
		t.Errorf("NaN write: %v", err)
	}
}

type registeredShape interface{ isRegisteredShape() }

type registeredShapeCircle struct{ r int }

func (registeredShapeCircle) isRegisteredShape() {}

func registeredShapeEncode(s registeredShape, e *Encoder) {
	c := EncoderSingleValueContainer(e)
	SingleValueEncodingContainerEncode(&c, s.(registeredShapeCircle).r)
}

func TestRegisteredEncoderEncodesInterfaceValues(t *testing.T) {
	RegisterEncoder(registeredShapeEncode)
	got := string(JSONEncoderEncode(JSONEncoderInit(), []registeredShape{registeredShapeCircle{3}}))
	if got != "[3]" {
		t.Errorf("encode = %s, want [3]", got)
	}
}

type thrownEnum interface{ isThrownEnum() }

type thrownEnumDuplicate struct{ F0 string }

func (thrownEnumDuplicate) isThrownEnum() {}

func TestThrownNonErrorValuesKeepTheirType(t *testing.T) {
	err := Catch(func() { panic(Throw(thrownEnum(thrownEnumDuplicate{"body"}))) })
	if !Is[thrownEnum](err) || !Equal(MustAs[thrownEnum](err), thrownEnum(thrownEnumDuplicate{"body"})) {
		t.Errorf("caught %#v", err)
	}
	if p := As[thrownEnumDuplicate](err); p == nil || p.F0 != "body" {
		t.Errorf("as? = %v", p)
	}
	if err.Error() != "{body}" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestClosedRangeFilterAndDisjointSequences(t *testing.T) {
	got := ClosedRangeFilter(ClosedRange[int8]{Lo: 120, Hi: 127}, func(i int8) bool { return i%2 == 1 })
	if !Equal(got, []int8{121, 123, 125, 127}) {
		t.Errorf("filter = %v", got)
	}
	set := map[int]struct{}{1: {}, 2: {}}
	if SetIsDisjointWith(set, []int{2, 5}) || !SetIsDisjointWith(set, []int{3}) || !SetIsDisjointWith(set, map[int]struct{}{4: {}}) {
		t.Error("isDisjoint(with:)")
	}
}

func TestCreateDirectoryAtPath(t *testing.T) {
	dir := t.TempDir() + "/a/b"
	if err := Catch(func() {
		NSFileManagerCreateDirectoryAtPathWithIntermediateDirectories(NSFileManagerDefault, dir, false)
	}); err == nil {
		t.Error("created a directory with a missing parent")
	}
	NSFileManagerCreateDirectoryAtPathWithIntermediateDirectories(NSFileManagerDefault, dir, true)
	NSFileManagerCreateDirectoryAtPathWithIntermediateDirectories(NSFileManagerDefault, dir, true)
	if err := Catch(func() {
		NSFileManagerCreateDirectoryAtPathWithIntermediateDirectories(NSFileManagerDefault, dir, false)
	}); err == nil {
		t.Error("created an existing directory without intermediates")
	}
}

func TestEscapedPatternAndCopyItem(t *testing.T) {
	var all strings.Builder
	for c := byte(0x20); c < 0x7f; c++ {
		all.WriteByte(c)
	}
	want := ` !"#\$%&'\(\)\*\+,-\.\/0123456789:;<=>\?@ABCDEFGHIJKLMNOPQRSTUVWXYZ\[\\]\^_` + "`" + `abcdefghijklmnopqrstuvwxyz\{\|\}~`
	if got := NSRegularExpressionEscapedPatternFor(all.String()); got != want {
		t.Errorf("escapedPattern = %s\nwant %s", got, want)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/src/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/src/sub/f", []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	NSFileManagerCopyItemAtTo(NSFileManagerDefault, URLInitFileURLWithPath(dir+"/src"), URLInitFileURLWithPath(dir+"/dst"))
	if b, err := os.ReadFile(dir + "/dst/sub/f"); err != nil || string(b) != "y" {
		t.Errorf("copied file = %q, %v", b, err)
	}
	var ns NSError
	err := Catch(func() {
		NSFileManagerCopyItemAtTo(NSFileManagerDefault, URLInitFileURLWithPath(dir+"/src"), URLInitFileURLWithPath(dir+"/dst"))
	})
	if !errors.As(err, &ns) || ns.Code != 516 {
		t.Errorf("existing destination: %v", err)
	}
	err = Catch(func() {
		NSFileManagerCopyItemAtTo(NSFileManagerDefault, URLInitFileURLWithPath(dir+"/none"), URLInitFileURLWithPath(dir+"/x"))
	})
	if !errors.As(err, &ns) || ns.Code != 260 {
		t.Errorf("missing source: %v", err)
	}
}

func TestOpenCreatesAndAppends(t *testing.T) {
	path := t.TempDir() + "/log"
	for _, line := range []string{"a\n", "b\n"} {
		fd := OpenUnsafePointerIntInt(path, CMacroO_WRONLY|CMacroO_APPEND|CMacroO_CREAT|CMacroO_CLOEXEC, 0o644)
		if fd < 0 {
			t.Fatal("open failed")
		}
		f := os.NewFile(uintptr(fd), path)
		_, _ = f.WriteString(line)
		f.Close()
	}
	if b, _ := os.ReadFile(path); string(b) != "a\nb\n" {
		t.Errorf("contents = %q", b)
	}
	if OpenUnsafePointerIntInt(path+"/x", CMacroO_RDONLY, 0) != -1 {
		t.Error("opened a path under a file")
	}
}
