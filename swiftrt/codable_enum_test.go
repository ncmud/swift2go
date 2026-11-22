package swiftrt

import "testing"

type shape interface{ isShape() }
type shapeCircle struct{ F0 int }
type shapeRect struct {
	F0 int
	F1 *string
}

func (shapeCircle) isShape() {}
func (shapeRect) isShape()   {}

func TestPayloadEnumCoding(t *testing.T) {
	RegisterPayloadEnum[shape](CaseOf[shapeCircle]("circle", "_0"), CaseOf[shapeRect]("rect", "w", "label"))
	if enc := string(YYJSONEncoderEncode(YYJSONEncoderInit(), shape(shapeRect{F0: 3}))); enc != `{"rect":{"w":3}}` {
		t.Fatalf("encoded %s", enc)
	}
	got := DecodeValue[shape](map[string]any{"circle": map[string]any{"_0": EncodeValue(7)}}, nil)
	if c, ok := got.(shapeCircle); !ok || c.F0 != 7 {
		t.Fatalf("decoded %#v", got)
	}
}
