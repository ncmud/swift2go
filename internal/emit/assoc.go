package emit

import (
	"slices"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// assocParam is an associated type of Self that a protocol-extension function keeps as a Go type parameter.
type assocParam struct {
	name  string // the dependent type, demangled: `τ_0_0.M.P.Assoc`
	proto string // the protocol that declares the associated type, M.P
	assoc string // Assoc
	bound string // the local protocol the extension requires the associated type to conform to
}

// assocTypeParams lists the associated types of Self that the protocol-extension function info takes as
// Go type parameters, ahead of its others: each one its extension requires to conform to a local
// protocol with static requirements (`extension P where Assoc: Q`). The function takes Self as P's
// interface, which says nothing of Assoc, so the caller names the conformer's Assoc and `Assoc.self`
// reaches its statics.
func (m *Module) assocTypeParams(info *DeclInfo) []assocParam {
	if info == nil || !info.ProtoExt || info.Kind != "func_decl" || info.Node == nil {
		return nil
	}
	sig := info.Node.Child("generic_signature")
	params := sig.Strings("generic_params")
	if len(params) == 0 {
		return nil
	}
	self := m.demangle(params[0])
	var out []assocParam
	for _, r := range sig.List("generic_reqs") {
		if r.Str("kind") != "conforms_to" {
			continue
		}
		name := m.demangle(r.Str("first_type"))
		rest, ok := strings.CutPrefix(name, self+".")
		if !ok {
			continue
		}
		dot := strings.LastIndex(rest, ".")
		if dot < 0 || !m.isLocalProtocol(rest[:dot]) {
			continue
		}
		bound := m.demangle(r.Str("protocol"))
		if !m.isLocalProtocol(bound) || !m.staticRequirements(bound) {
			continue
		}
		out = append(out, assocParam{name: name, proto: rest[:dot], assoc: rest[dot+1:], bound: bound})
	}
	return out
}

// staticRequirements reports whether the local protocol proto, declared here or in a dependency,
// has a static requirement.
func (m *Module) staticRequirements(proto string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.HasStaticRequirements(proto) {
			return true
		}
	}
	return false
}

// scopeAssocParams adds the type parameters of assocTypeParams to the generic scope g of the function
// fn being emitted. It returns the dependent types it scoped.
func (e *fileEmitter) scopeAssocParams(fn ast.Node, g map[string]string) map[string]bool {
	ps := e.m.assocTypeParams(e.m.decls[fn.Str("usr")])
	if len(ps) == 0 {
		return nil
	}
	scoped := map[string]bool{}
	var tps []string
	for i, p := range ps {
		tp := "A" + strconv.Itoa(i)
		tps = append(tps, tp)
		g[p.name] = tp
		e.typeBounds[tp] = e.m.GoType(&types.Type{Kind: types.Named, Name: p.bound}, nil)
		e.keptParams[tp] = true
		scoped[p.name] = true
	}
	e.typeParams = append(tps, e.typeParams...)
	return scoped
}

// assocTypeArgs renders the type arguments of assocTypeParams for a call of the protocol-extension
// function info through the reference d, whose substitutions say what the caller's Self binds each
// associated type to. Where they do not say, the argument is the protocol the type must conform to.
func (e *fileEmitter) assocTypeArgs(info *DeclInfo, d ast.Node) string {
	ps := e.m.assocTypeParams(info)
	if len(ps) == 0 {
		return ""
	}
	args := make([]string, len(ps))
	for i, p := range ps {
		args[i] = e.m.GoType(&types.Type{Kind: types.Named, Name: p.bound}, nil)
		for _, req := range d.Child("substitutions").List("reqs") {
			c := req.Child("conformance")
			if e.m.demangle(req.Str("type")) != p.name || e.m.demangle(c.Str("protocol")) != p.bound || c.Str("type") == "" {
				continue
			}
			if gt := e.m.GoType(e.m.typeOf(c.Str("type")), e.generics); gt != "" {
				args[i] = gt
			}
		}
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// conformerAssocTypeArgs renders the type arguments of assocTypeParams for the conformer q's forwarder to
// the protocol-extension function info: the types q binds the associated types to.
func (e *fileEmitter) conformerAssocTypeArgs(info *DeclInfo, q string, pm *Module) string {
	ps := e.m.assocTypeParams(info)
	if len(ps) == 0 {
		return ""
	}
	args := make([]string, len(ps))
	for i, p := range ps {
		args[i] = e.m.GoType(&types.Type{Kind: types.Named, Name: p.bound}, nil)
		if w := e.m.assocWitness(q, p.proto, p.assoc, pm); w != nil {
			if gt := e.m.GoType(w, nil); gt != "" {
				args[i] = gt
			}
		}
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// wrappedConformsTo reports whether the extension of Optional ext applies where Wrapped conforms to the
// protocol usr, and under no other requirement.
func wrappedConformsTo(ext ast.Node, usr string) bool {
	reqs := ext.List("where_requirements")
	return len(reqs) == 1 && reqs[0].Str("kind") == "type_constraint" &&
		reqs[0].Child("first_type").Str("id") == "Wrapped" && reqs[0].Child("second_type").Str("bind") == usr
}

// optionalWitness names the Go type that carries Optional's conformance to the local protocol proto, with
// the package prefix it needs here, when Optional conforms to proto where Wrapped does and proto and the
// protocols it refines require only statics. Go cannot give a pointer type, which an Optional is, methods
// of its own, so generic code reaches Optional's statics through this type instead.
func (m *Module) optionalWitness(proto string) string {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.optConforms[proto] == nil || !m.onlyStaticRequirements(proto, map[string]bool{}) {
			continue
		}
		name := "OptionalWitness" + mod.LocalTypeName(proto)
		if mod.Pkg != m.Pkg {
			name = mod.Pkg + "." + name
		}
		return name
	}
	return ""
}

func (m *Module) onlyStaticRequirements(proto string, seen map[string]bool) bool {
	if seen[proto] {
		return true
	}
	seen[proto] = true
	for _, mod := range append([]*Module{m}, m.Deps...) {
		for _, r := range mod.protoReqs[proto] {
			switch r.Kind() {
			case "func_decl", "var_decl":
				if !r.Bool("static") {
					return false
				}
			case "constructor_decl", "subscript_decl":
				return false
			}
		}
		for _, parent := range mod.protoParents[proto] {
			if !m.onlyStaticRequirements(parent, seen) {
				return false
			}
		}
	}
	return true
}

// protocolAndParents lists proto and the protocols it refines, nearest first.
func (m *Module) protocolAndParents(proto string) []string {
	out := []string{proto}
	for i := 0; i < len(out); i++ {
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, parent := range mod.protoParents[out[i]] {
				if !slices.Contains(out, parent) {
					out = append(out, parent)
				}
			}
		}
	}
	return out
}

// optionalWitnessTypes emits, for the extension n of Optional, the type optionalWitness names for each
// protocol n conforms Optional to: a struct whose methods are the protocol's static requirements, each
// calling Optional's implementation, or the protocol extension's default.
func (e *fileEmitter) optionalWitnessTypes(n ast.Node) {
	for _, c := range n.Child("inherits").List("conformances") {
		proto := e.m.demangle(c.Str("protocol"))
		if e.m.optConforms[proto] == nil || !e.m.onlyStaticRequirements(proto, map[string]bool{}) {
			continue
		}
		bound := e.m.GoType(&types.Type{Kind: types.Named, Name: proto}, nil)
		name := "OptionalWitness" + e.m.LocalTypeName(proto)
		e.w("// %s carries Optional's conformance to %s for generic code, whose type argument for an\n// Optional is a pointer that has only the methods of the type it points to.\n", name, bound)
		e.w("type %s[T0 %s] struct{}\n\n", name, bound)
		for _, p := range e.m.protocolAndParents(proto) {
			for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
				for _, r := range mod.protoReqs[p] {
					e.optionalWitnessMethod(name, proto, r, mod)
				}
			}
		}
	}
}

func (e *fileEmitter) optionalWitnessMethod(name, proto string, r ast.Node, pm *Module) {
	if !(r.Kind() == "func_decl" || r.Kind() == "var_decl") || !r.Bool("static") {
		return
	}
	req := pm.decls[r.Str("usr")]
	if req == nil {
		return
	}
	var params, args, res string
	if r.Kind() == "func_decl" {
		params, args = e.paramList(r)
		res = e.resultGo(r)
	} else {
		res = e.m.GoType(e.m.typeOf(r.Str("interface_type")), nil)
	}
	matches := func(d *DeclInfo) bool {
		return d.Kind == r.Kind() && d.Static && d.Node != nil && d.Node.Name() == r.Name() &&
			(d.Kind != "func_decl" || funcSignatureKey(d.Node) == funcSignatureKey(r))
	}
	target := ""
	for _, d := range e.m.sortedDecls() {
		if d.Owner == "Swift.Optional" && !d.ProtoExt && matches(d) {
			target = d.GoName + "[T0]"
			if d.Kind == "func_decl" {
				target += "(" + args + ")"
			} else {
				target += "()"
			}
		}
	}
	if target == "" {
		// A default from the extension of the protocol, or of one it refines, that declares the requirement.
		for _, p := range e.m.protocolAndParents(proto) {
			for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
				for _, d := range mod.sortedDecls() {
					if target == "" && d.ProtoExt && d.Owner == p && matches(d) {
						pfx := ""
						if mod.Pkg != e.m.Pkg {
							pfx = mod.Pkg + "."
						}
						target = pfx + d.GoName + "(self" + prefixComma(args) + ")"
					}
				}
			}
		}
	}
	if target == "" {
		return
	}
	ret := "return "
	if res == "" {
		ret = ""
	}
	e.w("func (self %s[T0]) %s(%s) %s { %s%s }\n\n", name, staticReqName(req), params, res, ret, target)
}

// optionalWitnessRegistrations registers, for the local type q, which the declaration n conforms to
// protocols, the optionalWitness of each protocol among them, or among those they refine, as the carrier
// of the statics of q's Optional.
func (e *fileEmitter) optionalWitnessRegistrations(n ast.Node, q string) {
	if len(e.m.TypeParams(q)) > 0 {
		return
	}
	named := &types.Type{Kind: types.Named, Name: q}
	goT := e.m.GoType(named, nil)
	optT := e.m.GoType(&types.Type{Kind: types.Optional, Args: []*types.Type{named}}, nil)
	if optT == goT {
		// A class or an enum emitted as an interface: its Optional is the same Go type.
		return
	}
	seen := map[string]bool{}
	for _, c := range n.Child("inherits").List("conformances") {
		for _, p := range e.m.protocolAndParents(e.m.demangle(c.Str("protocol"))) {
			w := e.m.optionalWitness(p)
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			e.w("func init() { swiftrt.RegisterWitness(func(%s) any { return %s[%s]{} }) }\n\n", optT, w, goT)
		}
	}
}

// staticCarrier is the Go value that carries the statics of the type parameter tp: the registered
// witness of its Optional conformance when its bound is a protocol Optional conforms to, or else its
// MetaZero.
func (e *fileEmitter) staticCarrier(tp string) string {
	if bound := e.typeBounds[tp]; bound != "" {
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			for proto := range mod.optConforms {
				if e.m.GoType(&types.Type{Kind: types.Named, Name: proto}, nil) == bound && e.m.optionalWitness(proto) != "" {
					return "swiftrt.StaticWitness[" + tp + ", " + bound + "]()"
				}
			}
		}
	}
	return "swiftrt.MetaZero[" + tp + "]()"
}

// optionalMetaValue is the value that stands for the metatype of t, an Optional, passed to fn for its
// generic parameter param, which fn knows only as its protocol: the optionalWitness of that protocol.
// It is "" when there is none.
func (e *fileEmitter) optionalMetaValue(t *types.Type, fn ast.Node, param *types.Type) string {
	if t == nil || t.Kind != types.Optional || len(t.Args) != 1 {
		return ""
	}
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") != "conforms_to" || e.m.typeOf(r.Str("first_type")).Name != param.Name {
			continue
		}
		if w := e.m.optionalWitness(e.m.demangle(r.Str("protocol"))); w != "" {
			return w + "[" + e.m.GoType(t.Args[0], e.generics) + "]{}"
		}
	}
	return ""
}
