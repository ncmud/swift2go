package emit

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

func (e *fileEmitter) expr(n ast.Node) string {
	out := e.exprRaw(n)
	if n == nil || !strings.HasPrefix(out, "swiftrt.") {
		return out
	}
	switch n.Kind() {
	case "call_expr", "subscript_expr", "member_ref_expr", "optional_try_expr", "conditional_checked_cast_expr":
	default:
		return out
	}
	for _, keep := range []string{"swiftrt.Unsupported", "swiftrt.CoalesceRef", "swiftrt.CoalesceOpt", "swiftrt.Flat(", "swiftrt.TryFlat", "swiftrt.Some",
		"swiftrt.TaskLocalGet", "swiftrt.TaskLocalWithValue", "swiftrt.MapSubscriptDefault", "swiftrt.ForceRef", "swiftrt.Clone", "swiftrt.MustAs",
		"swiftrt.Dynamic", "swiftrt.Ref", "swiftrt.MutexWithLock", "swiftrt.NSLockWithLock", "swiftrt.Coalesce(",
		"swiftrt.OptFlatMap", "swiftrt.RefFlatMap"} {
		if strings.HasPrefix(out, keep) {
			return out
		}
	}
	if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 {
		if e.m.isReferenceGo(t.Args[0], e.m.GoType(t.Args[0], e.generics)) {
			return "swiftrt.Flat(" + out + ")"
		}
	}
	return out
}

func (e *fileEmitter) exprRaw(n ast.Node) string {
	if n == nil {
		return e.m.Unsup("expr:nil", e.where, "")
	}
	switch n.Kind() {
	case "paren_expr":
		return "(" + e.expr(n.Child("sub_expr")) + ")"
	case "derived_to_base_expr":
		return e.upcastClass(n)
	case "erasure_expr":
		sub := n.Child("sub_expr")
		if nt, st := e.m.typeOf(n.Type()), e.m.typeOf(sub.Type()); nt != nil && st != nil && nt.Kind == types.Meta && st.Kind == types.Meta && len(st.Args) == 1 && st.Args[0].Kind == types.Named {
			// A concrete metatype stored as `any P.Type` becomes the type's zero value, which carries P's static requirements.
			gt := e.m.GoType(st.Args[0], e.generics)
			if e.m.HasMutatingMethods(st.Args[0].Name) {
				return "new(" + gt + ")"
			}
			return "*new(" + gt + ")"
		}
		if t := e.m.typeOf(sub.Type()); t != nil && t.Kind == types.Named && e.m.HasMutatingMethods(t.Name) {
			return "swiftrt.Ref(" + e.expr(sub) + ")"
		}
		return e.expr(sub)
	case "dot_syntax_base_ignored":
		return e.expr(n.Child("rhs"))
	case "archetype_to_super_expr":
		return e.expr(n.Child("sub_expr"))
	case "super_ref_expr":
		if t := e.m.typeOf(n.Type()); t != nil {
			return "(&self." + strings.TrimPrefix(e.m.GoType(t, e.generics), "*") + ")"
		}
		return "self"
	case "load_expr", "function_conversion_expr", "destructure_tuple_expr",
		"covariant_return_conversion_expr", "underlying_to_opaque_expr", "abi_safe_conversion_expr", "unresolved_type_conversion_expr",
		"collection_upcast_expr", "pointer_to_pointer", "metatype_conversion_expr", "existential_metatype_to_object",
		"unevaluated_instance", "dot_self_expr", "await_expr", "unsafe_expr", "borrow_expr", "copy_expr", "consume_expr", "any_hashable_erasure_expr":
		if n.Kind() == "collection_upcast_expr" || n.Kind() == "any_hashable_erasure_expr" {
			return e.upcast(n)
		}
		if ac := n.Child("sub_expr"); n.Kind() == "function_conversion_expr" && ac.Kind() == "autoclosure_expr" && len(ac.Child("params").List("params")) == 0 {
			// A method reference (`QuestRuntime.tick`) is an autoclosure around the call.
			if ft := e.m.typeOf(ac.Type()); ft != nil && ft.Kind == types.Func {
				if res := e.m.GoType(ft.Result, e.generics); res != "" && !isVoidGo(res) {
					return "func() " + res + " { return " + e.expr(ac) + " }"
				}
				return "func() { " + e.expr(ac) + " }"
			}
		}
		return e.expr(n.Child("sub_expr"))
	case "coerce_expr":
		sub := n.Child("sub_expr")
		if strings.HasSuffix(sub.Kind(), "literal_expr") && sub.Kind() != "string_literal_expr" {
			return e.exprGoType(n) + "(" + e.expr(sub) + ")"
		}
		return e.expr(sub)
	case "inout_expr":
		sub := n.Child("sub_expr")
		lv := e.lvalue(sub)
		if ds := stripLoads(sub); ds.Kind() == "subscript_expr" && e.m.Recv(e.m.typeOf(ds.Child("base").Type())) == "Map" && strings.HasSuffix(lv, "]") {
			// Go map elements are not addressable: mutate a copy, then store it back.
			tmp := e.newTmp("slot")
			e.w("%s := %s\n", tmp, lv)
			e.post = append(e.post, lv+" = "+tmp)
			return "&" + tmp
		}
		return "&" + lv
	case "string_literal_expr":
		if e.m.Recv(e.m.typeOf(n.Type())) == "Substring" {
			return "swiftrt.SubstringInitString(" + e.stringLiteral(n) + ")"
		}
		return e.stringLiteral(n)
	case "integer_literal_expr", "float_literal_expr":
		v := n.Str("value")
		if n.Bool("negative") {
			v = "-" + v
		}
		return v
	case "boolean_literal_expr":
		return n.Str("value")
	case "nil_literal_expr":
		return "nil"
	case "interpolated_string_literal_expr":
		return e.interpolation(n)
	case "magic_identifier_literal_expr":
		return e.magicLiteral(n)
	case "declref_expr":
		return e.declRef(n.Child("decl"), n)
	case "member_ref_expr":
		if n.Child("decl").Str("base_name") == "magnitude" && strings.HasPrefix(n.Child("decl").Str("decl_usr"), "s:s") {
			// The runtime returns uint; Swift's magnitude is the unsigned type of the operand's width.
			return e.exprGoType(n) + "(" + e.memberRef(n) + ")"
		}
		return e.memberRef(n)
	case "dot_syntax_call_expr":
		return e.unappliedDot(n)
	case "constructor_ref_call_expr":
		return e.m.Unsup("unapplied-init", e.where, e.exprGoType(n))
	case "call_expr":
		return e.call(n)
	case "binary_expr":
		return e.binary(n)
	case "prefix_unary_expr":
		return e.prefix(n)
	case "postfix_unary_expr":
		return e.postfix(n)
	case "tuple_expr":
		return e.tuple(n)
	case "tuple_element_expr":
		return e.expr(n.Child("base")) + ".F" + n.Str("field #")
	case "property_wrapper_value_placeholder_expr":
		if v := n.Child("original_wrapped_value"); v != nil {
			return e.expr(v)
		}
		if v, ok := e.opaque[n.Child("opaque_value_placeholder").Str("identity")]; ok {
			return v
		}
		return e.m.Unsup("expr:property_wrapper_value_placeholder_expr", e.where, e.exprGoType(n))
	case "single_value_stmt_expr":
		saved := e.b
		e.b = strings.Builder{}
		e.stmt(n.Child("stmt"))
		body := e.b.String()
		e.b = saved
		return "func() " + e.exprGoType(n) + " {\n" + body + "panic(\"unreachable\")\n}()"
	case "rebind_self_in_constructor_expr":
		sub := n.Child("sub_expr")
		call := e.expr(sub)
		if t := e.m.typeOf(sub.Type()); t != nil && t.Kind == types.Optional {
			tmp := e.newTmp("delegated")
			e.w("%s := %s\nif %s == nil {\nreturn nil\n}\n", tmp, call, tmp)
			if e.initRet == "&self" {
				return "self = *" + tmp
			}
			return "self = " + tmp
		}
		if fn := stripConv(sub.Child("fn")); fn != nil && fn.Kind() == "dot_syntax_call_expr" {
			if as := fn.Child("args").List("args"); len(as) > 0 && as[0].Child("expr").Kind() == "super_ref_expr" {
				sup := strings.TrimPrefix(e.m.GoType(e.m.typeOf(as[0].Child("expr").Type()), e.generics), "*")
				root := sup
				for e.m.Superclass(root) != "" {
					root = e.m.Superclass(root)
				}
				e.w("self.%s = *%s\n", sup, call)
				return "self.SwiftSub = self"
			}
		}
		return "self = " + call
	case "array_expr":
		var parts []string
		for _, el := range n.List("elements") {
			parts = append(parts, e.expr(el))
		}
		if lit, ok := e.literalInit(n, "arrayLiteral"); ok {
			return lit
		}
		gt := e.exprGoType(n)
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Named && e.m.IsOptionSet(t.Name) {
			return "swiftrt.OptionSetOf[" + gt + "](" + strings.Join(parts, ", ") + ")"
		}
		if strings.HasPrefix(gt, "map[") && strings.HasSuffix(gt, "]struct{}") {
			for i := range parts {
				parts[i] += ": {}"
			}
		}
		return "(" + gt + "{" + strings.Join(parts, ", ") + "})"
	case "dictionary_expr":
		if lit, ok := e.literalInit(n, "dictionaryLiteral"); ok {
			return lit
		}
		var parts []string
		for _, el := range n.List("elements") {
			kv := el.List("elements")
			if len(kv) == 2 {
				parts = append(parts, e.expr(kv[0])+": "+e.expr(kv[1]))
			}
		}
		return "(" + e.exprGoType(n) + "{" + strings.Join(parts, ", ") + "})"
	case "subscript_expr":
		return e.subscript(n)
	case "ternary_expr":
		return fmt.Sprintf("func() %s { if %s { return %s }; return %s }()", e.exprGoType(n), e.expr(n.Child("cond_expr")), e.expr(n.Child("then_expr")), e.expr(n.Child("else_expr")))
	case "inject_into_optional":
		sub := n.Child("sub_expr")
		t := e.m.typeOf(n.Type())
		if t.Kind == types.Optional {
			inner := e.m.GoType(t.Args[0], e.generics)
			if inner == "" {
				return e.expr(sub)
			}
			if e.m.isReferenceGo(t.Args[0], inner) {
				return e.expr(sub)
			}
			return "swiftrt.Some[" + inner + "](" + e.expr(sub) + ")"
		}
		return "swiftrt.Some(" + e.expr(sub) + ")"
	case "force_value_expr":
		sub := n.Child("sub_expr")
		return e.forceUnwrap(sub)
	case "bind_optional_expr":
		if v, ok := e.opaque[nodeID(n)]; ok {
			return v
		}
		return e.m.Unsup("bind_optional-outside-chain", e.where, e.exprGoType(n))
	case "optional_evaluation_expr":
		return e.optionalChain(n)
	case "opaque_value_expr":
		if v, ok := e.opaque[n.Str("identity")]; ok {
			return v
		}
		return e.m.Unsup("opaque_value", e.where, e.exprGoType(n))
	case "closure_expr":
		return e.closure(n)
	case "autoclosure_expr":
		if ps := n.Child("params").List("params"); len(ps) > 0 {
			return e.autoclosureFunc(n, ps)
		}
		return e.expr(n.Child("single_expr_body"))
	case "default_argument_expr":
		return e.defaultArg(n)
	case "assign_expr":
		saved := e.b
		e.b = strings.Builder{}
		e.assign(n)
		s := e.b.String()
		e.b = saved
		return "func() struct{} { " + strings.TrimSpace(s) + "; return struct{}{} }()"
	case "try_expr", "force_try_expr", "optional_try_expr":
		return e.tryExpr(n)
	case "is_expr":
		return "swiftrt.Is[" + e.m.GoType(e.m.typeOf(n.Str("cast_to")), e.generics) + "](" + e.expr(n.Child("sub_expr")) + ")"
	case "conditional_checked_cast_expr":
		return "swiftrt.As[" + e.castTarget(n) + "](" + e.expr(n.Child("sub_expr")) + ")"
	case "forced_checked_cast_expr":
		return "swiftrt.MustAs[" + e.castTarget(n) + "](" + e.expr(n.Child("sub_expr")) + ")"
	case "type_expr":
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Meta && len(t.Args) == 1 {
			inner := e.m.GoType(t.Args[0], e.generics)
			if inner == "" {
				inner = "struct{}"
			}
			return "swiftrt.TypeOf[" + inner + "]()"
		}
		return e.m.Unsup("type-as-value", e.where, "")
	case "if_expr", "switch_expr":
		return e.stmtExpr(n)
	case "keypath_expr":
		return e.keyPath(n)
	case "keypath_application_expr":
		kp := n.Child("keypath")
		if strings.HasPrefix(e.exprGoType(kp), "swiftrt.WritableKeyPath[") {
			return e.expr(kp) + ".Get(" + e.expr(n.Child("base")) + ")"
		}
		return e.expr(kp) + "(" + e.expr(n.Child("base")) + ")"
	case "open_existential_expr":
		e.opaque[n.Child("opaque_value").Str("identity")] = e.expr(n.Child("existential_value"))
		return e.expr(n.Child("sub_expr"))
	case "vararg_expansion_expr":
		return e.expr(n.Child("sub_expr"))
	case "capture_list":
		return e.captureList(n)
	case "macro_expansion_expr":
		return e.macro(n)
	}
	return e.m.Unsup("expr:"+n.Kind(), e.where, e.exprGoType(n))
}

func (e *fileEmitter) castTarget(n ast.Node) string {
	t := e.m.typeOf(n.Type())
	if t.Kind == types.Optional {
		t = t.Args[0]
	}
	return e.m.GoType(t, e.generics)
}

// stripConv removes implicit conversions wrapped around a function reference.
func stripConv(n ast.Node) ast.Node {
	for n != nil {
		switch n.Kind() {
		case "function_conversion_expr", "paren_expr", "load_expr", "covariant_function_conversion_expr":
			n = n.Child("sub_expr")
			continue
		}
		return n
	}
	return n
}

func nodeID(n ast.Node) string { return fmt.Sprintf("%p", map[string]any(n)) }

func (e *fileEmitter) upcast(n ast.Node) string {
	return e.expr(n.Child("sub_expr"))
}

func (e *fileEmitter) stringLiteral(n ast.Node) string {
	v := n.Str("value")
	t := e.m.typeOf(n.Type())
	switch e.m.Recv(t) {
	case "Scalar":
		r := []rune(v)
		if len(r) == 1 {
			return strconv.QuoteRune(r[0])
		}
	case "Int":
		r := []rune(v)
		if len(r) == 1 {
			return strconv.Itoa(int(r[0]))
		}
	}
	return strconv.Quote(v)
}

func (e *fileEmitter) interpolation(n ast.Node) string {
	tap := n.Child("appending_expr")
	var parts []string
	for _, el := range tap.Child("body").List("elements") {
		if el.Kind() != "call_expr" {
			continue
		}
		mc := callOf(el)
		args := el.Child("args").List("args")
		if mc == nil || len(args) == 0 {
			continue
		}
		switch mc.method {
		case "appendLiteral":
			lit := args[0].Child("expr")
			if lit.Kind() == "string_literal_expr" && lit.Str("value") == "" {
				continue
			}
			parts = append(parts, e.expr(lit))
		case "appendInterpolation":
			if len(args) != 1 || args[0].Str("label") != "" {
				parts = append(parts, e.m.Unsup("interpolation-with-labels", e.where, "string"))
				continue
			}
			a := args[0].Child("expr")
			if e.m.Recv(e.m.typeOf(a.Type())) == "String" {
				parts = append(parts, e.expr(a))
			} else {
				parts = append(parts, "swiftrt.Str("+e.expr(a)+")")
			}
		}
	}
	if len(parts) == 0 {
		return `""`
	}
	return strings.Join(parts, " + ")
}

// lookup finds a declaration in this module or a dependency; prefix qualifies it for Go.
func (e *fileEmitter) lookup(usr string) (*DeclInfo, string) {
	if usr == "" {
		return nil, ""
	}
	if info := e.m.decls[usr]; info != nil {
		return info, ""
	}
	for _, d := range e.m.Deps {
		if info := d.decls[usr]; info != nil {
			if d.Pkg == e.m.Pkg {
				return info, ""
			}
			e.m.importPkgs[d.Pkg] = d.ImportPath
			return info, d.Pkg + "."
		}
	}
	return nil, ""
}

func (e *fileEmitter) declRef(d ast.Node, n ast.Node) string {
	usr := d.Str("decl_usr")
	name := d.Str("base_name")
	if usr == "" {
		if name == "self" {
			if e.inoutParams["self"] {
				return "(*self)"
			}
			return "self"
		}
		if e.inoutParams[name] {
			return "(*" + goSafe(name) + ")"
		}
		return goSafe(name)
	}
	info, pfx := e.lookup(usr)
	if info == nil {
		dem := e.m.demangle(usr)
		if strings.Contains(dem, "(in ") || strings.HasPrefix(usr, "s:") && strings.Contains(usr, "L_") {
			return goSafe(name)
		}
		return e.stdFuncValue(dem, n)
	}
	switch info.Kind {
	case "var_decl":
		if info.Owner == "" || info.Static {
			if info.Computed || info.Lazy {
				return pfx + info.GoName + "()"
			}
			return pfx + info.GoName
		}
		return goSafe(name)
	case "enum_element_decl":
		if e.m.IsPayloadEnum(info.Owner) && !e.isPayloadCase(info) {
			targs := e.enumTypeArgs(info, e.m.typeOf(n.Type()))
			return e.asEnum(info, pfx, pfx+info.GoName+targs+"{}", targs)
		}
		return pfx + info.GoName
	case "func_decl":
		if info.Owner == "" || info.Static {
			return pfx + info.GoName
		}
		if info.Node.Bool("local") || e.isLocalFunc(info) {
			return goSafe(name)
		}
		return pfx + info.GoName
	case "parameter":
		return goSafe(name)
	}
	return goSafe(name)
}

func (e *fileEmitter) isLocalFunc(info *DeclInfo) bool { return false }

func (e *fileEmitter) stdFuncValue(dem string, n ast.Node) string {
	sig := parseDeclSig(dem)
	if sig.owner == "Swift" || sig.owner == "" {
		return "swiftrt." + capitalize(sig.base)
	}
	return e.m.Unsup("std-func-value:"+sig.owner+"."+sig.base, e.where, e.exprGoType(n))
}

func (e *fileEmitter) memberRef(n ast.Node) string {
	d := n.Child("decl")
	base := n.Child("base")
	usr := d.Str("decl_usr")
	info, pfx := e.lookup(usr)
	if info != nil && info.Kind == "var_decl" {
		switch {
		case info.Static && info.OwnerKind == "protocol":
			call := e.metaReceiver(base) + "." + staticReqName(info) + "()"
			declared := e.m.GoType(e.m.typeOf(info.Node.Str("interface_type")), nil)
			if gt := e.exprGoType(n); strings.HasPrefix(gt, "[]") && gt != "[]any" && declared == "[]any" {
				return "swiftrt.FromAnySlice[" + gt[2:] + "](" + call + ")"
			}
			return call
		case info.Static && info.ProtoExt:
			return pfx + info.GoName + "(" + e.metaReceiver(base) + ")"
		case info.Static:
			if info.Computed || info.Lazy {
				return pfx + info.GoName + "()"
			}
			return pfx + info.GoName
		case info.OwnerKind == "protocol":
			return e.expr(base) + ".Get" + info.GoName + "()"
		case info.FreeSelf():
			return pfx + info.GoName + "(" + e.expr(base) + ")"
		case info.Computed:
			return e.expr(base) + "." + info.GoName + "()"
		default:
			return e.recvExpr(base) + "." + info.GoName
		}
	}
	if info != nil {
		return e.m.Unsup("member-ref-"+info.Kind, e.where, e.exprGoType(n))
	}
	dem := e.m.demangle(usr)
	sig := parseDeclSig(dem)
	bt := e.m.typeOf(base.Type())
	if base.Kind() == "type_expr" || bt.Kind == types.Meta {
		name := "swiftrt." + e.m.Recv(e.m.typeOf(n.Type())) + capitalize(sig.base)
		if dynamicStatics[strings.TrimPrefix(name, "swiftrt.")] {
			name += "()"
		}
		return name
	}
	recv := e.m.Recv(bt)
	return "swiftrt." + recv + capitalize(sig.base) + "(" + e.recvArg(base) + ")"
}

// recvExpr renders a receiver for field access or a Go method call.
func (e *fileEmitter) recvExpr(base ast.Node) string {
	if base.Kind() == "inout_expr" {
		return e.lvalue(base.Child("sub_expr"))
	}
	s := e.expr(base)
	if strings.HasPrefix(s, "*") {
		return "(" + s + ")"
	}
	return s
}

// recvArg renders a receiver passed as a function argument (pointer when mutated).
func (e *fileEmitter) recvArg(base ast.Node) string {
	return e.expr(base)
}

func (e *fileEmitter) lvalue(n ast.Node) string {
	switch n.Kind() {
	case "declref_expr":
		d := n.Child("decl")
		if d.Str("decl_usr") == "" {
			name := d.Str("base_name")
			if name == "self" && !e.inoutParams["self"] {
				return "self"
			}
			if e.inoutParams[name] {
				return "(*" + goSafe(name) + ")"
			}
			return goSafe(name)
		}
		if info, pfx := e.lookup(d.Str("decl_usr")); info != nil && info.Lazy {
			return lazyCell(info, pfx)
		}
		return e.declRef(d, n)
	case "member_ref_expr":
		info, pfx := e.lookup(n.Child("decl").Str("decl_usr"))
		if info != nil && info.Kind == "var_decl" && !info.Computed {
			if info.Static && info.Lazy {
				return lazyCell(info, pfx)
			}
			if info.Static {
				return pfx + info.GoName
			}
			return e.lvalue(n.Child("base")) + "." + info.GoName
		}
		if info == nil && n.Child("decl").Str("decl_usr") != "" {
			return e.lvalue(n.Child("base")) + "." + capitalize(n.Child("decl").Str("base_name"))
		}
		if info != nil && info.Kind == "var_decl" && info.Computed {
			// A getter that returns a class reference can be written through directly.
			if t := e.m.typeOf(n.Type()); t != nil {
				if gt := e.m.GoType(t, e.generics); strings.HasPrefix(gt, "*") && e.m.isReferenceGo(t, gt) && t.Kind != types.Optional {
					return e.expr(n)
				}
			}
		}
		if info != nil && info.Kind == "var_decl" && info.Settable {
			tmp := e.newTmp("wb")
			e.w("%s := %s\n", tmp, e.expr(n))
			if call, ok := e.setterCall(n, tmp); ok {
				e.post = append(e.post, call)
				return tmp
			}
		}
		why := "foreign"
		if info != nil {
			why = info.Kind
			if info.Computed {
				why = "computed"
			}
		}
		return e.m.Unsup("lvalue:member:"+why, e.where+" ."+n.Child("decl").Str("base_name"), "")
	case "load_expr", "paren_expr", "inout_expr", "derived_to_base_expr", "dot_self_expr", "archetype_to_super_expr":
		return e.lvalue(n.Child("sub_expr"))
	case "dot_syntax_base_ignored":
		return e.lvalue(n.Child("rhs"))
	case "opaque_value_expr":
		if v, ok := e.opaque[n.Str("identity")]; ok {
			return v
		}
		return e.m.Unsup("lvalue:opaque_value_expr", e.where, "")
	case "open_existential_expr":
		e.opaque[n.Child("opaque_value").Str("identity")] = e.lvalue(n.Child("existential_value"))
		return e.lvalue(n.Child("sub_expr"))
	case "subscript_expr":
		bt := e.m.typeOf(n.Child("base").Type())
		args := n.Child("args").List("args")
		if len(args) == 1 && e.m.Recv(bt) == "Slice" && e.m.Recv(e.m.typeOf(args[0].Child("expr").Type())) == "Int" {
			return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "]"
		}
		if len(args) == 1 && e.m.Recv(bt) == "Map" {
			return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "]"
		}
		if len(args) == 2 && e.m.Recv(bt) == "Map" && args[1].Str("label") == "default" && isZeroLiteral(args[1].Child("expr")) {
			return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "]"
		}
		if len(args) == 2 && e.m.Recv(bt) == "Map" && args[1].Str("label") == "default" {
			m := e.lvalue(n.Child("base"))
			k := e.newTmp("key")
			e.w("%s := %s\nif _, ok := %s[%s]; !ok {\n%s[%s] = %s\n}\n", k, e.expr(args[0].Child("expr")), m, k, m, k, e.defaultValue(args[1].Child("expr")))
			return m + "[" + k + "]"
		}
		var labels []string
		for _, a := range args {
			labels = append(labels, a.Str("label")+":"+e.m.Recv(e.m.typeOf(a.Child("expr").Type())))
		}
		return e.m.Unsup("lvalue:subscript:"+e.m.Recv(bt)+"["+strings.Join(labels, ",")+"]", e.where, "")
	case "tuple_element_expr":
		return e.lvalue(n.Child("base")) + ".F" + n.Str("field #")
	case "force_value_expr":
		return "(*" + e.expr(n.Child("sub_expr")) + ")"
	case "discard_assignment_expr":
		return "_"
	case "bind_optional_expr":
		if v, ok := e.opaque[nodeID(n)]; ok {
			return v
		}
	}
	return e.m.Unsup("lvalue:"+n.Kind(), e.where, "")
}

func (e *fileEmitter) subscriptAssign(dest, src ast.Node) (string, bool) {
	bt := e.m.typeOf(dest.Child("base").Type())
	args := dest.Child("args").List("args")
	if e.m.Recv(bt) == "OrderedDictionary" && len(args) == 1 {
		v := e.expr(src)
		switch {
		case src.Kind() == "nil_literal_expr":
			v = "nil"
		case src.Kind() == "inject_into_optional":
			v = "swiftrt.Some(" + e.expr(src.Child("sub_expr")) + ")"
		default:
			if t := e.m.typeOf(src.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 && e.m.isReferenceGo(t.Args[0], e.m.GoType(t.Args[0], e.generics)) {
				v = "swiftrt.SomeRef(" + v + ")"
			}
		}
		return "swiftrt.OrderedDictionarySet(&" + e.lvalue(dest.Child("base")) + ", " + e.expr(args[0].Child("expr")) + ", " + v + ")", true
	}
	if e.m.Recv(bt) != "Map" {
		return "", false
	}
	if len(args) != 1 {
		return "", false
	}
	m := e.lvalue(dest.Child("base"))
	k := e.expr(args[0].Child("expr"))
	if src.Kind() == "nil_literal_expr" {
		return "delete(" + m + ", " + k + ")", true
	}
	if src.Kind() == "inject_into_optional" {
		return m + "[" + k + "] = " + e.expr(src.Child("sub_expr")), true
	}
	return "swiftrt.MapSet(" + m + ", " + k + ", " + e.expr(src) + ")", true
}

func (e *fileEmitter) subscript(n ast.Node) string {
	base := n.Child("base")
	for base.Kind() == "inout_expr" {
		base = base.Child("sub_expr")
	}
	args := n.Child("args").List("args")
	d := n.Child("decl")
	if info, _ := e.lookup(d.Str("decl_usr")); info != nil {
		if info.Kind == "subscript_decl" && !info.FreeSelf() && !info.Static {
			ss, _ := e.argStrings(e.args(n.Child("args")), false)
			return e.recvExpr(base) + "." + info.GoName + "(" + strings.Join(ss, ", ") + ")"
		}
		return e.m.Unsup("user-subscript", e.where, e.exprGoType(n))
	}
	bt := e.m.typeOf(base.Type())
	recv := e.m.Recv(bt)
	if len(args) == 2 && args[1].Str("label") == "default" && (recv == "Map" || recv == "OrderedDictionary") {
		fn := "swiftrt.MapSubscriptDefault"
		if recv == "OrderedDictionary" {
			fn = "swiftrt.OrderedDictionarySubscriptDefault"
		}
		return fn + "(" + e.expr(base) + ", " + e.expr(args[0].Child("expr")) + ", " + e.defaultValue(args[1].Child("expr")) + ")"
	}
	if len(args) != 1 {
		return e.m.Unsup("subscript-multi-arg", e.where, e.exprGoType(n))
	}
	a := args[0].Child("expr")
	at := e.m.Recv(e.m.typeOf(a.Type()))
	if at == "Func" || strings.Contains(e.m.demangle(a.Type()), "UnboundedRange") {
		return "swiftrt." + recv + "SubscriptAll(" + e.expr(base) + ")"
	}
	if recv == "Slice" && at == "Int" {
		return e.recvExpr(base) + "[" + e.expr(a) + "]"
	}
	if recv == "Map" {
		return "swiftrt.MapGet(" + e.expr(base) + ", " + e.expr(a) + ")"
	}
	if recv == "OrderedDictionary" {
		return "swiftrt.OrderedDictionaryGet(" + e.expr(base) + ", " + e.expr(a) + ")"
	}
	label := args[0].Str("label")
	return "swiftrt." + recv + "Subscript" + capitalize(label) + at + "(" + e.expr(base) + ", " + e.expr(a) + ")"
}

func (e *fileEmitter) forceUnwrap(sub ast.Node) string {
	t := e.m.typeOf(sub.Type())
	if t.Kind == types.Optional {
		inner := e.m.GoType(t.Args[0], e.generics)
		if e.m.isReferenceGo(t.Args[0], inner) {
			return "swiftrt.ForceRef(" + e.expr(sub) + ")"
		}
	}
	return "(*swiftrt.Force(" + e.expr(sub) + "))"
}

func (e *fileEmitter) optionalChain(n ast.Node) string {
	var binds []ast.Node
	var find func(x ast.Node)
	find = func(x ast.Node) {
		if x == nil {
			return
		}
		if x.Kind() == "bind_optional_expr" {
			find(x.Child("sub_expr"))
			binds = append(binds, x)
			return
		}
		if x.Kind() == "optional_evaluation_expr" {
			return
		}
		for _, v := range x {
			switch c := v.(type) {
			case map[string]any:
				find(ast.Node(c))
			case []any:
				for _, cc := range c {
					if m, ok := cc.(map[string]any); ok {
						find(ast.Node(m))
					}
				}
			}
		}
	}
	find(n.Child("sub_expr"))
	var pre, post strings.Builder
	mutates := false
	ast.Walk(n.Child("sub_expr"), func(x ast.Node) {
		if x.Kind() == "assign_expr" || x.Kind() == "inout_expr" {
			mutates = true
		}
	})
	for _, b := range binds {
		v := e.newTmp("bound")
		sub := b.Child("sub_expr")
		// `dict[k]?.field = v` mutates the dictionary entry in Swift; MapGet hands back a copy, so write it back.
		if ds := stripLoads(sub); mutates && ds.Kind() == "subscript_expr" && e.m.Recv(e.m.typeOf(ds.Child("base").Type())) == "Map" && len(ds.Child("args").List("args")) == 1 && e.assignable(stripLoads(ds.Child("base"))) {
			if vt := e.m.typeOf(sub.Type()); vt != nil && vt.Kind == types.Optional && len(vt.Args) == 1 && !e.m.isReferenceGo(vt.Args[0], e.m.GoType(vt.Args[0], e.generics)) {
				k := e.newTmp("key")
				m := e.lvalue(stripLoads(ds.Child("base")))
				fmt.Fprintf(&pre, "%s := %s\n%s := swiftrt.MapGet(%s, %s)\nif %s == nil { return nil }\n", k, e.expr(ds.Child("args").List("args")[0].Child("expr")), v, m, k, v)
				fmt.Fprintf(&post, "%s[%s] = *%s\n", m, k, v)
				e.opaque[nodeID(b)] = "(*" + v + ")"
				continue
			}
		}
		fmt.Fprintf(&pre, "%s := %s\nif %s == nil { return nil }\n", v, e.expr(sub), v)
		u := e.unwrap(v, sub)
		if strings.HasPrefix(u, "*") {
			u = "(" + u + ")"
		}
		e.opaque[nodeID(b)] = u
	}
	rt := e.exprGoType(n)
	body := e.expr(n.Child("sub_expr"))
	if rt == "" || rt == "*" || rt == "*struct{}" {
		return "func() { " + strings.ReplaceAll(pre.String(), "return nil", "return") + body + "\n" + post.String() + " }()"
	}
	if post.Len() > 0 {
		return "func() " + rt + " {\n" + pre.String() + "res := " + body + "\n" + post.String() + "return res\n}()"
	}
	return "func() " + rt + " {\n" + pre.String() + "return " + body + "\n}()"
}

func (e *fileEmitter) tuple(n ast.Node) string {
	els := n.List("elements")
	if len(els) == 0 {
		return "struct{}{}"
	}
	var parts []string
	for _, el := range els {
		parts = append(parts, e.expr(el))
	}
	return "(" + e.exprGoType(n) + "{" + strings.Join(parts, ", ") + "})"
}

func (e *fileEmitter) closure(n ast.Node) string {
	saveR, saveT, saveI, saveDo := e.resultType, e.throws, e.inInit, e.doRet
	e.doRet = nil
	saveIO := map[string]bool{}
	for k, v := range e.inoutParams {
		saveIO[k] = v
	}
	defer func() { e.resultType, e.throws, e.inInit, e.doRet, e.inoutParams = saveR, saveT, saveI, saveDo, saveIO }()
	ft := e.m.typeOf(n.Type())
	var params []string
	for i, p := range n.Child("params").List("params") {
		name := goSafe(p.Name())
		if name == "_" || name == "" {
			name = fmt.Sprintf("_p%d", i)
		}
		pt := e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics)
		if p.Bool("inout") {
			pt = "*" + pt
			if e.inoutParams == nil {
				e.inoutParams = map[string]bool{}
			}
			e.inoutParams[p.Name()] = true
		}
		params = append(params, name+" "+pt)
	}
	res := ""
	if ft.Kind == types.Func {
		res = e.m.GoType(ft.Result, e.generics)
	}
	e.resultType = res
	e.inInit = false
	saved := e.b
	e.b = strings.Builder{}
	e.w("func(%s) %s {\n", strings.Join(params, ", "), res)
	body := n.Child("body")
	e.stmts(body.List("elements"))
	if res != "" && !endsWithReturn(body) {
		e.w("panic(\"unreachable\")\n")
	}
	e.w("}")
	s := e.b.String()
	e.b = saved
	return s
}

func (e *fileEmitter) captureList(n ast.Node) string {
	saved := e.b
	e.b = strings.Builder{}
	for _, c := range n.List("captures") {
		meta := false
		for _, pe := range c.List("pattern_entries") {
			if t := e.m.typeOf(pe.Child("pattern").Type()); t != nil && t.Kind == types.Meta {
				meta = true
			}
		}
		if meta || isIdentityCapture(c) {
			continue
		}
		e.localBinding(c)
		for _, name := range declaredNames(c) {
			e.w("_ = %s\n", name)
		}
	}
	binds := e.b.String()
	e.b = saved
	body := n.Child("closure_body")
	return "func() " + e.exprGoType(body) + " {\n" + binds + "return " + e.expr(body) + "\n}()"
}

// keyPath renders a key path of stored or computed property components as a getter func.
func (e *fileEmitter) keyPath(n ast.Node) string {
	t := e.m.typeOf(n.Type())
	if len(t.Args) != 2 {
		return e.m.Unsup("keypath-type", e.where, "")
	}
	root := e.m.GoType(t.Args[0], e.generics)
	val := e.m.GoType(t.Args[1], e.generics)
	acc := "root"
	for _, c := range n.List("components") {
		switch c.Kind() {
		case "tuple_element":
			acc = derefParen(acc) + ".F" + c.Str("index")
			continue
		case "identity":
			continue
		case "optional_force":
			acc = "(*" + acc + ")"
			continue
		}
		if c.Kind() != "member" {
			return e.m.Unsup("keypath-component:"+c.Kind(), e.where, "func("+root+") "+val)
		}
		info, pfx := e.lookup(c.Child("decl").Str("decl_usr"))
		if info == nil {
			sig := parseDeclSig(e.m.demangle(c.Child("decl").Str("decl_usr")))
			ownerT := types.Parse(sig.owner)
			acc = "swiftrt." + e.m.Recv(ownerT) + capitalize(sig.base) + "(" + acc + ")"
			continue
		}
		if info.Kind != "var_decl" {
			return e.m.Unsup("keypath-foreign", e.where, "func("+root+") "+val)
		}
		switch {
		case info.OwnerKind == "protocol":
			acc = acc + ".Get" + info.GoName + "()"
		case info.FreeSelf():
			acc = pfx + info.GoName + "(" + acc + ")"
		case info.Computed:
			acc = acc + "." + info.GoName + "()"
		default:
			acc = acc + "." + info.GoName
		}
	}
	get := "func(root " + root + ") " + val + " { return " + acc + " }"
	if !strings.HasPrefix(e.exprGoType(n), "swiftrt.WritableKeyPath[") {
		return get
	}
	set := e.keyPathSetter(n, root, val)
	return "(" + e.exprGoType(n) + "{Get: " + get + ", Set: " + set + "})"
}

// keyPathSetter writes through a writable key path's member chain.
func (e *fileEmitter) keyPathSetter(n ast.Node, root, val string) string {
	acc := "(*root)"
	comps := n.List("components")
	for i, c := range comps {
		if c.Kind() == "tuple_element" {
			acc += ".F" + c.Str("index")
			continue
		}
		if c.Kind() != "member" {
			return e.m.Unsup("keypath-setter:"+c.Kind(), e.where, "func(*"+root+", "+val+")")
		}
		info, pfx := e.lookup(c.Child("decl").Str("decl_usr"))
		if info == nil || info.Kind != "var_decl" || info.FreeSelf() {
			return e.m.Unsup("keypath-setter-member", e.where, "func(*"+root+", "+val+")")
		}
		last := i == len(comps)-1
		switch {
		case last && (info.Settable || info.Observed):
			return "func(root *" + root + ", v " + val + ") { " + acc + ".Set" + info.GoName + "(v) }"
		case info.Computed:
			if last {
				return e.m.Unsup("keypath-setter-readonly", e.where, "func(*"+root+", "+val+")")
			}
			acc += "." + info.GoName + "()"
		default:
			acc += "." + pfx + info.GoName
		}
	}
	return "func(root *" + root + ", v " + val + ") { " + acc + " = v }"
}

func (e *fileEmitter) autoclosureFunc(n ast.Node, ps []ast.Node) string {
	ft := e.m.typeOf(n.Type())
	var params []string
	for _, p := range ps {
		params = append(params, goSafe(p.Name())+" "+e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics))
	}
	res := ""
	if ft.Kind == types.Func {
		res = e.m.GoType(ft.Result, e.generics)
	}
	body := e.expr(n.Child("single_expr_body"))
	if res == "" {
		return "func(" + strings.Join(params, ", ") + ") { " + body + " }"
	}
	return "func(" + strings.Join(params, ", ") + ") " + res + " { return " + body + " }"
}

// recvOf is Module.Recv, resolving generic parameters bound in the current scope.
func (e *fileEmitter) recvOf(t *types.Type) string {
	if t != nil && t.Kind == types.Generic {
		if g := e.generics[t.Name]; strings.HasPrefix(g, "[]") {
			return "Slice"
		}
	}
	return e.m.Recv(t)
}

func (e *fileEmitter) defaultArg(n ast.Node) string {
	owner := n.Child("default_args_owner").Str("decl_usr")
	idx, _ := strconv.Atoi(n.Str("param"))
	info, _ := e.lookup(owner)
	if info == nil {
		return e.m.Unsup("foreign-default-arg", e.where, e.exprGoType(n))
	}
	params := info.Node.Child("params").List("params")
	if idx < len(params) {
		if ex := params[idx].Child("expression"); ex != nil {
			return e.expr(ex)
		}
		if info.Kind == "constructor_decl" && info.Node.Implicit() {
			if init := e.m.varInit[info.Owner+"|"+params[idx].Name()]; init != nil {
				return e.expr(init)
			}
		}
	}
	return e.m.Unsup("default-arg-missing", e.where, e.exprGoType(n))
}

func (e *fileEmitter) tryExpr(n ast.Node) string {
	sub := n.Child("sub_expr")
	if n.Kind() == "optional_try_expr" {
		inner := e.exprGoType(sub)
		if inner == "" || inner == "struct{}" || inner == "*" || inner == "*struct{}" {
			return "swiftrt.TryVoid(func() { " + e.expr(sub) + " })"
		}
		if t := e.m.typeOf(sub.Type()); t != nil && t.Kind == types.Optional {
			return "swiftrt.TryFlat(func() " + inner + " { return " + e.expr(sub) + " })"
		}
		return "swiftrt.TryOpt(func() " + inner + " { return " + e.expr(sub) + " })"
	}
	return e.expr(sub)
}

func (e *fileEmitter) stmtExpr(n ast.Node) string {
	return e.m.Unsup("expr:"+n.Kind(), e.where, e.exprGoType(n))
}

func (e *fileEmitter) macro(n ast.Node) string {
	name := n.Str("name")
	as := e.args(n.Child("args"))
	switch name {
	case "dice":
		// #dice(count, size, bonus) expands to Dice(count:size:bonus:)!, validated at compile time.
		t := e.m.typeOf(n.Type())
		if ctor, pfx := e.ctorWithLabels(t.Name, "count", "size", "bonus"); ctor != nil && len(as) == 3 {
			ss, _ := e.argStrings(as, false)
			return "(*swiftrt.Force(" + pfx + ctor.GoName + "(" + strings.Join(ss, ", ") + ")))"
		}
	case "expect", "require":
		if len(as) == 0 {
			break
		}
		fatal := name == "require"
		cond := as[0].node
		msg := `""`
		if len(as) > 1 && as[1].label == "" && as[1].node.Kind() != "default_argument_expr" {
			msg = "swiftrt.Str(" + e.expr(as[1].node) + ")"
		}
		ct := e.m.typeOf(cond.Type())
		if ct.Kind == types.Optional {
			return "swiftrt.RequireSome(" + e.expr(cond) + ", " + msg + ")"
		}
		if cond.Kind() == "binary_expr" {
			op := e.opDecl(cond).Str("base_name")
			bas := e.args(cond.Child("args"))
			if (op == "==" || op == "!=") && len(bas) == 2 && bas[0].node.Kind() != "nil_literal_expr" && bas[1].node.Kind() != "nil_literal_expr" {
				fn := "swiftrt.ExpectEqual"
				if op == "!=" {
					fn = "swiftrt.ExpectNotEqual"
				}
				return fmt.Sprintf("%s(%s, %s, %t, %s)", fn, e.expr(bas[0].node), e.expr(bas[1].node), fatal, msg)
			}
		}
		return fmt.Sprintf("swiftrt.Expect(%s, %q, %t, %s)", e.expr(cond), "", fatal, msg)
	}
	return e.m.Unsup("macro:"+name, e.where, e.exprGoType(n))
}

// ---- calls ----

type arg struct {
	label string
	node  ast.Node
}

func (e *fileEmitter) args(list ast.Node) []arg {
	var out []arg
	for _, a := range list.List("args") {
		out = append(out, arg{label: a.Str("label"), node: a.Child("expr")})
	}
	return out
}

func (e *fileEmitter) argStrings(as []arg, foreign bool) ([]string, []arg) {
	var ss []string
	var kept []arg
	callee := e.callee
	e.callee = nil
	unit := e.unitCallee
	e.unitCallee = false
	var params []ast.Node
	if callee != nil && callee.Node != nil {
		params = callee.Node.Child("params").List("params")
	}
	for i, a := range as {
		if foreign && a.node.Kind() == "default_argument_expr" {
			continue
		}
		s := e.expr(a.node)
		if i < len(params) && params[i].Bool("autoclosure") && !foreign {
			if ac := stripConv(a.node); ac != nil && ac.Kind() == "autoclosure_expr" && len(ac.Child("params").List("params")) == 0 {
				if ft := e.m.typeOf(ac.Type()); ft != nil && ft.Kind == types.Func {
					if res := e.m.GoType(ft.Result, e.generics); res != "" && !isVoidGo(res) {
						s = "func() " + res + " { return " + s + " }"
					} else {
						s = "func() { " + s + " }"
					}
				}
			}
		}
		if ft := e.m.typeOf(a.node.Type()); foreign && ft != nil && ft.Kind == types.Func && len(ft.Args) == 1 && ft.Args[0].Kind == types.Tuple && len(ft.Args[0].Args) > 1 {
			if cl := stripConv(a.node); cl != nil && cl.Kind() == "closure_expr" && len(cl.Child("params").List("params")) == len(ft.Args[0].Args) {
				suffix := ""
				if ft.Result == nil || e.m.GoType(ft.Result, e.generics) == "" {
					suffix = "V"
				}
				s = "swiftrt.Tupled" + itoa(len(ft.Args[0].Args)) + suffix + "(" + s + ")"
			}
		}
		if ft := e.m.typeOf(a.node.Type()); ft != nil && ft.Kind == types.Func && (ft.Result == nil || e.m.GoType(ft.Result, e.generics) == "") {
			want := ""
			if i < len(params) {
				want = e.m.GoType(e.m.typeOf(params[i].Str("interface_type")), e.generics)
			}
			switch {
			case strings.HasSuffix(want, ") any"):
				s = "swiftrt.AnyFn" + itoa(len(ft.Args)) + "(" + s + ")"
			case foreign && unit:
				s = "swiftrt.UnitFn" + itoa(len(ft.Args)) + "(" + s + ")"
			}
		}
		ss = append(ss, s)
		kept = append(kept, a)
	}
	return ss, kept
}

func (e *fileEmitter) call(n ast.Node) string {
	fn := stripConv(n.Child("fn"))
	as := e.args(n.Child("args"))
	switch fn.Kind() {
	case "dot_syntax_call_expr":
		d := stripConv(fn.Child("fn")).Child("decl")
		selfArgs := fn.Child("args").List("args")
		var base ast.Node
		if len(selfArgs) > 0 {
			base = selfArgs[0].Child("expr")
		}
		return e.methodCall(n, d, base, as)
	case "constructor_ref_call_expr":
		d := stripConv(fn.Child("fn")).Child("decl")
		return e.ctorCall(n, d, as)
	case "declref_expr":
		d := fn.Child("decl")
		usr := d.Str("decl_usr")
		if isOperatorName(d.Str("base_name")) && len(as) == 2 {
			return e.binaryWith(n, d, as)
		}
		info, pfx := e.lookup(usr)
		if info != nil && info.Kind == "enum_element_decl" {
			return e.payloadCtor(n, info, pfx, as)
		}
		if info != nil || usr == "" {
			e.callee = info
			ss, _ := e.argStrings(as, false)
			return e.declRef(d, fn) + "(" + strings.Join(ss, ", ") + ")"
		}
		dem := e.m.demangle(usr)
		if strings.Contains(dem, "(in ") {
			ss, _ := e.argStrings(as, false)
			return goSafe(d.Str("base_name")) + "(" + strings.Join(ss, ", ") + ")"
		}
		sig := parseDeclSig(dem)
		ss, kept := e.argStrings(as, true)
		if (sig.base == "max" || sig.base == "min") && len(kept) >= 2 {
			if less := e.lessFunc(e.m.typeOf(kept[0].node.Type())); less != "" {
				return "swiftrt." + capitalize(sig.base) + "Less(" + less + ", " + strings.Join(ss, ", ") + ")"
			}
			if len(kept) > 2 {
				elem := e.m.GoType(e.m.typeOf(kept[0].node.Type()), e.generics)
				last := len(ss) - 1
				return "swiftrt." + capitalize(sig.base) + "Of([]" + elem + "{" + strings.Join(ss[:last], ", ") + "}, " + ss[last] + ")"
			}
		}
		return "swiftrt." + capitalize(sig.base) + e.suffix(sig, kept, true) + "(" + strings.Join(ss, ", ") + ")"
	}
	ss, _ := e.argStrings(as, false)
	f := e.expr(fn)
	if fn.Kind() == "closure_expr" {
		f = "(" + f + ")"
	}
	return f + "(" + strings.Join(ss, ", ") + ")"
}

func (e *fileEmitter) payloadCtor(n ast.Node, info *DeclInfo, pfx string, as []arg) string {
	ss, _ := e.argStrings(as, false)
	targs := e.enumTypeArgs(info, e.m.typeOf(n.Type()))
	return e.asEnum(info, pfx, pfx+info.GoName+targs+"{"+strings.Join(ss, ", ")+"}", targs)
}

// asEnum converts a payload enum case literal to its enum's interface type, so generic calls infer the
// enum rather than the case struct.
func (e *fileEmitter) asEnum(info *DeclInfo, pfx, lit, targs string) string {
	if owner := e.enumGoType(info); owner != "" {
		return pfx + owner + targs + "(" + lit + ")"
	}
	return "(" + lit + ")"
}

// enumTypeArgs is "[X, Y]" for a case of a generic payload enum instantiated as t, or "".
func (e *fileEmitter) enumTypeArgs(info *DeclInfo, t *types.Type) string {
	if info == nil || len(e.m.TypeParams(info.Owner)) == 0 || t == nil {
		return ""
	}
	for t.Kind == types.Func && t.Result != nil {
		t = t.Result
	}
	for t.Kind == types.Optional && len(t.Args) == 1 {
		t = t.Args[0]
	}
	gt := e.m.GoType(t, e.generics)
	if i := strings.Index(gt, "["); i >= 0 && strings.HasSuffix(gt, "]") && !strings.HasPrefix(gt, "[") {
		return gt[i:]
	}
	return ""
}

func (e *fileEmitter) enumGoType(info *DeclInfo) string {
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		if mod.typeKinds[info.Owner] == "enum_decl" {
			return mod.LocalTypeName(info.Owner)
		}
	}
	return ""
}

func (e *fileEmitter) methodCall(n ast.Node, d ast.Node, base ast.Node, as []arg) string {
	usr := d.Str("decl_usr")
	if strings.HasPrefix(usr, "s:s6ResultO7success") || strings.HasPrefix(usr, "s:s6ResultO7failure") {
		ss, _ := e.argStrings(as, false)
		v := "struct{}{}"
		if len(ss) == 1 {
			v = ss[0]
		}
		if strings.HasPrefix(usr, "s:s6ResultO7success") {
			return "(" + e.exprGoType(n) + "{Value: " + v + ", Ok: true})"
		}
		return "(" + e.exprGoType(n) + "{Err: " + v + "})"
	}
	if d.Str("base_name") == "init" {
		return e.ctorCall(n, d, as)
	}
	if isOperatorName(d.Str("base_name")) && len(as) == 2 {
		return e.binaryWith(n, d, as)
	}
	info, pfx := e.lookup(usr)
	if info != nil {
		e.callee = info
		ss, _ := e.argStrings(as, false)
		switch {
		case info.Kind == "enum_element_decl":
			return e.payloadCtor(n, info, pfx, as)
		case info.Static && info.OwnerKind == "protocol":
			call := e.metaReceiver(base) + "." + staticReqName(info) + "(" + strings.Join(ss, ", ") + ")"
			declared := e.m.GoType(e.m.typeOf(info.Node.Str("result")), nil)
			if gt := e.exprGoType(n); gt != "" && !strings.HasPrefix(gt, "any") && strings.HasPrefix(declared, "any") {
				return "swiftrt.MustAs[" + gt + "](" + call + ")"
			}
			if gt := e.exprGoType(n); strings.HasPrefix(gt, "[]") && gt != "[]any" && declared == "[]any" {
				return "swiftrt.FromAnySlice[" + gt[2:] + "](" + call + ")"
			}
			return call
		case info.Static && info.ProtoExt:
			return pfx + info.GoName + "(" + strings.Join(append([]string{e.metaReceiver(base)}, ss...), ", ") + ")"
		case info.Static || info.Owner == "":
			return pfx + info.GoName + "(" + strings.Join(ss, ", ") + ")"
		case info.FreeSelf():
			recv := e.recvArg(base)
			if info.ProtoExt {
				inner := base
				if inner.Kind() == "inout_expr" {
					inner = inner.Child("sub_expr")
				}
				recv = e.expr(inner)
				if t := e.m.typeOf(inner.Type()); t != nil && t.Kind == types.Named && e.m.HasMutatingMethods(t.Name) {
					if base.Kind() == "inout_expr" {
						recv = "&" + e.lvalue(inner)
					} else {
						recv = "swiftrt.Ref(" + recv + ")"
					}
				}
			}
			return pfx + info.GoName + "(" + strings.Join(append([]string{recv}, ss...), ", ") + ")"
		default:
			return e.recvExpr(base) + "." + info.GoName + "(" + strings.Join(ss, ", ") + ")"
		}
	}
	if base != nil {
		switch d.Str("base_name") {
		case "firstIndex", "lastIndex", "index", "startIndex", "endIndex":
			if lo, ok := e.sliceLowerBound(base); ok {
				saved := lo
				out := e.methodCallForeign(n, d, base, as)
				if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional {
					return "swiftrt.OffsetIndex(" + out + ", " + saved + ")"
				}
				return "(" + out + " + " + saved + ")"
			}
		}
	}
	return e.methodCallForeign(n, d, base, as)
}

// sliceLowerBound returns the lower bound of an ArraySlice made by subscripting an array with a range,
// since Swift slices keep their parent's indices while Go slices start at zero.
func (e *fileEmitter) sliceLowerBound(base ast.Node) (string, bool) {
	for base.Kind() == "load_expr" || base.Kind() == "paren_expr" || base.Kind() == "inout_expr" {
		base = base.Child("sub_expr")
	}
	if base.Kind() != "subscript_expr" {
		return "", false
	}
	if t := e.m.typeOf(base.Type()); t == nil || t.Kind != types.Named || t.Name != "Swift.ArraySlice" {
		return "", false
	}
	args := base.Child("args").List("args")
	if len(args) != 1 {
		return "", false
	}
	a := args[0].Child("expr")
	switch e.m.Recv(e.m.typeOf(a.Type())) {
	case "Range", "ClosedRange", "PartialRangeFrom":
		return "(" + e.expr(a) + ").Lo", true
	case "PartialRangeUpTo", "PartialRangeThrough":
		return "0", true
	}
	return "", false
}

func (e *fileEmitter) methodCallForeign(n ast.Node, d ast.Node, base ast.Node, as []arg) string {
	usr := d.Str("decl_usr")
	dem := e.m.demangle(usr)
	sig := parseDeclSig(dem)
	if b := d.Str("base_name"); strings.HasPrefix(b, "withLock") || strings.HasPrefix(b, "withValue") || b == "sync" || b == "withCriticalRegion" {
		e.unitCallee = true
	}
	if d.Str("base_name") == "joined" && len(as) == 0 && base != nil {
		if bt := e.m.typeOf(base.Type()); bt != nil && (bt.Kind == types.Array || e.m.Recv(bt) == "Slice") && len(bt.Args) == 1 {
			switch e.m.Recv(bt.Args[0]) {
			case "Slice":
				return "swiftrt.SliceFlatten(" + e.expr(base) + ")"
			case "Map":
				return "swiftrt.SliceFlattenMaps(" + e.expr(base) + ")"
			}
		}
	}
	refSuffix := ""
	if bt := e.m.typeOf(base.Type()); base != nil && len(as) == 0 && (d.Str("base_name") == "max" || d.Str("base_name") == "min") && bt != nil && len(bt.Args) == 1 && (e.m.Recv(bt) == "Slice" || e.m.Recv(bt) == "Set") {
		if less := e.lessFunc(bt.Args[0]); less != "" {
			return "swiftrt.Slice" + capitalize(d.Str("base_name")) + "By(swiftrt.Iterate[" + e.m.GoType(bt.Args[0], e.generics) + "](" + e.expr(base) + "), " + less + ")"
		}
	}
	if base != nil && e.m.Recv(e.m.typeOf(base.Type())) == "Set" && len(as) == 1 && as[0].label == "" && e.m.Recv(e.m.typeOf(as[0].node.Type())) == "Set" {
		switch d.Str("base_name") {
		case "union", "formUnion", "intersection", "subtracting", "subtract", "isSubset", "isSuperset", "isDisjoint", "symmetricDifference", "formIntersection":
			refSuffix = "Set"
		}
	}
	if d.Str("base_name") == "compactMap" && len(as) == 1 {
		if ft := e.m.typeOf(as[0].node.Type()); ft != nil && ft.Kind == types.Func && ft.Result != nil && ft.Result.Kind == types.Optional && len(ft.Result.Args) == 1 {
			if e.m.isReferenceGo(ft.Result.Args[0], e.m.GoType(ft.Result.Args[0], e.generics)) {
				refSuffix = "Ref"
			}
		}
	}
	ss, kept := e.argStrings(as, true)
	if base == nil || base.Kind() == "type_expr" || e.m.typeOf(base.Type()).Kind == types.Meta {
		owner := sig.owner
		if base != nil {
			bt := e.m.typeOf(base.Type())
			if bt.Kind == types.Meta {
				owner = e.m.Recv(bt.Args[0])
			}
		}
		return "swiftrt." + lastComponent(owner) + capitalize(sig.base) + e.suffix(sig, kept, false) + "(" + strings.Join(ss, ", ") + ")"
	}
	recv := e.recvOf(e.m.typeOf(base.Type()))
	if s, ok := e.nativeMethod(recv, sig.base, base, kept, ss); ok {
		return s
	}
	return "swiftrt." + recv + capitalize(sig.base) + e.suffix(sig, kept, false) + refSuffix + "(" + strings.Join(append([]string{e.recvArg(base)}, ss...), ", ") + ")"
}

func lastComponent(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// nativeMethod renders a few stdlib calls as plain Go.
func (e *fileEmitter) nativeMethod(recv, base string, self ast.Node, kept []arg, ss []string) (string, bool) {
	switch {
	case recv == "Slice" && base == "append" && len(kept) == 1 && kept[0].label == "" && self.Kind() == "inout_expr":
		lv := e.lvalue(self.Child("sub_expr"))
		return lv + " = append(" + lv + ", " + ss[0] + ")", true
	case recv == "Slice" && base == "append" && len(kept) == 1 && kept[0].label == "contentsOf" && self.Kind() == "inout_expr" && e.m.Recv(e.m.typeOf(kept[0].node.Type())) == "Slice":
		lv := e.lvalue(self.Child("sub_expr"))
		return lv + " = append(" + lv + ", " + ss[0] + "...)", true
	case recv == "Slice" && base == "append" && len(kept) == 1 && kept[0].label == "contentsOf" && self.Kind() == "inout_expr":
		lv := e.lvalue(self.Child("sub_expr"))
		elem := "any"
		if t := e.m.typeOf(self.Child("sub_expr").Type()); t != nil && len(t.Args) == 1 {
			elem = e.m.GoType(t.Args[0], e.generics)
		}
		return lv + " = append(" + lv + ", swiftrt.Iterate[" + elem + "](" + ss[0] + ")...)", true
	case recv == "String" && base == "append" && len(kept) == 1 && self.Kind() == "inout_expr" && kept[0].label == "":
		lv := e.lvalue(self.Child("sub_expr"))
		return lv + " += " + ss[0], true
	}
	return "", false
}

func (e *fileEmitter) ctorCall(n ast.Node, d ast.Node, as []arg) string {
	usr := d.Str("decl_usr")
	info, pfx := e.lookup(usr)
	if info != nil {
		e.callee = info
		ss, _ := e.argStrings(as, false)
		targs := ""
		if len(e.m.TypeParams(info.Owner)) > 0 {
			gt := strings.TrimPrefix(e.exprGoType(n), "*")
			if i := strings.Index(gt, "["); i >= 0 && strings.HasSuffix(gt, "]") {
				targs = gt[i:]
			}
		}
		return pfx + info.GoName + targs + "(" + strings.Join(ss, ", ") + ")"
	}
	sig := parseDeclSig(e.m.demangle(usr))
	ss, kept := e.argStrings(as, true)
	recv := e.m.Recv(e.m.typeOf(n.Type()))
	suffix := ""
	for _, a := range kept {
		if a.label != "" {
			suffix += capitalize(a.label)
		} else {
			suffix += e.m.Recv(e.m.typeOf(a.node.Type()))
		}
	}
	_ = sig
	gt := e.exprGoType(n)
	if recv == "Opt" {
		if t := e.m.typeOf(n.Type()); t != nil && len(t.Args) == 1 {
			recv = e.m.Recv(t.Args[0]) + "Opt"
			gt = e.m.GoType(t.Args[0], e.generics)
		}
	}
	if recv == "Heap" && len(ss) == 0 {
		if t := e.m.typeOf(n.Type()); t != nil && len(t.Args) == 1 {
			if less := e.lessFunc(t.Args[0]); less != "" {
				return "swiftrt.HeapInitLess(" + less + ")"
			}
		}
	}
	if recv == "TaskLocal" {
		if t := e.m.typeOf(n.Type()); t != nil && len(t.Args) == 1 {
			return "swiftrt.TaskLocalInit" + suffix + "[" + e.m.GoType(t.Args[0], e.generics) + "](" + strings.Join(ss, ", ") + ")"
		}
	}
	if recv == "Slice" || recv == "Map" || recv == "Range" || recv == "ClosedRange" || recv == "Int" || recv == "Float" || recv == "IntOpt" || recv == "FloatOpt" || recv == "Set" || recv == "OptionSet" || recv == "Heap" {
		return "swiftrt." + recv + "Init" + suffix + "[" + gt + "](" + strings.Join(ss, ", ") + ")"
	}
	return "swiftrt." + recv + "Init" + suffix + "(" + strings.Join(ss, ", ") + ")"
}

// suffix disambiguates stdlib overloads: labels, plus the argument kind where the parameter is concrete.
func (e *fileEmitter) suffix(sig declSig, kept []arg, global bool) string {
	var b strings.Builder
	for i, a := range kept {
		if a.label != "" {
			b.WriteString(capitalize(a.label))
			continue
		}
		pt := ""
		for j, p := range sig.params {
			if p.label == "" && j >= i {
				pt = p.typ
				break
			}
		}
		if pt != "" && strings.HasPrefix(pt, "Swift.") {
			b.WriteString(e.m.Recv(e.m.typeOf(a.node.Type())))
		}
	}
	return b.String()
}

func (e *fileEmitter) unappliedDot(n ast.Node) string {
	if fn := stripConv(n.Child("fn")); fn.Kind() == "autoclosure_expr" {
		// Swift 6 curry thunk: an autoclosure taking self that returns the method as a function.
		if args := n.Child("args").List("args"); len(args) == 1 {
			return "(" + e.expr(fn) + ")(" + e.expr(args[0].Child("expr")) + ")"
		}
	}
	d := stripConv(n.Child("fn")).Child("decl")
	info, pfx := e.lookup(d.Str("decl_usr"))
	if info != nil && info.Kind == "enum_element_decl" {
		if e.isPayloadCase(info) {
			return e.m.Unsup("payload-case-as-function", e.where, e.exprGoType(n))
		}
		if e.m.IsPayloadEnum(info.Owner) {
			targs := e.enumTypeArgs(info, e.m.typeOf(n.Type()))
			return e.asEnum(info, pfx, pfx+info.GoName+targs+"{}", targs)
		}
		return pfx + info.GoName
	}
	args := n.Child("args").List("args")
	var base ast.Node
	if len(args) > 0 && args[0].Child("expr").Kind() != "type_expr" {
		base = args[0].Child("expr")
	}
	if info != nil && info.Kind == "constructor_decl" {
		return pfx + info.GoName
	}
	if info != nil && info.Kind == "func_decl" {
		if base == nil || info.Static {
			return pfx + info.GoName
		}
		if !info.FreeSelf() {
			return e.recvExpr(base) + "." + info.GoName
		}
		return e.boundMethod(n, base, func(recv string, ps []string) string {
			return pfx + info.GoName + "(" + strings.Join(append([]string{recv}, ps...), ", ") + ")"
		})
	}
	if info == nil {
		usr := d.Str("decl_usr")
		sig := parseDeclSig(e.m.demangle(usr))
		t := e.m.typeOf(n.Type())
		if t != nil && t.Kind != types.Func {
			name := capitalize(d.Str("base_name"))
			if gt := e.m.GoType(t, e.generics); strings.HasPrefix(gt, "swiftrt.") {
				return gt + name
			}
			return "swiftrt." + e.m.Recv(t) + name
		}
		method := d.Str("base_name")
		if method == "" {
			method = sig.base
		}
		if base != nil && method != "" {
			recv := e.m.Recv(e.m.typeOf(base.Type()))
			return e.boundMethod(n, base, func(r string, ps []string) string {
				return "swiftrt." + recv + capitalize(method) + "(" + strings.Join(append([]string{r}, ps...), ", ") + ")"
			})
		}
	}
	return e.m.Unsup("unapplied-method", e.where, e.exprGoType(n))
}

// boundMethod turns `base.method` into a closure that captures base once, as Swift's partial
// application does.
func (e *fileEmitter) boundMethod(n, base ast.Node, call func(recv string, params []string) string) string {
	ft := e.m.typeOf(n.Type())
	if ft == nil || ft.Kind != types.Func {
		return e.m.Unsup("unapplied-method", e.where, e.exprGoType(n))
	}
	var decls, names []string
	for i, a := range ft.Args {
		name := "p" + itoa(i)
		decls = append(decls, name+" "+e.m.GoType(a, e.generics))
		names = append(names, name)
	}
	res := e.m.GoType(ft.Result, e.generics)
	fnT := e.exprGoType(n)
	body := call("r", names)
	if res != "" {
		body = "return " + body
	}
	return "func() " + fnT + " { r := " + e.expr(base) + "; return func(" + strings.Join(decls, ", ") + ") " + res + " { " + body + " } }()"
}

// ---- operators ----

func (e *fileEmitter) opDecl(n ast.Node) ast.Node {
	fn := stripConv(n.Child("fn"))
	if fn.Kind() == "dot_syntax_call_expr" {
		return stripConv(fn.Child("fn")).Child("decl")
	}
	return fn.Child("decl")
}

func (e *fileEmitter) binary(n ast.Node) string {
	return e.binaryWith(n, e.opDecl(n), e.args(n.Child("args")))
}

func (e *fileEmitter) binaryWith(n ast.Node, d ast.Node, as []arg) string {
	op := d.Str("base_name")
	if len(as) != 2 {
		return e.m.Unsup("binary-arity", e.where, e.exprGoType(n))
	}
	l, r := as[0].node, as[1].node
	if info, pfx := e.lookup(d.Str("decl_usr")); info != nil {
		return pfx + info.GoName + "(" + e.expr(l) + ", " + e.expr(r) + ")"
	}
	lt := e.m.typeOf(l.Type())
	switch op {
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		s, _ := e.compoundAssign(n)
		return "func() struct{} { " + s + "; return struct{}{} }()"
	case "&&", "||":
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "..<":
		return "(swiftrt.Range[" + e.m.GoType(lt, e.generics) + "]{Lo: " + e.expr(l) + ", Hi: " + e.expr(r) + "})"
	case "...":
		return "(swiftrt.ClosedRange[" + e.m.GoType(lt, e.generics) + "]{Lo: " + e.expr(l) + ", Hi: " + e.expr(r) + "})"
	case "??":
		if rt := e.m.typeOf(n.Type()); rt != nil && rt.Kind == types.Optional {
			return "swiftrt.CoalesceOpt(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if lt.Kind == types.Optional && len(lt.Args) == 1 {
			inner := e.m.GoType(lt.Args[0], e.generics)
			if e.m.isReferenceGo(lt.Args[0], inner) {
				return "swiftrt.CoalesceRef(" + e.expr(l) + ", " + e.expr(r) + ")"
			}
		}
		return "swiftrt.Coalesce(" + e.expr(l) + ", " + e.expr(r) + ")"
	case "~=":
		return e.expr(l) + " == " + e.expr(r)
	case "==", "!=":
		if rl, rr := e.m.Recv(lt), e.m.Recv(e.m.typeOf(r.Type())); (rl == "String" || rl == "Substring") && (rr == "String" || rr == "Substring") && rl != rr {
			return e.stringish(l) + " " + op + " " + e.stringish(r)
		}
		if r.Kind() == "nil_literal_expr" || l.Kind() == "nil_literal_expr" {
			return e.expr(l) + " " + op + " " + e.expr(r)
		}
		if rt := e.m.typeOf(r.Type()); lt.Kind == types.Named && rt != nil && rt.Kind == types.Named && isIntegerName(lt.Name) && isIntegerName(rt.Name) && lt.Name != rt.Name {
			return "swiftrt.IntCompare(" + e.expr(l) + ", " + e.expr(r) + ") " + op + " 0"
		}
		if lt.Kind == types.Optional || !e.m.isGoComparable(lt) {
			fn := "swiftrt.Equal"
			if op == "!=" {
				return "!" + fn + "(" + e.expr(l) + ", " + e.expr(r) + ")"
			}
			return fn + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "<", "<=", ">", ">=":
		if rt := e.m.typeOf(r.Type()); lt.Kind == types.Named && rt != nil && rt.Kind == types.Named && isIntegerName(lt.Name) && isIntegerName(rt.Name) && lt.Name != rt.Name {
			return "swiftrt.IntCompare(" + e.expr(l) + ", " + e.expr(r) + ") " + op + " 0"
		}
		if less := e.lessFunc(lt); less != "" {
			a, b := e.expr(l), e.expr(r)
			switch op {
			case "<":
				return less + "(" + a + ", " + b + ")"
			case ">":
				return less + "(" + b + ", " + a + ")"
			case "<=":
				return "!" + less + "(" + b + ", " + a + ")"
			}
			return "!" + less + "(" + a + ", " + b + ")"
		}
		if lt.Kind == types.Tuple {
			var cmps []string
			for _, el := range lt.Args {
				gt := e.m.GoType(el, e.generics)
				if less := e.lessFunc(el); less != "" {
					cmps = append(cmps, "swiftrt.CompareBy("+less+")")
				} else {
					cmps = append(cmps, "swiftrt.CompareOrdered["+gt+"]")
				}
			}
			return "swiftrt.TupleCompare(" + e.expr(l) + ", " + e.expr(r) + ", " + strings.Join(cmps, ", ") + ") " + op + " 0"
		}
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "+", "-", "*":
		if op == "-" && e.m.GoType(lt, e.generics) == "swiftrt.ContinuousClockInstant" {
			return "swiftrt.InstantSub(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if op == "+" && (e.m.Recv(lt) == "String" || e.m.Recv(lt) == "Substring") {
			return e.stringish(l) + " + " + e.stringish(r)
		}
		if lt.Kind == types.Named && isIntegerName(lt.Name) {
			fn := map[string]string{"+": "Add", "-": "Sub", "*": "Mul"}[op]
			return "swiftrt." + fn + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if op == "+" && (lt.Kind == types.Array || e.m.Recv(lt) == "Slice") {
			return "swiftrt.Concat(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "===", "!==":
		same := "swiftrt.Same(" + e.expr(l) + ", " + e.expr(r) + ")"
		if op == "!==" {
			return "!" + same
		}
		return same
	case "/", "%":
		if lt.Kind == types.Named && isIntegerName(lt.Name) {
			fn := map[string]string{"/": "Div", "%": "Rem"}[op]
			return "swiftrt." + fn + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "&", "|", "^", "<<", ">>":
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "&+", "&-", "&*":
		return e.expr(l) + " " + op[1:] + " " + e.expr(r)
	}
	return e.m.Unsup("operator:"+op, e.where, e.exprGoType(n))
}

func (e *fileEmitter) compoundAssign(n ast.Node) (string, bool) {
	if n.Kind() != "binary_expr" {
		return "", false
	}
	d := e.opDecl(n)
	op := d.Str("base_name")
	if !strings.HasSuffix(op, "=") || op == "==" || op == "!=" || op == "<=" || op == ">=" {
		return "", false
	}
	as := e.args(n.Child("args"))
	if len(as) != 2 {
		return "", false
	}
	l := as[0].node
	if l.Kind() == "inout_expr" {
		l = l.Child("sub_expr")
	}
	rs := e.expr(as[1].node)
	lt := e.m.typeOf(l.Type())
	base := op[:len(op)-1]
	if e.m.Recv(lt) == "String" && e.m.Recv(e.m.typeOf(as[1].node.Type())) == "Substring" {
		rs += ".String()"
	}
	combine := func(cur string) string {
		if len(base) == 2 && base[0] == '&' && strings.ContainsAny(base[1:], "+-*") {
			return cur + " " + base[1:] + " " + rs
		}
		if lt.Kind == types.Named && isIntegerName(lt.Name) {
			if fn, ok := map[string]string{"+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Rem"}[base]; ok {
				return "swiftrt." + fn + "(" + cur + ", " + rs + ")"
			}
		}
		if base == "+" && e.m.Recv(lt) == "Slice" {
			return "append(" + cur + ", " + rs + "...)"
		}
		if base == "+" && e.m.Recv(lt) == "String" {
			return cur + " + " + rs
		}
		return cur + " " + base + " " + rs
	}
	if info, _ := e.lookup(d.Str("decl_usr")); info == nil {
		if call, ok := e.setterCall(l, combine(e.expr(l))); ok {
			return call, true
		}
	}
	if info, pfx := e.lookup(d.Str("decl_usr")); info != nil {
		if as[0].node.Kind() == "inout_expr" {
			return pfx + info.GoName + "(" + e.expr(as[0].node) + ", " + rs + ")", true
		}
		return pfx + info.GoName + "(&" + e.lvalue(l) + ", " + rs + ")", true
	}
	lv := e.lvalue(l)
	if lt.Kind == types.Named && isIntegerName(lt.Name) || base == "+" && e.m.Recv(lt) == "Slice" || len(base) == 2 && base[0] == '&' {
		return lv + " = " + combine(lv), true
	}
	return lv + " " + op + " " + rs, true
}

func (e *fileEmitter) prefix(n ast.Node) string {
	d := e.opDecl(n)
	op := d.Str("base_name")
	as := e.args(n.Child("args"))
	if len(as) != 1 {
		return e.m.Unsup("prefix-arity", e.where, e.exprGoType(n))
	}
	x := e.expr(as[0].node)
	switch op {
	case "!":
		return "!" + x
	case "-":
		return "-" + x
	case "~":
		return "^" + x
	case "..<":
		return "(swiftrt.PartialRangeUpTo[" + e.m.GoType(e.m.typeOf(as[0].node.Type()), e.generics) + "]{Hi: " + x + "})"
	case "...":
		return "(swiftrt.PartialRangeThrough[" + e.m.GoType(e.m.typeOf(as[0].node.Type()), e.generics) + "]{Hi: " + x + "})"
	}
	return e.m.Unsup("prefix:"+op, e.where, e.exprGoType(n))
}

func (e *fileEmitter) postfix(n ast.Node) string {
	d := e.opDecl(n)
	op := d.Str("base_name")
	as := e.args(n.Child("args"))
	if len(as) == 1 && op == "..." {
		return "(swiftrt.PartialRangeFrom[" + e.m.GoType(e.m.typeOf(as[0].node.Type()), e.generics) + "]{Lo: " + e.expr(as[0].node) + "})"
	}
	return e.m.Unsup("postfix:"+op, e.where, e.exprGoType(n))
}

// ---- demangled declaration signatures ----

type sigParam struct{ label, typ string }

type declSig struct {
	static bool
	owner  string // qualified owner ("Swift.String"), or module for globals
	base   string
	params []sigParam
}

func stripGenerics(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '<' && !(i > 0 && (s[i-1] == '.' || s[i-1] == ' ' || s[i-1] == '-')) || c == '<' && i+1 < len(s) && s[i+1] == ' ':
			depth++
			continue
		case c == '>' && depth > 0 && !(i > 0 && s[i-1] == '-'):
			depth--
			continue
		}
		if depth == 0 {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func parseDeclSig(dem string) declSig {
	var sig declSig
	s := dem
	if strings.HasPrefix(s, "static ") {
		sig.static = true
		s = s[len("static "):]
	}
	if strings.HasPrefix(s, "(extension in ") {
		if i := strings.Index(s, "):"); i >= 0 {
			s = s[i+2:]
		}
	}
	s = stripGenerics(s)
	head := s
	rest := ""
	if i := strings.IndexAny(s, "( "); i >= 0 {
		head, rest = s[:i], s[i:]
	}
	if strings.HasPrefix(rest, " infix") || strings.HasPrefix(rest, " prefix") || strings.HasPrefix(rest, " postfix") {
		rest = strings.TrimLeft(rest[1:], "abcdefghijklmnopqrstuvwxyz")
	}
	// operators: "Swift.Int.+" — split at the last '.' that precedes a non-identifier run
	if i := strings.LastIndex(head, "."); i >= 0 && i < len(head)-1 {
		sig.owner, sig.base = head[:i], head[i+1:]
		for j := 0; j < len(head); j++ {
			if head[j] == '.' && j+1 < len(head) && isOpChar(head[j+1]) {
				sig.owner, sig.base = head[:j], head[j+1:]
				break
			}
		}
	} else {
		sig.base = head
	}
	if strings.HasPrefix(rest, "(") {
		inner := matchParen(rest)
		for _, p := range splitTop(inner) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			label, typ := "", p
			if k := strings.Index(p, ": "); k > 0 && !strings.ContainsAny(p[:k], "(<[ ") {
				label, typ = p[:k], p[k+2:]
			}
			typ = strings.TrimPrefix(typ, "__owned ")
			typ = strings.TrimPrefix(typ, "__shared ")
			typ = strings.TrimPrefix(typ, "inout ")
			sig.params = append(sig.params, sigParam{label, typ})
		}
	}
	return sig
}

func isOpChar(c byte) bool {
	return strings.IndexByte("+-*/%<>=!&|^~?.", c) >= 0
}

func matchParen(s string) string {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[1:i]
			}
		}
	}
	return s
}

func splitTop(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			if s[i] == '>' && i > 0 && s[i-1] == '-' {
				continue
			}
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// upcastClass converts a subclass reference to its superclass through the generated AsX helper.
func (e *fileEmitter) upcastClass(n ast.Node) string {
	sub := n.Child("sub_expr")
	from := strings.TrimPrefix(e.exprGoType(sub), "*")
	to := strings.TrimPrefix(e.exprGoType(n), "*")
	if from != to && e.m.IsAncestor(to, from) {
		return e.expr(sub) + ".As" + to + "()"
	}
	return e.expr(sub)
}

// magicLiteral renders #line, #column, #file, #fileID and #function at the call site.
func (e *fileEmitter) magicLiteral(n ast.Node) string {
	kind := n.Str("kind")
	off := -1
	if r := n.Child("range"); r != nil {
		if f, ok := r["start"].(float64); ok {
			off = int(f)
		}
	}
	switch kind {
	case "#line", "#column":
		line, col := e.m.lineCol(e.file.Filename, off)
		if kind == "#column" {
			return strconv.Itoa(col)
		}
		return strconv.Itoa(line)
	case "#file", "#fileID", "#filePath":
		return strconv.Quote(shortPath(e.file.Filename))
	case "#function":
		fn := e.where
		if i := strings.LastIndex(fn, "."); i >= 0 {
			fn = fn[i+1:]
		}
		return strconv.Quote(fn)
	}
	return strconv.Quote(kind)
}

// isZeroLiteral reports whether n is a literal equal to its type's Go zero value ([], [:], 0, "", false).
func isZeroLiteral(n ast.Node) bool {
	for n != nil && (n.Kind() == "paren_expr" || n.Kind() == "autoclosure_expr" || n.Kind() == "erasure_expr") {
		if n.Kind() == "autoclosure_expr" {
			n = n.Child("single_expr_body")
		} else {
			n = n.Child("sub_expr")
		}
	}
	if n == nil {
		return false
	}
	switch n.Kind() {
	case "array_expr", "dictionary_expr":
		return len(n.List("elements")) == 0
	case "integer_literal_expr", "float_literal_expr":
		v := n.Str("value")
		return v == "0" || v == "0.0"
	case "string_literal_expr":
		return n.Str("value") == ""
	case "boolean_literal_expr":
		return n.Str("value") == "false"
	}
	return false
}

// literalInit renders an array or dictionary literal of a local type through its init(arrayLiteral:) or init(dictionaryLiteral:).
func (e *fileEmitter) literalInit(n ast.Node, label string) (string, bool) {
	t := e.m.typeOf(n.Type())
	if t == nil || t.Kind != types.Named {
		return "", false
	}
	var ctor *DeclInfo
	pfx := ""
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.decls {
			if d.Kind != "constructor_decl" || d.Owner != t.Name || d.Node == nil {
				continue
			}
			ps := d.Node.Child("params").List("params")
			if len(ps) == 1 && ps[0].Str("apiName") == label || len(ps) == 1 && ps[0].Name() == label {
				ctor = d
				if mod.Pkg != e.m.Pkg {
					e.m.importPkgs[mod.Pkg] = mod.ImportPath
					pfx = mod.Pkg + "."
				}
			}
		}
	}
	if ctor == nil {
		return "", false
	}
	pt := e.m.GoType(e.m.typeOf(ctor.Node.Child("params").List("params")[0].Str("interface_type")), nil)
	if !strings.HasPrefix(pt, "[]") {
		pt = "[]" + pt
	}
	var parts []string
	for _, el := range n.List("elements") {
		if label == "dictionaryLiteral" {
			kv := el.List("elements")
			if len(kv) == 2 {
				parts = append(parts, "{"+e.expr(kv[0])+", "+e.expr(kv[1])+"}")
			}
			continue
		}
		parts = append(parts, e.expr(el))
	}
	return pfx + ctor.GoName + "(" + pt + "{" + strings.Join(parts, ", ") + "})", true
}

// stringish renders a String or Substring expression as a Go string.
func (e *fileEmitter) stringish(n ast.Node) string {
	if e.m.Recv(e.m.typeOf(n.Type())) == "Substring" {
		return e.expr(n) + ".String()"
	}
	return e.expr(n)
}

// lazyCell is the addressable storage of a lazily initialized static or global.
func lazyCell(info *DeclInfo, pfx string) string {
	return "(*" + pfx + "Lazy_" + info.GoName + ".Ptr())"
}

// storedGlobal emits a static or global stored property, lazily when its initializer isn't a literal.
func (e *fileEmitter) storedGlobal(info *DeclInfo, goName, gt string, init ast.Node) {
	if init == nil {
		e.w("var %s %s\n\n", goName, gt)
		return
	}
	saved := e.b
	e.b = strings.Builder{}
	val := e.expr(init)
	pre := e.b.String()
	e.b = saved
	if info == nil || !info.Lazy {
		if pre != "" {
			e.w("var %s %s = func() %s {\n%sreturn %s\n}()\n\n", goName, gt, gt, pre, val)
			return
		}
		e.w("var %s %s = %s\n\n", goName, gt, val)
		return
	}
	e.w("var Lazy_%s swiftrt.Lazy[%s]\n\n", goName, gt)
	e.w("func init() {\nLazy_%s.Init = func() %s {\n%sreturn %s\n}\n}\n\n", goName, gt, pre, val)
	e.w("func %s() %s { return *Lazy_%s.Ptr() }\n\n", goName, gt, goName)
}

// isIdentityCapture reports a capture like `[self]` or `[x]` that rebinds a name to itself.
func isIdentityCapture(c ast.Node) bool {
	pes := c.List("pattern_entries")
	if len(pes) != 1 {
		return false
	}
	init := pes[0].Child("processed_init")
	for init != nil && (init.Kind() == "load_expr" || init.Kind() == "paren_expr") {
		init = init.Child("sub_expr")
	}
	return init != nil && init.Kind() == "declref_expr" && init.Child("decl").Str("base_name") == patternName(pes[0].Child("pattern"))
}

// metaReceiver turns a metatype expression (`A.self`, `Self`, `Foo.self`) into a value whose methods
// carry the type's static protocol requirements: the zero value of the type.
func (e *fileEmitter) metaReceiver(base ast.Node) string {
	for base != nil && (base.Kind() == "load_expr" || base.Kind() == "paren_expr" || base.Kind() == "dot_self_expr" || base.Kind() == "function_conversion_expr" || base.Kind() == "metatype_conversion_expr") {
		base = base.Child("sub_expr")
	}
	if base == nil {
		return "self"
	}
	if base.Kind() == "declref_expr" && base.Child("decl").Str("decl_usr") == "" && base.Child("decl").Str("base_name") == "self" {
		return "self"
	}
	t := e.m.typeOf(base.Type())
	if t != nil && t.Kind == types.Meta && len(t.Args) == 1 && (t.Args[0].Kind == types.Existential || t.Args[0].Kind == types.Named && e.m.isLocalProtocol(t.Args[0].Name)) {
		// `any P.Type` values are carried as a zero value of the concrete type, typed as P.
		return e.expr(base)
	}
	if t != nil && t.Kind == types.Meta && len(t.Args) == 1 {
		inner := t.Args[0]
		gt := e.m.GoType(inner, e.generics)
		if inner.Kind == types.Generic && strings.HasPrefix(gt, "any") {
			return "self"
		}
		if inner.Kind == types.Named && e.m.HasMutatingMethods(inner.Name) {
			return "new(" + gt + ")"
		}
		return "(*new(" + gt + "))"
	}
	return e.expr(base)
}

// dynamicStatics are foreign static properties Swift recomputes on every read, so Go calls them.
var dynamicStatics = map[string]bool{
	"TimeZoneCurrent": true, "TimeZoneAutoupdatingCurrent": true, "CalendarCurrent": true,
	"CalendarAutoupdatingCurrent": true, "LocaleCurrent": true, "InstantNow": true,
	"ContinuousClockInstantNow": true, "SuspendingClockInstantNow": true, "DateNow": true,
}

// lessFunc names the Go function for a local type's `<` operator, or "" if it has none.
func (e *fileEmitter) lessFunc(t *types.Type) string {
	if t == nil || t.Kind != types.Named {
		return ""
	}
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.sortedDecls() {
			if d.Kind == "func_decl" && d.Owner == t.Name && d.Node != nil && d.Node.Name() == "<" {
				if mod.Pkg != e.m.Pkg {
					e.m.importPkgs[mod.Pkg] = mod.ImportPath
					return mod.Pkg + "." + d.GoName
				}
				return d.GoName
			}
		}
	}
	return ""
}

// ctorWithLabels finds a type's init with exactly these argument labels.
func (e *fileEmitter) ctorWithLabels(owner string, labels ...string) (*DeclInfo, string) {
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.sortedDecls() {
			if d.Kind != "constructor_decl" || d.Owner != owner || d.Node == nil {
				continue
			}
			ps := d.Node.Child("params").List("params")
			if len(ps) != len(labels) {
				continue
			}
			ok := true
			for i, p := range ps {
				l := p.Str("apiName")
				if l == "" {
					l = p.Name()
				}
				if l != labels[i] {
					ok = false
				}
			}
			if ok {
				pfx := ""
				if mod.Pkg != e.m.Pkg {
					e.m.importPkgs[mod.Pkg] = mod.ImportPath
					pfx = mod.Pkg + "."
				}
				return d, pfx
			}
		}
	}
	return nil, ""
}

// defaultValue renders a subscript's `default:` argument, which Swift passes as an autoclosure.
func (e *fileEmitter) defaultValue(n ast.Node) string {
	for n != nil && (n.Kind() == "autoclosure_expr" || n.Kind() == "paren_expr") {
		if n.Kind() == "autoclosure_expr" {
			n = n.Child("single_expr_body")
		} else {
			n = n.Child("sub_expr")
		}
	}
	return e.expr(n)
}

// assignable reports an expression the emitter can write to directly: a local, parameter, or a chain of
// stored properties.
func (e *fileEmitter) assignable(n ast.Node) bool {
	switch n.Kind() {
	case "declref_expr":
		info, _ := e.lookup(n.Child("decl").Str("decl_usr"))
		return info == nil || !info.Computed
	case "member_ref_expr":
		info, _ := e.lookup(n.Child("decl").Str("decl_usr"))
		return info != nil && info.Kind == "var_decl" && !info.Computed && e.assignable(stripLoads(n.Child("base")))
	}
	return false
}
