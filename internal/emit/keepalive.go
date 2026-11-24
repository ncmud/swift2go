package emit

import (
	"slices"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// Swift keeps a local's class reference alive to the end of its scope whenever code that could observe
// the object's release runs after the local's last use (lexical lifetimes). Go's collector frees an
// object once no live variable holds it, so a swiftrt.Weak would read nil where Swift still sees the
// object. The emitter keeps each class-typed local of a block reachable by writing swiftrt.KeepAlive
// for it wherever control leaves the block: at the block's end, and before a return, throw, break or
// continue that leaves it.

// keepFrame is one open scope of the function being emitted: the class-typed locals it keeps alive, and
// whether a break or continue can target it.
type keepFrame struct {
	names []string
	jump  keepJump
	label string
}

type keepJump int

const (
	keepBlock keepJump = iota
	keepLoop
	keepSwitch
)

// keptLocal is the Go name of the local p declares when p is a named pattern whose value holds a class
// reference, or "" when the local needs no keeping.
func (e *fileEmitter) keptLocal(p ast.Node) string {
	if p.Kind() != "pattern_named" {
		return ""
	}
	name := p.Name()
	if name == "" || name == "_" || e.localGetters[name] {
		return ""
	}
	t := e.m.typeOf(p.Type())
	if !e.m.holdsObject(t) || strings.HasPrefix(e.m.GoType(t, e.generics), "swiftrt.Opt[") {
		return ""
	}
	return goSafe(name)
}

// holdsObject reports whether a value of type t is a class reference, or an Optional of one.
func (m *Module) holdsObject(t *types.Type) bool {
	if t != nil && t.Kind == types.Optional && len(t.Args) == 1 {
		t = t.Args[0]
	}
	if t == nil {
		return false
	}
	switch t.Kind {
	case types.Named:
		for _, mod := range append([]*Module{m}, m.Deps...) {
			switch mod.typeKinds[t.Name] {
			case "class_decl":
				return true
			case "protocol":
				return m.classOnly(t.Name, map[string]bool{})
			case "":
			default:
				return false
			}
		}
		return m.mappedKind(t.Name) == types.KindClass
	case types.Existential:
		return t.Name == "Swift.AnyObject" || m.classOnly(t.Name, map[string]bool{})
	}
	return false
}

// keptPatterns lists the named patterns a statement binds into its enclosing block.
func keptPatterns(s ast.Node) []ast.Node {
	var roots []ast.Node
	switch s.Kind() {
	case "pattern_binding_decl":
		for _, pe := range s.List("pattern_entries") {
			roots = append(roots, pe.Child("pattern"))
		}
	case "guard_stmt":
		for _, c := range s.List("conditions") {
			if p := c.Child("pattern"); p != nil {
				roots = append(roots, p)
			}
		}
	}
	var out []ast.Node
	for _, r := range roots {
		ast.Walk(r, func(n ast.Node) {
			if n.Kind() == "pattern_named" {
				out = append(out, n)
			}
		})
	}
	return out
}

// mayObserveRelease reports whether running list could notice an object being freed: it calls a
// function, reads a weak or computed property, or runs other code a library or the program supplies.
func (e *fileEmitter) mayObserveRelease(list []ast.Node) bool {
	found := false
	for _, s := range list {
		ast.Walk(s, func(n ast.Node) {
			if found {
				return
			}
			switch n.Kind() {
			case "call_expr", "subscript_expr", "dynamic_member_ref_expr", "dynamic_subscript_expr",
				"binary_expr", "prefix_unary_expr", "postfix_unary_expr", "for_each_stmt", "keypath_application_expr":
				found = true
			case "member_ref_expr":
				info := e.m.declByUSR(n.Child("decl").Str("decl_usr"))
				found = info == nil || info.Weak || info.Computed || info.Observed || info.Lazy
			}
		})
		if found {
			return true
		}
	}
	return false
}

// declByUSR is the declaration of this module or a dependency with the given USR, or nil.
func (m *Module) declByUSR(usr string) *DeclInfo {
	if usr == "" {
		return nil
	}
	if info := m.decls[usr]; info != nil {
		return info
	}
	for _, d := range m.Deps {
		if info := d.decls[usr]; info != nil {
			return info
		}
	}
	return nil
}

// openKeepFrame starts a block's frame, holding the class-typed locals the conditions of an enclosing
// if or while bound for it.
func (e *fileEmitter) openKeepFrame(list []ast.Node) {
	var f keepFrame
	bound := e.keepBound
	e.keepBound = nil
	if len(bound) > 0 && e.mayObserveRelease(list) {
		for _, p := range bound {
			if name := e.keptLocal(p); name != "" {
				f.names = append(f.names, name)
			}
		}
	}
	e.keep = append(e.keep, f)
}

// keepDeclared adds the class-typed locals s binds to the innermost frame, when rest, the statements
// after s in its block, could notice them being freed.
func (e *fileEmitter) keepDeclared(s ast.Node, rest []ast.Node) {
	pats := keptPatterns(s)
	if len(pats) == 0 || len(e.keep) == 0 || !e.mayObserveRelease(rest) {
		return
	}
	f := &e.keep[len(e.keep)-1]
	for _, p := range pats {
		if name := e.keptLocal(p); name != "" {
			f.names = append(f.names, name)
		}
	}
}

// closeKeepFrame ends the innermost frame, keeping its locals alive up to here unless list ends by
// leaving the block.
func (e *fileEmitter) closeKeepFrame(list []ast.Node) {
	f := e.keep[len(e.keep)-1]
	e.keep = e.keep[:len(e.keep)-1]
	if len(list) > 0 && leavesBlock(list[len(list)-1]) {
		return
	}
	e.w("%s", keepAliveCalls(f.names))
}

func leavesBlock(s ast.Node) bool {
	switch s.Kind() {
	case "return_stmt", "throw_stmt", "break_stmt", "continue_stmt", "fail_stmt", "fallthrough_stmt":
		return true
	}
	return false
}

// keptToReturn is the code keeping every class-typed local of the function alive, for a return or throw.
func (e *fileEmitter) keptToReturn() string {
	var names []string
	for i := len(e.keep) - 1; i >= 0; i-- {
		names = append(names, e.keep[i].names...)
	}
	return keepAliveCalls(names)
}

// keptToJump is the code keeping alive the class-typed locals of the scopes a break or continue leaves.
func (e *fileEmitter) keptToJump(kind, label string) string {
	var names []string
	for i := len(e.keep) - 1; i >= 0; i-- {
		f := e.keep[i]
		names = append(names, f.names...)
		switch {
		case f.jump == keepBlock:
		case label != "":
			if f.label == label {
				return keepAliveCalls(names)
			}
		case kind == "continue":
			if f.jump == keepLoop {
				return keepAliveCalls(names)
			}
		default:
			return keepAliveCalls(names)
		}
	}
	return keepAliveCalls(names)
}

func keepAliveCalls(names []string) string {
	var b strings.Builder
	var seen []string
	for _, n := range names {
		if slices.Contains(seen, n) {
			continue
		}
		seen = append(seen, n)
		b.WriteString("swiftrt.KeepAlive(" + n + ")\n")
	}
	return b.String()
}

// withKeepBoundary runs emit for the body of a new function, whose scopes start empty.
func (e *fileEmitter) withKeepBoundary(emit func()) {
	saveK, saveB := e.keep, e.keepBound
	e.keep, e.keepBound = nil, nil
	defer func() { e.keep, e.keepBound = saveK, saveB }()
	emit()
}

// condPatterns lists the named patterns the conditions of an if or while bind for its body.
func condPatterns(cs []ast.Node) []ast.Node {
	var out []ast.Node
	for _, c := range cs {
		if p := c.Child("pattern"); p != nil {
			ast.Walk(p, func(n ast.Node) {
				if n.Kind() == "pattern_named" {
					out = append(out, n)
				}
			})
		}
	}
	return out
}
