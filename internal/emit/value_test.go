package emit

import (
	"testing"

	"github.com/ncmud/swift2go/internal/ast"
)

func TestOriginCopies(t *testing.T) {
	cases := []struct {
		o    origin
		s    sink
		want bool
	}{
		{originFresh, sinkStore, false},
		{originFresh, sinkReturn, false},
		{originOwnedLet, sinkLet, false},
		{originOwnedLet, sinkStore, true},
		{originOwnedLet, sinkReturn, false},
		{originBorrowedLet, sinkLet, false},
		{originBorrowedLet, sinkReturn, true},
		{originBorrowedLet, sinkBorrow, false},
		{originLocalVar, sinkLet, true},
		{originLocalVar, sinkReturn, false},
		{originLocalVar, sinkBorrow, false},
		{originExclusive, sinkReturn, true},
		{originExclusive, sinkBorrow, false},
		{originCaptured, sinkLet, true},
		{originCaptured, sinkReturn, true},
		{originCaptured, sinkBorrow, true},
		{originShared, sinkLet, true},
		{originShared, sinkStore, true},
		{originShared, sinkReturn, true},
		{originShared, sinkBorrow, true},
	}
	for _, c := range cases {
		if got := c.o.copies(c.s); got != c.want {
			t.Errorf("origin %d into sink %d: copies = %v, want %v", c.o, c.s, got, c.want)
		}
	}
}

func declRef(name string) ast.Node {
	return ast.Node{"_kind": "declref_expr", "decl": map[string]any{"base_name": name, "decl_usr": ""}}
}

func load(n ast.Node) ast.Node {
	return ast.Node{"_kind": "load_expr", "sub_expr": map[string]any(n)}
}

// TestResolveLocals checks scoping over a function and a closure inside it:
//
//	func f(p, q: inout) { var v; let l = v; { v; w } }
func TestResolveLocals(t *testing.T) {
	inClosure, unknown, own, param := declRef("v"), declRef("w"), declRef("v"), declRef("p")
	closure := ast.Node{"_kind": "closure_expr", "body": map[string]any{"_kind": "brace_stmt", "elements": []any{
		map[string]any(load(inClosure)), map[string]any(unknown),
	}}}
	letPattern := ast.Node{"_kind": "pattern_named", "name": map[string]any{"base_name": map[string]any{"name": "l"}}}
	fn := ast.Node{
		"_kind": "func_decl",
		"params": map[string]any{"params": []any{
			map[string]any{"_kind": "parameter", "name": map[string]any{"base_name": map[string]any{"name": "p"}}},
			map[string]any{"_kind": "parameter", "inout": true, "name": map[string]any{"base_name": map[string]any{"name": "q"}}},
		}},
		"body": map[string]any{"_kind": "brace_stmt", "elements": []any{
			map[string]any{"_kind": "var_decl", "name": map[string]any{"base_name": map[string]any{"name": "v"}}},
			map[string]any{"_kind": "pattern_binding_decl", "pattern_entries": []any{
				map[string]any{"pattern": map[string]any(letPattern), "processed_init": map[string]any(load(own))},
			}},
			map[string]any{"_kind": "var_decl", "let": true, "name": map[string]any{"base_name": map[string]any{"name": "l"}}},
			map[string]any(param),
			map[string]any(closure),
		}},
	}
	vs := resolveLocals([]ast.Node{fn})
	f := vs.bodies[nodeKey(fn)]
	if f == nil {
		t.Fatal("no frame for the function")
	}
	if !f.vars["v"] || f.lets["l"] != 1 || f.inits["l"] == nil {
		t.Errorf("declarations: vars %v, lets %v, inits %v", f.vars, f.lets, f.inits)
	}
	if inout, ok := f.params["q"]; !ok || !inout {
		t.Errorf("q should be an inout parameter: %v", f.params)
	}
	if ref := vs.refs[nodeKey(own)]; ref.frame != f || ref.cross {
		t.Errorf("v in its own function: %+v", ref)
	}
	if ref := vs.refs[nodeKey(inClosure)]; ref.frame != f || !ref.cross {
		t.Errorf("v inside the closure should resolve across to the function: %+v", ref)
	}
	if !f.captured["v"] || f.escapes["v"] || f.written["v"] {
		t.Errorf("v is read by a closure that does not escape: captured %v, escapes %v, written %v", f.captured["v"], f.escapes["v"], f.written["v"])
	}
	if _, ok := vs.refs[nodeKey(unknown)]; ok {
		t.Error("w is declared nowhere")
	}
	if ref := vs.refs[nodeKey(param)]; ref.frame != f {
		t.Errorf("p resolves to the function's parameter: %+v", ref)
	}
	if vs.patFrames[nodeKey(letPattern)] != f || !vs.stmtPats[nodeKey(letPattern)] {
		t.Error("the let pattern belongs to the function's statements")
	}
}

func TestMutatesLocal(t *testing.T) {
	read := map[string]any(load(declRef("xs")))
	write := map[string]any{"_kind": "inout_expr", "sub_expr": map[string]any(declRef("xs"))}
	if mutatesLocal([]any{read}, "xs") {
		t.Error("a load is not a write")
	}
	if !mutatesLocal([]any{read, write}, "xs") {
		t.Error("an inout use is a write")
	}
	if mutatesLocal([]any{write}, "ys") {
		t.Error("a write to xs is not a write to ys")
	}
}
