package emit

import (
	"github.com/ncmud/swift2go/internal/ast"
)

// effects records which functions of a module can write a place other code can see: a property of a
// class instance, a global, or anything reached through a call the analysis cannot follow. A value
// lent to a function that cannot do so needs no copy, since nothing can change it during the call.
type effects struct {
	writes map[string]bool // function USR → may write a shared place
	getter map[string]string
}

// region is what a walk over a function body, a loop body or a closure found.
type region struct {
	writes   bool     // writes a shared place directly, or calls code the analysis cannot see
	unknown  bool     // calls a closure, a function value or a local function
	thorough bool     // keep walking after the first write, to find every call
	calls    []string // USRs of the program's functions it calls or refers to
}

// declOf finds a declaration of this module or a transpiled dependency.
func (m *Module) declOf(usr string) *DeclInfo {
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

// valueScopes resolves the local variables of every file of the module, once.
func (m *Module) valueScopes() *valueScopes {
	if m.scopes == nil {
		var items []ast.Node
		for _, f := range m.Files {
			items = append(items, f.Items...)
		}
		m.scopes = resolveLocals(items)
	}
	return m.scopes
}

// mayWrite reports whether calling the function or reading the computed property with this USR can
// write a shared place. An unknown USR can.
func (m *Module) mayWrite(usr string) bool {
	w := m.mayWriteFact(usr)
	m.observe(obsEffect, "", usr, boolObs(w))
	return w
}

// mayWriteFact is [Module.mayWrite] without recording the answer for an incremental run.
func (m *Module) mayWriteFact(usr string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if _, ok := mod.decls[usr]; ok {
			w, known := mod.effectsOf().writes[usr]
			return w || !known
		}
	}
	return true
}

// effectsOf analyses every function body of the module, then spreads "may write" from callee to caller.
func (m *Module) effectsOf() *effects {
	if m.fx != nil {
		return m.fx
	}
	fx := &effects{writes: map[string]bool{}, getter: map[string]string{}}
	m.fx = fx
	callers := map[string][]string{}
	var dirty []string
	add := func(usr string, body ast.Node, ctor bool) {
		if usr == "" {
			return
		}
		r := region{}
		if body == nil {
			// A protocol requirement, or a declaration without a body here: any conformer may run.
			r.writes = true
		} else {
			m.scan(map[string]any(body), ctor, false, &r)
		}
		for _, c := range r.calls {
			if callee := m.decls[c]; callee == nil {
				// A dependency's function: its answer is final.
				r.writes = r.writes || m.mayWrite(c)
			} else {
				callers[c] = append(callers[c], usr)
			}
		}
		if _, seen := fx.writes[usr]; !seen || r.writes {
			fx.writes[usr] = r.writes
		}
		if r.writes {
			dirty = append(dirty, usr)
		}
	}
	for usr, d := range m.decls {
		if d.Node == nil {
			continue
		}
		switch d.Kind {
		case "func_decl":
			add(usr, d.Node.Child("body"), false)
		case "constructor_decl":
			if d.Node.Implicit() {
				fx.writes[usr] = false
				continue
			}
			add(usr, d.Node.Child("body"), true)
		case "var_decl", "subscript_decl":
			for _, a := range d.Node.List("accessors") {
				if !a.Bool("get") {
					continue
				}
				if d.Computed || d.Kind == "subscript_decl" {
					add(usr, a.Child("body"), false)
				}
			}
			if _, ok := fx.writes[usr]; !ok {
				// A stored property: reading it runs no code.
				fx.writes[usr] = d.Kind == "subscript_decl"
			}
		}
		if d.Overrides != "" {
			// A call of the overridden member may run this one.
			callers[usr] = append(callers[usr], d.Overrides)
		}
	}
	for len(dirty) > 0 {
		usr := dirty[len(dirty)-1]
		dirty = dirty[:len(dirty)-1]
		for _, c := range callers[usr] {
			if !fx.writes[c] {
				fx.writes[c] = true
				dirty = append(dirty, c)
			}
		}
	}
	return fx
}

// regionWrites reports whether running the given statements or closures can write a shared place.
func (m *Module) regionWrites(nodes ...any) bool {
	r := region{}
	for _, n := range nodes {
		m.scan(n, false, false, &r)
		if r.writes {
			return true
		}
	}
	for _, c := range r.calls {
		if m.mayWrite(c) {
			return true
		}
	}
	return false
}

// regionCalls reports whether running the given statements or closures can run any of the program's
// own code: a function, an accessor, a closure.
func (m *Module) regionCalls(nodes ...any) bool {
	r := region{thorough: true}
	for _, n := range nodes {
		m.scan(n, false, false, &r)
	}
	return r.unknown || len(r.calls) > 0
}

// scan walks n collecting writes and calls. ctor says n is a class initializer's body, where `self` is
// a new object nothing else can reach yet; loaded says n is only read (it sits under a load_expr).
func (m *Module) scan(n any, ctor, loaded bool, r *region) {
	if r.writes && !r.thorough {
		return
	}
	switch v := n.(type) {
	case []any:
		for _, c := range v {
			m.scan(c, ctor, loaded, r)
		}
		return
	case map[string]any:
		node := ast.Node(v)
		switch node.Kind() {
		case "load_expr":
			loaded = true
		case "argument_list":
			loaded = false
		case "assign_expr":
			if !m.localPlace(node.Child("dest"), ctor) {
				r.writes = true
			}
		case "inout_expr":
			if !loaded && !m.localPlace(node.Child("sub_expr"), ctor) {
				r.writes = true
			}
		case "call_expr", "binary_expr", "prefix_unary_expr", "postfix_unary_expr":
			fn := stripConv(node.Child("fn"))
			if fn != nil && fn.Kind() == "dot_syntax_call_expr" {
				fn = stripConv(fn.Child("fn"))
			}
			if fn != nil && fn.Kind() == "constructor_ref_call_expr" {
				fn = stripConv(fn.Child("fn"))
			}
			if fn == nil || fn.Kind() != "declref_expr" || fn.Child("decl").Str("decl_usr") == "" {
				// A closure, a function value or a local function: not followed.
				r.writes, r.unknown = true, true
			}
		case "declref_expr", "member_ref_expr", "subscript_expr":
			if usr := node.Child("decl").Str("decl_usr"); usr != "" {
				if d := m.declOf(usr); d != nil {
					switch d.Kind {
					case "func_decl", "constructor_decl", "subscript_decl":
						r.calls = append(r.calls, usr)
					case "var_decl":
						if d.Computed {
							r.calls = append(r.calls, usr)
						}
					}
				}
			}
		}
		for _, c := range v {
			switch c.(type) {
			case map[string]any, []any:
				m.scan(c, ctor, loaded, r)
			}
		}
	}
}

// localPlace reports whether a written place belongs to the running function alone: a local variable,
// a parameter, or a part of one reached without passing through a class instance or an accessor.
func (m *Module) localPlace(n ast.Node, ctor bool) bool {
	for n != nil {
		switch n.Kind() {
		case "declref_expr":
			if n.Child("decl").Str("decl_usr") != "" {
				return false
			}
			return true
		case "discard_assignment_expr":
			return true
		case "tuple_expr":
			for _, el := range n.List("elements") {
				if !m.localPlace(el, ctor) {
					return false
				}
			}
			return true
		case "member_ref_expr":
			if d := m.declOf(n.Child("decl").Str("decl_usr")); d != nil && (d.Computed || d.Observed || d.Static) {
				return false
			}
			base := n.Child("base")
			if bt := m.typeOf(base.Type()); bt == nil || m.isReferenceGo(bt, m.GoType(bt, nil)) {
				root := stripLoads(base)
				for root != nil && root.Kind() == "inout_expr" {
					root = stripLoads(root.Child("sub_expr"))
				}
				return ctor && root != nil && root.Kind() == "declref_expr" && root.Child("decl").Str("base_name") == "self"
			}
			n = base
		case "subscript_expr":
			if d := m.declOf(n.Child("decl").Str("decl_usr")); d != nil {
				return false
			}
			n = n.Child("base")
		case "tuple_element_expr":
			n = n.Child("base")
		case "load_expr", "paren_expr", "inout_expr", "force_value_expr", "bind_optional_expr", "optional_evaluation_expr", "dot_self_expr":
			n = n.Child("sub_expr")
		default:
			return false
		}
	}
	return false
}
