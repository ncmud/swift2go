package swiftmod

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	bodyA  = `{"_kind":"source_file","filename":"A.swift","items":[{"_kind":"func_decl","usr":"s:A1fyyF","name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"f"}},"interface_type":"$syycD","decl_context":"replaced-pointer-1","range":{"start":10,"end":40},"body":{"_kind":"brace_stmt","elements":[]}}]}`
	bodyA2 = `{"_kind":"source_file","filename":"A.swift","items":[{"_kind":"func_decl","usr":"s:A1fyyF","name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"f"}},"interface_type":"$syycD","decl_context":"replaced-pointer-1","range":{"start":10,"end":52},"body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr","decl_context":"replaced-pointer-2"}]}}]}`
	sigA   = `{"_kind":"source_file","filename":"A.swift","items":[{"_kind":"func_decl","usr":"s:A1fyySiF","name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"f"}},"interface_type":"$sySicD","decl_context":"replaced-pointer-1","range":{"start":10,"end":40},"body":{"_kind":"brace_stmt","elements":[]}}]}`
	fileB  = `{"_kind":"source_file","filename":"B.swift","items":[{"_kind":"var_decl","usr":"s:B1xSivp","name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"x"}},"interface_type":"$sSimD"}]}`
	macroB = `{"_kind":"source_file","filename":"B.swift","items":[{"_kind":"struct_decl","usr":"s:B1SV","attrs":[{"_kind":"custom_attr","macro":"m"}]}]}`
	// usesF calls f; other mentions nothing of A's.
	usesF = `{"_kind":"source_file","filename":"C.swift","items":[{"_kind":"func_decl","usr":"s:C1gyyF","name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"g"}},"interface_type":"$syycD","body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr","fn":{"_kind":"declref_expr","decl":{"_kind":"decl_ref","base_name":"f","decl_usr":"s:A1fyyF"}}}]}}]}`
	other = `{"_kind":"source_file","filename":"D.swift","items":[{"_kind":"func_decl","usr":"s:D1hyyF","name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"h"}},"interface_type":"$syycD","body":{"_kind":"brace_stmt","elements":[]}}]}`
)

func init() {
	// The fake documents' symbols stand for themselves.
	newDemangler = func([]string) (func(string) string, error) { return func(s string) string { return s }, nil }
}

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
	d, err := DumpCached(dir, env, nil, sources, f.dump)
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
	if !slices.Equal(docs(first), docs(again)) || first.Interface.Hash != again.Interface.Hash {
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
	if d.Interface.Hash != first.Interface.Hash {
		t.Fatal("a body edit changed the interface")
	}
	if again := dumpOK(t, dir, "env", sources, f); again.Dumped != 0 || string(again.Docs[0]) != bodyA2 {
		t.Fatalf("the run after a body edit dumped %d files, want the edit cached", again.Dumped)
	}
}

func TestDumpCachedRedumpsTheFilesThatMentionAnEditedDeclaration(t *testing.T) {
	root := t.TempDir()
	var sources []string
	for _, f := range []struct{ name, doc string }{{"A.swift", bodyA}, {"B.swift", fileB}, {"C.swift", usesF}, {"D.swift", other}} {
		sources = append(sources, filepath.Join(root, f.name))
		writeNow(t, sources[len(sources)-1], f.doc)
	}
	dir, f := filepath.Join(root, "cache"), &fakeDumper{sources: sources}
	first := dumpOK(t, dir, "env", sources, f)
	writeNow(t, sources[0], sigA)
	d := dumpOK(t, dir, "env", sources, f)
	if want := []string{sources[0], sources[2]}; !slices.Equal(slices.Concat(f.calls[1:]...), want) || d.Dumped != 2 {
		t.Fatalf("declaration edit dumped %v, want A and C, which calls f", f.calls[1:])
	}
	if d.Interface.Hash == first.Interface.Hash {
		t.Fatal("a declaration edit kept the interface")
	}
	if !slices.Equal(docs(d), []string{sigA, fileB, usesF, other}) {
		t.Fatalf("declaration edit returned %q", docs(d))
	}
}

func TestDumpCachedFollowsAnImportedDeclaration(t *testing.T) {
	root := t.TempDir()
	core := []string{filepath.Join(root, "Core", "A.swift")}
	writeNow(t, core[0], bodyA)
	app := []string{filepath.Join(root, "App", "C.swift"), filepath.Join(root, "App", "D.swift")}
	writeNow(t, app[0], usesF)
	writeNow(t, app[1], other)
	coreDir, appDir := filepath.Join(root, "cache", "Core"), filepath.Join(root, "cache", "App")
	fc, fa := &fakeDumper{sources: core}, &fakeDumper{sources: app}
	dumpApp := func(iface *Interface) CachedDump {
		t.Helper()
		d, err := DumpCached(appDir, "env", map[string]*Interface{"Core": iface}, app, fa.dump)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	dumpApp(dumpOK(t, coreDir, "env", core, fc).Interface)
	writeNow(t, core[0], bodyA2)
	if d := dumpApp(dumpOK(t, coreDir, "env", core, fc).Interface); d.Dumped != 0 {
		t.Fatalf("a body edit in an import dumped %v", fa.calls[1:])
	}
	writeNow(t, core[0], sigA)
	if d := dumpApp(dumpOK(t, coreDir, "env", core, fc).Interface); d.Dumped != 1 || !slices.Equal(fa.last(), app[:1]) {
		t.Fatalf("a signature edit in an import dumped %v, want C, which calls f", fa.calls[1:])
	}
	// An interface the cache no longer keeps leaves nothing to compare: every file is dumped again.
	if err := os.RemoveAll(filepath.Join(coreDir, "tables")); err != nil {
		t.Fatal(err)
	}
	writeNow(t, core[0], bodyA)
	if d := dumpApp(dumpOK(t, coreDir, "env", core, fc).Interface); d.Dumped != 2 {
		t.Fatalf("an import with an unknown earlier interface dumped %d files, want both", d.Dumped)
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

func TestDependencyKeyNamesTheDependenciesDumpedInThisRun(t *testing.T) {
	targets := map[string]Target{
		"App":  {Name: "App", Deps: []string{"Core"}, ModuleType: "SwiftTarget"},
		"Core": {Name: "Core", Deps: []string{"Base"}, ModuleType: "SwiftTarget"},
		"Base": {Name: "Base", ModuleType: "SwiftTarget"},
	}
	key := func(ifaces map[string]*Interface) string {
		return DependencyKey(targets["App"], targets, ifaces, nil)
	}
	both := map[string]*Interface{"Core": {Hash: "c1"}, "Base": {Hash: "b1"}}
	if key(both) != key(map[string]*Interface{"Core": {Hash: "c2"}, "Base": {Hash: "b2"}}) {
		t.Error("the interfaces of dependencies dumped in this run changed the key; DumpCached follows them")
	}
	if key(both) == key(map[string]*Interface{"Core": {Hash: "c1"}}) {
		t.Error("an indirect dependency no longer dumped in this run kept the key")
	}
}
