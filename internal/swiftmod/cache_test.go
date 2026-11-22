package swiftmod

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeDumper stands in for the frontend. Each source holds the JSON its AST dump would be; a
// source reading "crash" fails to dump, and one whose JSON contains "macro" gets a copy. It
// records the primaries and copies of every call.
type fakeDumper struct {
	sources []string
	calls   [][]string
	copies  []map[int]string
}

func (f *fakeDumper) dump(primaries []int, copies map[int]string) ([]FileDump, error) {
	var names []string
	out := make([]FileDump, len(primaries))
	for k, i := range primaries {
		names = append(names, f.sources[i])
		data, err := os.ReadFile(f.sources[i])
		if err != nil || string(data) == "crash" {
			continue
		}
		out[k].Doc = data
		if bytes.Contains(data, []byte("macro")) {
			out[k].Copy = f.sources[i] + ".expanded"
			if err := os.WriteFile(out[k].Copy, data, 0o644); err != nil {
				return nil, err
			}
		}
	}
	f.calls = append(f.calls, names)
	f.copies = append(f.copies, maps.Clone(copies))
	return out, nil
}

func (f *fakeDumper) last() []string { return f.calls[len(f.calls)-1] }

const (
	bodyA  = `{"_kind":"source_file","items":[{"_kind":"func_decl","usr":"s:A1fyyF","interface_type":"$syycD","decl_context":"replaced-pointer-1","range":{"start":10,"end":40},"body":{"_kind":"brace_stmt","elements":[]}}]}`
	bodyA2 = `{"_kind":"source_file","items":[{"_kind":"func_decl","usr":"s:A1fyyF","interface_type":"$syycD","decl_context":"replaced-pointer-1","range":{"start":10,"end":52},"body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr","decl_context":"replaced-pointer-2"}]}}]}`
	sigA   = `{"_kind":"source_file","items":[{"_kind":"func_decl","usr":"s:A1fyySiF","interface_type":"$sySicD","decl_context":"replaced-pointer-1","range":{"start":10,"end":40},"body":{"_kind":"brace_stmt","elements":[]}}]}`
	fileB  = `{"_kind":"source_file","items":[{"_kind":"var_decl","usr":"s:B1xSivp","interface_type":"$sSimD"}]}`
	macroB = `{"_kind":"source_file","items":[{"_kind":"struct_decl","usr":"s:B1SV","attrs":[{"_kind":"custom_attr","macro":"m"}]}]}`
)

func writeNow(t *testing.T, path, content string) {
	t.Helper()
	write(t, path, content, time.Now())
}

// setup writes A.swift and B.swift with the given documents and returns the cache directory, the
// sources and a dumper for them.
func setup(t *testing.T, a, b string) (string, []string, *fakeDumper) {
	t.Helper()
	root := t.TempDir()
	sources := []string{filepath.Join(root, "A.swift"), filepath.Join(root, "B.swift")}
	writeNow(t, sources[0], a)
	writeNow(t, sources[1], b)
	return filepath.Join(root, "cache"), sources, &fakeDumper{sources: sources}
}

func dumpOK(t *testing.T, dir, env string, sources []string, f *fakeDumper) CachedDump {
	t.Helper()
	f.sources = sources
	d, err := DumpCached(dir, env, sources, f.dump)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func docs(d CachedDump) []string {
	var out []string
	for _, doc := range d.Docs {
		out = append(out, string(doc))
	}
	return out
}

func TestDumpCachedReusesUnchangedFiles(t *testing.T) {
	dir, sources, f := setup(t, bodyA, fileB)
	first := dumpOK(t, dir, "env", sources, f)
	if first.Dumped != 2 || !slices.Equal(docs(first), []string{bodyA, fileB}) {
		t.Fatalf("first run dumped %d files: %q", first.Dumped, docs(first))
	}
	again := dumpOK(t, dir, "env", sources, f)
	if again.Dumped != 0 || len(f.calls) != 1 {
		t.Fatalf("unchanged run dumped %d files in %d calls, want none", again.Dumped, len(f.calls)-1)
	}
	if !slices.Equal(docs(first), docs(again)) || first.Interface != again.Interface {
		t.Fatal("unchanged run returned different documents or interface")
	}
}

func TestDumpCachedRedumpsOnlyABodyEdit(t *testing.T) {
	dir, sources, f := setup(t, bodyA, fileB)
	first := dumpOK(t, dir, "env", sources, f)
	writeNow(t, sources[0], bodyA2)
	d := dumpOK(t, dir, "env", sources, f)
	if !slices.Equal(f.last(), sources[:1]) || d.Dumped != 1 {
		t.Fatalf("body edit dumped %v, want only A", f.last())
	}
	if !slices.Equal(docs(d), []string{bodyA2, fileB}) {
		t.Fatalf("body edit returned %q", docs(d))
	}
	if d.Interface != first.Interface {
		t.Fatal("a body edit changed the interface")
	}
	if again := dumpOK(t, dir, "env", sources, f); again.Dumped != 0 || string(again.Docs[0]) != bodyA2 {
		t.Fatalf("the run after a body edit dumped %d files, want the edit cached", again.Dumped)
	}
}

func TestDumpCachedRedumpsEverythingAfterADeclarationEdit(t *testing.T) {
	dir, sources, f := setup(t, bodyA, fileB)
	first := dumpOK(t, dir, "env", sources, f)
	writeNow(t, sources[0], sigA)
	d := dumpOK(t, dir, "env", sources, f)
	if !slices.Equal(f.last(), sources) || d.Dumped != 2 {
		t.Fatalf("declaration edit last dumped %v, want every file", f.last())
	}
	if d.Interface == first.Interface {
		t.Fatal("a declaration edit kept the interface")
	}
	if !slices.Equal(docs(d), []string{sigA, fileB}) {
		t.Fatalf("declaration edit returned %q", docs(d))
	}
}

func TestDumpCachedRedumpsEverythingWhenTheEnvironmentChanges(t *testing.T) {
	dir, sources, f := setup(t, bodyA, fileB)
	dumpOK(t, dir, "env", sources, f)
	if d := dumpOK(t, dir, "a dependency's interface changed", sources, f); d.Dumped != 2 {
		t.Fatalf("a new environment dumped %d files, want 2", d.Dumped)
	}
	if d := dumpOK(t, dir, "a dependency's interface changed", sources[:1], f); d.Dumped != 1 || len(d.Docs) != 1 {
		t.Fatalf("a removed source dumped %d files, want a fresh dump of the one left", d.Dumped)
	}
}

func TestDumpCachedKeepsAFileThatFailsToDump(t *testing.T) {
	dir, sources, f := setup(t, bodyA, "crash")
	first := dumpOK(t, dir, "env", sources, f)
	if first.Docs[1] != nil {
		t.Fatal("a failed file has a document")
	}
	if d := dumpOK(t, dir, "env", sources, f); d.Dumped != 0 || d.Docs[1] != nil {
		t.Fatalf("an unchanged failing file was dumped again (%d files)", d.Dumped)
	}
	writeNow(t, sources[1], fileB)
	if d := dumpOK(t, dir, "env", sources, f); d.Dumped != 2 {
		t.Fatalf("a file that dumps now refreshed %d files, want both", d.Dumped)
	}
}

func TestDumpCachedRedumpsAMissingDocument(t *testing.T) {
	dir, sources, f := setup(t, bodyA, fileB)
	dumpOK(t, dir, "env", sources, f)
	if err := os.Remove(docPath(dir, sources[1])); err != nil {
		t.Fatal(err)
	}
	d := dumpOK(t, dir, "env", sources, f)
	if !slices.Equal(f.last(), sources[1:]) || string(d.Docs[1]) != fileB {
		t.Fatalf("a missing document dumped %v", f.last())
	}
}

func TestDumpCachedPassesTheCopiesOfUnchangedFiles(t *testing.T) {
	dir, sources, f := setup(t, bodyA, macroB)
	dumpOK(t, dir, "env", sources, f)
	writeNow(t, sources[0], bodyA2)
	dumpOK(t, dir, "env", sources, f)
	if got := f.copies[len(f.copies)-1]; !maps.Equal(got, map[int]string{1: sources[1] + ".expanded"}) {
		t.Fatalf("A was dumped with copies %v, want B's", got)
	}
	if err := os.Remove(sources[1] + ".expanded"); err != nil {
		t.Fatal(err)
	}
	dumpOK(t, dir, "env", sources, f)
	if !slices.Equal(f.last(), sources[1:]) {
		t.Fatalf("a missing copy dumped %v, want B", f.last())
	}
}

func TestFingerprintIgnoresBodiesAndPositions(t *testing.T) {
	if Fingerprint([]byte(bodyA)) != Fingerprint([]byte(bodyA2)) {
		t.Error("a body edit changed the fingerprint")
	}
	if Fingerprint([]byte(bodyA)) == Fingerprint([]byte(sigA)) {
		t.Error("a signature edit kept the fingerprint")
	}
	opaque := `{"_kind":"func_decl","interface_type":"$s1A1fQrycD","body":{"_kind":"brace_stmt","elements":[]}}`
	opaque2 := `{"_kind":"func_decl","interface_type":"$s1A1fQrycD","body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr"}]}}`
	if Fingerprint([]byte(opaque)) == Fingerprint([]byte(opaque2)) {
		t.Error("the body of a function returning an opaque type is not part of its fingerprint")
	}
	entry := func(typ, init string) []byte {
		e := `{"_kind":"pattern_entry","pattern":{"_kind":"pattern_named","type":"` + typ + `"}`
		if init != "" {
			e += `,"original_init":{"_kind":"integer_literal_expr","type":"` + typ + `","value":"` + init + `"}`
		}
		return []byte(e + "}")
	}
	if Fingerprint(entry("$sSiD", "1")) != Fingerprint(entry("$sSiD", "2")) {
		t.Error("a variable's new initial value of the same type changed the fingerprint")
	}
	if Fingerprint(entry("$sSiD", "1")) == Fingerprint(entry("$sSdD", "1")) {
		t.Error("a variable's new type kept the fingerprint")
	}
	if Fingerprint(entry("$sSiD", "1")) == Fingerprint(entry("$sSiD", "")) {
		t.Error("removing a variable's initial value, which a memberwise initializer defaults to, kept the fingerprint")
	}
	if Fingerprint(entry("$s1A1xQrvpQOyQo_D", "1")) == Fingerprint(entry("$s1A1xQrvpQOyQo_D", "2")) {
		t.Error("the initial value of an opaque-typed variable is not part of the fingerprint")
	}
	top1 := `{"_kind":"top_level_code_decl","body":{"_kind":"brace_stmt","elements":[{"_kind":"integer_literal_expr","value":"1"}]}}`
	top2 := strings.Replace(top1, `"1"`, `"2"`, 1)
	if Fingerprint([]byte(top1)) == Fingerprint([]byte(top2)) {
		t.Error("top-level code, which declares globals, is not part of the fingerprint")
	}
	if Fingerprint(nil) != "" {
		t.Error("a missing document has a fingerprint")
	}
}

func TestDependencyKeyFollowsDependencyInterfaces(t *testing.T) {
	targets := map[string]Target{
		"App":  {Name: "App", Deps: []string{"Core"}, ModuleType: "SwiftTarget"},
		"Core": {Name: "Core", Deps: []string{"Base"}, ModuleType: "SwiftTarget"},
		"Base": {Name: "Base", ModuleType: "SwiftTarget"},
	}
	key := func(ifaces map[string]string) string {
		return DependencyKey(targets["App"], targets, ifaces, nil)
	}
	k1 := key(map[string]string{"Core": "c1", "Base": "b1"})
	if k1 != key(map[string]string{"Core": "c1", "Base": "b1"}) {
		t.Error("the same interfaces gave different keys")
	}
	if k1 == key(map[string]string{"Core": "c1", "Base": "b2"}) {
		t.Error("an indirect dependency's interface change kept the key")
	}
}
