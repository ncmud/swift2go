package emit

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/types"
)

// A declaration of a mapped Swift module, one that is not transpiled and is replaced by a hand-written Go
// package, is referenced by a name built from the Swift declaration alone, so the package can be written
// against a fixed contract:
//
//   - a member is named after the type or protocol that declares it, not the type it is used on:
//     `record.insert(db)` is `<pkg>.PersistableRecordInsert(record, db)` when the method comes from an
//     extension of PersistableRecord. Nested owners are flattened (`Database.ColumnType.text` is
//     `DatabaseColumnTypeText`), and free functions take no owner. The argument-label suffix follows the
//     runtime's rules, so one Swift declaration has one Go name per set of labels used at call sites;
//   - an operator is `<Owner>Op<Name>` (`SQLSpecificExpressibleOpEqual`), or `Op<Name>` when declared at
//     module level, with the runtime's suffix for concrete standard-library operand types;
//   - type arguments are explicit when Go cannot infer them: the conforming type for a static member of a
//     protocol (`TableRecordFilter[MailRecord]`), then every type parameter of the member itself when one
//     of them appears only in its result (`RowSubscriptString[int64]`);
//   - a class is a pointer to the package's type, and a protocol is the package's interface type;
//   - an array or dictionary literal of a package type calls its `<Type>InitArrayLiteral(elements...)` or
//     `<Type>InitDictionaryLiteral(pairs...)` with `swiftrt.KV` pairs.

// usrModule is the module a Swift USR ("s:4GRDB8DatabaseC…") belongs to, or "" for other symbols.
func usrModule(usr string) string {
	s, ok := strings.CutPrefix(usr, "s:")
	if !ok {
		return ""
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil || i+n > len(s) {
		return ""
	}
	return s[i : i+n]
}

// mappedDecl returns the mapping of the Swift module that declares usr, when that module is mapped to a Go
// package and not transpiled in this run. A member declared in an extension belongs to the extension's
// module: GRDB's `Sequence.contains(_:)` is GRDB's.
func (m *Module) mappedDecl(usr string) *Mapping {
	if len(m.Mappings) == 0 || !strings.HasPrefix(usr, "s:") {
		return nil
	}
	dem := strings.TrimPrefix(m.demangle(usr), "static ")
	if rest, ok := strings.CutPrefix(dem, "(extension in "); ok {
		if mod, _, ok := strings.Cut(rest, "):"); ok {
			return m.mappedModule(mod)
		}
	}
	if mod := usrModule(usr); mod != "" {
		return m.mappedModule(mod)
	}
	return nil
}

// mappedTypeModule returns the mapping for the module of a qualified Swift type name ("GRDB.Row").
func (m *Module) mappedTypeModule(name string) *Mapping {
	mod, _, ok := strings.Cut(name, ".")
	if !ok {
		return nil
	}
	return m.mappedModule(mod)
}

// mappedKind is the nominal kind (types.KindClass, …) of a type of a mapped module, or 0 when unknown.
func (m *Module) mappedKind(name string) byte {
	mp := m.mappedTypeModule(name)
	if mp == nil || m.dm == nil {
		return 0
	}
	if m.mappedKinds == nil {
		m.mappedKinds = map[string]map[string]byte{}
	}
	kinds, ok := m.mappedKinds[mp.Module]
	if !ok {
		var err error
		kinds, err = m.dm.NominalKinds(mp.Module)
		if err != nil {
			m.Unsup("mapped-kinds", err.Error(), "")
			kinds = map[string]byte{}
		}
		m.mappedKinds[mp.Module] = kinds
	}
	return kinds[name]
}

// mappedOwner flattens a declaring type's qualified name to the Go name prefix: "GRDB.Database.ColumnType"
// becomes "DatabaseColumnType", and the module itself, for a free function, becomes "".
func mappedOwner(owner string) string {
	_, rest, ok := strings.Cut(owner, ".")
	if !ok {
		return ""
	}
	return strings.ReplaceAll(rest, ".", "")
}

var operatorNames = map[string]string{
	"==": "Equal", "!=": "NotEqual", "<": "Less", "<=": "LessEqual", ">": "Greater", ">=": "GreaterEqual",
	"&&": "And", "||": "Or", "!": "Not", "+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Rem",
	"+=": "AddAssign", "-=": "SubAssign", "*=": "MulAssign", "/=": "DivAssign", "~=": "Match",
	"===": "Identical", "!==": "NotIdentical", "&": "BitAnd", "|": "BitOr", "^": "BitXor", "~": "BitNot",
	"<<": "ShiftLeft", ">>": "ShiftRight", "??": "Coalesce", "..<": "HalfOpenRange", "...": "ClosedRange",
}

var operatorCharNames = map[rune]string{
	'=': "Eq", '!': "Bang", '<': "Lt", '>': "Gt", '&': "Amp", '|': "Pipe", '+': "Plus", '-': "Minus",
	'*': "Star", '/': "Slash", '%': "Percent", '^': "Caret", '~': "Tilde", '?': "Quest", '.': "Dot",
}

// operatorName is the Go name of an operator: "Op" and its word ("OpEqual"), spelled character by
// character for operators without one. A prefix minus is "OpNegate".
func operatorName(op string, prefix bool) string {
	if prefix && op == "-" {
		return "OpNegate"
	}
	if prefix && op == "+" {
		return "OpPlus"
	}
	if w, ok := operatorNames[op]; ok {
		return "Op" + w
	}
	var b strings.Builder
	b.WriteString("Op")
	for _, r := range op {
		if w, ok := operatorCharNames[r]; ok {
			b.WriteString(w)
		} else {
			b.WriteString("X")
		}
	}
	return b.String()
}

// mappedName is the qualified Go name of a declaration of a mapped module, without type arguments: its
// declaring owner, its base name, and the label suffix of the arguments the call site passes.
func (e *fileEmitter) mappedName(mp *Mapping, sig declSig, kept []arg, prefixOp bool) string {
	base := capitalize(sig.base)
	if isOperatorName(sig.base) {
		base = operatorName(sig.base, prefixOp)
	}
	return mp.Alias() + "." + mappedOwner(sig.owner) + base + e.suffix(sig, kept, sig.owner == mp.Module)
}

// mappedTypeArgs renders the explicit type arguments of a reference to a mapped declaration, "[A, B]" or "":
// the conforming type when the declaration is a static member of a protocol, then the member's own type
// parameters when one of them occurs in its result, which Go cannot infer.
func (e *fileEmitter) mappedTypeArgs(d ast.Node, dem string, static bool) string {
	sig := parseDeclSig(dem)
	subs := map[string]string{}
	for _, s := range d.Child("substitutions").List("substitutions") {
		subs[e.m.demangle(s.Str("generic_param"))] = s.Str("replacement_type")
	}
	var params []string
	if static && e.m.mappedKind(sig.owner) == types.KindProtocol {
		params = append(params, "A")
	}
	own := ownGenericParams(strings.TrimPrefix(dem, "static "))
	if arrow := strings.LastIndex(dem, " -> "); arrow >= 0 && len(own) > 0 {
		result := dem[arrow+4:]
		for _, p := range own {
			if regexp.MustCompile(`(^|[^\w.])` + p + `($|[^\w.])`).MatchString(result) {
				params = append(params, own...)
				break
			}
		}
	}
	var args []string
	for _, p := range params {
		r, ok := subs[p]
		if !ok {
			return ""
		}
		gt := e.m.GoType(e.m.typeOf(r), e.generics)
		if isVoidGo(gt) {
			gt = "struct{}"
		}
		args = append(args, gt)
	}
	if len(args) == 0 {
		return ""
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// mappedCallExpr renders a call of a mapped declaration: recv, when non-empty, is passed first.
func (e *fileEmitter) mappedCallExpr(mp *Mapping, d ast.Node, recv string, as []arg, static, prefixOp bool) string {
	usr := d.Str("decl_usr")
	dem := e.m.demangle(usr)
	sig := parseDeclSig(dem)
	ss, kept := e.argStrings(as, true)
	for i, a := range kept {
		if e.returnsVoidTypeParam(d, sig, a) {
			ss[i] = "swiftrt.UnitFn" + itoa(len(e.m.typeOf(a.node.Type()).Args)) + "(" + ss[i] + ")"
		}
	}
	if recv != "" {
		ss = append([]string{recv}, ss...)
	}
	return e.mappedName(mp, sig, kept, prefixOp) + e.mappedTypeArgs(d, dem, static) + "(" + strings.Join(ss, ", ") + ")"
}

// returnsVoidTypeParam reports whether an argument is a function without a result passed for a parameter
// whose function type returns a type parameter bound to Void here (`write { db in … }` with T == Void): the
// Go function returns struct{}.
func (e *fileEmitter) returnsVoidTypeParam(d ast.Node, sig declSig, a arg) bool {
	ft := e.m.typeOf(a.node.Type())
	if ft == nil || ft.Kind != types.Func || ft.Result != nil && !isVoidGo(e.m.GoType(ft.Result, e.generics)) {
		return false
	}
	for _, s := range d.Child("substitutions").List("substitutions") {
		if r := e.m.GoType(e.m.typeOf(s.Str("replacement_type")), e.generics); !isVoidGo(r) {
			continue
		}
		p := e.m.demangle(s.Str("generic_param"))
		for _, sp := range sig.params {
			if strings.HasSuffix(sp.typ, "-> "+p) {
				return true
			}
		}
	}
	return false
}

// mappedCtor renders an initializer of a mapped type: `<Type>Init<labels>(args)`.
func (e *fileEmitter) mappedCtor(mp *Mapping, n, d ast.Node, as []arg) string {
	dem := e.m.demangle(d.Str("decl_usr"))
	sig := parseDeclSig(dem)
	ss, kept := e.argStrings(as, true)
	targs := e.m.mappedTypeArgs(e.m.typeOf(n.Type()), e.generics)
	if targs == "" {
		targs = e.mappedTypeArgs(d, dem, false)
	}
	// Initializers keep the runtime's suffix: labels, and the argument kind for each unlabeled argument.
	suffix := ""
	for _, a := range kept {
		if a.label != "" {
			suffix += capitalize(a.label)
		} else {
			suffix += e.m.Recv(e.m.typeOf(a.node.Type()))
		}
	}
	return mp.Alias() + "." + mappedOwner(sig.owner) + "Init" + suffix + targs + "(" + strings.Join(ss, ", ") + ")"
}

// mappedMember renders a property of a mapped declaration. A static property is a package variable, or a
// function of the conforming type when a protocol declares it; an instance property is a function of the
// value it is read from.
func (e *fileEmitter) mappedMember(mp *Mapping, n, d, base ast.Node) string {
	dem := e.m.demangle(d.Str("decl_usr"))
	sig := parseDeclSig(dem)
	name := e.mappedName(mp, sig, nil, false)
	if bt := e.m.typeOf(base.Type()); base.Kind() == "type_expr" || bt.Kind == types.Meta {
		if e.m.mappedKind(sig.owner) == types.KindProtocol {
			return name + e.mappedTypeArgs(d, dem, true) + "()"
		}
		return name
	}
	return name + "(" + e.recvArg(base) + ")"
}

// mappedSetter renders an assignment to a property of a mapped declaration as a call to its setter,
// `<Owner>Set<Name>(&value, newValue)`, which takes a struct by pointer and a class as it is.
func (e *fileEmitter) mappedSetter(mp *Mapping, d, base ast.Node, value string) string {
	sig := parseDeclSig(e.m.demangle(d.Str("decl_usr")))
	name := mp.Alias() + "." + mappedOwner(sig.owner) + "Set" + capitalize(sig.base)
	if base == nil || base.Kind() == "type_expr" || e.m.typeOf(base.Type()).Kind == types.Meta {
		return name + "(" + value + ")"
	}
	recv := e.lvalue(base)
	if !e.m.isReferenceGo(e.m.typeOf(base.Type()), e.exprGoType(base)) {
		recv = "&" + recv
	}
	return name + "(" + recv + ", " + value + ")"
}

// mappedLiteral renders an array or dictionary literal of a mapped type as a call of the type's
// `InitArrayLiteral(elements...)` or `InitDictionaryLiteral(pairs...)`, the pairs as swiftrt.KV values.
func (e *fileEmitter) mappedLiteral(mp *Mapping, n ast.Node, t *types.Type, label string) string {
	var parts []string
	for _, el := range n.List("elements") {
		if label != "dictionaryLiteral" {
			parts = append(parts, e.expr(el))
			continue
		}
		kv := el.List("elements")
		tt := e.m.typeOf(el.Type())
		if len(kv) != 2 || tt == nil || tt.Kind != types.Tuple || len(tt.Args) != 2 {
			return e.m.Unsup("mapped-dictionary-literal", e.where, e.exprGoType(n))
		}
		parts = append(parts, "swiftrt.KV["+e.m.GoType(tt.Args[0], e.generics)+", "+e.m.GoType(tt.Args[1], e.generics)+"]{Key: "+e.expr(kv[0])+", Value: "+e.expr(kv[1])+"}")
	}
	fn := "InitArrayLiteral"
	if label == "dictionaryLiteral" {
		fn = "InitDictionaryLiteral"
	}
	return mp.Alias() + "." + mappedOwner(t.Name) + fn + e.m.mappedTypeArgs(t, e.generics) + "(" + strings.Join(parts, ", ") + ")"
}

// foreignOperatorCall renders a comparison or logical operator declared by a foreign module other than the
// standard library, whose result is not a Bool, as a runtime call `swiftrt.<Owner><Op>(…)` instead of the
// Go operator: a query-building DSL's `column == value` builds an expression, and a Go `==` would compile
// to a bool. The call fails to build until the runtime provides it, which surfaces the gap.
func (e *fileEmitter) foreignOperatorCall(n, d ast.Node, as []arg, prefixOp bool) (string, bool) {
	usr := d.Str("decl_usr")
	if !strings.HasPrefix(usr, "s:") || strings.HasPrefix(usr, "s:s") || strings.HasPrefix(usr, "s:S") {
		return "", false
	}
	if mod := usrModule(usr); mod == "" || mod == e.m.Name || e.m.isTranspiledModule(mod) {
		return "", false
	}
	switch d.Str("base_name") {
	case "==", "!=", "<", "<=", ">", ">=", "&&", "||", "!":
	default:
		return "", false
	}
	if rt := e.m.typeOf(n.Type()); rt == nil || rt.Kind == types.Named && rt.Name == "Swift.Bool" {
		return "", false
	}
	sig := parseDeclSig(e.m.demangle(usr))
	ss, _ := e.argStrings(as, true)
	return "swiftrt." + mappedOwner(sig.owner) + operatorName(sig.base, prefixOp) + "(" + strings.Join(ss, ", ") + ")", true
}

// isTranspiledModule reports whether a module is transpiled in this run, as this module's dependency.
func (m *Module) isTranspiledModule(name string) bool {
	for _, d := range m.Deps {
		if d.Name == name {
			return true
		}
	}
	return false
}

// conformanceRegistrations emits an init function that registers a local type's conformances to protocols
// of mapped modules declared by n, a type or an extension of the type q:
//
//	func init() {
//		pkg.RegisterConformance[T](swiftrt.Conformance{Type: "T", Protocols: []string{…}, Witnesses: map[string]any{…}})
//	}
//
// Protocols lists every mapped protocol the declaration conforms to, implied ones included. Witnesses maps
// each requirement of those protocols that the type implements, itself or through a default in a local
// protocol's extension, to a Go function: a static property is `func() V`, a static method or initializer
// its Go function, and an instance member a function taking the receiver first, as `*T` for a struct.
func (e *fileEmitter) conformanceRegistrations(n ast.Node, q string) {
	if len(e.m.Mappings) == 0 || len(e.m.TypeParams(q)) > 0 {
		return
	}
	type reg struct {
		mp     *Mapping
		protos []string
	}
	var regs []*reg
	byAlias := map[string]*reg{}
	for _, c := range n.Child("inherits").List("conformances") {
		p := c.Str("protocol")
		mp := e.m.mappedModule(usrModule(p))
		if mp == nil || c.Kind() == "builtin_conformance" {
			continue
		}
		r := byAlias[mp.Alias()]
		if r == nil {
			r = &reg{mp: mp}
			byAlias[mp.Alias()] = r
			regs = append(regs, r)
		}
		name := strings.TrimPrefix(e.m.demangle(p), mp.Module+".")
		if !slices.Contains(r.protos, name) {
			r.protos = append(r.protos, name)
		}
	}
	if len(regs) == 0 {
		return
	}
	goT := e.m.GoType(&types.Type{Kind: types.Named, Name: q}, nil)
	for _, r := range regs {
		var witnesses []string
		seen := map[string]bool{}
		for _, proto := range r.protos {
			for _, req := range r.mp.Requirements[proto] {
				if seen[req.Name] {
					continue
				}
				seen[req.Name] = true
				if w := e.witness(q, goT, req); w != "" {
					witnesses = append(witnesses, strconv.Quote(req.Name)+": "+w)
				}
			}
		}
		quoted := make([]string, len(r.protos))
		for i, p := range r.protos {
			quoted[i] = strconv.Quote(p)
		}
		e.w("func init() {\n\t%s.RegisterConformance[%s](swiftrt.Conformance{\n\t\tType: %q,\n\t\tProtocols: []string{%s},\n",
			r.mp.Alias(), goT, lastComponent(q), strings.Join(quoted, ", "))
		if len(witnesses) > 0 {
			e.w("\t\tWitnesses: map[string]any{\n")
			for _, w := range witnesses {
				e.w("\t\t\t%s,\n", w)
			}
			e.w("\t\t},\n")
		}
		e.w("\t})\n}\n\n")
	}
}

// swiftMemberName spells a declaration as a protocol requirement names it: "didInsert(_:)", "tableName",
// "init(row:)".
func swiftMemberName(d *DeclInfo) string {
	if d.Kind == "var_decl" {
		return d.Node.Name()
	}
	base := d.Node.Name()
	if d.Kind == "constructor_decl" {
		base = "init"
	}
	var b strings.Builder
	b.WriteString(base + "(")
	for _, l := range paramLabels(d.Node) {
		if l == "" {
			l = "_"
		}
		b.WriteString(l + ":")
	}
	return b.String() + ")"
}

// witness renders the Go function for the type q's implementation of a requirement, or "" when q relies on
// the mapped package's default.
func (e *fileEmitter) witness(q, goT string, req swiftmod.Requirement) string {
	var impl, def *DeclInfo
	var implMod, defMod *Module
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.sortedDecls() {
			if d.Node == nil || d.Node.Bool("implicit") || d.Static != req.Static || swiftMemberName(d) != req.Name {
				continue
			}
			if d.Kind != "var_decl" && d.Kind != "func_decl" && d.Kind != "constructor_decl" {
				continue
			}
			if d.Node.Child("generic_params") != nil || strings.Contains(d.Node.Str("generic_signature"), "<") && d.Kind == "func_decl" && !d.ProtoExt {
				continue
			}
			switch {
			case d.Owner == q && !d.ProtoExt && impl == nil:
				impl, implMod = d, mod
			case d.ProtoExt && def == nil && slices.Contains(e.m.allConformers(d.Owner), q):
				def, defMod = d, mod
			}
		}
	}
	if impl == nil && def == nil {
		return ""
	}
	if impl == nil {
		impl, implMod = def, defMod
	}
	pfx := ""
	if implMod.Pkg != e.m.Pkg {
		e.m.importPkgs[implMod.Pkg] = implMod.ImportPath
		pfx = implMod.Pkg + "."
	}
	saveM, saveIO := e.m, e.inoutParams
	e.m, e.inoutParams = implMod, nil
	defer func() { e.m, e.inoutParams = saveM, saveIO }()
	ptrT := goT
	if !strings.HasPrefix(goT, "*") {
		ptrT = "*" + goT
	}
	switch {
	case impl.Kind == "constructor_decl" || impl.Kind == "func_decl" && impl.Static && !impl.ProtoExt:
		return pfx + impl.GoName
	case impl.Kind == "var_decl" && impl.Static && !impl.ProtoExt:
		if impl.Computed || impl.Lazy {
			return pfx + impl.GoName
		}
		return "func() " + e.m.GoType(e.m.typeOf(impl.Node.Str("interface_type")), nil) + " { return " + pfx + impl.GoName + " }"
	case impl.Kind == "var_decl":
		vt := e.m.GoType(e.m.typeOf(impl.Node.Str("interface_type")), nil)
		body := "self." + impl.GoName
		switch {
		case impl.ProtoExt:
			body = pfx + impl.GoName + "(" + e.protoExtSelf(q, goT, impl.Owner) + ")"
		case impl.FreeSelf():
			body = pfx + impl.GoName + "(" + derefSelf(goT) + ")"
		case impl.Computed:
			body += "()"
		}
		return "func(self " + ptrT + ") " + vt + " { return " + body + " }"
	default:
		params, args := e.paramList(impl.Node)
		res := e.resultGo(impl.Node)
		call := "self." + impl.GoName + "(" + args + ")"
		switch {
		case impl.ProtoExt && impl.Static:
			call = pfx + impl.GoName + "(*new(" + goT + ")" + prefixComma(args) + ")"
		case impl.ProtoExt:
			call = pfx + impl.GoName + "(" + e.protoExtSelf(q, goT, impl.Owner) + prefixComma(args) + ")"
		case impl.FreeSelf() && impl.Mutating:
			call = pfx + impl.GoName + "(self" + prefixComma(args) + ")"
		case impl.FreeSelf():
			call = pfx + impl.GoName + "(" + derefSelf(goT) + prefixComma(args) + ")"
		}
		if impl.Static {
			if res == "" {
				return "func(" + params + ") { " + call + " }"
			}
			return "func(" + params + ") " + res + " { return " + call + " }"
		}
		if res == "" {
			return "func(self " + ptrT + prefixComma(params) + ") { " + call + " }"
		}
		return "func(self " + ptrT + prefixComma(params) + ") " + res + " { return " + call + " }"
	}
}

// protoSelf passes a witness's receiver, a *T, to a protocol-extension function: as the pointer when only
// *T satisfies the protocol, else as the value.
func (e *fileEmitter) protoSelf(q, goT string) string {
	if strings.HasPrefix(goT, "*") || e.m.HasMutatingMethods(q) {
		return "self"
	}
	return "*self"
}

// protoExtSelf passes a witness's receiver to a default in the extension of the local protocol proto.
func (e *fileEmitter) protoExtSelf(q, goT, proto string) string {
	if s, ok := e.retroactiveSelf(q, goT, proto); ok {
		return s
	}
	return e.protoSelf(q, goT)
}

// retroactiveSelf passes a witness's receiver to a default of the local protocol proto when the type q
// is declared in another module and conforms to proto in this one: Go cannot add proto's accessors to the
// type, so the receiver is wrapped in an adapter that embeds it and adds them. ok is false when q is
// declared in this module.
func (e *fileEmitter) retroactiveSelf(q, goT, proto string) (string, bool) {
	if e.m.typeKinds[q] != "" || e.m.typeKinds[proto] != "protocol" {
		return "", false
	}
	name := lowerFirst(lastComponent(q)) + lastComponent(proto) + "Adapter"
	if e.m.adapters == nil {
		e.m.adapters = map[string]bool{}
	}
	ptrT := goT
	if !strings.HasPrefix(goT, "*") {
		ptrT = "*" + goT
	}
	field := strings.TrimPrefix(ptrT[strings.LastIndex(ptrT, ".")+1:], "*")
	if !e.m.adapters[name] {
		e.m.adapters[name] = true
		var b strings.Builder
		fmt.Fprintf(&b, "// %s gives %s the accessors of %s, which it conforms to here.\n", name, lastComponent(q), lastComponent(proto))
		fmt.Fprintf(&b, "type %s struct{ %s }\n\n", name, ptrT)
		for _, p := range e.m.protocolAndAncestors(proto) {
			for _, r := range e.m.protoReqs[p] {
				if r.Kind() != "var_decl" || r.Bool("static") {
					continue
				}
				req := e.m.decls[r.Str("usr")]
				if req == nil {
					continue
				}
				access := e.memberAccess(q, r.Name())
				if access == "" {
					continue
				}
				vt := e.m.GoType(e.m.typeOf(r.Str("interface_type")), nil)
				fmt.Fprintf(&b, "func (a %s) Get%s() %s { return a.%s.%s }\n\n", name, req.GoName, vt, field, access)
				if r.Str("writeImpl") == "setter" && !strings.HasSuffix(access, ")") {
					fmt.Fprintf(&b, "func (a %s) Set%s(newValue %s) { a.%s.%s = newValue }\n\n", name, req.GoName, vt, field, access)
				}
			}
		}
		e.extraDecls.WriteString(b.String())
	}
	return name + "{self}", true
}

// memberAccess renders reading the instance property named name of the type q: a field, or a getter call.
func (e *fileEmitter) memberAccess(q, name string) string {
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.decls {
			if d.Kind == "var_decl" && d.Owner == q && !d.Static && d.Node != nil && d.Node.Name() == name {
				if d.Computed {
					return d.GoName + "()"
				}
				return d.GoName
			}
		}
	}
	return ""
}

// protocolAndAncestors lists a local protocol and the local protocols it inherits from.
func (m *Module) protocolAndAncestors(proto string) []string {
	out := []string{proto}
	for i := 0; i < len(out); i++ {
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, p := range mod.protoParents[out[i]] {
				if !slices.Contains(out, p) {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func derefSelf(goT string) string {
	if strings.HasPrefix(goT, "*") {
		return "self"
	}
	return "*self"
}

// allConformers lists the local types conforming to the local protocol proto, directly or through a
// protocol that refines it.
func (m *Module) allConformers(proto string) []string {
	var out []string
	for _, mod := range append([]*Module{m}, m.Deps...) {
		for p, qs := range mod.conformers {
			if p == proto {
				out = append(out, qs...)
			}
		}
	}
	return out
}
