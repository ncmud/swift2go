package swiftmod

import (
	"strings"
	"testing"
)

const kickSource = `enum Outer {
    @MudCommand(name: "nest")
    struct Inner { }
}
@Marked private struct P {}
struct Inner { }
`

// The stderr of `swiftc -frontend -dump-ast -dump-macro-expansions` for kickSource.
const kickExpansions = `@__swiftmacro_1T5OuterO5Inner10MudCommandfMe_.swift
------------------------------
extension Outer.Inner: MudCommand {
    static var name: String {
        "nest"
    }
}
------------------------------
@__swiftmacro_1T1P33_6ECFB85FE7B14C521A0D3B5FF3DE31E2LL6MarkedfMe_.swift
------------------------------
extension P: Marker {
}
------------------------------
@__swiftmacro_1T5Flags7FlagSetfMm_.swift
------------------------------
static let all = 1
------------------------------
`

func kickDemangle(mangled []string) ([]string, error) {
	return []string{
		"extension macro @MudCommand expansion #1 of Inner in T.Outer",
		"extension macro @Marked expansion #1 of (P in _6ECFB85FE7B14C521A0D3B5FF3DE31E2) in T",
		"member macro @FlagSet expansion #1 of Flags in T",
	}, nil
}

func TestSpliceExtensionsReplacesAttributesWithTheirExtensions(t *testing.T) {
	exps, err := parseExpansions(kickExpansions, kickDemangle)
	if err != nil {
		t.Fatal(err)
	}
	if len(exps) != 3 || exps[0].Role != 'e' || exps[0].Decl != "Inner" || exps[0].Context != "Outer" || exps[1].Decl != "P" || exps[1].Context != "T" || exps[2].Role != 'm' {
		t.Fatalf("parseExpansions = %+v", exps)
	}
	nest := strings.Index(kickSource, "@MudCommand")
	marked := strings.Index(kickSource, "@Marked")
	attrs := []attachedMacro{
		{Macro: "MudCommand", Decl: "Inner", Context: "Outer", Start: nest, End: strings.Index(kickSource, ")")},
		{Macro: "Marked", Decl: "P", Context: "", Start: marked, End: marked + 1},
	}
	got, err := spliceExtensions([]byte(kickSource), attrs, exps)
	if err != nil {
		t.Fatal(err)
	}
	want := `enum Outer {
                             
    struct Inner { }
}
        private struct P {}
struct Inner { }

extension Outer.Inner: MudCommand {
    static var name: String {
        "nest"
    }
}

extension P: Marker {
}
`
	if string(got) != want {
		t.Errorf("spliced source:\n%s\nwant:\n%s", got, want)
	}
}

func TestSpliceExtensionsLeavesFilesWithoutExtensionMacrosAlone(t *testing.T) {
	exps := []macroExpansion{{Role: 'm', Macro: "FlagSet", Decl: "Flags", Text: "static let all = 1"}}
	got, err := spliceExtensions([]byte("@FlagSet struct Flags {}\n"), []attachedMacro{{Macro: "FlagSet", Decl: "Flags", End: 1}}, exps)
	if got != nil || err != nil {
		t.Errorf("spliceExtensions = %q, %v; want no copy", got, err)
	}
}

func TestSpliceExtensionsSkipsExpansionsOfOtherFiles(t *testing.T) {
	exps := []macroExpansion{{Role: 'e', Macro: "M", Decl: "Elsewhere", Context: "T", Text: "extension Elsewhere {}"}}
	got, err := spliceExtensions([]byte("@M struct A {}\n"), []attachedMacro{{Macro: "M", Decl: "A", End: 1}}, exps)
	if got != nil || err != nil {
		t.Errorf("spliceExtensions = %q, %v; want no copy", got, err)
	}
}

func TestSpliceExtensionsRejectsAmbiguousAndMultiRoleMacros(t *testing.T) {
	ext := macroExpansion{Role: 'e', Macro: "M", Decl: "A", Context: "T", Text: "extension A {}"}
	twice := []attachedMacro{{Macro: "M", Decl: "A", Context: "", End: 1}, {Macro: "M", Decl: "A", Context: "", End: 1}}
	if _, err := spliceExtensions([]byte("@M struct A {}"), twice, []macroExpansion{ext}); err == nil {
		t.Error("two candidate declarations: want an error")
	}
	member := macroExpansion{Role: 'm', Macro: "M", Decl: "A", Context: "T", Text: "var x = 1"}
	if _, err := spliceExtensions([]byte("@M struct A {}"), twice[:1], []macroExpansion{ext, member}); err == nil {
		t.Error("extension and member roles on one attribute: want an error")
	}
}

func TestAttachedMacrosReadsAttributeRangesAndContext(t *testing.T) {
	const doc = `{"_kind":"source_file","items":[{"_kind":"enum_decl","name":{"base_name":{"name":"Outer"}},"members":[
		{"_kind":"struct_decl","name":{"base_name":{"name":"Inner"}},"attrs":[
			{"_kind":"custom_attr","range":{"start":17,"end":42},"macro":{"base_name":"MudCommand"}},
			{"_kind":"custom_attr","range":{"start":1,"end":2}}]}]}]}`
	got, err := attachedMacros([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (attachedMacro{Macro: "MudCommand", Decl: "Inner", Context: "Outer", Start: 17, End: 42}) {
		t.Errorf("attachedMacros = %+v", got)
	}
}
