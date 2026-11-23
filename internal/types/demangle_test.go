package types

import (
	"testing"

	"github.com/ncmud/swift2go/internal/swifttest"
)

func TestNormalizeLocal(t *testing.T) {
	cases := map[string]string{
		"Foo #1 in Mod.Type.fn() -> ()": "Mod.Foo",
		"Entry #1 in static Mod.Router.shortestPaths(from: Swift.Int, map: Mod.Map) -> [Swift.Int : Swift.Int]":        "Mod.Entry",
		"[Swift.String : [Appearance #1 in static Mod.Search.dropDuplicates([Mod.Search.Slot]) -> [Mod.Search.Slot]]]": "[Swift.String : [Mod.Appearance]]",
		"Swift.Int": "Swift.Int",
		"Permanent #1 in Mod.Instance.fingerprint : Swift.String":                             "Mod.Permanent",
		"[Permanent #1 in Mod.Instance.fingerprint : Swift.String]":                           "[Mod.Permanent]",
		"[GroupKey #1 in Mod.Exporter.Result.report() -> Swift.String : [Mod.Exporter.Item]]": "[Mod.GroupKey : [Mod.Exporter.Item]]",
		"Header #2 in Tool":   "Tool.Header",
		"[Header #1 in Tool]": "[Tool.Header]",
	}
	for in, want := range cases {
		if got := normalizeLocal(in); got != want {
			t.Errorf("normalizeLocal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripClosures(t *testing.T) {
	cases := map[string]string{
		"Reach #1 in closure #1 () throws -> () in Mod.S.pacing() throws -> Swift.Int":                      "Reach #1 in Mod.S.pacing() throws -> Swift.Int",
		"[Reach #1 in closure #2 (Swift.Int) -> [Swift.Int] in closure #1 () -> () in Mod.f() -> ()]":       "[Reach #1 in Mod.f() -> ()]",
		"Key #1 in implicit closure #1 (Mod.A) -> (Swift.Int, Swift.String) in static Mod.T.g(Mod.A) -> ()": "Key #1 in static Mod.T.g(Mod.A) -> ()",
		"Swift.Int": "Swift.Int",
	}
	for in, want := range cases {
		if got := stripClosures(in); got != want {
			t.Errorf("stripClosures(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeLocal(stripClosures("[Reach #1 in closure #1 () throws -> () in Mod.S.pacing() throws -> Swift.Int]")); got != "[Mod.Reach]" {
		t.Errorf("got %q", got)
	}
}

func TestLocalWrappers(t *testing.T) {
	raw := "Named #1 in static Mod.Lines.existingName(Mod.Instance) -> Swift.String?"
	cases := map[string]string{
		"$s3Mod5LinesO12existingNameySSSgAA8InstanceCFZ5NamedL_VD":   "Mod.Named",
		"$s3Mod5LinesO12existingNameySSSgAA8InstanceCFZ5NamedL_VSgD": "Mod.Named?",
		"$s3Mod5LinesO12existingNameySSSgAA8InstanceCFZ5NamedL_VmD":  "Mod.Named.Type",
	}
	for mangled, want := range cases {
		if got := localWrappers(mangled, raw, normalizeLocal(raw)); got != want {
			t.Errorf("localWrappers(%q) = %q, want %q", mangled, got, want)
		}
	}
}

func TestLocalWrappersInClosureResult(t *testing.T) {
	raw := "() throws -> Anything #1 in Mod.f() -> ()?"
	cases := map[string]string{
		"$s3Mod1fyyF8AnythingL_VSgyKXED": "() throws -> Mod.Anything?",
		"$s3Mod1fyyF8AnythingL_VyKXED":   "() throws -> Mod.Anything",
	}
	for mangled, want := range cases {
		if got := localWrappers(mangled, raw, normalizeLocal(raw)); got != want {
			t.Errorf("localWrappers(%q) = %q, want %q", mangled, got, want)
		}
	}
}

func TestLocalWrappersInTopLevelCode(t *testing.T) {
	cases := map[string][2]string{
		"$s4Tool6HeaderL0_VD":   {"Header #2 in Tool", "Tool.Header"},
		"$s4Tool6HeaderL0_VmD":  {"Header #2 in Tool.Type", "Tool.Header.Type"},
		"$s4Tool6HeaderL_VSgD":  {"Header #1 in Tool?", "Tool.Header?"},
		"$s4Tool6HeaderL1_VSgD": {"Header #3 in Tool?", "Tool.Header?"},
	}
	for mangled, c := range cases {
		if got := localWrappers(mangled, c[0], normalizeLocal(c[0])); got != c[1] {
			t.Errorf("localWrappers(%q) = %q, want %q", mangled, got, c[1])
		}
	}
}

// TestDemanglerSplitsLargeBatches checks that symbols demangled in parallel chunks get the same names
// as in one swift-demangle run.
func TestDemanglerSplitsLargeBatches(t *testing.T) {
	swifttest.Require(t, "xcrun")
	syms := []string{
		"s:4GRDB8DatabaseC10ColumnTypeV4textAEvpZ",
		"$s4GRDB8DatabaseC15TransactionKindOD",
		"$sSaySiGD",
		"$sSDySSSiGD",
		"$sSiSgD",
		"s:5World13AbandonedGearO",
		"$s5World13AbandonedGearOmD",
	}
	whole, err := NewDemangler(syms)
	if err != nil {
		t.Fatal(err)
	}
	defer func(n int) { demangleChunk = n }(demangleChunk)
	demangleChunk = 2
	split, err := NewDemangler(syms)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range syms {
		if whole.Get(s) != split.Get(s) || split.Get(s) == s {
			t.Errorf("%s: %q in chunks, %q in one run", s, split.Get(s), whole.Get(s))
		}
	}
}

func TestNameApart(t *testing.T) {
	distinct := map[string]bool{"Counter": true, "Shared": true}
	for in, want := range map[string]string{
		"Counter #1 in KP.localTypesA() -> Swift.Int":             "Counter_localTypesA #1 in KP.localTypesA() -> Swift.Int",
		"Counter #2 in static KP.Suite.run(a: [Swift.Int]) -> ()": "Counter_run2 #2 in static KP.Suite.run(a: [Swift.Int]) -> ()",
		"KP.(Shared in _BF9D067622127EBB8CB9A441B762713A).v":      "KP.(Shared_BF9D0676 in _BF9D067622127EBB8CB9A441B762713A).v",
		"Other #1 in KP.f() -> ()":                                "Other #1 in KP.f() -> ()",
		"KP.Edge.(Shared in _BF9D067622127EBB8CB9A441B762713A)":   "KP.Edge.(Shared in _BF9D067622127EBB8CB9A441B762713A)",
		"[KP.(Shared in _AE80)]":                                  "[KP.(Shared_AE80 in _AE80)]",
	} {
		if got := nameApart(in, distinct); got != want {
			t.Errorf("nameApart(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeLocal(nameApart("Counter #1 in KP.localTypesA() -> Swift.Int", distinct)); got != "KP.Counter_localTypesA" {
		t.Errorf("normalized = %q", got)
	}
}
