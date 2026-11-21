package types

import "testing"

func TestNormalizeLocal(t *testing.T) {
	cases := map[string]string{
		"Foo #1 in Mod.Type.fn() -> ()": "Mod.Foo",
		"Entry #1 in static Mod.Router.shortestPaths(from: Swift.Int, map: Mod.Map) -> [Swift.Int : Swift.Int]":        "Mod.Entry",
		"[Swift.String : [Appearance #1 in static Mod.Search.dropDuplicates([Mod.Search.Slot]) -> [Mod.Search.Slot]]]": "[Swift.String : [Mod.Appearance]]",
		"Swift.Int": "Swift.Int",
		"[GroupKey #1 in Mod.Exporter.Result.report() -> Swift.String : [Mod.Exporter.Item]]": "[Mod.GroupKey : [Mod.Exporter.Item]]",
	}
	for in, want := range cases {
		if got := normalizeLocal(in); got != want {
			t.Errorf("normalizeLocal(%q) = %q, want %q", in, got, want)
		}
	}
}
