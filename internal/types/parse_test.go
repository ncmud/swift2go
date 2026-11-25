package types

import "testing"

func TestParse(t *testing.T) {
	cases := map[string]string{
		"Swift.Int":                                 "Swift.Int",
		"[Swift.Character]":                         "[Swift.Character]",
		"(token: Swift.String, next: Swift.Int)":    "(token: Swift.String, next: Swift.Int)",
		"(Swift.UInt8) -> Swift.Bool":               "(Swift.UInt8) -> Swift.Bool",
		"Swift.Int?":                                "Swift.Int?",
		"Commands.ProtocolMudCommand.Type":          "Commands.ProtocolMudCommand.Type",
		"Commands.TypeTable":                        "Commands.TypeTable",
		"[Swift.Character : Swift.String]":          "[Swift.Character : Swift.String]",
		"Swift.Range<Swift.Int>":                    "Swift.Range<Swift.Int>",
		"A":                                         "A",
		"TermCore.ColorCodec":                       "TermCore.ColorCodec",
		"[[Swift.String]]":                          "[[Swift.String]]",
		"(Swift.Int, Swift.Int) throws -> ()":       "(Swift.Int, Swift.Int) -> ()",
		"Swift.String.Index":                        "Swift.String.Index",
		"Swift.Dictionary<Swift.String, Swift.Int>": "Swift.Dictionary<Swift.String, Swift.Int>",
		"any Any<Self: ~Swift.Copyable, Self: ~Swift.Escapable>.Type?": "any Any.Type?",
		"t.Box<Self.Type>":                                  "t.Box<Self.Type>",
		"Swift.Result<Self.Type, Swift.Error>":              "Swift.Result<Self.Type, Swift.Error>",
		"[any Swift.Collection<Self.Element == Swift.Int>]": "[any Swift.Collection]",
	}
	for in, want := range cases {
		if got := Parse(in).String(); got != want {
			t.Errorf("Parse(%q) = %q, want %q", in, got, want)
		}
	}
	if Parse("A").Kind != Generic {
		t.Errorf("A should be generic")
	}
}

func TestSendingInoutParam(t *testing.T) {
	ft := Parse("(sending inout [Mod.Item]) -> sending Swift.Int")
	if ft.Kind != Func || len(ft.Inout) != 1 || !ft.Inout[0] {
		t.Fatalf("inout lost after sending: %+v %v", ft, ft.Inout)
	}
}
