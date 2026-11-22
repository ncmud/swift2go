package emit

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMappedForeignModule transpiles App against the Swift library Ledger mapped to the hand-written Go
// package testdata/foreignapi/go/ledger, then runs App's transpiled tests against that package. App calls
// protocol-extension members, operators, generic members with result-only type parameters, literals of
// Ledger types, a class, a setter and enum cases, and registers conformances to Ledger's protocols: one
// with a witness from a local protocol's default, and one declared on Model's type in App. Closures passed
// for function types over a generic parameter record which of their parameters are Optional.
func TestMappedForeignModule(t *testing.T) {
	out := transpileMapped(t, "foreignapi", map[string]string{"Ledger": "go/ledger"}, "Model", "App", "AppTests")
	src := readGo(t, filepath.Join(out, "app"))
	for _, want := range []string{
		"ledger.TableFilter[Mail](",
		"ledger.SpecificOpAnd(ledger.SpecificOpEqual(ledger.ColumnInitString(\"to\"), recipient), ledger.SpecificOpEqualBool(",
		"ledger.OpOr(ledger.SequenceContains(ids, ",
		"ledger.OpNot(",
		"ledger.PersistableInsert(&mail, b)",
		"ledger.BookWrite[int](",
		"ledger.RowSubscriptString[int64](row, \"id\")",
		"ledger.ArgumentsInitArrayLiteral(1, 2, 3)",
		"ledger.ArgumentsInitDictionaryLiteral(swiftrt.KV[string, int]{Key: \"min\", Value: 3})",
		"ledger.OptionsInitArrayLiteral(ledger.OptionsStrict, ledger.OptionsTemporary)",
		"ledger.ConfigSetTimeout(&config, 5)",
		"ledger.ConfigTimeout(config)",
		"ledger.ModeSafeRetries(3)",
		"book *ledger.Book",
		"ledger.RegisterConformance[Log](swiftrt.Conformance{",
		"ledger.RegisterConformance[model.Score](swiftrt.Conformance{",
		`func(self *Log, id int64) { AutoIncrementedDidInsert(self, id) },`,
		`func() string { return LogTableName },`,
		`func(self *Mail, id int64) { self.DidInsert(id) },`,
		`AutoIncrementedDidInsert(scoreAutoIncrementedAdapter{self}, id)`,
		"Optional: []bool{false, false}}",
		"Optional: []bool{false, true}}",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("transpiled App lacks %q", want)
		}
	}
	if t.Failed() {
		t.Log(src)
	}
	goTest(t, filepath.Join(out, "app"))
}
