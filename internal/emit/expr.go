package emit

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// slot is a dictionary element copied out for an inout argument and stored back after the call.
type slot struct{ pre, writeBack string }

// wrapSlots runs call inside a closure that copies the slots it takes out of their dictionaries
// first and stores them back after it returns.
func (e *fileEmitter) wrapSlots(n ast.Node, call string, mark int) string {
	var pre, wb strings.Builder
	for _, s := range e.slots[mark:] {
		pre.WriteString(s.pre)
		wb.WriteString(s.writeBack)
	}
	e.slots = e.slots[:mark]
	rt := e.exprGoType(n)
	if isVoidGo(rt) {
		return "func() {\n" + pre.String() + call + "\n" + wb.String() + "}()"
	}
	return "func() " + rt + " {\n" + pre.String() + "res := " + call + "\n" + wb.String() + "return res\n}()"
}

func (e *fileEmitter) expr(n ast.Node) string {
	if v, ok := e.exprSubst[nodeKey(n)]; ok && n != nil {
		return v
	}
	if s, ok := e.snapshots[nodeKey(n)]; ok && n != nil {
		return s
	}
	mark := len(e.slots)
	out := e.exprRaw(n)
	out = e.inlineOptUse(n, out)
	if out == "swiftrt.IntMax" || out == "swiftrt.IntMin" {
		if c, ok := intBound[e.exprGoType(n)+out[len("swiftrt.Int"):]]; ok {
			return c
		}
	}
	if out == "swiftrt.IntBitWidth" {
		if w, ok := bitWidth[e.bitWidthOwner(n)]; ok {
			return w
		}
	}
	if len(e.slots) > mark {
		switch n.Kind() {
		case "call_expr", "binary_expr", "prefix_unary_expr", "postfix_unary_expr":
			out = e.wrapSlots(n, out, mark)
		}
	}
	if e.pinned != nil {
		out = e.pin(n, out)
	}
	if n != nil && n.Kind() == "call_expr" && (!strings.HasPrefix(out, "swiftrt.") || strings.HasPrefix(out, "swiftrt.Copy(")) && e.returnsGenericOptional(n) {
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 && t.Args[0].Kind != types.Optional && e.m.isReferenceGo(t.Args[0], e.m.GoType(t.Args[0], e.generics)) {
			// A generic `T?` with T a class comes back as **C; Swift's C? is *C.
			return "swiftrt.Flat(" + out + ")"
		}
	}
	if n == nil || !strings.HasPrefix(out, "swiftrt.") {
		return out
	}
	switch n.Kind() {
	case "call_expr", "subscript_expr", "member_ref_expr", "optional_try_expr", "conditional_checked_cast_expr":
	default:
		return out
	}
	for _, keep := range []string{"swiftrt.Unsupported", "swiftrt.CoalesceRef", "swiftrt.CoalesceOpt", "swiftrt.Flat(", "swiftrt.TryFlat", "swiftrt.TryNil", "swiftrt.Some",
		"swiftrt.TaskLocalGet", "swiftrt.TaskLocalWithValue", "swiftrt.MapSubscriptDefault", "swiftrt.ForceRef", "swiftrt.Clone", "swiftrt.MustAs", "swiftrt.Copy(",
		"swiftrt.Dynamic", "swiftrt.Ref(", "swiftrt.MutexWithLock", "swiftrt.NSLockWithLock", "swiftrt.NSRecursiveLockWithLock", "swiftrt.Coalesce(",
		"swiftrt.OptFlatMap", "swiftrt.RefFlatMap"} {
		if strings.HasPrefix(out, keep) {
			return out
		}
	}
	if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 && t.Args[0].Kind != types.Optional {
		// C strings come back from the runtime as *any already.
		if gt := e.m.GoType(t.Args[0], e.generics); gt != "*any" && e.m.isReferenceGo(t.Args[0], gt) {
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
	case "lazy_initializer_expr":
		return e.expr(n.Child("sub_expr"))
	case "derived_to_base_expr":
		return e.upcastClass(n)
	case "erasure_expr":
		sub := n.Child("sub_expr")
		if nt, st := e.m.typeOf(n.Type()), e.m.typeOf(sub.Type()); nt != nil && st != nil && nt.Kind == types.Meta && st.Kind == types.Meta && len(st.Args) == 1 && st.Args[0].Kind == types.Generic &&
			!strings.Contains(st.Args[0].Name, ".") && e.m.protocolGoName(e.m.GoType(st.Args[0], e.generics)) {
			// `Self.self` in a protocol extension, stored as `any P.Type`: the receiver carries the type.
			return e.metaReceiver(sub)
		}
		if nt, st := e.m.typeOf(n.Type()), e.m.typeOf(sub.Type()); nt != nil && st != nil && nt.Kind == types.Meta && st.Kind == types.Meta && len(st.Args) == 1 && st.Args[0].Kind == types.Named {
			// A concrete metatype stored as `any P.Type` becomes the type's zero value, which carries P's static requirements.
			if e.m.isLocalProtocol(st.Args[0].Name) || sub.Kind() == "opaque_value_expr" {
				// Already an existential metatype value, or the dynamic type of an opened existential: keep it.
				return e.expr(sub)
			}
			gt := e.m.GoType(st.Args[0], e.generics)
			if e.m.HasMutatingMethods(st.Args[0].Name) {
				return "new(" + gt + ")"
			}
			return "*new(" + gt + ")"
		}
		if t := e.m.typeOf(sub.Type()); t != nil && t.Kind == types.Named && e.m.HasMutatingMethods(t.Name) && !e.m.isProtocol(t.Name) {
			// An existential is already an interface value; boxing it again would lose its identity.
			return "swiftrt.Ref(" + e.expr(sub) + ")"
		}
		if e.exprGoType(n) == "error" && e.exprGoType(sub) != "error" {
			// A Swift error value need not implement Go's error; the runtime carries it as one.
			return "swiftrt.ErrorOf(" + e.expr(sub) + ")"
		}
		if e.typedErasure[nodeKey(n)] {
			return e.exprGoType(n) + "(" + e.expr(sub) + ")"
		}
		return e.expr(sub)
	case "dot_syntax_base_ignored":
		return e.expr(n.Child("rhs"))
	case "archetype_to_super_expr":
		return e.expr(n.Child("sub_expr"))
	case "super_ref_expr":
		if t := e.m.typeOf(n.Type()); t != nil {
			return "(&self." + embeddedField(e.m.GoType(t, e.generics)) + ")"
		}
		return "self"
	case "destructure_tuple_expr":
		sub, res := n.Child("sub_expr"), n.Child("result_expr")
		if res == nil || e.exprGoType(n) == e.exprGoType(sub) {
			return e.expr(sub)
		}
		// A tuple conversion (`(String, R?)` returned as `(String?, R?)`) rebuilds the tuple element by element.
		tmp := e.newTmp("tup")
		for i, d := range n.List("destructured") {
			e.opaque[d.Str("identity")] = tmp + ".F" + itoa(i)
		}
		return "func() " + e.exprGoType(n) + " {\n" + tmp + " := " + e.expr(sub) + "\nreturn " + e.expr(res) + "\n}()"
	case "load_expr", "function_conversion_expr",
		"covariant_return_conversion_expr", "underlying_to_opaque_expr", "abi_safe_conversion_expr", "unresolved_type_conversion_expr",
		"collection_upcast_expr", "pointer_to_pointer", "metatype_conversion_expr", "existential_metatype_to_object",
		"unevaluated_instance", "dot_self_expr", "await_expr", "unsafe_expr", "borrow_expr", "copy_expr", "consume_expr", "any_hashable_erasure_expr":
		if n.Kind() == "collection_upcast_expr" || n.Kind() == "any_hashable_erasure_expr" {
			return e.upcast(n)
		}
		if n.Kind() == "function_conversion_expr" {
			if f, ok := e.paramConversion(n); ok {
				return f
			}
		}
		if ac := n.Child("sub_expr"); n.Kind() == "function_conversion_expr" && ac.Kind() == "autoclosure_expr" {
			if f, ok := e.methodRef(ac); ok {
				return f
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
		if isUnicodeScalarsOfString(sub) {
			// Mutating a String's unicodeScalars: mutate its runes, then store the String back.
			str := e.lvalue(stripLoads(sub).Child("base"))
			tmp := e.newTmp("scalars")
			e.slots = append(e.slots, slot{pre: tmp + " := []rune(" + str + ")\n", writeBack: str + " = string(" + tmp + ")\n"})
			return "&" + tmp
		}
		if ds := stripLoads(sub); ds.Kind() == "subscript_expr" && isFuncDict(e.m.Recv(e.m.typeOf(ds.Child("base").Type()))) {
			if args := ds.Child("args").List("args"); len(args) == 2 && args[1].Str("label") == "default" {
				// A runtime dictionary's element mutated in place: copy it out, then store it back.
				fn := "swiftrt." + e.m.Recv(e.m.typeOf(ds.Child("base").Type()))
				d := e.lvalue(stripLoads(ds.Child("base")))
				k, tmp := e.newTmp("key"), e.newTmp("slot")
				pre := k + " := " + e.expr(args[0].Child("expr")) + "\n" + tmp + " := " + fn + "SubscriptDefault(" + d + ", " + k + ", " + e.defaultValue(args[1].Child("expr")) + ")\n"
				e.slots = append(e.slots, slot{pre: pre, writeBack: fn + "Set(&" + d + ", " + k + ", &" + tmp + ")\n"})
				return "&" + tmp
			}
		}
		if root := mapElementRoot(e, stripLoads(sub)); root != nil {
			// A member of a Go map element: copy the element out, mutate it, store it back.
			saved := e.b
			e.b = strings.Builder{}
			rlv := e.lvalue(root)
			if strings.HasSuffix(rlv, "]") {
				tmp := e.newTmp("slot")
				e.lvSubst[fmt.Sprintf("%p", root)] = tmp
				lv := e.lvalue(sub)
				delete(e.lvSubst, fmt.Sprintf("%p", root))
				pre := e.b.String()
				e.b = saved
				e.slots = append(e.slots, slot{pre: pre + tmp + " := " + rlv + "\n", writeBack: rlv + " = " + tmp + "\n"})
				return "&" + lv
			}
			e.b = saved
		}
		saved := e.b
		e.b = strings.Builder{}
		lv := e.lvalue(sub)
		pre := e.b.String()
		e.b = saved
		ds := stripLoads(sub)
		if ds.Kind() == "force_value_expr" {
			ds = stripLoads(ds.Child("sub_expr"))
		}
		if ds.Kind() == "subscript_expr" && e.m.Recv(e.m.typeOf(ds.Child("base").Type())) == "Map" && strings.HasSuffix(lv, "]") {
			// Go map elements are not addressable: the enclosing call mutates a copy and stores it back.
			tmp := e.newTmp("slot")
			e.slots = append(e.slots, slot{pre: pre + tmp + " := " + lv + "\n", writeBack: lv + " = " + tmp + "\n"})
			return "&" + tmp
		}
		e.w("%s", pre)
		return "&" + lv
	case "regex_literal_expr":
		text := n.Str("text")
		for strings.HasPrefix(text, "#") && strings.HasSuffix(text, "#") {
			text = text[1 : len(text)-1]
		}
		text = strings.TrimSuffix(strings.TrimPrefix(text, "/"), "/")
		lit := "`" + text + "`"
		if strings.Contains(text, "`") {
			lit = strconv.Quote(text)
		}
		gt := e.exprGoType(n)
		if after, ok := strings.CutPrefix(gt, "swiftrt.SwiftRegex["); ok {
			return "swiftrt.RegexLiteral[" + strings.TrimSuffix(after, "]") + "](" + lit + ")"
		}
		return e.m.Unsup("regex-literal-type", e.where, gt)
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
		if gt := e.exprGoType(n); n.Kind() == "float_literal_expr" && n.Bool("negative") && (gt == "float64" || gt == "float32") {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f == 0 {
				// Go has no negative zero constant.
				return "swiftrt.NegativeZero[" + gt + "]()"
			}
		}
		if e.exprGoType(n) == "swiftrt.Int128" {
			if _, err := strconv.ParseInt(v, 10, 64); err == nil {
				return "swiftrt.Int128Of(" + v + ")"
			}
			return "swiftrt.Int128Parse(\"" + v + "\")"
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
	case "string_to_pointer":
		// A String passed for a C string parameter.
		return "swiftrt.CStringOf(" + e.expr(n.Child("sub_expr")) + ")"
	case "inout_to_pointer":
		return "&" + e.lvalue(stripLoads(n.Child("sub_expr")))
	case "array_to_pointer":
		// An array passed for a C pointer parameter: `&array` passes the array itself, which the
		// callee may write through; a plain `array` passes its elements to read.
		if sub := n.Child("sub_expr"); sub.Kind() == "inout_expr" {
			return "&" + e.lvalue(stripLoads(sub.Child("sub_expr")))
		}
		return e.expr(n.Child("sub_expr"))
	case "member_ref_expr":
		if usr := n.Child("decl").Str("decl_usr"); strings.HasPrefix(usr, "c:@S@") && strings.Contains(usr, "@FI@") {
			// A C struct field is a Go field of the runtime struct.
			return e.expr(n.Child("base")) + "." + capitalize(usr[strings.Index(usr, "@FI@")+4:])
		}
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
		e.lendClosures(n)
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
		if e.initFail == "false" {
			return e.classDelegation(sub)
		}
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
				sup := embeddedField(e.m.GoType(e.m.typeOf(as[0].Child("expr").Type()), e.generics))
				e.w("self.%s = *%s\n", sup, call)
				return "self.SwiftSub = self"
			}
		}
		return "self = " + call
	case "array_expr":
		var parts []string
		e.pinOperands(n.List("elements")...)
		elemSink := e.partsSink(n)
		for _, el := range n.List("elements") {
			parts = append(parts, e.part(el, *elemSink))
		}
		if lit, ok := e.literalInit(n, "arrayLiteral"); ok {
			return lit
		}
		gt := e.exprGoType(n)
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Named && e.m.IsOptionSet(t.Name) {
			return "swiftrt.OptionSetOf[" + gt + "](" + strings.Join(parts, ", ") + ")"
		}
		if gt == "swiftrt.NSStringCompareOptions" {
			// The runtime models String.CompareOptions as the bit mask NSString's options are.
			return "(" + strings.Join(append([]string{gt + "(0)"}, parts...), " | ") + ")"
		}
		if after, ok := strings.CutPrefix(gt, "swiftrt.HSet["); ok {
			return "swiftrt.HSetOf[" + strings.TrimSuffix(after, "]") + "](" + strings.Join(parts, ", ") + ")"
		}
		if strings.HasPrefix(gt, "map[") && strings.HasSuffix(gt, "]struct{}") {
			for i := range parts {
				parts[i] += ": {}"
			}
		}
		return "(" + gt + "{" + strings.Join(parts, ", ") + "})"
	case "dictionary_expr":
		var ops []ast.Node
		for _, el := range n.List("elements") {
			ops = append(ops, el.List("elements")...)
		}
		e.pinOperands(ops...)
		if lit, ok := e.literalInit(n, "dictionaryLiteral"); ok {
			return lit
		}
		gt := e.exprGoType(n)
		elemSink := e.partsSink(n)
		if t := e.m.typeOf(n.Type()); e.m.Recv(t) == "HMap" && len(n.List("elements")) > 0 {
			var keys, vals []string
			for _, el := range n.List("elements") {
				if kv := el.List("elements"); len(kv) == 2 {
					keys, vals = append(keys, e.part(kv[0], *elemSink)), append(vals, e.part(kv[1], *elemSink))
				}
			}
			kt, vt := e.m.GoType(t.Args[0], e.generics), e.m.GoType(t.Args[1], e.generics)
			return "swiftrt.HMapOf([]" + kt + "{" + strings.Join(keys, ", ") + "}, []" + vt + "{" + strings.Join(vals, ", ") + "})"
		}
		var parts []string
		pair := ": "
		if t := e.m.typeOf(n.Type()); t != nil && t.Name == "Swift.KeyValuePairs" {
			// KeyValuePairs keeps its literal's order: a slice of key-value tuples.
			pair = ", "
		}
		for _, el := range n.List("elements") {
			kv := el.List("elements")
			if len(kv) == 2 {
				part := e.part(kv[0], *elemSink) + pair + e.part(kv[1], *elemSink)
				if pair == ", " {
					part = "{" + part + "}"
				}
				parts = append(parts, part)
			}
		}
		return "(" + gt + "{" + strings.Join(parts, ", ") + "})"
	case "subscript_expr":
		return e.subscript(n)
	case "ternary_expr":
		if gt := e.exprGoType(n); isVoidGo(gt) {
			// A Void ternary runs one branch for its effect.
			return fmt.Sprintf("func() {\nif %s {\n%s\n} else {\n%s\n}\n}()", e.expr(n.Child("cond_expr")), e.expr(n.Child("then_expr")), e.expr(n.Child("else_expr")))
		}
		return fmt.Sprintf("func() %s { if %s { return %s }; return %s }()", e.exprGoType(n), e.expr(n.Child("cond_expr")), e.expr(n.Child("then_expr")), e.expr(n.Child("else_expr")))
	case "inject_into_optional":
		sub := n.Child("sub_expr")
		t := e.m.typeOf(n.Type())
		if t.Kind == types.Optional {
			inner := e.m.GoType(t.Args[0], e.generics)
			if inner == "" {
				// Void wrapped in an Optional is a non-nil pointer to the empty struct.
				if sub.Kind() == "tuple_expr" && len(sub.List("elements")) == 0 {
					return "&struct{}{}"
				}
				if v := e.expr(sub); v != "struct{}{}" {
					return "func() *struct{} { " + v + "; return &struct{}{} }()"
				}
				return "&struct{}{}"
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
		saved, savedPost := e.b, e.post
		e.b, e.post = strings.Builder{}, nil
		e.assign(n)
		for _, p := range e.post {
			e.w("%s\n", p)
		}
		s := e.b.String()
		e.b, e.post = saved, savedPost
		return "func() struct{} {\n" + strings.TrimSpace(s) + "\nreturn struct{}{}\n}()"
	case "try_expr", "force_try_expr", "optional_try_expr":
		return e.tryExpr(n)
	case "is_expr", "is_subtype_expr":
		// `x is T`; is_subtype_expr is the form the compiler uses when T is a subclass of x's class.
		to := n.Str("cast_to")
		if to == "" {
			to = n.Str("written_type")
		}
		return "swiftrt.Is[" + e.m.GoType(e.m.typeOf(to), e.generics) + "](" + e.expr(n.Child("sub_expr")) + ")"
	case "conditional_checked_cast_expr":
		return "swiftrt.As[" + e.castTarget(n) + "](" + e.expr(n.Child("sub_expr")) + ")"
	case "forced_checked_cast_expr":
		return "swiftrt.MustAs[" + e.castTarget(n) + "](" + e.expr(n.Child("sub_expr")) + ")"
	case "type_expr":
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Meta && len(t.Args) == 1 {
			inner := e.m.GoType(t.Args[0], e.generics)
			if v, ok := e.metaValues[t.Args[0].Name]; ok && t.Args[0].Kind == types.Generic {
				// A type known only as its protocol: the parameter that carries its metatype.
				return v
			}
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
		// A writable key path read through its KeyPath superclass is still the runtime's struct.
		for inner := kp; inner != nil; inner = inner.Child("sub_expr") {
			if strings.HasPrefix(e.exprGoType(inner), "swiftrt.WritableKeyPath[") {
				return e.expr(kp) + ".Get(" + e.expr(n.Child("base")) + ")"
			}
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
	switch n.Kind() {
	case "make_temporarily_escapable_expr":
		// withoutActuallyEscaping: the body sees the closure under the placeholder; Go closures
		// have no escape rule to satisfy.
		e.opaque[n.Child("opaque_value").Str("identity")] = e.expr(n.Child("nonescaping_closure_value"))
		return e.expr(n.Child("sub_expr"))
	case "metatype_expr":
		// `type(of: value)`.
		if base := n.Child("base"); base != nil {
			return "swiftrt.TypeOfValue(" + e.expr(base) + ")"
		}
	case "enum_is_case_expr":
		// `optional is .some`, as the compiler writes `x is T?`-style checks: a nil test.
		if name := n.Child("name").Child("base_name").Str("name"); name == "some" || name == "none" {
			op := " != nil"
			if name == "none" {
				op = " == nil"
			}
			return "(" + e.expr(n.Child("sub_expr")) + op + ")"
		}
	}
	if n.Kind() == "bridge_to_objc_expr" {
		// A Swift value passed where Objective-C's counterpart is expected (a String as a `%@`
		// format argument): the runtime takes the Swift value.
		return e.expr(n.Child("sub_expr"))
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
	sub := n.Child("sub_expr")
	src, dst := e.exprGoType(sub), e.exprGoType(n)
	if src != dst && strings.HasPrefix(src, "[]") && strings.HasPrefix(dst, "[]") {
		// [Subclass] as [Base], or [Concrete] as [any Protocol]: convert element by element.
		se, de := src[2:], dst[2:]
		conv := "v"
		if f, t := strings.TrimPrefix(se, "*"), strings.TrimPrefix(de, "*"); strings.HasPrefix(se, "*") && strings.HasPrefix(de, "*") && e.m.IsAncestor(t, f) {
			conv = "v." + upcastMethod(t) + "()"
		}
		return "swiftrt.SliceMap(" + e.expr(sub) + ", func(v " + se + ") " + de + " { return " + conv + " })"
	}
	if src != dst && strings.HasPrefix(src, "map[") && strings.HasSuffix(src, "]struct{}") && strings.HasPrefix(dst, "map[") && strings.HasSuffix(dst, "]struct{}") {
		// Set<Concrete> as Set<AnyHashable>.
		return "swiftrt.SetUpcast[" + dst + "](" + e.expr(sub) + ")"
	}
	if src != dst && (strings.HasPrefix(dst, "swiftrt.HSet[") || strings.HasPrefix(dst, "swiftrt.HMap[")) {
		// Set<[Int]> as Set<AnyHashable>, or [K: V] as [AnyHashable: V]: element by element.
		return "swiftrt.MustAs[" + dst + "](" + e.expr(sub) + ")"
	}
	if st, dt := e.m.typeOf(sub.Type()), e.m.typeOf(n.Type()); src != dst && e.m.Recv(st) == "Map" && e.m.Recv(dt) == "Map" && len(st.Args) == 2 && len(dt.Args) == 2 && e.m.GoType(st.Args[0], e.generics) == e.m.GoType(dt.Args[0], e.generics) {
		// [K: Concrete] as [K: Any]: convert value by value.
		sv, dv := e.m.GoType(st.Args[1], e.generics), e.m.GoType(dt.Args[1], e.generics)
		return "swiftrt.MapMapValues(" + e.expr(sub) + ", func(v " + sv + ") " + dv + " { return v })"
	}
	return e.expr(sub)
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
	var ops []ast.Node
	var elements []ast.Node
	for _, el := range tap.Child("body").List("elements") {
		// An existential argument, such as `\(error)` of an `any Error`, arrives opened around its call.
		for el.Kind() == "open_existential_expr" {
			e.opaque[el.Child("opaque_value").Str("identity")] = e.expr(el.Child("existential_value"))
			el = el.Child("sub_expr")
		}
		elements = append(elements, el)
	}
	for _, el := range elements {
		if mc := callOf(el); mc != nil && mc.method == "appendInterpolation" {
			if args := el.Child("args").List("args"); len(args) == 1 {
				ops = append(ops, args[0].Child("expr"))
			}
		}
	}
	e.pinOperands(ops...)
	for _, op := range ops {
		if e.m.Recv(e.m.typeOf(op.Type())) != "String" {
			// swiftrt.Str is a call, which Go already orders.
			delete(e.pinned, nodeKey(op))
		}
	}
	for _, el := range elements {
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
// lookupModule finds the declaration usr names and the module that declares it.
func (e *fileEmitter) lookupModule(usr string) (*DeclInfo, *Module) {
	if usr == "" {
		return nil, nil
	}
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		if info := mod.decls[usr]; info != nil {
			return info, mod
		}
	}
	return nil, nil
}

// calleeUSR is the USR of the declaration a call, operator or method reference n applies, or "".
func calleeUSR(n ast.Node) string {
	fn := stripConv(n.Child("fn"))
	if fn == nil {
		return ""
	}
	if lhs := fn.Child("lhs"); fn.Kind() == "dot_syntax_base_ignored" && lhs.Kind() == "declref_expr" && lhs.Type() == "" {
		fn = stripConv(fn.Child("rhs"))
	}
	switch fn.Kind() {
	case "dot_syntax_call_expr":
		return stripConv(fn.Child("fn")).Child("decl").Str("decl_usr")
	case "declref_expr":
		return fn.Child("decl").Str("decl_usr")
	}
	return ""
}

// inlineOptUse turns the Go form out of n into the Optional's pointer form where n reaches a
// function that returns swiftrt.Opt: a call takes Ptr of the result, and a reference to the
// function as a value forwards to it.
func (e *fileEmitter) inlineOptUse(n ast.Node, out string) string {
	switch n.Kind() {
	case "call_expr", "binary_expr", "prefix_unary_expr", "postfix_unary_expr":
		if info, mod := e.lookupModule(calleeUSR(n)); e.m.inlineOptResult(info, mod) && !strings.HasPrefix(out, "swiftrt.Unsupported") {
			return e.optPtr(out)
		}
	case "declref_expr":
		if n.Child("function_ref").Str("apply_level") != "unapplied" {
			return out
		}
		if info, mod := e.lookupModule(n.Child("decl").Str("decl_usr")); e.m.inlineOptResult(info, mod) {
			return e.inlineOptForwarder(n, out)
		}
	}
	return out
}

// optPtr is opt, a swiftrt.Opt expression, in the Optional's pointer form. It remembers the result
// so that inlineOpt can take the Opt back without matching on text.
func (e *fileEmitter) optPtr(opt string) string {
	if e.optPtrs == nil {
		e.optPtrs = map[string]bool{}
	}
	out := opt + ".Ptr()"
	e.optPtrs[out] = true
	return out
}

// inlineOptForwarder wraps fn, a function value of n's Swift function type whose Go result is
// swiftrt.Opt, as a function returning the Optional's pointer form. fn is evaluated once.
func (e *fileEmitter) inlineOptForwarder(n ast.Node, fn string) string {
	ft := e.m.typeOf(n.Type())
	if ft == nil || ft.Kind != types.Func || ft.Result == nil || ft.Result.Kind != types.Optional {
		return fn
	}
	var params, names []string
	for i, a := range ft.Args {
		pt := e.m.GoType(a, e.generics)
		if i < len(ft.Inout) && ft.Inout[i] {
			pt = "*" + pt
		}
		params = append(params, "p"+itoa(i)+" "+pt)
		names = append(names, "p"+itoa(i))
	}
	res := e.m.GoType(ft.Result, e.generics)
	if goIdent.MatchString(fn) {
		// A function name: calling it directly lets Go infer a generic function's type arguments.
		return "func(" + strings.Join(params, ", ") + ") " + res + " { return " + fn + "(" + strings.Join(names, ", ") + ").Ptr() }"
	}
	return "func() " + e.m.GoType(ft, e.generics) + " { f := " + fn + "; return func(" + strings.Join(params, ", ") + ") " +
		res + " { return f(" + strings.Join(names, ", ") + ").Ptr() } }()"
}

var goIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

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
			e.m.importPkg(d)
			return info, d.Pkg + "."
		}
	}
	return nil, ""
}

func (e *fileEmitter) declRef(d ast.Node, n ast.Node) string {
	usr := d.Str("decl_usr")
	name := d.Str("base_name")
	localKey := usr
	if localKey == "" {
		localKey = "local:" + name
	}
	if fn, ok := e.localFuncs[localKey]; ok && n != nil && n.Child("function_ref").Str("apply_level") == "unapplied" && len(fn.Child("generic_signature").Strings("generic_params")) > 0 {
		// A local generic function used as a value at one specialization: Go's closure takes the
		// protocol's interface, so a function of the specialized type forwards to it.
		if ft := e.m.typeOf(n.Type()); ft != nil && ft.Kind == types.Func {
			var params, names []string
			for i, a := range ft.Args {
				params = append(params, "p"+itoa(i)+" "+e.m.GoType(a, e.generics))
				names = append(names, "p"+itoa(i))
			}
			res, ret := "", ""
			if ft.Result != nil {
				if res = e.m.GoType(ft.Result, e.generics); res != "" {
					ret = "return "
				}
			}
			return "func(" + strings.Join(params, ", ") + ") " + res + " { " + ret + goSafe(name) + "(" + strings.Join(names, ", ") + ") }"
		}
	}
	if usr == "" {
		if name == "self" {
			if e.inoutParams["self"] {
				return "(*self)"
			}
			if alias, ok := e.renames["self"]; ok {
				return alias
			}
			return "self"
		}
		if alias, ok := e.renames[name]; ok {
			return alias
		}
		if e.inoutParams[name] {
			return "(*" + goSafe(name) + ")"
		}
		return goSafe(name)
	}
	if name == "max" || name == "min" {
		// max/min passed as a function value over a type with its own `<`.
		if ft := e.m.typeOf(n.Type()); n != nil && ft != nil && ft.Kind == types.Func && len(ft.Args) >= 1 {
			if less := e.lessFunc(ft.Args[0]); less != "" {
				gt := e.m.GoType(ft.Args[0], e.generics)
				return "func(a, b " + gt + ") " + gt + " { return swiftrt." + capitalize(name) + "Less(" + less + ", a, b) }"
			}
		}
	}
	info, pfx := e.lookup(usr)
	if info == nil && usr == "s:Sq4noneyxSgABmlF" {
		// `Optional.none` written out, as in `x == .none`: a nil of the Optional's Go type.
		t := e.m.typeOf(n.Type())
		if t != nil && t.Kind == types.Func && t.Result != nil {
			t = t.Result
		}
		if gt := e.m.GoType(t, e.generics); gt != "" && gt != "any" {
			return "(" + gt + ")(nil)"
		}
	}
	if info == nil {
		dem := e.m.demangle(usr)
		if strings.Contains(dem, "(in ") || strings.HasPrefix(usr, "s:") && strings.Contains(usr, "L_") {
			return goSafe(name)
		}
		if name == "$kp$" {
			// The key path a key-path function binds, which an attribute's argument (`@Test(arguments:)`)
			// declares outside any function.
			return goSafe(name)
		}
		if name := e.m.foreignValueName(usr, name, parseDeclSig(dem)); name != "" {
			return "swiftrt." + name
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

// stdOperatorValue renders a standard-library comparison operator passed as a function (`by: ==`), such as the
// == of tuples, as a Go func literal. It returns "" for other operators.
func (e *fileEmitter) stdOperatorValue(op string, n ast.Node) string {
	ft := e.m.typeOf(n.Type())
	if ft == nil || ft.Kind != types.Func || len(ft.Args) != 2 {
		return ""
	}
	var body string
	switch op {
	case "==":
		body = "swiftrt.Equal(a, b)"
	case "!=":
		body = "!swiftrt.Equal(a, b)"
	case "<", ">", "<=", ">=":
		body = lessOp("swiftrt.Less", op, "a", "b")
	default:
		return ""
	}
	gt := e.m.GoType(ft.Args[0], e.generics)
	return "func(a, b " + gt + ") bool { return " + body + " }"
}

func (e *fileEmitter) stdFuncValue(dem string, n ast.Node) string {
	sig := parseDeclSig(dem)
	if dem == "c:@environ" {
		// Darwin's overlay computes environ; Linux's C libraries export the global itself.
		return "swiftrt.CEnviron()"
	}
	if sig.owner == "Swift" && isOperatorName(sig.base) {
		if f := e.stdOperatorValue(sig.base, n); f != "" {
			return f
		}
	}
	if sig.owner == "Swift" || sig.owner == "" {
		return "swiftrt." + capitalize(sig.base)
	}
	if sig.base == "errno" && (sig.owner == "Darwin" || sig.owner == "Glibc" || sig.owner == "Musl") {
		// The C library's errno is a computed global in each platform's overlay.
		return "swiftrt.CErrno()"
	}
	if sig.base == "environ" && (sig.owner == "Darwin" || sig.owner == "Glibc" || sig.owner == "Musl") {
		return "swiftrt.CEnviron()"
	}
	if (sig.base == "SIG_DFL" || sig.base == "SIG_IGN" || sig.base == "SIG_ERR") && (sig.owner == "Darwin" || sig.owner == "Glibc" || sig.owner == "Musl") {
		// The platform overlays define the signal dispositions as computed globals.
		return "swiftrt.CMacro" + sig.base
	}
	return e.m.Unsup("std-func-value:"+sig.owner+"."+sig.base, e.where, e.exprGoType(n))
}

func (e *fileEmitter) memberRef(n ast.Node) string {
	d := n.Child("decl")
	base := n.Child("base")
	usr := d.Str("decl_usr")
	if sub := stripLoads(base); d.Str("base_name") == "indices" && sub != nil && sub.Kind() == "subscript_expr" {
		// `text.unicodeScalars[from...].indices`: String indices, one per scalar.
		if str := e.scalarViewOf(sub.Child("base")); str != nil {
			if as := sub.Child("args").List("args"); len(as) == 1 && strings.HasPrefix(e.m.demangle(as[0].Child("expr").Type()), "Swift.PartialRangeFrom<Swift.String.Index>") {
				return "swiftrt.StringScalarIndicesFrom(" + e.expr(str) + ", " + e.expr(as[0].Child("expr")) + ")"
			}
		}
	}
	if name := d.Str("base_name"); strings.HasPrefix(usr, "s:s12MemoryLayout") && (name == "size" || name == "stride" || name == "alignment") {
		// `MemoryLayout<T>.size`: the size of T's Go form.
		if of, ok := strings.CutPrefix(e.m.demangle(base.Type()), "Swift.MemoryLayout<"); ok {
			if of, ok = strings.CutSuffix(of, ">.Type"); ok {
				return "swiftrt.MemoryLayout" + capitalize(name) + "[" + e.m.GoType(types.Parse(of), e.generics) + "]()"
			}
		}
	}
	if bt := e.m.typeOf(base.Type()); d.Str("base_name") == "pointee" && bt != nil && len(bt.Args) == 1 && (e.m.Recv(bt) == "UnsafeMutablePointer" || e.m.Recv(bt) == "UnsafePointer") {
		// A pointer to anything but C characters is a Go pointer, read by dereferencing it.
		if el := e.m.GoType(bt.Args[0], e.generics); el != "int8" && el != "uint8" && el != "" && el != "any" {
			return "(*" + e.expr(base) + ")"
		}
	}
	info, pfx := e.lookup(usr)
	if info != nil && info.Kind == "var_decl" {
		if info.Computed {
			defer e.lendReceiver(base, info)()
		}
		switch {
		case info.Static && info.OwnerKind == "protocol":
			call := e.metaReceiver(base) + "." + staticReqName(info) + "()"
			declared := e.m.GoType(e.m.typeOf(info.Node.Str("interface_type")), nil)
			if gt := e.exprGoType(n); gt != "" && !strings.HasPrefix(gt, "any") && strings.HasPrefix(declared, "any") {
				return "swiftrt.MustAs[" + gt + "](" + call + ")"
			}
			if gt := e.exprGoType(n); strings.HasPrefix(gt, "[]") && gt != "[]any" && declared == "[]any" {
				return "swiftrt.FromAnySlice[" + gt[2:] + "](" + call + ")"
			}
			return call
		case info.Static && info.ProtoExt:
			return pfx + info.GoName + "(" + e.metaReceiver(base) + ")"
		case info.Static:
			if info.Computed && info.OwnerKind == "foreign" {
				return pfx + info.GoName + e.foreignStaticTypeArgs(info, base) + "()"
			}
			if info.Computed || info.Lazy {
				return pfx + info.GoName + "()"
			}
			return pfx + info.GoName
		case info.OwnerKind == "protocol":
			return e.expr(base) + ".Get" + info.GoName + "()"
		case info.FreeSelf():
			return pfx + info.GoName + "(" + e.expr(base) + ")"
		case info.Computed:
			return e.expr(base) + "." + e.superTarget(info, base) + "()"
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
	if mp := e.m.mappedDecl(usr); mp != nil {
		return e.mappedMember(mp, n, d, base)
	}
	if base.Kind() == "type_expr" || bt.Kind == types.Meta {
		// A static member is named after the type it belongs to (`Task.isCancelled`), which is
		// usually also its result type.
		if bt.Kind == types.Meta && len(bt.Args) == 1 && isGenericParam(bt.Args[0]) && sig.base == "allCases" {
			return "swiftrt.AllCasesOf[" + e.m.GoType(bt.Args[0], e.generics) + "]()"
		}
		recvT := e.m.typeOf(n.Type())
		if bt.Kind == types.Meta && len(bt.Args) == 1 && bt.Args[0].Kind == types.Named {
			recvT = bt.Args[0]
		}
		name := "swiftrt." + e.m.Recv(recvT) + capitalize(sig.base)
		if dynamicStatics[strings.TrimPrefix(name, "swiftrt.")] {
			name += "()"
		}
		return name
	}
	recv := e.m.Recv(bt)
	if recv == "Task" && sig.base == "value" {
		// The runtime's task holds its result as any; the member's Swift type says what it is.
		if gt := e.exprGoType(n); gt != "" && gt != "any" && gt != "struct{}" {
			return "swiftrt.MustAs[" + gt + "](swiftrt.TaskValue(" + e.expr(base) + "))"
		}
	}
	if recv == "KeyedDecodingContainer" && sig.base == "allKeys" {
		if bt.Kind != types.Named || len(bt.Args) != 1 {
			return e.m.Unsup("allKeys-untyped", e.where, e.exprGoType(n))
		}
		ctor := e.stringValueInit(bt.Args[0].Name)
		if ctor == "" {
			return e.m.Unsup("allKeys-no-init-stringValue", e.where, e.exprGoType(n))
		}
		return "swiftrt.KeyedDecodingContainerAllKeys(" + e.recvArg(base) + ", " + ctor + ")"
	}
	switch sig.base {
	case "startIndex", "endIndex", "indices":
		if lo := e.sliceOffset(base); lo != "" {
			switch sig.base {
			case "startIndex":
				return lo
			case "endIndex":
				return "(" + lo + " + len(" + e.expr(base) + "))"
			}
			return "(swiftrt.Range[int]{Lo: " + lo + ", Hi: " + lo + " + len(" + e.expr(base) + ")})"
		}
	}
	return "swiftrt." + recv + capitalize(sig.base) + "(" + e.recvArg(base) + ")"
}

// stringValueInit names the Go function for a CodingKey type's `init?(stringValue:)`, or "" when it isn't declared.
func (e *fileEmitter) stringValueInit(owner string) string {
	find := func(decls map[string]*DeclInfo) *DeclInfo {
		for _, info := range decls {
			if info.Kind != "constructor_decl" || info.Owner != owner {
				continue
			}
			if ps := info.Node.Child("params").List("params"); len(ps) == 1 && ps[0].Name() == "stringValue" {
				return info
			}
		}
		return nil
	}
	if info := find(e.m.decls); info != nil {
		return info.GoName
	}
	for _, d := range e.m.Deps {
		if info := find(d.decls); info != nil {
			if d.Pkg == e.m.Pkg {
				return info.GoName
			}
			e.m.importPkg(d)
			return d.Pkg + "." + info.GoName
		}
	}
	return ""
}

// recvExpr renders a receiver for field access or a Go method call.
func (e *fileEmitter) recvExpr(base ast.Node) string {
	if base.Kind() == "inout_expr" {
		if ds := stripLoads(base.Child("sub_expr")); ds.Kind() == "subscript_expr" && e.m.Recv(e.m.typeOf(ds.Child("base").Type())) == "Map" {
			// A mutating method on a dictionary element runs on a slot the enclosing call stores back.
			return "(" + e.expr(base) + ")"
		}
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
	if v, ok := e.lvSubst[fmt.Sprintf("%p", n)]; ok {
		return v
	}
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
		if info != nil && info.Kind == "var_decl" && !info.Computed && info.Observed && !info.Static && !e.initializing(info) && !n.Bool("direct_to_storage") {
			if simpleDidSet(info.Node) {
				// A didSet that does not read oldValue observes an in-place modification (SE-0268):
				// the storage is mutated where it is, and the observer runs afterwards.
				stored := e.lvalue(n.Child("base")) + "." + info.GoName
				if call, ok := e.setterCall(n, stored); ok {
					e.post = append(e.post, call)
					return stored
				}
			}
			// Mutating part of an observed property runs its observers: mutate a copy, then set it.
			tmp := e.newTmp("wb")
			e.w("%s := swiftrt.Copy(%s)\n", tmp, e.expr(n))
			if call, ok := e.setterCall(n, tmp); ok {
				e.post = append(e.post, call)
				return tmp
			}
		}
		if info != nil && info.Kind == "var_decl" && !info.Computed {
			if info.Static && info.Lazy {
				return lazyCell(info, pfx)
			}
			if info.Static {
				return pfx + info.GoName
			}
			return e.lvalue(n.Child("base")) + "." + info.GoName
		}
		if usr := n.Child("decl").Str("decl_usr"); info == nil && strings.HasPrefix(usr, "c:@S@") && strings.Contains(usr, "@FI@") {
			return e.lvalue(n.Child("base")) + "." + capitalize(usr[strings.Index(usr, "@FI@")+4:])
		}
		if mp := e.m.mappedDecl(n.Child("decl").Str("decl_usr")); info == nil && mp != nil {
			// A mapped property is reached through its getter and setter: mutate a copy, then set it.
			tmp := e.newTmp("wb")
			e.w("%s := %s\n", tmp, e.expr(n))
			e.post = append(e.post, e.mappedSetter(mp, n.Child("decl"), n.Child("base"), tmp))
			return tmp
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
			if lo := e.sliceOffset(n.Child("base")); lo != "" {
				return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "-" + lo + "]"
			}
			return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "]"
		}
		if len(args) == 1 && e.m.Recv(bt) == "Map" {
			return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "]"
		}
		// A missing key reads as Go's zero value, which serves for a zero default, except that a nil
		// map (an empty dictionary or set) cannot be written into.
		if len(args) == 2 && e.m.Recv(bt) == "Map" && args[1].Str("label") == "default" && isZeroLiteral(args[1].Child("expr")) && !strings.HasPrefix(e.exprGoType(n), "map[") &&
			!isFuncDict(e.m.Recv(e.m.typeOf(n.Type()))) {
			return e.lvalue(n.Child("base")) + "[" + e.expr(args[0].Child("expr")) + "]"
		}
		if len(args) == 2 && e.m.Recv(bt) == "Map" && args[1].Str("label") == "default" {
			m := e.lvalue(n.Child("base"))
			k := e.newTmp("key")
			e.w("%s := %s\nif _, ok := %s[%s]; !ok {\n%s[%s] = %s\n}\n", k, e.expr(args[0].Child("expr")), m, k, m, k, e.defaultValue(args[1].Child("expr")))
			if isFuncDict(e.m.Recv(e.m.typeOf(n.Type()))) {
				// A runtime dictionary is a struct, and a Go map's element cannot be addressed: change a copy,
				// then store it back.
				tmp := e.newTmp("elem")
				e.w("%s := %s[%s]\n", tmp, m, k)
				e.post = append(e.post, m+"["+k+"] = "+tmp)
				return tmp
			}
			return m + "[" + k + "]"
		}
		if recv := e.m.Recv(bt); isFuncDict(recv) && (len(args) == 1 || len(args) == 2 && args[1].Str("label") == "default") {
			// Assigning through a runtime dictionary's element: copy it out, store it back after the statement.
			fn := "swiftrt." + recv
			d := e.lvalue(n.Child("base"))
			k, tmp := e.newTmp("key"), e.newTmp("elem")
			if len(args) == 2 {
				e.w("%s := %s\n%s := %sSubscriptDefault(%s, %s, %s)\n", k, e.expr(args[0].Child("expr")), tmp, fn, d, k, e.defaultValue(args[1].Child("expr")))
				e.post = append(e.post, fn+"Set(&"+d+", "+k+", &"+tmp+")")
				return tmp
			}
			e.w("%s := %s\n%s := %sGet(%s, %s)\n", k, e.expr(args[0].Child("expr")), tmp, fn, d, k)
			e.post = append(e.post, fn+"Set(&"+d+", "+k+", "+tmp+")")
			return "(*" + tmp + ")"
		}
		if e.m.Recv(bt) == "UnsafeMutablePointer" && len(args) == 1 {
			// An element of C memory: read it, let the statement change the copy, then store the copy.
			p, k, tmp := e.newTmp("ptr"), e.newTmp("key"), e.newTmp("elem")
			fn := "swiftrt.UnsafeMutablePointer"
			if pointsToPointer(bt) {
				fn += "Pointer"
			}
			e.w("%s := %s\n%s := %s\n%s := %sSubscriptInt(%s, %s)\n", p, e.expr(n.Child("base")), k, e.expr(args[0].Child("expr")), tmp, fn, p, k)
			e.post = append(e.post, fn+"SetSubscriptInt("+p+", "+k+", "+tmp+")")
			return tmp
		}
		if mp := e.m.mappedDecl(n.Child("decl").Str("decl_usr")); mp != nil {
			return e.mappedSubscriptLvalue(mp, n)
		}
		var labels []string
		for _, a := range args {
			labels = append(labels, a.Str("label")+":"+e.m.Recv(e.m.typeOf(a.Child("expr").Type())))
		}
		return e.m.Unsup("lvalue:subscript:"+e.m.Recv(bt)+"["+strings.Join(labels, ",")+"]", e.where, "")
	case "tuple_element_expr":
		return e.lvalue(n.Child("base")) + ".F" + n.Str("field #")
	case "force_value_expr":
		if ds := stripLoads(n.Child("sub_expr")); ds.Kind() == "subscript_expr" && len(ds.Child("args").List("args")) == 1 {
			if bt := e.m.typeOf(ds.Child("base").Type()); e.m.Recv(bt) == "Map" && bt.Kind == types.Dict && len(bt.Args) == 2 && bt.Args[1].Kind != types.Optional &&
				!e.m.isReferenceGo(bt.Args[1], e.m.GoType(bt.Args[1], e.generics)) {
				// `d[k]!` names the dictionary's own element: MapGet would hand back a copy.
				m, k := e.lvalue(ds.Child("base")), e.newTmp("key")
				e.w("%s := %s\nswiftrt.Force(swiftrt.MapGet(%s, %s))\n", k, e.expr(ds.Child("args").List("args")[0].Child("expr")), m, k)
				return m + "[" + k + "]"
			}
		}
		return "(*" + e.expr(n.Child("sub_expr")) + ")"
	case "discard_assignment_expr":
		return "_"
	case "bind_optional_expr":
		if v, ok := e.opaque[nodeID(n)]; ok {
			return v
		}
	case "call_expr", "dot_syntax_call_expr":
		// Swift only writes through a call result that is a class reference; bind it once so the call runs once.
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind != types.Optional {
			if gt := e.m.GoType(t, e.generics); strings.HasPrefix(gt, "*") && e.m.isReferenceGo(t, gt) {
				tmp := e.newTmp("ref")
				e.w("%s := %s\n", tmp, e.expr(n))
				return tmp
			}
		}
	}
	return e.m.Unsup("lvalue:"+n.Kind(), e.where, "")
}

func (e *fileEmitter) subscriptAssign(dest, src ast.Node) (string, bool) {
	bt := e.m.typeOf(dest.Child("base").Type())
	args := dest.Child("args").List("args")
	if recv := e.m.Recv(bt); isFuncDict(recv) && len(args) == 1 {
		v := e.owned(src, sinkStore)
		switch {
		case src.Kind() == "nil_literal_expr":
			v = "nil"
		case src.Kind() == "inject_into_optional":
			v = "swiftrt.Some(" + e.owned(src.Child("sub_expr"), sinkStore) + ")"
		default:
			if t := e.m.typeOf(src.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 && e.m.isReferenceGo(t.Args[0], e.m.GoType(t.Args[0], e.generics)) {
				v = "swiftrt.SomeRef(" + v + ")"
			}
		}
		return "swiftrt." + recv + "Set(&" + e.lvalue(dest.Child("base")) + ", " + e.expr(args[0].Child("expr")) + ", " + v + ")", true
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
		return m + "[" + k + "] = " + e.owned(src.Child("sub_expr"), sinkStore), true
	}
	if bt.Kind == types.Dict && len(bt.Args) == 2 {
		if vt := e.m.GoType(bt.Args[1], e.generics); bt.Args[1].Kind != types.Optional && e.m.isReferenceGo(bt.Args[1], vt) && vt != "any" {
			// An optional class reference is the pointer itself: nil removes the entry.
			return "swiftrt.MapSetRef(" + m + ", " + k + ", " + e.expr(src) + ")", true
		}
	}
	return "swiftrt.MapSet(" + m + ", " + k + ", " + e.owned(src, sinkStore) + ")", true
}

// synthesizedMember reports whether d names a member the compiler synthesized for a type of the
// transpiled modules, such as the `encode(to:)` of a derived Encodable conformance, which has no
// declaration of its own to emit.
func (e *fileEmitter) synthesizedMember(d ast.Node) bool {
	usr := d.Str("decl_usr")
	if usr == "" || e.m.mappedDecl(usr) != nil || usrModule(usr) == "" {
		return false
	}
	if !slices.ContainsFunc(append([]*Module{e.m}, e.m.Deps...), func(m *Module) bool { return m.Name == usrModule(usr) }) {
		return false
	}
	info, _ := e.lookup(usr)
	return info == nil || info.Node.Bool("implicit") || info.Node.Child("body") == nil
}

// returnsGenericParam reports whether t is a function type, or the Optional of one, whose result is
// a bare generic parameter.
func returnsGenericParam(t *types.Type) bool {
	if t != nil && t.Kind == types.Optional && len(t.Args) == 1 {
		t = t.Args[0]
	}
	return t != nil && t.Kind == types.Func && t.Result != nil && isGenericParam(t.Result)
}

// isFuncDict reports whether a dictionary family is a runtime struct reached only through its
// swiftrt functions, never Go map syntax.
func isFuncDict(recv string) bool { return recv == "OrderedDictionary" || recv == "HMap" }

func (e *fileEmitter) subscript(n ast.Node) string {
	base := n.Child("base")
	for base.Kind() == "inout_expr" {
		base = base.Child("sub_expr")
	}
	args := n.Child("args").List("args")
	d := n.Child("decl")
	if info, pfx := e.lookup(d.Str("decl_usr")); info != nil {
		if info.Kind == "subscript_decl" && !info.FreeSelf() && !info.Static {
			defer e.lendReceiver(base, info)()
			ss, _ := e.argStrings(e.args(n.Child("args")), false)
			return e.recvExpr(base) + "." + info.GoName + "(" + strings.Join(ss, ", ") + ")"
		}
		if info.Kind == "subscript_decl" && info.FreeSelf() && !info.Static {
			// A subscript emitted as a free function, as on a generic struct: self comes first.
			defer e.lendReceiver(base, info)()
			ss, _ := e.argStrings(e.args(n.Child("args")), false)
			return pfx + info.GoName + "(" + strings.Join(append([]string{e.recvArg(base)}, ss...), ", ") + ")"
		}
		return e.m.Unsup("user-subscript", e.where, e.exprGoType(n))
	}
	if mp := e.m.mappedDecl(d.Str("decl_usr")); mp != nil {
		return e.mappedCallExpr(mp, d, e.expr(base), e.args(n.Child("args")), false, false)
	}
	if str := e.scalarViewOf(base); str != nil && len(args) == 1 {
		// A String's unicodeScalars share the String's indices, which are byte offsets here, so they
		// are read from the string and not from a slice of its scalars.
		a := args[0].Child("expr")
		switch at := e.m.demangle(a.Type()); {
		case at == "Swift.String.Index":
			return "swiftrt.StringScalarAt(" + e.expr(str) + ", " + e.expr(a) + ")"
		case strings.HasPrefix(at, "Swift.PartialRangeFrom<Swift.String.Index>"):
			return "swiftrt.StringScalarsFrom(" + e.expr(str) + ", " + e.expr(a) + ")"
		}
	}
	bt := e.m.typeOf(base.Type())
	recv := e.m.Recv(bt)
	if len(args) == 2 && args[1].Str("label") == "default" && (recv == "Map" || isFuncDict(recv)) {
		return "swiftrt." + recv + "SubscriptDefault(" + e.expr(base) + ", " + e.expr(args[0].Child("expr")) + ", " + e.defaultValue(args[1].Child("expr")) + ")"
	}
	if len(args) != 1 {
		return e.m.Unsup("subscript-multi-arg", e.where, e.exprGoType(n))
	}
	a := args[0].Child("expr")
	at := e.m.Recv(e.m.typeOf(a.Type()))
	if at == "Func" || strings.Contains(e.m.demangle(a.Type()), "UnboundedRange") {
		return "swiftrt." + recv + "SubscriptAll(" + e.expr(base) + ")"
	}
	if (recv == "UnsafePointer" || recv == "UnsafeMutablePointer") && at == "Int" && pointsToPointer(bt) {
		// An array of C pointers, such as argv: each element is a pointer, not a C character.
		return "swiftrt.UnsafeMutablePointerPointerSubscriptInt(" + e.expr(base) + ", " + e.expr(a) + ")"
	}
	if (recv == "UnsafePointer" || recv == "UnsafeMutablePointer") && at == "Int" && len(bt.Args) == 1 && bt.Args[0].Name == "Swift.UInt8" {
		// C memory holds C characters; a pointer rebound to UInt8 reads the same bits unsigned.
		return "uint8(swiftrt." + recv + "SubscriptInt(" + e.expr(base) + ", " + e.expr(a) + "))"
	}
	if recv == "Slice" && at == "Int" {
		if lo := e.sliceOffset(base); lo != "" {
			return e.recvExpr(base) + "[" + e.expr(a) + "-" + lo + "]"
		}
		return e.recvExpr(base) + "[" + e.expr(a) + "]"
	}
	if recv == "Slice" && strings.Contains(at, "Range") {
		if lo := e.sliceOffset(base); lo != "" {
			return "swiftrt.ArraySliceSubscript" + at + "(" + e.expr(base) + ", " + lo + ", " + e.expr(a) + ")"
		}
	}
	if recv == "Map" {
		return "swiftrt.MapGet(" + e.expr(base) + ", " + e.expr(a) + ")"
	}
	if isFuncDict(recv) {
		return "swiftrt." + recv + "Get(" + e.expr(base) + ", " + e.expr(a) + ")"
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
	stmt := e.chainStmt
	e.chainStmt = false
	defer func() { e.chainStmt = stmt }()
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
	// Swift marks the collection a bound subscript reads from as inout even when the chain only reads.
	access := map[string]bool{}
	for _, b := range binds {
		if ds := stripLoads(b.Child("sub_expr")); ds.Kind() == "subscript_expr" && ds.Child("base").Kind() == "inout_expr" {
			access[nodeID(ds.Child("base"))] = true
		}
	}
	mutates := false
	ast.Walk(n.Child("sub_expr"), func(x ast.Node) {
		if x.Kind() == "assign_expr" || x.Kind() == "inout_expr" && !access[nodeID(x)] {
			mutates = true
		}
	})
	for _, b := range binds {
		v := e.newTmp("bound")
		sub := b.Child("sub_expr")
		// `dict[k]?.field = v` mutates the dictionary entry in Swift; the Get functions hand back a copy, so write it back.
		if ds := stripLoads(sub); mutates && ds.Kind() == "subscript_expr" && len(ds.Child("args").List("args")) == 1 {
			recv := e.m.Recv(e.m.typeOf(ds.Child("base").Type()))
			vt := e.m.typeOf(sub.Type())
			if (recv == "Map" || isFuncDict(recv)) && vt != nil && vt.Kind == types.Optional && len(vt.Args) == 1 && !e.m.isReferenceGo(vt.Args[0], e.m.GoType(vt.Args[0], e.generics)) {
				if m, ok := e.dictRoot(stripLoads(ds.Child("base")), &pre); ok {
					k := e.newTmp("key")
					fmt.Fprintf(&pre, "%s := %s\n%s := swiftrt.%sGet(%s, %s)\nif %s == nil { %s }\n", k, e.expr(ds.Child("args").List("args")[0].Child("expr")), v, recv, m, k, v, chainReturn)
					if recv == "Map" {
						fmt.Fprintf(&post, "%s[%s] = *%s\n", m, k, v)
					} else {
						fmt.Fprintf(&post, "swiftrt.%sSet(&%s, %s, %s)\n", recv, m, k, v)
					}
					e.opaque[nodeID(b)] = "(*" + v + ")"
					continue
				}
			}
		}
		fmt.Fprintf(&pre, "%s := %s\nif %s == nil { %s }\n", v, e.expr(sub), v, chainReturn)
		u := e.unwrap(v, sub)
		if strings.HasPrefix(u, "*") {
			u = "(" + u + ")"
		}
		e.opaque[nodeID(b)] = u
	}
	rt := e.exprGoType(n)
	// Statements the body emits (write-back temporaries) belong inside the closure, after the binds.
	savedB, savedPost := e.b, e.post
	e.b, e.post = strings.Builder{}, nil
	body := e.expr(n.Child("sub_expr"))
	pre.WriteString(e.b.String())
	for _, p := range e.post {
		post.WriteString(p + "\n")
	}
	e.b, e.post = savedB, savedPost
	if rt == "" || rt == "*" || rt == "*struct{}" && stmt {
		return "func() { " + strings.ReplaceAll(pre.String(), chainReturn, "return") + body + "\n" + post.String() + " }()"
	}
	if rt == "*struct{}" {
		// A `()?` value: nil when the chain stopped at a nil, else the empty tuple.
		return "func() *struct{} {\n" + strings.ReplaceAll(pre.String(), chainReturn, "return nil") + body + "\n" + post.String() + "return &struct{}{}\n}()"
	}
	if opt := e.m.inlineOptGo(e.m.typeOf(n.Type()), e.generics); opt != "" {
		// The closure returns the value inline, so a value it makes need not escape to the heap.
		chainPre := strings.ReplaceAll(pre.String(), chainReturn, "return "+opt+"{}")
		if post.Len() > 0 {
			return e.optPtr("func() " + opt + " {\n" + chainPre + "res := " + e.inlineOpt(opt, body) + "\n" + post.String() + "return res\n}()")
		}
		return e.optPtr("func() " + opt + " {\n" + chainPre + "return " + e.inlineOpt(opt, body) + "\n}()")
	}
	chainPre := strings.ReplaceAll(pre.String(), chainReturn, "return nil")
	if post.Len() > 0 {
		return "func() " + rt + " {\n" + chainPre + "res := " + body + "\n" + post.String() + "return res\n}()"
	}
	return "func() " + rt + " {\n" + chainPre + "return " + body + "\n}()"
}

// dictRoot renders a dictionary an optional chain writes back into: an assignable path, or a stored
// property of a class reference, whose reference it binds once into pre.
func (e *fileEmitter) dictRoot(n ast.Node, pre *strings.Builder) (string, bool) {
	if e.assignable(n) {
		return e.lvalue(n), true
	}
	if n.Kind() != "member_ref_expr" {
		return "", false
	}
	info, _ := e.lookup(n.Child("decl").Str("decl_usr"))
	if info == nil || info.Kind != "var_decl" || info.Computed || info.Static || info.Observed {
		return "", false
	}
	base := stripLoads(n.Child("base"))
	bt := e.m.typeOf(base.Type())
	if bt == nil || bt.Kind == types.Optional {
		return "", false
	}
	if gt := e.m.GoType(bt, e.generics); !strings.HasPrefix(gt, "*") || !e.m.isReferenceGo(bt, gt) {
		return "", false
	}
	ref := e.newTmp("ref")
	fmt.Fprintf(pre, "%s := %s\n", ref, e.expr(base))
	return ref + "." + info.GoName, true
}

func (e *fileEmitter) tuple(n ast.Node) string {
	els := n.List("elements")
	if len(els) == 0 {
		return "struct{}{}"
	}
	var parts []string
	e.pinOperands(els...)
	for _, el := range els {
		parts = append(parts, e.expr(el))
	}
	return "(" + e.exprGoType(n) + "{" + strings.Join(parts, ", ") + "})"
}

// tupleFor emits a tuple whose elements go into sink s.
func (e *fileEmitter) tupleFor(n ast.Node, s sink) string {
	els := n.List("elements")
	if len(els) == 0 {
		return "struct{}{}"
	}
	var parts []string
	for _, el := range els {
		parts = append(parts, e.owned(el, s))
	}
	return "(" + e.exprGoType(n) + "{" + strings.Join(parts, ", ") + "})"
}

func (e *fileEmitter) closure(n ast.Node) string {
	saveR, saveT, saveI, saveDo := e.resultType, e.throws, e.inInit, e.doRet
	e.doRet = nil
	saveIO := map[string]bool{}
	maps.Copy(saveIO, e.inoutParams)
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
		} else if e.inoutParams[p.Name()] {
			// A parameter that shadows an outer inout one (`$0` in nested closures) is a plain value.
			e.inoutParams = maps.Clone(e.inoutParams)
			delete(e.inoutParams, p.Name())
		}
		params = append(params, name+" "+pt)
	}
	res := ""
	if ft.Kind == types.Func {
		res = e.m.GoType(ft.Result, e.generics)
	}
	captures := e.captureValues(n)
	e.resultType = res
	e.inInit = false
	saved := e.b
	e.b = strings.Builder{}
	e.w("func(%s) %s {\n", strings.Join(params, ", "), res)
	body := n.Child("body")
	e.paramNames = map[string]bool{}
	for _, p := range n.Child("params").List("params") {
		e.paramNames[goSafe(p.Name())] = true
	}
	saveTop := e.topLevel
	e.topLevel = map[uintptr]bool{}
	for _, st := range body.List("elements") {
		e.topLevel[nodeKey(st)] = true
	}
	defer func() { e.topLevel = saveTop }()
	e.stmts(body.List("elements"))
	if res != "" && !endsWithReturn(body) {
		e.w("panic(\"unreachable\")\n")
	}
	e.w("}")
	s := e.b.String()
	e.b = saved
	if len(captures) > 0 {
		// The closure's own copies of the constants it captures, made when it is.
		s = "func() func(" + strings.Join(params, ", ") + ") " + res + " {\n" + strings.Join(captures, "") + "return " + s + "\n}()"
	}
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
		if meta || (isIdentityCapture(c) && !capturesVar(c)) {
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
	b := e.expr(body)
	if f, ok := e.methodRef(body); ok {
		b = f
	}
	return "func() " + e.exprGoType(body) + " {\n" + binds + "return " + b + "\n}()"
}

// methodRef renders a method reference (`Clock.tick`, `tick`), which Swift represents as a
// zero-parameter autoclosure around the call, as a function value.
func (e *fileEmitter) methodRef(ac ast.Node) (string, bool) {
	if ac.Kind() != "autoclosure_expr" || len(ac.Child("params").List("params")) != 0 {
		return "", false
	}
	ft := e.m.typeOf(ac.Type())
	if ft == nil || ft.Kind != types.Func {
		return "", false
	}
	if res := e.m.GoType(ft.Result, e.generics); res != "" && !isVoidGo(res) {
		return "func() " + res + " { return " + e.expr(ac) + " }", true
	}
	return "func() { " + e.expr(ac) + " }", true
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
	pre := ""
	for i, c := range n.List("components") {
		switch c.Kind() {
		case "tuple_element":
			acc = derefParen(acc) + ".F" + c.Str("index")
			continue
		case "identity":
			continue
		case "optional_force":
			acc = "(*" + acc + ")"
			continue
		case "optional_chain":
			// `\.a?.b`: the path ends in nil when a is nil, and goes on from a's value otherwise.
			tmp := "opt" + itoa(i)
			pre += tmp + " := " + acc + "\nif " + tmp + " == nil {\nreturn nil\n}\n"
			acc = "(*" + tmp + ")"
			if ct := e.m.typeOf(c.Type()); ct != nil && e.m.isReferenceGo(ct, e.m.GoType(ct, e.generics)) {
				acc = tmp
			}
			continue
		case "optional_wrap":
			if ct := e.m.typeOf(c.Type()); ct != nil && ct.Kind == types.Optional && len(ct.Args) == 1 {
				if inner := e.m.GoType(ct.Args[0], e.generics); !e.m.isReferenceGo(ct.Args[0], inner) {
					acc = "swiftrt.Some[" + inner + "](" + acc + ")"
				}
			}
			continue
		case "subscript":
			// An array element by index: `\.words[0]`.
			if args := c.Child("args").List("args"); c.Child("decl").Str("decl_usr") == "s:SayxSicip" && len(args) == 1 {
				acc = acc + "[" + e.expr(args[0].Child("expr")) + "]"
				continue
			}
		}
		if c.Kind() != "member" {
			return e.m.Unsup("keypath-component:"+c.Kind(), e.where, "func("+root+") "+val)
		}
		info, pfx := e.lookup(c.Child("decl").Str("decl_usr"))
		if mp := e.m.mappedDecl(c.Child("decl").Str("decl_usr")); info == nil && mp != nil {
			sig := parseDeclSig(e.m.demangle(c.Child("decl").Str("decl_usr")))
			acc = e.m.mappedPropertyName(mp, e.mappedName(mp, sig, nil, false)) + "(" + acc + ")"
			continue
		}
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
	get := "func(root " + root + ") " + val + " {\n" + pre + "return " + acc + "\n}"
	if pre == "" {
		get = "func(root " + root + ") " + val + " { return " + acc + " }"
	}
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
		info, _ := e.lookup(c.Child("decl").Str("decl_usr"))
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
			acc += "." + info.GoName
		}
	}
	return "func(root *" + root + ", v " + val + ") { " + acc + " = v }"
}

func (e *fileEmitter) autoclosureFunc(n ast.Node, ps []ast.Node) string {
	ft := e.m.typeOf(n.Type())
	var params []string
	saveIO := e.inoutParams
	defer func() { e.inoutParams = saveIO }()
	for _, p := range ps {
		params = append(params, goSafe(p.Name())+" "+e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics))
		if e.inoutParams[p.Name()] {
			// The parameter (a curry thunk's self) shadows an outer inout one.
			e.inoutParams = maps.Clone(e.inoutParams)
			delete(e.inoutParams, p.Name())
		}
	}
	res := ""
	if ft.Kind == types.Func {
		res = e.m.GoType(ft.Result, e.generics)
	}
	inner := n.Child("single_expr_body")
	body := e.expr(inner)
	if inner.Kind() == "autoclosure_expr" && len(ps) > 0 && len(inner.Child("params").List("params")) == 0 {
		// A curry thunk of a method without parameters: applying it to self yields a function.
		body = e.autoclosureFunc(inner, nil)
	}
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
		key := owner
		if key == "" {
			key = "local:" + n.Child("default_args_owner").Str("base_name")
		}
		if fn, ok := e.localFuncs[key]; ok {
			info = &DeclInfo{Node: fn, Kind: fn.Kind()}
		}
	}
	if info == nil {
		return e.m.Unsup("foreign-default-arg", e.where, e.exprGoType(n))
	}
	params := info.Node.Child("params").List("params")
	// An empty literal is typed in the callee's generic context; the call site knows the type.
	emptyAtCallSite := func(ex ast.Node) (string, bool) {
		if (ex.Kind() == "array_expr" || ex.Kind() == "dictionary_expr") && len(ex.List("elements")) == 0 {
			if gt := e.exprGoType(n); gt != "" && gt != "any" {
				return "(" + gt + "{})", true
			}
		}
		return "", false
	}
	if idx < len(params) {
		if ex := params[idx].Child("expression"); ex != nil {
			if s, ok := emptyAtCallSite(ex); ok {
				return s
			}
			if inner := stripLoads(ex); (inner.Kind() == "dot_self_expr" || inner.Kind() == "type_expr") && namesGeneric(e.m.typeOf(inner.Type())) {
				// `as type: T.Type = T.self` names the callee's parameter; the call site knows the type.
				if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Meta && len(t.Args) == 1 {
					if inner := e.m.GoType(t.Args[0], e.generics); inner != "" {
						return "swiftrt.TypeOf[" + inner + "]()"
					}
				}
			}
			if ex.Kind() == "magic_identifier_literal_expr" {
				// `line: Int = #line` takes the caller's line, file and function.
				return e.magicLiteralAt(ex.Str("kind"), n)
			}
			if body := ex.Child("single_expr_body"); ex.Kind() == "autoclosure_expr" && len(ex.Child("params").List("params")) == 0 && body.Kind() == "call_expr" && body.Implicit() {
				// A function named as the default (`= Type.method`) arrives as a thunk that calls it.
				return e.autoclosureFunc(ex, nil)
			}
			return e.expr(ex)
		}
		if info.Kind == "constructor_decl" && info.Node.Implicit() {
			if init := e.m.initialValue(info.Owner + "|" + params[idx].Name()); init != nil {
				if s, ok := emptyAtCallSite(init); ok {
					return s
				}
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
			if !strings.HasPrefix(inner, "*") {
				// An optional whose Go form is itself nil-able (an interface or closure).
				return "swiftrt.TryNil(func() " + inner + " { return " + e.expr(sub) + " })"
			}
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
	case "expect", "require":
		if len(as) == 0 {
			break
		}
		fatal := name == "require"
		if as[0].label == "throws" && len(as) >= 2 {
			return e.expectThrows(as, fatal)
		}
		cond := as[0].node
		msg := `""`
		if len(as) > 1 && as[1].label == "" && as[1].node.Kind() != "default_argument_expr" {
			msg = "swiftrt.Str(" + e.expr(as[1].node) + ")"
		}
		ct := e.m.typeOf(cond.Type())
		if ct.Kind == types.Optional {
			if len(ct.Args) == 1 && e.m.isReferenceGo(ct.Args[0], e.m.GoType(ct.Args[0], e.generics)) {
				return "swiftrt.RequireRef(" + e.expr(cond) + ", " + msg + ")"
			}
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
	if name == "_sourceLocation" {
		return "swiftrt.SourceLocationHere()"
	}
	// Any other expression macro is emitted from the expansion the compiler recorded.
	if r := n.Child("rewritten"); r != nil {
		return e.expr(r)
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
	argSink, storeInto := e.argSink, e.storeInto
	e.argSink, e.storeInto = nil, nil
	var params []ast.Node
	if callee != nil && callee.Node != nil {
		params = callee.Node.Child("params").List("params")
	}
	ops := make([]ast.Node, len(as))
	for i, a := range as {
		ops[i] = a.node
	}
	e.pinOperands(ops...)
	for i, a := range as {
		if foreign && a.node.Kind() == "default_argument_expr" {
			continue
		}
		lazy := false
		if i < len(params) && params[i].Bool("autoclosure") && !foreign {
			if ac := stripConv(a.node); ac != nil && ac.Kind() == "autoclosure_expr" && len(ac.Child("params").List("params")) == 0 {
				if ft := e.m.typeOf(ac.Type()); ft != nil && ft.Kind == types.Func {
					lazy = true
				}
			}
		}
		var s string
		if lazy {
			// The argument is evaluated when the callee calls it: keep its statements inside the closure.
			ft := e.m.typeOf(stripConv(a.node).Type())
			saved, savedPost := e.b, e.post
			e.b, e.post = strings.Builder{}, nil
			body := e.expr(a.node)
			pre, post := e.b.String(), ""
			for _, p := range e.post {
				post += p + "\n"
			}
			e.b, e.post = saved, savedPost
			if res := e.m.GoType(ft.Result, e.generics); res != "" && !isVoidGo(res) {
				if post == "" {
					s = "func() " + res + " {\n" + pre + "return " + body + "\n}"
				} else {
					s = "func() " + res + " {\n" + pre + "res := " + body + "\n" + post + "return res\n}"
				}
			} else {
				s = "func() {\n" + pre + body + "\n" + post + "}"
			}
		} else {
			switch {
			case a.node.Kind() == "inout_expr":
				s = e.expr(a.node)
			case argSink != nil && *argSink == sinkStore:
				s = e.stored(a.node, storeInto)
			case argSink != nil:
				s = e.part(a.node, *argSink)
			case foreign:
				s = e.expr(a.node)
			case callee != nil && callee.Node != nil && !e.m.mayWrite(callee.Node.Str("usr")):
				// Nothing the callee runs can change the place the argument was read from.
				s = e.expr(a.node)
			default:
				s = e.owned(a.node, sinkBorrow)
			}
		}
		if i < len(params) && !foreign {
			// A closure for a generic `(…) -> T?` parameter, with T a class, returns the class pointer;
			// the generic signature wants a pointer to it.
			if pt := e.m.typeOf(params[i].Str("interface_type")); pt != nil && pt.Kind == types.Func && pt.Result != nil && pt.Result.Kind == types.Optional && len(pt.Result.Args) == 1 && isGenericParam(pt.Result.Args[0]) && len(pt.Args) <= 2 {
				if at := e.m.typeOf(a.node.Type()); at != nil && at.Kind == types.Func && at.Result != nil && at.Result.Kind == types.Optional && len(at.Result.Args) == 1 && at.Result.Args[0].Kind != types.Optional && e.m.isReferenceGo(at.Result.Args[0], e.m.GoType(at.Result.Args[0], e.generics)) {
					s = "swiftrt.LiftOpt" + itoa(len(pt.Args)) + "(" + s + ")"
				}
			}
		}
		if i < len(params) && !foreign {
			// A class reference or existential passed for a generic `T?` parameter is the reference itself in Go,
			// from which Go would infer T as the class's struct: pass a pointer to the reference instead.
			if pt := e.m.typeOf(params[i].Str("interface_type")); pt != nil && pt.Kind == types.Optional && len(pt.Args) == 1 && isGenericParam(pt.Args[0]) {
				if at := e.m.typeOf(a.node.Type()); at != nil && at.Kind == types.Optional && len(at.Args) == 1 && !isGenericParam(at.Args[0]) && at.Args[0].Kind != types.Optional {
					if gt := e.m.GoType(at.Args[0], e.generics); e.m.isReferenceGo(at.Args[0], gt) || gt == "error" {
						s = "swiftrt.SomeRef(" + s + ")"
					}
				}
			}
		}
		if i < len(params) && !foreign && callee != nil && callee.Node != nil {
			// [Concrete] passed for [S] where S: LocalProtocol, which is a slice of the protocol's interface.
			if pt := e.m.typeOf(params[i].Str("interface_type")); pt != nil && pt.Kind == types.Array && len(pt.Args) == 1 && isGenericParam(pt.Args[0]) {
				// A static or free function keeps S as a type parameter, so the slice fits as it is.
				keeps := (callee.Static || callee.Owner == "" || callee.FreeSelf()) && callee.Node.Kind() == "func_decl" && !callee.Node.Bool("local") && nestedInParamTypes(e.m, callee.Node, pt.Args[0].Name)
				if iface := protocolBoundOf(e.m, callee.Node, pt.Args[0]); iface != "" && !keeps {
					if at := e.exprGoType(a.node); strings.HasPrefix(at, "[]") && at != "[]"+iface {
						s = "swiftrt.SliceMap(" + s + ", func(v " + at[2:] + ") " + iface + " { return v })"
					}
				}
			}
			// A metatype passed for `C.Type` where the callee knows C only as its protocol: the value
			// that stands for the type.
			if pt := e.m.typeOf(params[i].Str("interface_type")); pt != nil && pt.Kind == types.Meta && len(pt.Args) == 1 && pt.Args[0].Kind == types.Generic && erasesMetaGeneric(e.m, callee.Node, pt.Args[0].Name) {
				if at := e.m.typeOf(a.node.Type()); at != nil && at.Kind == types.Meta && len(at.Args) == 1 {
					inner := at.Args[0]
					gt := e.m.GoType(inner, e.generics)
					switch {
					case inner.Kind == types.Named && !e.m.isLocalProtocol(inner.Name) && e.m.HasMutatingMethods(inner.Name):
						s = "new(" + gt + ")"
					case inner.Kind == types.Named && !e.m.isLocalProtocol(inner.Name):
						s = "*new(" + gt + ")"
					case inner.Kind == types.Generic && slices.Contains(e.typeParams, gt):
						s = "*new(" + gt + ")"
					default:
						s = e.metaReceiver(a.node)
					}
				}
			}
			// A Substring passed for a StringProtocol parameter, which is a Go string.
			if pt := e.m.typeOf(params[i].Str("interface_type")); isGenericParam(pt) && paramConformsTo(e.m, callee.Node, pt, "Swift.StringProtocol") && e.m.Recv(e.m.typeOf(a.node.Type())) == "Substring" {
				s += ".String()"
			}
			// A Set passed to a generic Sequence parameter, which is a slice here.
			if pt := e.m.typeOf(params[i].Str("interface_type")); isGenericParam(pt) && sequenceParam(e.m, callee.Node, pt) {
				if at := e.m.typeOf(a.node.Type()); at != nil && e.m.Recv(at) == "Set" && len(at.Args) == 1 {
					s = "swiftrt.Iterate[" + e.m.GoType(at.Args[0], e.generics) + "](" + s + ")"
				}
			}
		}
		if ft := e.m.typeOf(a.node.Type()); foreign && ft != nil && ft.Kind == types.Func && len(ft.Args) == 1 && ft.Args[0].Kind == types.Tuple && len(ft.Args[0].Args) > 1 {
			// A closure, or an operator or function reference (`contains(where: ==)`), taking the tuple's
			// elements as separate parameters.
			if cl := stripConv(a.node); cl != nil && (cl.Kind() == "closure_expr" && len(cl.Child("params").List("params")) == len(ft.Args[0].Args) || isOperatorRef(cl) && strings.HasPrefix(s, "func(")) {
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
			case !foreign && i < len(params) && returnsGenericParam(e.m.typeOf(params[i].Str("interface_type"))):
				// `writeTx { db in … }` with the body's result type parameter bound to Void: the Go
				// function returns struct{}, so Go infers the parameter.
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
	if lhs := fn.Child("lhs"); fn.Kind() == "dot_syntax_base_ignored" && lhs.Kind() == "declref_expr" && lhs.Type() == "" {
		// A module-qualified function (`Swift.print`) is the function itself.
		fn = stripConv(fn.Child("rhs"))
	}
	as := e.args(n.Child("args"))
	if d := fn.Child("decl"); fn.Kind() == "declref_expr" && d.Str("decl_usr") == "" {
		if lg, ok := e.localGenerics[d.Str("base_name")]; ok {
			ss, _ := e.argStrings(as, false)
			var caps []string
			for _, c := range lg.captures {
				if e.inoutParams[c] {
					caps = append(caps, c)
				} else {
					caps = append(caps, "&"+c)
				}
			}
			return lg.goName + "(" + strings.Join(append(caps, ss...), ", ") + ")"
		}
	}
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
			ref := e.declRef(d, fn)
			if info != nil && (info.Static || info.Owner == "") && !strings.HasSuffix(ref, "]") {
				ref += e.uninferableTypeArgs(info, d)
			}
			return ref + "(" + strings.Join(ss, ", ") + ")"
		}
		dem := e.m.demangle(usr)
		if strings.Contains(dem, "(in ") {
			ss, _ := e.argStrings(as, false)
			return goSafe(d.Str("base_name")) + "(" + strings.Join(ss, ", ") + ")"
		}
		if mp := e.m.mappedDecl(usr); mp != nil {
			return e.mappedCallExpr(mp, d, "", as, false, false)
		}
		sig := parseDeclSig(dem)
		if sig.base == "withTaskCancellationHandler" {
			// The operation's result is the call's type parameter, Void as often as not.
			e.unitCallee = true
		}
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
		if sig.base == "zip" {
			// zip takes any two sequences; a String or Substring zips its Characters.
			for i, a := range kept {
				if r := e.m.Recv(e.m.typeOf(a.node.Type())); i < len(ss) && (r == "String" || r == "Substring") {
					ss[i] = "swiftrt.Iterate[swiftrt.Character](" + ss[i] + ")"
				}
			}
		}
		fn := capitalize(sig.base) + e.suffix(sig, kept, true)
		call := "swiftrt." + fn + e.mappedResultTypeArg(fn, dem, n, true) + "(" + strings.Join(ss, ", ") + ")"
		switch sig.base {
		case "withCheckedThrowingContinuation", "withCheckedContinuation", "withThrowingTaskGroup", "withTaskGroup":
			// The runtime's continuation and task group carry any; the call's Swift type says what it
			// produces.
			if gt := e.exprGoType(n); gt != "" && gt != "any" && gt != "struct{}" {
				return "swiftrt.MustAs[" + gt + "](" + call + ")"
			}
		}
		return call
	}
	ss, _ := e.argStrings(as, false)
	f := e.expr(fn)
	if fn.Kind() == "closure_expr" {
		f = "(" + f + ")"
	}
	return f + "(" + strings.Join(ss, ", ") + ")"
}

func (e *fileEmitter) payloadCtor(n ast.Node, info *DeclInfo, pfx string, as []arg) string {
	e.argSink = e.partsSink(n)
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
		store := sinkStore
		e.argSink = &store
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
	if str := e.scalarViewOf(base); str != nil && d.Str("base_name") == "index" && len(as) == 1 && as[0].label == "after" {
		// The scalar view's indices are the String's.
		return "swiftrt.StringScalarIndexAfter(" + e.expr(str) + ", " + e.expr(as[0].node) + ")"
	}
	if base != nil && d.Str("base_name") == "encode" && len(as) == 1 && as[0].label == "to" && e.synthesizedMember(d) {
		ss, _ := e.argStrings(as, false)
		return "swiftrt.EncodeInto(" + e.expr(base) + ", " + ss[0] + ")"
	}
	info, pfx := e.lookup(usr)
	if info != nil {
		defer e.lendReceiver(base, info)()
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
			return pfx + info.GoName + e.uninferableTypeArgs(info, d) + "(" + strings.Join(ss, ", ") + ")"
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
			targs := ""
			if !info.ProtoExt {
				targs = e.uninferableTypeArgs(info, d)
			}
			return pfx + info.GoName + targs + "(" + strings.Join(append([]string{recv}, ss...), ", ") + ")"
		default:
			call := e.recvExpr(base) + "." + e.superTarget(info, base) + "(" + strings.Join(ss, ", ") + ")"
			if info.OwnerKind != "protocol" && !info.ProtoExt {
				// A witness for a requirement typed by an associated type returns any.
				if _, erased := e.m.assocErasure(info.Node, info.Owner); erased {
					if gt := e.exprGoType(n); gt != "" && !strings.HasPrefix(gt, "any") {
						return "swiftrt.MustAs[" + gt + "](" + call + ")"
					}
				}
			}
			return call
		}
	}
	if base != nil {
		switch d.Str("base_name") {
		case "firstIndex", "lastIndex", "index", "startIndex", "endIndex":
			if lo := e.sliceOffset(base); lo != "" {
				out := e.methodCallForeign(n, d, base, as)
				if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional {
					return "swiftrt.OffsetIndex(" + out + ", " + lo + ")"
				}
				return "(" + out + " + " + lo + ")"
			}
		}
	}
	return e.methodCallForeign(n, d, base, as)
}

func (e *fileEmitter) methodCallForeign(n ast.Node, d ast.Node, base ast.Node, as []arg) string {
	if base != nil && base.Kind() != "inout_expr" {
		// A function the library calls back may change the place the receiver was read from.
		var called []any
		for _, a := range as {
			if t := e.m.typeOf(a.node.Type()); t != nil && t.Kind == types.Func {
				called = append(called, map[string]any(a.node))
			}
		}
		if _, done := e.snapshots[nodeKey(base)]; len(called) > 0 && !done && e.snapshotNeeded(base, called...) {
			if e.snapshots == nil {
				e.snapshots = map[uintptr]string{}
			}
			e.snapshots[nodeKey(base)] = "swiftrt.Copy(" + e.expr(base) + ")"
			defer delete(e.snapshots, nodeKey(base))
		}
	}
	usr := d.Str("decl_usr")
	dem := e.m.demangle(usr)
	sig := parseDeclSig(dem)
	if mp := e.m.mappedDecl(usr); mp != nil {
		if base == nil || base.Kind() == "type_expr" || e.m.typeOf(base.Type()).Kind == types.Meta {
			return e.mappedCallExpr(mp, d, "", as, true, false)
		}
		return e.mappedCallExpr(mp, d, e.recvArg(base), as, sig.static, false)
	}
	if b := d.Str("base_name"); strings.HasPrefix(b, "withLock") || strings.HasPrefix(b, "withValue") || b == "sync" || b == "withCriticalRegion" || b == "withCString" || strings.HasPrefix(b, "withUnsafe") {
		e.unitCallee = true
	}
	if strings.HasPrefix(d.Str("base_name"), "withValue") && strings.Contains(sig.owner, "TaskLocal") && len(as) > 0 {
		// A concrete value bound to a protocol-typed task local: Go infers the task local's type from both
		// arguments, so the value is converted to the interface.
		v := as[0].node
		for v != nil && v.Kind() == "inject_into_optional" {
			v = v.Child("sub_expr")
		}
		if v != nil && v.Kind() == "erasure_expr" {
			if e.typedErasure == nil {
				e.typedErasure = map[uintptr]bool{}
			}
			e.typedErasure[nodeKey(v)] = true
		}
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
	if bt := e.m.typeOf(base.Type()); base != nil && d.Str("base_name") == "flatMap" && len(as) == 1 && bt != nil && e.m.Recv(bt) == "Slice" {
		if ft := e.m.typeOf(as[0].node.Type()); ft != nil && ft.Kind == types.Func && ft.Result != nil && e.m.Recv(ft.Result) == "Set" {
			ss, _ := e.argStrings(as, false)
			return "swiftrt.SliceFlatMapSet(" + e.expr(base) + ", " + ss[0] + ")"
		}
	}
	if bt := e.m.typeOf(base.Type()); base != nil && d.Str("base_name") == "map" && len(as) == 1 && bt != nil && bt.Kind == types.Optional {
		if ft := e.m.typeOf(as[0].node.Type()); ft != nil && ft.Kind == types.Func && (ft.Result == nil || isVoidGo(e.m.GoType(ft.Result, e.generics))) {
			// optional.map { sideEffect($0) } runs the closure for its effect.
			ss, _ := e.argStrings(as, false)
			if len(bt.Args) == 1 && e.m.isReferenceGo(bt.Args[0], e.m.GoType(bt.Args[0], e.generics)) {
				// A reference's Optional is the reference itself.
				return "swiftrt.OptDoRef(" + e.expr(base) + ", " + ss[0] + ")"
			}
			return "swiftrt.OptDo(" + e.expr(base) + ", " + ss[0] + ")"
		}
	}
	if base != nil && d.Str("base_name") == "encode" && len(as) == 1 && as[0].label == "to" && isGenericParam(e.m.typeOf(base.Type())) {
		ss, _ := e.argStrings(as, false)
		return "swiftrt.EncodeInto(" + e.expr(base) + ", " + ss[0] + ")"
	}
	if base != nil && d.Str("base_name") == "filter" && len(as) == 1 && sig.owner == "Swift.Sequence" {
		// Sequence.filter (chosen over Set.filter when the context wants an Array) returns an Array.
		if bt := e.m.typeOf(base.Type()); bt != nil && e.m.Recv(bt) == "Set" && len(bt.Args) == 1 {
			ss, _ := e.argStrings(as, false)
			return "swiftrt.SliceFilter(swiftrt.Iterate[" + e.m.GoType(bt.Args[0], e.generics) + "](" + e.expr(base) + "), " + ss[0] + ")"
		}
	}
	refSuffix := ""
	if bt := e.m.typeOf(base.Type()); base != nil && len(as) == 0 && (d.Str("base_name") == "max" || d.Str("base_name") == "min") && bt != nil && len(bt.Args) == 1 && (e.m.Recv(bt) == "Slice" || e.m.Recv(bt) == "Set") {
		if less := e.lessFunc(bt.Args[0]); less != "" {
			return "swiftrt.Slice" + capitalize(d.Str("base_name")) + "By(swiftrt.Iterate[" + e.m.GoType(bt.Args[0], e.generics) + "](" + e.expr(base) + "), " + less + ")"
		}
	}
	if bt := e.m.typeOf(base.Type()); base != nil && len(as) == 0 && d.Str("base_name") == "sorted" && bt != nil && len(bt.Args) == 1 && (e.m.Recv(bt) == "Slice" || e.m.Recv(bt) == "Set") {
		// sorted() orders by the element type's own `<`.
		if less := e.lessFunc(bt.Args[0]); less != "" && less != "swiftrt.Int128Less" {
			return "swiftrt.SliceSortedBy(swiftrt.Iterate[" + e.m.GoType(bt.Args[0], e.generics) + "](" + e.expr(base) + "), " + less + ")"
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
	if base != nil && base.Kind() == "inout_expr" || d.Str("base_name") == "reduce" {
		// A mutating library call keeps its arguments in the receiver; reduce(into:) mutates its first.
		store := sinkStore
		e.argSink = &store
		if base != nil && base.Kind() == "inout_expr" {
			e.storeInto = e.m.typeOf(base.Child("sub_expr").Type())
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
		fn := lastComponent(owner) + capitalize(sig.base) + e.suffix(sig, kept, false)
		if fn == "OptSome" && len(ss) == 1 {
			// `.some(x)`: name the wrapped type, which Go cannot infer from an argument such as nil.
			if rt := e.m.typeOf(n.Type()); rt != nil && rt.Kind == types.Optional && len(rt.Args) == 1 {
				inner := e.m.GoType(rt.Args[0], e.generics)
				if rt.Args[0].Kind != types.Optional && e.m.isReferenceGo(rt.Args[0], inner) {
					// A class reference is its own Optional.
					return ss[0]
				}
				return "swiftrt.OptSome[" + inner + "](" + ss[0] + ")"
			}
		}
		return "swiftrt." + fn + e.mappedResultTypeArg(fn, dem, n, true) + "(" + strings.Join(ss, ", ") + ")"
	}
	bt := e.m.typeOf(base.Type())
	recv := e.recvOf(bt)
	if bt != nil && bt.Kind == types.Named && e.m.isTranspiledType(bt.Name) && recv == lastComponent(bt.Name) && sig.owner != "" && !e.m.isTranspiledType(sig.owner) {
		// A stdlib protocol extension method on a transpiled conforming type is the runtime's
		// function for the protocol (`executor.asUnownedTaskExecutor()` is TaskExecutor's).
		recv = lastComponent(sig.owner)
	}
	if s, ok := e.nativeMethod(recv, sig.base, base, kept, ss); ok {
		return s
	}
	if (recv == "TaskGroup" || recv == "ThrowingTaskGroup") && sig.base == "next" {
		// The runtime's group carries child results as any; the call's Swift type says what they are.
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 {
			if inner := e.m.GoType(t.Args[0], e.generics); inner != "" && inner != "any" && inner != "struct{}" {
				call := "swiftrt." + recv + "Next(" + e.recvArg(base) + ")"
				if e.m.isReferenceGo(t.Args[0], inner) {
					return "swiftrt.OptAnyAsRef[" + inner + "](" + call + ")"
				}
				return "swiftrt.OptAnyAs[" + inner + "](" + call + ")"
			}
		}
	}
	fn := recv + capitalize(sig.base) + e.suffix(sig, kept, false) + refSuffix
	return "swiftrt." + fn + e.mappedResultTypeArg(fn, dem, n, false) + "(" + strings.Join(append([]string{e.recvArg(base)}, ss...), ", ") + ")"
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
	case base == "append" && len(kept) == 1 && self.Kind() == "inout_expr" && isUnicodeScalarsOfString(self.Child("sub_expr")):
		// s.unicodeScalars.append(x) appends to the String itself.
		strExpr := stripLoads(self.Child("sub_expr")).Child("base")
		lv := e.lvalue(strExpr)
		if goAddressable(strExpr, lv) {
			return "swiftrt.StringAppend(&" + lv + ", string(" + ss[0] + "))", true
		}
		return lv + " += string(" + ss[0] + ")", true
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
		if e.m.GoType(e.m.typeOf(self.Child("sub_expr").Type()), e.generics) == "string" && goAddressable(self.Child("sub_expr"), lv) {
			return "swiftrt.StringAppend(&" + lv + ", " + ss[0] + ")", true
		}
		return lv + " += " + ss[0], true
	}
	return "", false
}

// classDelegation emits `super.init(...)` or `self.init(...)` inside a class initializer body as a
// call to the delegated initializer's body on this object: super.init runs the superclass's own
// initializer on the embedded base, and self.init dispatches to the most-derived override.
func (e *fileEmitter) classDelegation(sub ast.Node) string {
	call, forced := sub, false
	for {
		switch call.Kind() {
		case "covariant_return_conversion_expr", "try_expr", "force_try_expr", "paren_expr", "load_expr", "inject_into_optional", "derived_to_base_expr":
			call = call.Child("sub_expr")
			continue
		case "force_value_expr":
			forced = true
			call = call.Child("sub_expr")
			continue
		}
		break
	}
	fn := stripConv(call.Child("fn"))
	if call.Kind() != "call_expr" || fn == nil || fn.Kind() != "dot_syntax_call_expr" {
		return e.m.Unsup("expr:class-init-delegation", e.where, "")
	}
	info, _ := e.lookup(stripConv(fn.Child("fn")).Child("decl").Str("decl_usr"))
	selfArgs := fn.Child("args").List("args")
	if info == nil || len(selfArgs) == 0 {
		return e.m.Unsup("expr:class-init-delegation", e.where, "")
	}
	e.callee = info
	ss, _ := e.argStrings(e.args(call.Child("args")), false)
	args := strings.Join(ss, ", ")
	invoke := e.initDispatch(info, args)
	if target := stripConv(selfArgs[0].Child("expr")); target.Kind() == "super_ref_expr" {
		invoke = "self." + embeddedField(e.m.GoType(e.m.typeOf(target.Type()), e.generics)) + "." + e.initBodyName(info) + "(" + args + ")"
	}
	if !info.Node.Bool("failable") {
		return invoke
	}
	if forced {
		return "if !" + invoke + " {\nswiftrt.Trap(\"Unexpectedly found nil while unwrapping an Optional value\")\n}"
	}
	return "if !" + invoke + " {\nreturn false\n}"
}

// constructedClass is the Go name of the class an initializer call creates, from the metatype it is
// called on.
func (e *fileEmitter) constructedClass(n ast.Node) string {
	fn := stripConv(n.Child("fn"))
	if fn == nil || fn.Kind() != "constructor_ref_call_expr" {
		return ""
	}
	as := fn.Child("args").List("args")
	if len(as) == 0 {
		return ""
	}
	meta := as[0].Child("expr")
	for meta.Kind() == "metatype_conversion_expr" {
		meta = meta.Child("sub_expr")
	}
	t := e.m.typeOf(meta.Type())
	if t == nil || t.Kind != types.Meta || len(t.Args) != 1 {
		return ""
	}
	return strings.TrimPrefix(e.m.GoType(t.Args[0], e.generics), "*")
}

// superTarget is the method a call to info on base names: `super.f()` reaches the superclass's own
// implementation of an overridden member, not the dispatching method.
func (e *fileEmitter) superTarget(info *DeclInfo, base ast.Node) string {
	for base != nil && base.Kind() == "derived_to_base_expr" {
		base = stripConv(base.Child("sub_expr"))
	}
	if base != nil && stripConv(base).Kind() == "super_ref_expr" && info.OwnerKind == "class_decl" {
		if root := e.overrideRoot(info, &DeclInfo{Kind: "class_decl"}); root != nil {
			return overrideHook(root)
		}
	}
	return info.GoName
}

func (e *fileEmitter) ctorCall(n ast.Node, d ast.Node, as []arg) string {
	usr := d.Str("decl_usr")
	info, pfx := e.lookup(usr)
	if info != nil && info.OwnerKind == "class_decl" {
		owner := pfx + e.m.ownerGoName(info)
		if made := e.constructedClass(n); made != owner && e.m.IsAncestor(owner, made) {
			// An initializer a subclass inherits from its superclass runs on a new subclass object.
			e.callee = info
			ss, _ := e.argStrings(as, false)
			obj := e.newTmp("obj")
			call := obj + "." + classMethodName(info) + "(" + strings.Join(ss, ", ") + ")"
			if info.Node.Bool("failable") {
				call = "if !" + call + " {\nreturn nil\n}"
			}
			return "func() *" + made + " {\n" + obj + " := &" + made + "{}\n" + obj + ".SwiftSub = " + obj + "\n" + call + "\nreturn " + obj + "\n}()"
		}
	}
	if info != nil && info.OwnerKind == "protocol" {
		// An initializer requirement called on a metatype known only as the protocol.
		if fn := stripConv(n.Child("fn")); fn != nil && fn.Kind() == "constructor_ref_call_expr" && len(fn.Child("args").List("args")) > 0 {
			e.callee = info
			ss, _ := e.argStrings(as, false)
			return e.metaReceiver(fn.Child("args").List("args")[0].Child("expr")) + "." + staticInitName(info.Node) + "(" + strings.Join(ss, ", ") + ")"
		}
	}
	if info != nil {
		e.callee = info
		if memberwise(info) {
			// A synthesized memberwise initializer stores its arguments as they are.
			e.argSink = e.partsSink(n)
		}
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
	if mp := e.m.mappedDecl(usr); mp != nil {
		return e.mappedCtor(mp, n, d, as)
	}
	if rt := e.m.typeOf(n.Type()); (usr == "s:SqyxSgxcfC" || usr == "s:SqsRi_zRi0_zrlEyxSgxcfc") && len(as) == 1 && rt != nil && rt.Kind == types.Optional && len(rt.Args) == 1 {
		// `Optional(x)`: the value wrapped, which for a reference is the reference itself.
		inner := e.m.GoType(rt.Args[0], e.generics)
		if e.m.isReferenceGo(rt.Args[0], inner) {
			return e.expr(as[0].node)
		}
		if inner != "" {
			return "swiftrt.Some[" + inner + "](" + e.expr(as[0].node) + ")"
		}
	}
	sig := parseDeclSig(e.m.demangle(usr))
	ss, kept := e.argStrings(as, true)
	if rt := e.m.typeOf(n.Type()); len(kept) == 1 {
		inner := rt
		if inner.Kind == types.Optional && len(inner.Args) == 1 {
			inner = inner.Args[0]
		}
		if isGenericParam(inner) {
			// A protocol initializer on a type parameter goes through the runtime's registries.
			switch kept[0].label {
			case "rawValue":
				return "swiftrt.RawInitOf[" + e.m.GoType(inner, e.generics) + "](" + ss[0] + ")"
			case "from":
				return "swiftrt.DecodeAs[" + e.m.GoType(inner, e.generics) + "](" + ss[0] + ")"
			}
		}
	}
	if rt := e.m.typeOf(n.Type()); rt != nil && rt.Kind == types.Named && rt.Name == "Swift.AnyHashable" && len(kept) == 1 && kept[0].label == "" {
		// AnyHashable(x) is x itself, held as any.
		return "any(" + ss[0] + ")"
	}
	recv := e.m.Recv(e.m.typeOf(n.Type()))
	if recv == "Map" && len(kept) == 2 && len(ss) == 2 && kept[0].label == "grouping" && e.m.Recv(e.m.typeOf(kept[0].node.Type())) == "Map" {
		// Grouping a dictionary groups its (key, value) elements.
		ss[0] = "swiftrt.MapOrdered(" + ss[0] + ")"
	}
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
	if recv == "Int" && len(as) == 1 && e.exprGoType(as[0].node) == "swiftrt.Int128" {
		fn := "Int128ToInt"
		if as[0].label == "clamping" {
			fn = "Int128ToIntClamping"
		}
		return "swiftrt." + fn + "[" + gt + "](" + strings.Join(ss, ", ") + ")"
	}
	if recv == "TaskLocal" || recv == "Mutex" || recv == "Atomic" {
		if t := e.m.typeOf(n.Type()); t != nil && len(t.Args) == 1 {
			if recv == "Atomic" {
				// The literal's kind does not tell the element type: Atomic<UInt32>(0).
				return "swiftrt.AtomicInit[" + e.m.GoType(t.Args[0], e.generics) + "](" + strings.Join(ss, ", ") + ")"
			}
			if recv == "Mutex" {
				return "swiftrt.MutexInit[" + e.m.GoType(t.Args[0], e.generics) + "](" + strings.Join(ss, ", ") + ")"
			}
			return "swiftrt.TaskLocalInit" + suffix + "[" + e.m.GoType(t.Args[0], e.generics) + "](" + strings.Join(ss, ", ") + ")"
		}
	}
	if recv == "Slice" || recv == "Map" || recv == "Range" || recv == "ClosedRange" || recv == "Int" || recv == "Float" || recv == "IntOpt" || recv == "FloatOpt" || recv == "Set" || recv == "OptionSet" || recv == "Heap" {
		return "swiftrt." + recv + "Init" + suffix + "[" + gt + "](" + strings.Join(ss, ", ") + ")"
	}
	return "swiftrt." + recv + "Init" + suffix + e.m.mappedTypeArgs(e.m.typeOf(n.Type()), e.generics) + "(" + strings.Join(ss, ", ") + ")"
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
	if d.Str("decl_usr") == "s:Sq4noneyxSgABmlF" {
		// `Optional.none` written out, as in `x == .none`: a nil of the Optional's Go type.
		if gt := e.exprGoType(n); gt != "" && gt != "any" {
			return "(" + gt + ")(nil)"
		}
	}
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
		_, mod := e.lookupModule(d.Str("decl_usr"))
		inlineOpt := e.m.inlineOptResult(info, mod)
		if base == nil || info.Static || !info.FreeSelf() {
			fn := pfx + info.GoName
			if base != nil && !info.Static {
				fn = e.recvExpr(base) + "." + info.GoName
			}
			if inlineOpt {
				return e.inlineOptForwarder(n, fn)
			}
			return fn
		}
		return e.boundMethod(n, base, func(recv string, ps []string) string {
			call := pfx + info.GoName + "(" + strings.Join(append([]string{recv}, ps...), ", ") + ")"
			if inlineOpt {
				call += ".Ptr()"
			}
			return call
		})
	}
	if info == nil {
		usr := d.Str("decl_usr")
		sig := parseDeclSig(e.m.demangle(usr))
		t := e.m.typeOf(n.Type())
		if mp := e.m.mappedDecl(usr); mp != nil && t != nil && t.Kind != types.Func {
			return e.mappedName(mp, sig, nil, false)
		}
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

// optionalEqual is the runtime function comparing two Optionals of Swift types lt and rt, when
// both are the same Optional of a scalar or a payload-free enum, whose Go == is Swift's ==.
func (e *fileEmitter) optionalEqual(lt, rt *types.Type) (string, bool) {
	if lt == nil || rt == nil || lt.Kind != types.Optional || len(lt.Args) != 1 {
		return "", false
	}
	inner := lt.Args[0]
	gt := e.m.GoType(inner, e.generics)
	if gt == "" || "*"+gt != e.m.GoType(rt, e.generics) || e.m.GoType(lt, e.generics) != "*"+gt || inner.Kind != types.Named {
		return "", false
	}
	switch gt {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64", "bool", "string":
		return "swiftrt.OptEqual", true
	}
	if k := e.m.declKind(inner.Name); k == "enum_decl" && !e.m.IsPayloadEnum(inner.Name) && !e.m.declaresMember(inner.Name, "==") {
		return "swiftrt.OptEqual", true
	}
	return "", false
}

// unboundedTypeParam reports whether t is one of the function's type parameters with no Go constraint that
// allows == or <, such as a parameter that is only Equatable or Comparable.
func (e *fileEmitter) unboundedTypeParam(t *types.Type) bool {
	gt := e.m.GoType(t, e.generics)
	if !slices.Contains(e.typeParams, gt) {
		return false
	}
	_, bounded := e.typeBounds[gt]
	return !bounded
}

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
	switch op {
	case "&&", "||", "??", "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
	default:
		e.pinOperands(l, r)
	}
	if info, pfx := e.lookup(d.Str("decl_usr")); info != nil {
		return pfx + info.GoName + "(" + e.expr(l) + ", " + e.expr(r) + ")"
	}
	if mp := e.m.mappedDecl(d.Str("decl_usr")); mp != nil {
		return e.mappedCallExpr(mp, d, "", as, false, false)
	}
	if call, ok := e.foreignOperatorCall(n, d, as, false); ok {
		return call
	}
	lt := e.m.typeOf(l.Type())
	if e.m.GoType(lt, e.generics) == "swiftrt.Int128" {
		if fn, ok := int128Ops[op]; ok {
			return "swiftrt.Int128" + fn + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if op == "==" || op == "!=" || op == "<" || op == "<=" || op == ">" || op == ">=" {
			return "swiftrt.Int128Compare(" + e.expr(l) + ", " + e.expr(r) + ") " + op + " 0"
		}
	}
	switch op {
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		s, _ := e.compoundAssign(n)
		return "func() struct{} { " + s + "; return struct{}{} }()"
	case "&&", "||":
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "..<", "...":
		typ := map[string]string{"..<": "Range", "...": "ClosedRange"}[op] + "[" + e.m.GoType(lt, e.generics) + "]"
		if lo, hi, ok := integerLiterals(l, r); ok && lo <= hi {
			return "(swiftrt." + typ + "{Lo: " + e.expr(l) + ", Hi: " + e.expr(r) + "})"
		}
		return "swiftrt.New" + typ + "(" + e.expr(l) + ", " + e.expr(r) + ")"
	case "??":
		return e.coalesce(n, lt, l, r)
	case "~=":
		return e.expr(l) + " == " + e.expr(r)
	case "==", "!=":
		if rl, rr := e.m.Recv(lt), e.m.Recv(e.m.typeOf(r.Type())); (rl == "String" || rl == "Substring") && (rr == "String" || rr == "Substring") && rl != rr {
			return e.stringish(l) + " " + op + " " + e.stringish(r)
		}
		if r.Kind() == "nil_literal_expr" || l.Kind() == "nil_literal_expr" {
			return e.expr(l) + " " + op + " " + e.expr(r)
		}
		if op == "!=" && lt.Kind == types.Named && e.m.declaresMember(lt.Name, "==") {
			// The standard library's != negates the type's own ==.
			return "!swiftrt.Equal(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if rt := e.m.typeOf(r.Type()); lt.Kind == types.Named && rt != nil && rt.Kind == types.Named && isIntegerName(lt.Name) && isIntegerName(rt.Name) && lt.Name != rt.Name {
			return "swiftrt.IntCompare(" + e.expr(l) + ", " + e.expr(r) + ") " + op + " 0"
		}
		if eq, ok := e.optionalEqual(lt, e.m.typeOf(r.Type())); ok {
			// Both sides are pointers to a value Go's == compares as Swift does: no reflection, and
			// neither pointer escapes.
			s := eq + "(" + e.expr(l) + ", " + e.expr(r) + ")"
			if op == "!=" {
				return "!" + s
			}
			return s
		}
		if lt.Kind == types.Optional || !e.m.isGoComparable(lt) || e.unboundedTypeParam(lt) {
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
		less := e.lessFunc(lt)
		if less == "" && e.unboundedTypeParam(lt) {
			less = "swiftrt.Less"
		}
		if less != "" {
			a, b := e.expr(l), e.expr(r)
			if op == "<" || op == ">=" || (e.isPure(l) && e.isPure(r)) {
				return lessOp(less, op, a, b)
			}
			gt := e.m.GoType(lt, e.generics)
			return "func(a, b " + gt + ") bool { return " + lessOp(less, op, "a", "b") + " }(" + a + ", " + b + ")"
		}
		if lt.Kind == types.Tuple {
			return e.tupleCompare(n, lt, op, l, r)
		}
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "+", "-", "*":
		if rl := e.m.Recv(lt); (op == "+" || op == "-") && (rl == "UnsafeMutablePointer" || rl == "UnsafePointer") {
			// Pointer arithmetic moves a pointer into C memory by a number of elements.
			by := e.expr(r)
			if op == "-" {
				by = "-(" + by + ")"
			}
			return "swiftrt.PointerAdvanced(" + e.expr(l) + ", " + by + ")"
		}
		if op == "-" && e.m.GoType(lt, e.generics) == "swiftrt.ContinuousClockInstant" {
			return "swiftrt.InstantSub(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if op == "+" && (e.m.Recv(lt) == "String" || e.m.Recv(lt) == "Substring") {
			cat := e.stringish(l) + " + " + e.stringish(r)
			if e.m.Recv(e.m.typeOf(n.Type())) == "Substring" {
				return "swiftrt.SubstringInitString(" + cat + ")"
			}
			return cat
		}
		if lt.Kind == types.Named && isIntegerName(lt.Name) {
			fn := map[string]string{"+": "Add", "-": "Sub", "*": "Mul"}[op]
			return "swiftrt." + fn + e.intTypeArg(lt) + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if op == "+" && (lt.Kind == types.Array || e.m.Recv(lt) == "Slice" || e.m.Recv(lt) == "Data") {
			rs := e.expr(r)
			if lg, rg := e.m.GoType(lt, e.generics), e.exprGoType(r); strings.HasPrefix(lg, "[]") && !strings.HasPrefix(rg, "[]") {
				// `array + sequence`: the other sequence (a lazy one, a Set) is read into a slice.
				rs = "swiftrt.Iterate[" + lg[2:] + "](" + rs + ")"
			}
			return "swiftrt.Concat(" + e.expr(l) + ", " + rs + ")"
		}
		return e.floatOperands(lt, e.expr(l), e.expr(r), op)
	case "===", "!==":
		same := "swiftrt.Same(" + e.expr(l) + ", " + e.expr(r) + ")"
		if op == "!==" {
			return "!" + same
		}
		return same
	case "/", "%":
		if op == "/" && e.m.GoType(lt, e.generics) == "swiftrt.Duration" && e.exprGoType(r) == "swiftrt.Duration" {
			return "swiftrt.DurationRatio(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		if lt.Kind == types.Named && isIntegerName(lt.Name) {
			fn := map[string]string{"/": "Div", "%": "Rem"}[op]
			return "swiftrt." + fn + e.intTypeArg(lt) + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		return e.floatOperands(lt, e.expr(l), e.expr(r), op)
	case "<<", ">>":
		if lt.Kind == types.Named && isIntegerName(lt.Name) && !isNonNegativeLiteral(r) {
			return "swiftrt." + map[string]string{"<<": "Shl", ">>": "Shr"}[op] + e.intTypeArg(lt) + "(" + e.expr(l) + ", " + e.expr(r) + ")"
		}
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "&", "|", "^":
		return e.expr(l) + " " + op + " " + e.expr(r)
	case "&+", "&-", "&*":
		return e.expr(l) + " " + op[1:] + " " + e.expr(r)
	}
	return e.m.Unsup("operator:"+op, e.where, e.exprGoType(n))
}

// floatOperands joins the operands of a floating-point operator. Two constant operands would make a
// Go constant expression, which is evaluated exactly and fails to compile on overflow; Swift computes
// in the operand type at run time, so the left operand is passed through a function.
func (e *fileEmitter) floatOperands(lt *types.Type, l, r, op string) string {
	if gt := e.m.GoType(lt, e.generics); (gt == "float64" || gt == "float32") && isConstText(l) && isConstText(r) {
		l = "swiftrt.FloatValue[" + gt + "](" + l + ")"
	}
	return l + " " + op + " " + r
}

var constText = regexp.MustCompile(`^[0-9.eE+\-*/() ]+$`)

// isConstText reports whether s, an emitted expression, is a numeric Go constant.
func isConstText(s string) bool {
	s = strings.NewReplacer("float32(", "(", "float64(", "(").Replace(s)
	return constText.MatchString(s)
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
	if mp := e.m.mappedDecl(d.Str("decl_usr")); mp != nil {
		// A mapped module's compound operator is a call that takes its left side by pointer.
		return e.mappedCallExpr(mp, d, "", as, false, false), true
	}
	l := as[0].node
	if l.Kind() == "inout_expr" {
		l = l.Child("sub_expr")
	}
	hoisted := e.hoistLvalue(l)
	if len(hoisted) > 0 && !e.isEagerSafe(as[1].node) {
		// Swift evaluates the right side after the subscripts, then reads the destination.
		hoisted = append(hoisted, e.hoist(as[1].node))
	}
	defer func() {
		for _, key := range hoisted {
			delete(e.exprSubst, key)
		}
	}()
	return e.compoundAssignOnce(n, d, op, as, l)
}

// hoist writes a statement evaluating n into a temporary, which later emissions of n use, and
// returns n's key in exprSubst.
func (e *fileEmitter) hoist(n ast.Node) uintptr {
	tmp := e.newTmp("v")
	e.w("%s := %s\n", tmp, e.expr(n))
	if e.exprSubst == nil {
		e.exprSubst = map[uintptr]string{}
	}
	e.exprSubst[nodeKey(n)] = tmp
	return nodeKey(n)
}

// hoistLvalue evaluates, in Swift's order, the subscript indices and class-reference bases of the
// destination of a compound assignment that are not plain reads, so the read and the write of the
// destination share them.
func (e *fileEmitter) hoistLvalue(n ast.Node) []uintptr {
	var out []uintptr
	var visit func(n ast.Node)
	visit = func(n ast.Node) {
		switch n.Kind() {
		case "load_expr", "paren_expr", "inout_expr":
			visit(n.Child("sub_expr"))
		case "member_ref_expr":
			base := stripLoads(n.Child("base"))
			if base.Kind() == "call_expr" {
				if t := e.m.typeOf(base.Type()); t != nil && e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
					out = append(out, e.hoist(base))
					return
				}
			}
			visit(base)
		case "subscript_expr":
			visit(n.Child("base"))
			for _, a := range e.args(n.Child("args")) {
				// A `default:` value is an autoclosure, evaluated only for a missing key.
				if a.label != "default" && !e.isEagerSafe(a.node) {
					out = append(out, e.hoist(a.node))
				}
			}
		}
	}
	visit(n)
	return out
}

func (e *fileEmitter) compoundAssignOnce(n, d ast.Node, op string, as []arg, l ast.Node) (string, bool) {
	lt := e.m.typeOf(l.Type())
	base := op[:len(op)-1]
	rs := ""
	if info, _ := e.lookup(d.Str("decl_usr")); info == nil && base == "+" && e.m.Recv(lt) == "Slice" {
		rs = e.stored(as[1].node, lt)
	} else if info != nil {
		rs = e.owned(as[1].node, sinkBorrow)
	} else {
		rs = e.expr(as[1].node)
	}
	if e.m.Recv(lt) == "String" && e.m.Recv(e.m.typeOf(as[1].node.Type())) == "Substring" {
		rs += ".String()"
	}
	combine := func(cur string) string {
		if fn, ok := int128Ops[base]; ok && e.m.GoType(lt, e.generics) == "swiftrt.Int128" {
			return "swiftrt.Int128" + fn + "(" + cur + ", " + rs + ")"
		}
		if len(base) == 2 && base[0] == '&' && strings.ContainsAny(base[1:], "+-*") {
			return cur + " " + base[1:] + " " + rs
		}
		if lt.Kind == types.Named && isIntegerName(lt.Name) {
			if fn, ok := map[string]string{"+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Rem"}[base]; ok {
				return "swiftrt." + fn + e.intTypeArg(lt) + "(" + cur + ", " + rs + ")"
			}
		}
		if (base == "<<" || base == ">>") && lt.Kind == types.Named && isIntegerName(lt.Name) && !isNonNegativeLiteral(as[1].node) {
			return "swiftrt." + map[string]string{"<<": "Shl", ">>": "Shr"}[base] + e.intTypeArg(lt) + "(" + cur + ", " + rs + ")"
		}
		if base == "+" && (e.m.Recv(lt) == "Slice" || e.m.Recv(lt) == "Data") {
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
			mark := len(e.slots)
			call := pfx + info.GoName + "(" + e.expr(as[0].node) + ", " + rs + ")"
			if len(e.slots) > mark {
				call = e.wrapSlots(n, call, mark)
			}
			return call, true
		}
		return pfx + info.GoName + "(&" + e.lvalue(l) + ", " + rs + ")", true
	}
	lv := e.lvalue(l)
	if lt.Kind == types.Named && isIntegerName(lt.Name) || base == "+" && (e.m.Recv(lt) == "Slice" || e.m.Recv(lt) == "Data") || len(base) == 2 && base[0] == '&' || e.m.GoType(lt, e.generics) == "swiftrt.Int128" {
		return lv + " = " + combine(lv), true
	}
	if base == "+" && e.m.GoType(lt, e.generics) == "string" && goAddressable(l, lv) {
		// Swift appends to a String in amortised constant time; Go's += copies it.
		return "swiftrt.StringAppend(&" + lv + ", " + rs + ")", true
	}
	return lv + " " + op + " " + rs, true
}

// goAddressable reports whether lv, the Go form of the Swift lvalue n, is a variable or a field
// Go can take the address of: no map or slice index, which could be a map element.
func goAddressable(n ast.Node, lv string) bool {
	n = stripLoads(n)
	for n.Kind() == "paren_expr" {
		n = stripLoads(n.Child("sub_expr"))
	}
	switch n.Kind() {
	case "declref_expr", "member_ref_expr":
		return !strings.ContainsAny(lv, "[(")
	}
	return false
}

// integerLiterals returns the values of two integer literals.
func integerLiterals(a, b ast.Node) (x, y int64, ok bool) {
	parse := func(n ast.Node) (int64, bool) {
		n = stripConv(n)
		if n.Kind() != "integer_literal_expr" {
			return 0, false
		}
		v, err := strconv.ParseInt(strings.ReplaceAll(n.Str("value"), "_", ""), 0, 64)
		if n.Bool("negative") {
			v = -v
		}
		return v, err == nil
	}
	x, ok1 := parse(a)
	y, ok2 := parse(b)
	return x, y, ok1 && ok2
}

// isNonNegativeLiteral reports whether n is an integer literal Go can use as a shift count as is.
func isNonNegativeLiteral(n ast.Node) bool {
	n = stripConv(n)
	return n.Kind() == "integer_literal_expr" && !n.Bool("negative") && !strings.HasPrefix(n.Str("value"), "-")
}

func (e *fileEmitter) prefix(n ast.Node) string {
	d := e.opDecl(n)
	op := d.Str("base_name")
	as := e.args(n.Child("args"))
	if len(as) != 1 {
		return e.m.Unsup("prefix-arity", e.where, e.exprGoType(n))
	}
	if info, pfx := e.lookup(d.Str("decl_usr")); info != nil {
		return pfx + info.GoName + "(" + e.expr(as[0].node) + ")"
	}
	if mp := e.m.mappedDecl(d.Str("decl_usr")); mp != nil {
		return e.mappedCallExpr(mp, d, "", as, false, true)
	}
	if call, ok := e.foreignOperatorCall(n, d, as, true); ok {
		return call
	}
	x := e.expr(as[0].node)
	switch op {
	case "!":
		return "!" + x
	case "-":
		if e.exprGoType(as[0].node) == "swiftrt.Int128" {
			return "swiftrt.Int128Neg(" + x + ")"
		}
		if t := e.m.typeOf(as[0].node.Type()); t != nil && t.Kind == types.Named && isIntegerName(t.Name) && stripConv(as[0].node).Kind() != "integer_literal_expr" {
			return "swiftrt.Neg(" + x + ")"
		}
		return "-" + x
	case "~":
		if lit := stripConv(as[0].node); lit.Kind() == "integer_literal_expr" {
			// A literal operand needs its type: ^1 as an untyped constant is -2.
			return "^" + e.exprGoType(as[0].node) + "(" + x + ")"
		}
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
		case c == '<' && !(i > 0 && (s[i-1] == '.' || s[i-1] == ' ' || s[i-1] == '-')) || c == '<' && i+1 < len(s) && s[i+1] == ' ' && !strings.HasPrefix(s[i:], "< infix"):
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
		return e.expr(sub) + "." + upcastMethod(to) + "()"
	}
	return e.expr(sub)
}

// magicLiteral renders #line, #column, #file, #fileID and #function at the call site.
func (e *fileEmitter) magicLiteral(n ast.Node) string {
	return e.magicLiteralAt(n.Str("kind"), n)
}

// magicLiteralAt is the value of the magic literal `kind` written where at stands.
func (e *fileEmitter) magicLiteralAt(kind string, at ast.Node) string {
	off := -1
	if r := at.Child("range"); r != nil {
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
	case "#fileID":
		// The module and the file's name, as Swift writes it: "Commands/BossCommand.swift".
		return strconv.Quote(e.m.Name + "/" + filepath.Base(e.file.Filename))
	case "#file", "#filePath":
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
	if mp := e.m.mappedTypeModule(t.Name); mp != nil {
		return e.mappedLiteral(mp, n, t, label), true
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
					e.m.importPkg(mod)
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
	val := e.owned(init, sinkStore)
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
	if gt == "" {
		// A Void global runs its initializer once for the side effect.
		gt, pre, val = "struct{}", pre+val+"\n", "struct{}{}"
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

// capturesVar reports an identity capture of a `var`, whose value Swift copies when the closure is
// created; a `let` cannot change afterwards, so the closure may read it directly.
func capturesVar(c ast.Node) bool {
	init := c.List("pattern_entries")[0].Child("processed_init")
	for init != nil && init.Kind() == "paren_expr" {
		init = init.Child("sub_expr")
	}
	return init != nil && init.Kind() == "load_expr"
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
	if t := e.m.typeOf(base.Type()); base.Kind() == "declref_expr" && base.Child("decl").Str("decl_usr") == "" && t != nil && t.Kind == types.Meta && len(t.Args) == 1 && t.Args[0].Kind == types.Generic && e.m.protocolGoName(e.m.GoType(t.Args[0], e.generics)) {
		// A parameter or local that holds the metatype of a type known only as its protocol.
		return e.expr(base)
	}
	t := e.m.typeOf(base.Type())
	if t != nil && t.Kind == types.Meta && len(t.Args) == 1 && t.Args[0].Kind == types.Generic && base.Kind() == "type_expr" {
		if v, ok := e.metaValues[t.Args[0].Name]; ok {
			return v
		}
	}
	if t != nil && t.Kind == types.Meta && len(t.Args) == 1 && (t.Args[0].Kind == types.Existential || t.Args[0].Kind == types.Named && e.m.isLocalProtocol(t.Args[0].Name)) {
		// `any P.Type` values are carried as a zero value of the concrete type, typed as P.
		return e.expr(base)
	}
	if t != nil && t.Kind == types.Meta && len(t.Args) == 1 {
		inner := t.Args[0]
		gt := e.m.GoType(inner, e.generics)
		if inner.Kind == types.Generic && (strings.HasPrefix(gt, "any") || e.m.isLocalProtocol(e.m.Name+"."+gt) || e.m.protocolGoName(gt) && !strings.Contains(inner.Name, ".")) {
			// Self in a protocol extension: the receiver carries the dynamic type.
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
	"ContinuousClockNow": true, "NSThreadCallStackSymbols": true, "TaskIsCancelled": true, "TimeZoneGmt": true,
	"TimeZoneCurrent": true, "TimeZoneAutoupdatingCurrent": true, "CalendarCurrent": true,
	"CalendarAutoupdatingCurrent": true, "LocaleCurrent": true, "InstantNow": true,
	"ContinuousClockInstantNow": true, "SuspendingClockInstantNow": true, "DateNow": true,
}

// lessOp renders `a op b` through a type's `<` function. It evaluates b before a for `>` and `<=`, so
// callers pass pure operands there.
func lessOp(less, op, a, b string) string {
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

// isPure reports an operand whose evaluation has no effects, so it can be evaluated out of order: a
// literal, a stored local or global, or a stored property or tuple element of one.
func (e *fileEmitter) isPure(n ast.Node) bool {
	switch n.Kind() {
	case "integer_literal_expr", "float_literal_expr", "boolean_literal_expr", "string_literal_expr", "nil_literal_expr":
		return true
	case "declref_expr":
		info, _ := e.lookup(n.Child("decl").Str("decl_usr"))
		return info == nil || !info.Computed
	case "member_ref_expr":
		info, _ := e.lookup(n.Child("decl").Str("decl_usr"))
		return info != nil && info.Kind == "var_decl" && !info.Computed && e.isPure(n.Child("base"))
	case "load_expr", "paren_expr", "erasure_expr":
		return e.isPure(n.Child("sub_expr"))
	case "tuple_element_expr":
		return e.isPure(n.Child("base"))
	}
	return false
}

// coalesce emits `l ?? r`. Swift passes r as an autoclosure, so a right side that could call code,
// trap or allocate is emitted as a func literal that runs only when l is nil.
func (e *fileEmitter) coalesce(n ast.Node, lt *types.Type, l, r ast.Node) string {
	ls := e.expr(l)
	fn := "swiftrt.Coalesce"
	switch rt := e.m.typeOf(n.Type()); {
	case lt.Kind == types.Optional && len(lt.Args) == 1 && lt.Args[0].Kind == types.Optional:
		// T?? ?? T? unwraps one level, unless `try?` already flattened it to T?.
		if strings.HasPrefix(strings.TrimLeft(ls, "("), "swiftrt.TryFlat(") {
			fn = "swiftrt.CoalesceOpt"
		}
	case rt != nil && rt.Kind == types.Optional:
		fn = "swiftrt.CoalesceOpt"
	case lt.Kind == types.Optional && len(lt.Args) == 1 && e.m.isReferenceGo(lt.Args[0], e.m.GoType(lt.Args[0], e.generics)):
		fn = "swiftrt.CoalesceRef"
	}
	if e.isEagerSafe(r) {
		return fn + "(" + ls + ", " + e.expr(r) + ")"
	}
	// Statements the right side needs belong inside the func literal, so they run only with it.
	saved, savedPost := e.b, e.post
	e.b, e.post = strings.Builder{}, nil
	rs := e.expr(r)
	pre, post := e.b.String(), e.post
	e.b, e.post = saved, savedPost
	gt := e.exprGoType(n)
	if pre == "" && len(post) == 0 {
		return fn + "Func(" + ls + ", func() " + gt + " { return " + rs + " })"
	}
	return fn + "Func(" + ls + ", func() " + gt + " {\n" + pre + "res := " + rs + "\n" + strings.Join(post, "\n") + "\nreturn res\n})"
}

// isEagerSafe reports whether evaluating n early is unobservable: it cannot call code, trap or
// allocate.
func (e *fileEmitter) isEagerSafe(n ast.Node) bool {
	n = stripConv(n)
	if k := n.Kind(); k == "member_ref_expr" || k == "declref_expr" {
		if info := e.m.ownDecl(n.Child("decl").Str("decl_usr")); info != nil && info.Lazy {
			// Reading a lazy static runs its initializer the first time.
			return false
		}
	}
	switch n.Kind() {
	case "autoclosure_expr":
		return len(n.Child("params").List("params")) == 0 && e.isEagerSafe(n.Child("single_expr_body"))
	case "array_expr", "dictionary_expr":
		for _, el := range n.List("elements") {
			if !e.isEagerSafe(el) {
				return false
			}
		}
		return len(n.List("elements")) == 0
	case "inject_into_optional", "function_conversion_expr", "bind_optional_expr", "optional_evaluation_expr":
		return e.isEagerSafe(n.Child("sub_expr"))
	case "declref_expr":
		if d := n.Child("decl"); d.Str("decl_usr") != "" {
			if info, _ := e.lookup(d.Str("decl_usr")); info != nil && info.Kind == "enum_element_decl" {
				return true
			}
		}
	case "dot_syntax_call_expr":
		// A payload-free enum case such as `.none`.
		if fn := n.Child("fn"); fn != nil && fn.Kind() == "declref_expr" {
			if info, _ := e.lookup(fn.Child("decl").Str("decl_usr")); info != nil && info.Kind == "enum_element_decl" {
				return true
			}
		}
		return false
	case "binary_expr":
		if d := e.opDecl(n); d.Str("base_name") == "??" {
			as := e.args(n.Child("args"))
			return len(as) == 2 && e.isEagerSafe(as[0].node) && e.isEagerSafe(as[1].node)
		}
		return false
	}
	return e.isPure(n)
}

// tupleCompare expands Swift's lexicographic tuple ordering element by element: the first unequal
// element decides, and the last element decides with op itself. Each operand is evaluated once, left
// to right.
func (e *fileEmitter) tupleCompare(n ast.Node, lt *types.Type, op string, l, r ast.Node) string {
	gt := e.m.GoType(lt, e.generics)
	var body strings.Builder
	for i, el := range lt.Args {
		a, b := "a.F"+itoa(i), "b.F"+itoa(i)
		if i == len(lt.Args)-1 {
			c, ok := e.elementCompare(el, op, a, b)
			if !ok {
				return e.m.Unsup("tuple-compare:"+e.m.Recv(el), e.where, e.exprGoType(n))
			}
			body.WriteString("return " + c)
			break
		}
		ne, ok1 := e.elementCompare(el, "!=", a, b)
		c, ok2 := e.elementCompare(el, op, a, b)
		if !ok1 || !ok2 {
			return e.m.Unsup("tuple-compare:"+e.m.Recv(el), e.where, e.exprGoType(n))
		}
		body.WriteString("if " + ne + " { return " + c + " }; ")
	}
	return "func(a, b " + gt + ") bool { " + body.String() + " }(" + e.expr(l) + ", " + e.expr(r) + ")"
}

// elementCompare renders `a op b` for one tuple element of type t, using the type's own operators where it
// declares them. It reports false when t has no known ordering.
func (e *fileEmitter) elementCompare(t *types.Type, op, a, b string) (string, bool) {
	gt := e.m.GoType(t, e.generics)
	if gt == "swiftrt.Int128" {
		return "swiftrt.Int128Compare(" + a + ", " + b + ") " + op + " 0", true
	}
	if t.Kind == types.Tuple {
		return "", false
	}
	if op == "==" || op == "!=" {
		if eq := e.operatorFunc(t, "=="); eq != "" {
			if op == "!=" {
				return "!" + eq + "(" + a + ", " + b + ")", true
			}
			return eq + "(" + a + ", " + b + ")", true
		}
		if t.Kind == types.Optional || !e.m.isGoComparable(t) {
			if op == "!=" {
				return "!swiftrt.Equal(" + a + ", " + b + ")", true
			}
			return "swiftrt.Equal(" + a + ", " + b + ")", true
		}
		return a + " " + op + " " + b, true
	}
	if less := e.lessFunc(t); less != "" {
		return lessOp(less, op, a, b), true
	}
	if goOrdered[gt] || e.caseOrderedEnum(t) {
		return a + " " + op + " " + b, true
	}
	return "", false
}

var goOrdered = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "string": true,
	"swiftrt.Date": true, "swiftrt.Duration": true, "swiftrt.DispatchTime": true, "swiftrt.LoggerLevel": true,
}

// lessFunc names the Go function for a local type's `<` operator, or "" if it has none.
func (e *fileEmitter) lessFunc(t *types.Type) string {
	if t != nil && t.Kind == types.Named && t.Name == "Swift.Int128" {
		return "swiftrt.Int128Less"
	}
	return e.operatorFunc(t, "<")
}

// operatorFunc names the Go function for a local type's static operator, or "" if it declares none.
func (e *fileEmitter) operatorFunc(t *types.Type, op string) string {
	if t == nil || t.Kind != types.Named {
		return ""
	}
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, d := range mod.sortedDecls() {
			if d.Kind == "func_decl" && d.Owner == t.Name && d.Node != nil && d.Node.Name() == op {
				if mod.Pkg != e.m.Pkg {
					e.m.importPkg(mod)
					return mod.Pkg + "." + d.GoName
				}
				return d.GoName
			}
		}
	}
	return ""
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

var int128Ops = map[string]string{"+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "%": "Rem"}

// expectThrows emits `#expect(throws: E.self) { ... }`, `#expect(throws: value) { ... }` and their
// #require forms.
func (e *fileEmitter) expectThrows(as []arg, fatal bool) string {
	body := as[len(as)-1].node
	msg := `""`
	for _, a := range as[1 : len(as)-1] {
		if a.label == "" && a.node.Kind() != "default_argument_expr" {
			msg = "swiftrt.Str(" + e.expr(a.node) + ")"
		}
	}
	fn := e.expr(body)
	what := as[0].node
	for what.Kind() == "paren_expr" || what.Kind() == "erasure_expr" || what.Kind() == "load_expr" {
		what = what.Child("sub_expr")
	}
	if wt := e.m.typeOf(what.Type()); wt != nil && wt.Kind == types.Meta {
		inner := e.m.GoType(wt.Args[0], e.generics)
		switch {
		case wt.Args[0].Name == "Swift.Never":
			return fmt.Sprintf("swiftrt.ExpectNoThrow(%s, %t, %s)", fn, fatal, msg)
		case inner == "error" || inner == "any" || strings.HasPrefix(inner, "any"):
			return fmt.Sprintf("swiftrt.ExpectThrows(%s, %t, %s)", fn, fatal, msg)
		}
		return fmt.Sprintf("swiftrt.ExpectThrowsType[%s](%s, %t, %s)", inner, fn, fatal, msg)
	}
	return fmt.Sprintf("swiftrt.ExpectThrowsEqual(%s, %s, %t, %s)", e.expr(what), fn, fatal, msg)
}

// intBound is `.max`/`.min` of each fixed-width integer type as a typed Go constant.
var intBound = map[string]string{
	"int8Max": "int8(math.MaxInt8)", "int8Min": "int8(math.MinInt8)",
	"int16Max": "int16(math.MaxInt16)", "int16Min": "int16(math.MinInt16)",
	"int32Max": "int32(math.MaxInt32)", "int32Min": "int32(math.MinInt32)",
	"int64Max": "int64(math.MaxInt64)", "int64Min": "int64(math.MinInt64)",
	"uintMax": "uint(math.MaxUint)", "uintMin": "uint(0)",
	"uint8Max": "uint8(math.MaxUint8)", "uint8Min": "uint8(0)",
	"uint16Max": "uint16(math.MaxUint16)", "uint16Min": "uint16(0)",
	"uint32Max": "uint32(math.MaxUint32)", "uint32Min": "uint32(0)",
	"uint64Max": "uint64(math.MaxUint64)", "uint64Min": "uint64(0)",
}

// bitWidth is `.bitWidth` of each fixed-width integer type narrower than Int.
var bitWidth = map[string]string{"int8": "8", "uint8": "8", "int16": "16", "uint16": "16", "int32": "32", "uint32": "32"}

// bitWidthOwner is the Go type whose `bitWidth` n reads: the type of `Int8.bitWidth`, or of the
// value in `x.bitWidth`.
func (e *fileEmitter) bitWidthOwner(n ast.Node) string {
	base := n.Child("lhs")
	if base == nil {
		base = n.Child("base")
	}
	if base == nil {
		return ""
	}
	t := e.m.typeOf(base.Type())
	if t != nil && t.Kind == types.Meta && len(t.Args) == 1 {
		t = t.Args[0]
	}
	if t == nil {
		return ""
	}
	return e.m.GoType(t, e.generics)
}

// intTypeArg is "[T]" for a checked-arithmetic call on a fixed-width integer type other than Int, so
// untyped literal operands take the operation's type.
func (e *fileEmitter) intTypeArg(t *types.Type) string {
	if gt := e.m.GoType(t, e.generics); gt != "int" {
		return "[" + gt + "]"
	}
	return ""
}

var genericParamName = regexp.MustCompile(`^(?:[A-Z]|τ_\d+_\d+)(?:\.|$)`)

// returnsGenericOptional reports whether call's callee is one of ours whose declared result is
// Optional of a generic parameter.
func (e *fileEmitter) returnsGenericOptional(call ast.Node) bool {
	fn := stripConv(call.Child("fn"))
	if fn.Kind() == "dot_syntax_call_expr" {
		fn = stripConv(fn.Child("fn"))
	}
	usr := fn.Child("decl").Str("decl_usr")
	info, _ := e.lookup(usr)
	if info == nil || info.Node == nil {
		// A mapped module's generic function takes its type parameter as a type argument and returns *T.
		if e.m.mappedDecl(usr) == nil {
			return false
		}
		dem := e.m.demangle(usr)
		arrow := strings.LastIndex(dem, " -> ")
		if arrow < 0 {
			return false
		}
		result := strings.TrimSuffix(dem[arrow+4:], "?")
		return result != dem[arrow+4:] && slices.Contains(ownGenericParams(strings.TrimPrefix(dem, "static ")), result)
	}
	rt := e.m.typeOf(info.Node.Str("result"))
	return rt != nil && rt.Kind == types.Optional && len(rt.Args) == 1 && isGenericParam(rt.Args[0])
}

func isGenericParam(t *types.Type) bool {
	return t.Kind == types.Generic || t.Kind == types.Named && genericParamName.MatchString(t.Name)
}

// isUnicodeScalarsOfString reports whether n is `someString.unicodeScalars`.
func isUnicodeScalarsOfString(n ast.Node) bool {
	n = stripLoads(n)
	return n.Kind() == "member_ref_expr" && n.Child("decl").Str("base_name") == "unicodeScalars" && strings.HasPrefix(n.Child("decl").Str("decl_usr"), "s:SS")
}

// sequenceParam reports whether generic parameter t of fn is constrained to Sequence or Collection,
// which the emitter represents as a slice.
func sequenceParam(m *Module, fn ast.Node, t *types.Type) bool {
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") != "conforms_to" || m.typeOf(r.Str("first_type")).Name != t.Name {
			continue
		}
		switch m.demangle(r.Str("protocol")) {
		case "Swift.Sequence", "Swift.Collection", "Swift.BidirectionalCollection", "Swift.RandomAccessCollection":
			return true
		}
	}
	return false
}

// chainReturn marks an optional chain's own early exit until the closure's result type is known, so
// rewriting it never reaches a nested closure's returns.
const chainReturn = "return /*chain*/ nil"

// protocolBoundOf is the Go interface of the local protocol (without static requirements) that
// generic parameter t of fn conforms to, or "".
func protocolBoundOf(m *Module, fn ast.Node, t *types.Type) string {
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") != "conforms_to" || m.typeOf(r.Str("first_type")).Name != t.Name {
			continue
		}
		if proto := m.demangle(r.Str("protocol")); strings.HasPrefix(proto, m.Name+".") && !m.HasStaticRequirements(proto) {
			return m.LocalTypeName(proto)
		}
	}
	return ""
}

// mapElementRoot is the Dictionary subscript a stored-property chain like `d[k].a.b` is rooted at,
// or nil. It stops at classes, whose members are addressable through their pointer.
func mapElementRoot(e *fileEmitter, n ast.Node) ast.Node {
	for n.Kind() == "member_ref_expr" {
		base := stripLoads(n.Child("base"))
		if t := e.m.typeOf(base.Type()); t == nil || e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
			return nil
		}
		if base.Kind() == "subscript_expr" && e.m.Recv(e.m.typeOf(base.Child("base").Type())) == "Map" {
			return base
		}
		n = base
	}
	return nil
}

// paramConformsTo reports whether fn's generic signature requires parameter t to conform to proto.
func paramConformsTo(m *Module, fn ast.Node, t *types.Type, proto string) bool {
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") == "conforms_to" && m.typeOf(r.Str("first_type")).Name == t.Name && m.demangle(r.Str("protocol")) == proto {
			return true
		}
	}
	return false
}

// isOperatorRef reports whether n is an operator or other function (`==`) passed as a value.
func isOperatorRef(n ast.Node) bool {
	switch n.Kind() {
	case "autoclosure_expr":
		// The compiler wraps an operator passed as a function value in an implicit closure.
		return true
	case "dot_syntax_call_expr":
		n = n.Child("fn")
	}
	if n == nil || n.Kind() != "declref_expr" {
		return false
	}
	name := n.Child("decl").Str("base_name")
	return name != "" && strings.ContainsAny(name[:1], "=<>!+-*/%&|^~")
}

// paramConversion wraps a one-parameter function passed where Swift expects a different parameter type:
// an inout T parameter is dereferenced, a T parameter is wrapped as T? by taking its address,
// and a subclass parameter is upcast to the base class the function takes.
func (e *fileEmitter) paramConversion(n ast.Node) (string, bool) {
	sub := n.Child("sub_expr")
	from, to := e.m.typeOf(sub.Type()), e.m.typeOf(n.Type())
	if f, ok := e.resultConversion(sub, from, to); ok {
		return f, true
	}
	if from == nil || to == nil || from.Kind != types.Func || to.Kind != types.Func || len(from.Args) != 1 || len(to.Args) != 1 {
		return "", false
	}
	fp, tp := from.Args[0], to.Args[0]
	if len(to.Inout) == 1 && to.Inout[0] && (len(from.Inout) == 0 || !from.Inout[0]) {
		// A function taking T passed where one taking inout T is expected: read through the pointer.
		pg := e.m.GoType(fp, e.generics)
		res := ""
		if from.Result != nil {
			res = e.m.GoType(from.Result, e.generics)
		}
		body := "f(*v)"
		if res != "" {
			body = "return " + body
		}
		return "func(f func(" + pg + ") " + res + ") func(*" + pg + ") " + res + " { return func(v *" + pg + ") " + res + " { " + body + " } }(" + e.expr(sub) + ")", true
	}
	fg, tg := e.m.GoType(fp, e.generics), e.m.GoType(tp, e.generics)
	arg := ""
	switch {
	case fp.Kind == types.Optional && len(fp.Args) == 1 && tp.Kind != types.Optional && fg == "*"+tg:
		arg = "&v"
	case fg != tg && strings.HasPrefix(fg, "*") && strings.HasPrefix(tg, "*") && e.m.IsAncestor(fg[1:], tg[1:]):
		// A function taking a base class passed where one taking a subclass is expected: upcast the argument.
		arg = "v." + upcastMethod(fg[1:]) + "()"
	default:
		return "", false
	}
	res := ""
	if from.Result != nil {
		res = e.m.GoType(from.Result, e.generics)
	}
	body := "f(" + arg + ")"
	if res != "" {
		body = "return " + body
	}
	return "func(f func(" + fg + ") " + res + ") func(" + tg + ") " + res + " { return func(v " + tg + ") " + res + " { " + body + " } }(" + e.expr(sub) + ")", true
}

// foreignStaticTypeArgs is "[X]" for a static computed property of a generic foreign type read as
// `Optional<X>.p`: Go cannot infer type parameters of a function without arguments.
func (e *fileEmitter) foreignStaticTypeArgs(info *DeclInfo, base ast.Node) string {
	var getter ast.Node
	for _, a := range info.Node.List("accessors") {
		if a.Bool("get") || a.Str("accessor_kind") == "get" {
			getter = a
		}
	}
	for base != nil && (base.Kind() == "load_expr" || base.Kind() == "paren_expr" || base.Kind() == "dot_self_expr") {
		base = base.Child("sub_expr")
	}
	if getter == nil || base == nil {
		return ""
	}
	t := e.m.typeOf(base.Type())
	if t == nil || t.Kind != types.Meta || len(t.Args) != 1 {
		return ""
	}
	params := getter.Child("generic_signature").Strings("generic_params")
	if len(params) == 0 || len(params) != len(t.Args[0].Args) {
		return ""
	}
	saveG, saveTP, saveTB := e.generics, e.typeParams, e.typeBounds
	e.generics = nil
	g := e.genericScope(getter)
	declared := e.typeParams
	e.generics, e.typeParams, e.typeBounds = saveG, saveTP, saveTB
	args := make([]string, len(declared))
	for i, p := range params {
		if j := slices.Index(declared, g[e.m.demangle(p)]); j >= 0 {
			args[j] = e.m.GoType(t.Args[0].Args[i], e.generics)
		}
	}
	if len(args) == 0 || slices.Contains(args, "") {
		return ""
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// resultConversion wraps a function value passed where a function with the same parameters and a
// protocol result is expected (`() -> Concrete` as `() -> any P`): Go function types are
// invariant, so the wrapper returns the concrete result as the protocol's interface.
func (e *fileEmitter) resultConversion(sub ast.Node, from, to *types.Type) (string, bool) {
	if from == nil || to == nil || from.Kind != types.Func || to.Kind != types.Func || len(from.Args) != len(to.Args) || from.Result == nil || to.Result == nil {
		return "", false
	}
	fr, tr := e.m.GoType(from.Result, e.generics), e.m.GoType(to.Result, e.generics)
	upcast := fr != tr && strings.HasPrefix(fr, "*") && strings.HasPrefix(tr, "*") && e.m.IsAncestor(tr[1:], fr[1:])
	if fr == tr || fr == "" || tr == "" || from.Result.Kind != types.Named || !(upcast || to.Result.Kind == types.Existential || to.Result.Kind == types.Named && (e.m.isLocalProtocol(to.Result.Name) || e.m.mappedKind(to.Result.Name) == types.KindProtocol)) {
		return "", false
	}
	var params, names []string
	for i := range from.Args {
		fg, tg := e.m.GoType(from.Args[i], e.generics), e.m.GoType(to.Args[i], e.generics)
		if fg != tg || i < len(from.Inout) && from.Inout[i] || i < len(to.Inout) && to.Inout[i] {
			return "", false
		}
		params = append(params, "p"+itoa(i)+" "+tg)
		names = append(names, "p"+itoa(i))
	}
	ret := "f(" + strings.Join(names, ", ") + ")"
	if e.m.HasMutatingMethods(from.Result.Name) && !strings.HasPrefix(fr, "*") {
		// Only a pointer to a struct with mutating methods satisfies the interface.
		ret = "swiftrt.Ref(" + ret + ")"
	}
	if upcast {
		// A function returning a subclass, where one returning its superclass is expected.
		ret += "." + upcastMethod(tr[1:]) + "()"
	}
	return "func(f func(" + strings.Join(tparams(params), ", ") + ") " + fr + ") func(" + strings.Join(params, ", ") + ") " + tr +
		" { return func(" + strings.Join(params, ", ") + ") " + tr + " { return " + ret + " } }(" + e.expr(sub) + ")", true
}

// tparams strips the names from "name type" parameters.
func tparams(params []string) []string {
	out := make([]string, len(params))
	for i, p := range params {
		_, out[i], _ = strings.Cut(p, " ")
	}
	return out
}

// pointsToPointer reports a pointer type whose elements are themselves pointers, optional or not.
func pointsToPointer(t *types.Type) bool {
	if t == nil || len(t.Args) != 1 {
		return false
	}
	el := t.Args[0]
	if el.Kind == types.Optional && len(el.Args) == 1 {
		el = el.Args[0]
	}
	return el.Kind == types.Named && (el.Name == "Swift.UnsafeMutablePointer" || el.Name == "Swift.UnsafePointer")
}

// caseOrderedEnum reports an enum without payloads, which is a Go integer: its synthesized
// Comparable orders the cases as declared, as the integers do.
func (e *fileEmitter) caseOrderedEnum(t *types.Type) bool {
	if t == nil || t.Kind != types.Named || e.m.IsPayloadEnum(t.Name) {
		return false
	}
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		if mod.typeKinds[t.Name] == "enum_decl" && !mod.IsPayloadEnum(t.Name) {
			return true
		}
	}
	return false
}

// uninferableTypeArgs renders explicit type arguments for a call of a local generic function when
// one of its type parameters occurs in no parameter type, so Go could not infer it (`Failure` in
// `func f<Failure>(…) -> Outcome<Failure>`). It applies to functions whose parameters all stay Go
// type parameters: a non-generic owner and no constraint that erases or reshapes one.
// typeParamKeepingProtocols are the standard library protocols a type parameter can conform to and
// stay a plain `any` parameter in Go.
var typeParamKeepingProtocols = map[string]bool{"Swift.Error": true, "Swift.Sendable": true, "Swift.Decodable": true, "Swift.Encodable": true}

func (e *fileEmitter) uninferableTypeArgs(info *DeclInfo, d ast.Node) string {
	if info.Node == nil || len(e.m.TypeParams(info.Owner)) > 0 {
		return ""
	}
	sig := info.Node.Child("generic_signature")
	var names []string
	for _, gp := range sig.Strings("generic_params") {
		names = append(names, e.m.demangle(gp))
	}
	if len(names) == 0 {
		return ""
	}
	for _, r := range sig.List("generic_reqs") {
		proto := e.m.demangle(r.Str("protocol"))
		if r.Str("kind") != "conforms_to" || e.m.isLocalProtocol(proto) || strings.HasPrefix(proto, "Swift.") && !typeParamKeepingProtocols[proto] {
			return ""
		}
	}
	// Look for each parameter in the Go types, which can drop it (a Result's failure type).
	marks := map[string]string{}
	for _, name := range names {
		marks[name] = "T_" + name + "_T"
	}
	needed := false
	for _, name := range names {
		used := false
		for _, p := range info.Node.Child("params").List("params") {
			if strings.Contains(e.m.GoType(e.m.typeOf(p.Str("interface_type")), marks), marks[name]) {
				used = true
			}
		}
		if !used {
			if !strings.Contains(e.m.GoType(e.m.typeOf(info.Node.Str("result")), marks), marks[name]) {
				// In neither a parameter nor the result: the declaration drops the parameter.
				return ""
			}
			needed = true
		}
	}
	if !needed {
		return ""
	}
	subs := map[string]string{}
	for _, sub := range d.Child("substitutions").List("substitutions") {
		subs[e.m.demangle(sub.Str("generic_param"))] = sub.Str("replacement_type")
	}
	var args []string
	for _, name := range names {
		r, ok := subs[name]
		if !ok {
			return ""
		}
		gt := e.m.GoType(e.m.typeOf(r), e.generics)
		if gt == "" {
			gt = "struct{}"
		}
		args = append(args, gt)
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// erasesMetaGeneric reports whether fn's generic parameter is known to its Go form only as its
// protocol's interface because the function takes the type itself, as `C.Type`, and mentions C
// nowhere else: the metatype parameter is then a value of the conforming type, which also serves
// a caller that holds the type only as an existential metatype.
func erasesMetaGeneric(m *Module, fn ast.Node, param string) bool {
	if fn == nil || strings.Contains(param, ".") {
		return false
	}
	constrained := false
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") == "conforms_to" && m.demangle(r.Str("first_type")) == param && m.isLocalProtocol(m.demangle(r.Str("protocol"))) {
			constrained = true
		}
	}
	if !constrained {
		return false
	}
	re := regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(param) + `($|[^\w])`)
	meta := false
	for _, p := range fn.Child("params").List("params") {
		dem := m.demangle(p.Str("interface_type"))
		switch {
		case dem == param+".Type":
			meta = true
		case re.MatchString(dem):
			return false
		}
	}
	return meta && !re.MatchString(m.demangle(fn.Str("result")))
}

// simpleDidSet reports a stored property whose only observer is a didSet that never reads
// oldValue, which Swift runs after modifying the property in place.
func simpleDidSet(v ast.Node) bool {
	var did ast.Node
	for _, a := range v.List("accessors") {
		switch {
		case a.Bool("willSet"):
			return false
		case a.Bool("didSet"):
			did = a
		}
	}
	if did == nil {
		return false
	}
	reads := false
	ast.Walk(map[string]any(did), func(n ast.Node) {
		if n.Kind() == "declref_expr" && n.Child("decl").Str("base_name") == "oldValue" {
			reads = true
		}
	})
	return !reads
}

// scalarViewOf returns the String expression s when n is `s.unicodeScalars`.
func (e *fileEmitter) scalarViewOf(n ast.Node) ast.Node {
	n = stripLoads(n)
	if n == nil || n.Kind() != "member_ref_expr" || n.Child("decl").Str("base_name") != "unicodeScalars" {
		return nil
	}
	str := n.Child("base")
	if r := e.m.Recv(e.m.typeOf(str.Type())); r != "String" {
		return nil
	}
	return str
}
