package deps

import (
	"slices"
	"testing"

	"github.com/ncmud/swift2go/internal/ast"
)

func node(t *testing.T, doc string) ast.Node {
	t.Helper()
	files, err := ast.Load([]byte(`{"_kind":"source_file","filename":"F.swift","items":[` + doc + `]}`))
	if err != nil || len(files) != 1 || len(files[0].Items) != 1 {
		t.Fatalf("load %s: %v", doc, err)
	}
	return files[0].Items[0]
}

func TestDigestIgnoresBodiesAndPositions(t *testing.T) {
	digest := func(doc string) string { return Digest(node(t, doc)) }
	body := `{"_kind":"func_decl","usr":"s:A1fyyF","interface_type":"$syycD","decl_context":"replaced-pointer-1","range":{"start":10,"end":40},"body":{"_kind":"brace_stmt","elements":[]}}`
	body2 := `{"_kind":"func_decl","usr":"s:A1fyyF","interface_type":"$syycD","decl_context":"replaced-pointer-2","range":{"start":12,"end":52},"body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr"}]}}`
	sig := `{"_kind":"func_decl","usr":"s:A1fyySiF","interface_type":"$sySicD","range":{"start":10,"end":40},"body":{"_kind":"brace_stmt","elements":[]}}`
	if digest(body) != digest(body2) {
		t.Error("a body edit changed the digest")
	}
	if digest(body) == digest(sig) {
		t.Error("a signature edit kept the digest")
	}
	opaque := `{"_kind":"func_decl","interface_type":"$s1A1fQrycD","body":{"_kind":"brace_stmt","elements":[]}}`
	opaque2 := `{"_kind":"func_decl","interface_type":"$s1A1fQrycD","body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr"}]}}`
	if digest(opaque) == digest(opaque2) {
		t.Error("the body of a function returning an opaque type is not part of its digest")
	}
	entry := func(typ, init string) string {
		e := `{"_kind":"pattern_entry","pattern":{"_kind":"pattern_named","type":"` + typ + `"}`
		if init != "" {
			e += `,"original_init":{"_kind":"integer_literal_expr","type":"` + typ + `","value":"` + init + `"}`
		}
		return e + "}"
	}
	if digest(entry("$sSiD", "1")) != digest(entry("$sSiD", "2")) {
		t.Error("a variable's new initial value of the same type changed the digest")
	}
	if digest(entry("$sSiD", "1")) == digest(entry("$sSdD", "1")) {
		t.Error("a variable's new type kept the digest")
	}
	if digest(entry("$sSiD", "1")) == digest(entry("$sSiD", "")) {
		t.Error("removing a variable's initial value, which a memberwise initializer defaults to, kept the digest")
	}
	if digest(entry("$s1A1xQrvpQOyQo_D", "1")) == digest(entry("$s1A1xQrvpQOyQo_D", "2")) {
		t.Error("the initial value of an opaque-typed variable is not part of the digest")
	}
}

func name(n string) string {
	return `"name":{"_kind":"decl_name","base_name":{"_kind":"base_name","name":"` + n + `"}}`
}

func scan(t *testing.T, doc string) (Table, Names) {
	t.Helper()
	return Scan("F.swift", []ast.Node{node(t, doc)}, nil, func(s string) string { return s })
}

func TestScanKeysDeclarationsByOwnerAndName(t *testing.T) {
	doc := `{"_kind":"struct_decl",` + name("Point") + `,"access":"public","members":[
		{"_kind":"var_decl",` + name("x") + `,"access":"public","readImpl":"stored","interface_type":"$sSiD"},
		{"_kind":"var_decl",` + name("secret") + `,"access":"private","readImpl":"stored","interface_type":"$sSiD"},
		{"_kind":"func_decl",` + name("moved") + `,"access":"public","interface_type":"$sSiD","body":{"_kind":"brace_stmt"}},
		{"_kind":"func_decl",` + name("helper") + `,"access":"private","interface_type":"$sSiD"}]}`
	table, _ := scan(t, doc)
	var keys []Key
	for k := range table {
		keys = append(keys, k)
	}
	for _, want := range []Key{{Owner: "Point"}, {Owner: "Point", Name: "x"}, {Owner: "Point", Name: "moved"}, {File: "F.swift", Owner: "Point", Name: "secret"}, {File: "F.swift", Owner: "Point", Name: "helper"}} {
		if !slices.Contains(keys, want) {
			t.Errorf("no key %+v in %+v", want, keys)
		}
	}
	// A private stored property still decides the memberwise initializer.
	before := table[Key{Owner: "Point"}]
	changed, _ := scan(t, `{"_kind":"struct_decl",`+name("Point")+`,"access":"public","members":[
		{"_kind":"var_decl",`+name("x")+`,"access":"public","readImpl":"stored","interface_type":"$sSiD"},
		{"_kind":"var_decl",`+name("secret")+`,"access":"private","readImpl":"stored","interface_type":"$sSdD"},
		{"_kind":"func_decl",`+name("moved")+`,"access":"public","interface_type":"$sSiD","body":{"_kind":"brace_stmt","elements":[{"_kind":"call_expr"}]}},
		{"_kind":"func_decl",`+name("helper")+`,"access":"private","interface_type":"$sSdD"}]}`)
	got := Changed(table, changed)
	want := []Key{{Owner: "Point"}, {File: "F.swift", Owner: "Point", Name: "helper"}, {File: "F.swift", Owner: "Point", Name: "secret"}}
	if !slices.Equal(got, want) || changed[Key{Owner: "Point"}] == before {
		t.Errorf("Changed = %+v, want %+v", got, want)
	}
}

func TestScanNamesWhatTheFileMentions(t *testing.T) {
	doc := `{"_kind":"func_decl",` + name("caller") + `,"body":{"_kind":"brace_stmt","elements":[
		{"_kind":"call_expr","type":"$sMangledResult","fn":{"_kind":"declref_expr","decl":{"_kind":"decl_ref","base_name":"sendToChar","decl_usr":"s:World.GameCharacter.sendToChar"}}}]}}`
	demangled := map[string]string{"$sMangledResult": "World.Room", "s:World.GameCharacter.sendToChar": "World.GameCharacter.sendToChar(Swift.String) -> ()"}
	_, names := Scan("F.swift", []ast.Node{node(t, doc)}, []byte("func caller() { x == y }"), func(s string) string { return demangled[s] })
	for _, n := range []string{"sendToChar", "GameCharacter", "Room", "caller", "==", "String"} {
		if !names.Has(n) {
			t.Errorf("names %q lack %q", names, n)
		}
	}
}

func TestAffects(t *testing.T) {
	// Nearly every file mentions init; an initializer counts for the files that mention its type.
	names := Names{"GameCharacter", "Point", "init", "sendToChar"}
	for _, c := range []struct {
		key  Key
		want bool
	}{
		{Key{Owner: "GameCharacter", Name: "sendToChar"}, true},
		{Key{Owner: "Player", Name: "sendToChar"}, true},
		{Key{Owner: "GameCharacter", Name: "incrProbe"}, false},
		{Key{Owner: "Point"}, true},
		{Key{Owner: "Room"}, false},
		{Key{Owner: "Point", Name: "init"}, true},
		{Key{Owner: "Room", Name: "init"}, false},
		{Key{Name: "sendToChar"}, true},
		{Key{File: "F.swift", Owner: "GameCharacter", Name: "sendToChar"}, true},
		{Key{File: "G.swift", Owner: "GameCharacter", Name: "sendToChar"}, false},
		{All, true},
	} {
		if got := Affects(c.key, "F.swift", names); got != c.want {
			t.Errorf("Affects(%+v) = %v, want %v", c.key, got, c.want)
		}
	}
}
