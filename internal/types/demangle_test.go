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
	}
	for in, want := range cases {
		if got := normalizeLocal(in); got != want {
			t.Errorf("normalizeLocal(%q) = %q, want %q", in, got, want)
		}
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
