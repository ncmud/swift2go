package emit

import (
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// A foreign type, such as Int or Array, that an extension conforms to a local protocol has a Go form, such
// as int or []T, that cannot gain methods. Its conformance is carried by a Go type defined on that form
// (`type IntAsP int`), whose methods are the protocol's requirements, and that swiftrt.RegisterWitness
// records so that swiftrt.Conforming, StaticWitness and casts find it. Only the standard library's types
// have carriers: a transpiled module's types gain methods in their own package.

// foreignWitness describes the carrier of a foreign type's conformance to a local protocol.
type foreignWitness struct {
	name    string // the carrier's Go type, without type arguments
	pkg     string // the package that declares it
	generic bool   // the foreign type is generic, and the carrier takes its type arguments
}

// foreignWitnessOf returns the carrier of the foreign type q's conformance to the local protocol proto,
// declared in this module or a dependency, if an extension declares that conformance.
func (m *Module) foreignWitnessOf(q, proto string) (foreignWitness, bool) {
	if !strings.HasPrefix(q, "Swift.") {
		return foreignWitness{}, false
	}
	for _, mod := range append([]*Module{m}, m.Deps...) {
		for _, ext := range mod.foreignConforms[proto] {
			if m.typeOf(ext.Str("extended_type")).Name != q {
				continue
			}
			if !m.witnessableRequirements(proto) {
				return foreignWitness{}, false
			}
			return foreignWitness{name: witnessName(q, mod.LocalTypeName(proto)), pkg: mod.Pkg, generic: hasGenericMembers(ext)}, true
		}
	}
	return foreignWitness{}, false
}

func witnessName(q, proto string) string {
	return capitalize(lastComponent(q)) + "As" + proto
}

func hasGenericMembers(ext ast.Node) bool {
	for _, mem := range ext.List("members") {
		if fn := memberFunc(mem); fn != nil && len(fn.Child("generic_signature").Strings("generic_params")) > 0 {
			return true
		}
	}
	return false
}

// memberFunc is the function node that carries a member's generic signature: the member itself, or a
// property's getter.
func memberFunc(mem ast.Node) ast.Node {
	switch mem.Kind() {
	case "func_decl":
		return mem
	case "var_decl":
		for _, a := range mem.List("accessors") {
			if a.Str("accessor_kind") == "get" || a.Bool("get") || len(mem.List("accessors")) == 1 {
				return a
			}
		}
	}
	return nil
}

// witnessed reports whether the local protocol proto has foreign conformers that carriers stand for.
func (m *Module) witnessed(proto string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		for _, ext := range mod.foreignConforms[proto] {
			if _, ok := m.foreignWitnessOf(m.typeOf(ext.Str("extended_type")).Name, proto); ok {
				return true
			}
		}
	}
	return false
}

// witnessableRequirements reports whether a carrier can implement the requirements of proto and the local
// protocols it refines: none sets a property, mutates, initializes or subscripts.
func (m *Module) witnessableRequirements(proto string) bool {
	for _, p := range m.protocolAndParents(proto) {
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, r := range mod.protoReqs[p] {
				switch r.Kind() {
				case "var_decl":
					if r.Str("writeImpl") == "setter" {
						return false
					}
				case "func_decl":
					if mod.decls[r.Str("usr")] != nil && mod.decls[r.Str("usr")].Mutating {
						return false
					}
				case "constructor_decl", "subscript_decl":
					return false
				}
			}
		}
	}
	return true
}

// foreignWitnessTypes emits, for n, an extension of the foreign type q, the carrier of each conformance
// to a local protocol that n declares, and registers a non-generic one.
func (e *fileEmitter) foreignWitnessTypes(n ast.Node, q string) {
	// A member whose self is a value of the extended type, with its generic parameters bound, gives the
	// carrier's type parameters and Go form; a non-generic type's extension may have only statics.
	var fn ast.Node
	var self *types.Type
	for _, mem := range n.List("members") {
		f := memberFunc(mem)
		if f == nil {
			continue
		}
		if st := f.Child("implicit_self_decl").Str("interface_type"); st != "" && e.m.typeOf(st).Kind != types.Meta {
			t := e.m.typeOf(st)
			fn, self = f, t
			break
		}
		if fn == nil && !hasGenericMembers(n) {
			fn, self = f, e.m.typeOf(n.Str("extended_type"))
		}
	}
	for _, c := range n.Child("inherits").List("conformances") {
		proto := e.m.demangle(c.Str("protocol"))
		w, ok := e.m.foreignWitnessOf(q, proto)
		if !ok || w.pkg != e.m.Pkg || fn == nil {
			continue
		}
		saveG, saveTP, saveTB, saveKP := e.generics, e.typeParams, e.typeBounds, e.keptParams
		e.emittingFree = true
		e.generics = e.genericScope(fn)
		e.emittingFree = false
		tps, tparams := e.typeParamList(), e.typeParams
		goSelf := e.m.GoType(self, e.generics)
		targs := ""
		if len(tparams) > 0 {
			targs = "[" + strings.Join(tparams, ", ") + "]"
		}
		var methods strings.Builder
		complete := true
		for _, p := range e.m.protocolAndParents(proto) {
			for _, pm := range append([]*Module{e.m}, e.m.Deps...) {
				for _, r := range pm.protoReqs[p] {
					if m, ok := e.witnessMethod(w.name+targs, q, proto, goSelf, targs, r, pm); ok {
						methods.WriteString(m)
					} else {
						complete = false
					}
				}
			}
		}
		if complete {
			e.w("// %s carries %s's conformance to %s, which Go's %s cannot.\n", w.name, lastComponent(q), e.m.LocalTypeName(proto), goSelf)
			e.w("type %s%s %s\n\n", w.name, tps, goSelf)
			e.w("func (self %s%s) SwiftWitnessed() any { return %s(self) }\n\n", w.name, targs, goSelf)
			e.w("%s", methods.String())
			if len(tparams) == 0 {
				e.w("func init() { swiftrt.RegisterWitness(func(v %s) any { return %s(v) }) }\n\n", goSelf, w.name)
			}
		}
		e.generics, e.typeParams, e.typeBounds, e.keptParams = saveG, saveTP, saveTB, saveKP
	}
}

// witnessMethod renders the carrier recv's method for the requirement r of a protocol that proto is or
// refines: a call of q's implementation of r, or else of the protocol extension's default.
func (e *fileEmitter) witnessMethod(recv, q, proto, goSelf, targs string, r ast.Node, pm *Module) (string, bool) {
	req := pm.decls[r.Str("usr")]
	if req == nil || !(r.Kind() == "func_decl" || r.Kind() == "var_decl") {
		return "", r.Kind() != "constructor_decl" && r.Kind() != "subscript_decl"
	}
	var params, args, res, name string
	if r.Kind() == "func_decl" {
		params, args = e.paramList(r)
		res = e.resultGo(r)
	} else {
		res = e.m.GoType(e.m.typeOf(r.Str("interface_type")), nil)
	}
	switch {
	case req.Static:
		name = staticReqName(req)
	case r.Kind() == "var_decl":
		name = "Get" + req.GoName
	default:
		name = req.GoName
	}
	matches := func(d *DeclInfo) bool {
		return d.Kind == r.Kind() && d.Static == req.Static && d.Node != nil && d.Node.Name() == r.Name() &&
			(d.Kind != "func_decl" || funcSignatureKey(d.Node) == funcSignatureKey(r))
	}
	selfArg := ""
	if !req.Static {
		selfArg = goSelf + "(self)"
	}
	target := ""
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.sortedDecls() {
			if target != "" || d.Owner != q || d.ProtoExt || !matches(d) {
				continue
			}
			pfx := ""
			if mod.Pkg != e.m.Pkg {
				pfx = mod.Pkg + "."
			}
			if d.Kind == "func_decl" {
				target = pfx + d.GoName + targs + "(" + strings.Join(nonEmpty(selfArg, args), ", ") + ")"
			} else {
				target = pfx + d.GoName + targs + "(" + selfArg + ")"
			}
		}
	}
	if target == "" {
		for _, p := range e.m.protocolAndParents(proto) {
			for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
				for _, d := range mod.sortedDecls() {
					if target != "" || !d.ProtoExt || d.Owner != p || !matches(d) || !e.m.extensionApplies(d.Node, q, p, mod) {
						continue
					}
					pfx := ""
					if mod.Pkg != e.m.Pkg {
						pfx = mod.Pkg + "."
					}
					target = pfx + d.GoName + "(self" + prefixComma(args) + ")"
				}
			}
		}
	}
	if target == "" {
		return "", false
	}
	ret := "return "
	if res == "" {
		ret = ""
	}
	return "func (self " + recv + ") " + name + "(" + params + ") " + res + " { " + ret + target + " }\n\n", true
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// witnessRecv renders the receiver recv of a call of a requirement of the local protocol that the
// generic parameter's type, the Swift type of the receiver, is bound by: through swiftrt.Witness when
// the parameter's Go form is unbounded because foreign types conform to the protocol.
func (e *fileEmitter) witnessRecv(base ast.Node, recv string) string {
	t := e.m.typeOf(base.Type())
	if t == nil || t.Kind != types.Generic {
		return recv
	}
	if bound := e.witnessOf[e.m.GoType(t, e.generics)]; bound != "" {
		return "swiftrt.Conforming[" + bound + "](" + recv + ")"
	}
	return recv
}

// registerWitnesses registers, for the generic reference d, the carriers of generic foreign types its
// substitutions conform to witnessed protocols: `[String]` passed where a parameter conforms to P registers
// `ArrayAsP[string]` for []string, which no other declaration knows to.
func (e *fileEmitter) registerWitnesses(d ast.Node) {
	for _, req := range d.Child("substitutions").List("reqs") {
		c := req.Child("conformance")
		proto := e.m.demangle(c.Str("protocol"))
		if !e.m.isLocalProtocol(proto) {
			continue
		}
		t := e.m.typeOf(c.Str("type"))
		if t == nil || len(t.Args) == 0 || t.Kind == types.Generic {
			continue
		}
		q := t.Name
		switch t.Kind {
		case types.Array:
			q = "Swift.Array"
		case types.Dict:
			q = "Swift.Dictionary"
		}
		w, ok := e.m.foreignWitnessOf(q, proto)
		if !ok || !w.generic {
			continue
		}
		var args []string
		for _, a := range t.Args {
			args = append(args, e.m.GoType(a, e.generics))
		}
		goT := e.m.GoType(t, e.generics)
		if strings.Contains(goT, "any") || e.registered[goT+" "+w.name] {
			continue
		}
		if e.registered == nil {
			e.registered = map[string]bool{}
		}
		e.registered[goT+" "+w.name] = true
		name := w.name
		if w.pkg != e.m.Pkg {
			name = w.pkg + "." + name
		}
		e.extraDecls.WriteString("func init() { swiftrt.RegisterWitness(func(v " + goT + ") any { return " + name + "[" + strings.Join(args, ", ") + "](v) }) }\n\n")
	}
}

// witnessCarrier is the carrier type, with its type arguments, that a value of the foreign type from
// converts to when it becomes the existential to of a local protocol, or "" when there is none.
func (e *fileEmitter) witnessCarrier(to, from *types.Type) string {
	if to == nil || from == nil || !(to.Kind == types.Existential || to.Kind == types.Named) || !e.m.isLocalProtocol(to.Name) {
		return ""
	}
	q := from.Name
	switch from.Kind {
	case types.Array:
		q = "Swift.Array"
	case types.Dict:
		q = "Swift.Dictionary"
	case types.Named:
	default:
		return ""
	}
	w, ok := e.m.foreignWitnessOf(q, to.Name)
	if !ok {
		return ""
	}
	name := w.name
	if w.pkg != e.m.Pkg {
		name = w.pkg + "." + name
	}
	if w.generic {
		var args []string
		for _, a := range from.Args {
			args = append(args, e.m.GoType(a, e.generics))
		}
		name += "[" + strings.Join(args, ", ") + "]"
	}
	return name
}
