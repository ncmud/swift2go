package emit

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

func (e *fileEmitter) stmts(list []ast.Node) {
	scope := map[string]bool{}
	if e.paramNames != nil {
		for k := range e.paramNames {
			scope[k] = true
		}
		e.paramNames = nil
	}
	opened := 0
	for _, s := range list {
		names := declaredNames(s)
		for _, n := range names {
			if scope[n] {
				e.w("{\n")
				opened++
				scope = map[string]bool{}
				break
			}
		}
		for _, n := range names {
			scope[n] = true
		}
		savePost := e.post
		e.post = nil
		mark := len(e.slots)
		e.stmt(s)
		if len(e.slots) > mark {
			e.slots = e.slots[:mark]
			e.w("_ = %s\n", e.m.Unsup("inout-slot-unwrapped", e.where, ""))
		}
		for _, p := range e.post {
			e.w("%s\n", p)
		}
		e.post = savePost
	}
	for ; opened > 0; opened-- {
		e.w("}\n")
	}
}

// declaredNames lists the Go variables a statement introduces into its enclosing block.
func declaredNames(s ast.Node) []string {
	var out []string
	collect := func(p ast.Node) {
		ast.Walk(p, func(n ast.Node) {
			if n.Kind() == "pattern_named" {
				out = append(out, goSafe(n.Child("name").Child("base_name").Str("name")))
			}
		})
	}
	switch s.Kind() {
	case "pattern_binding_decl":
		for _, pe := range s.List("pattern_entries") {
			collect(pe.Child("pattern"))
		}
	case "guard_stmt":
		for _, c := range s.List("conditions") {
			if p := c.Child("pattern"); p != nil {
				collect(p)
			}
		}
	}
	return out
}

func (e *fileEmitter) block(n ast.Node) {
	e.w("{\n")
	if n != nil {
		if n.Kind() == "brace_stmt" {
			e.stmts(n.List("elements"))
		} else {
			e.stmt(n)
		}
	}
	e.w("}")
}

func (e *fileEmitter) stmt(n ast.Node) {
	switch n.Kind() {
	case "while_stmt", "repeat_while_stmt", "for_each_stmt", "switch_stmt":
		e.jumpDepth++
		defer func() { e.jumpDepth-- }()
		if label := e.stmtLabel(n); label != "" && e.labelUsed(n, label) {
			if e.doRet != nil {
				e.doRet.inner[label] = true
			}
			saved := e.b
			e.b = strings.Builder{}
			e.stmtUnlabeled(n)
			text := e.b.String()
			e.b = saved
			e.w("%s", insertLabel(text, label))
			return
		}
	}
	e.stmtUnlabeled(n)
}

// insertLabel puts "label:" before the first for or switch statement in text.
func insertLabel(text, label string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, l := range lines {
		t := strings.TrimLeft(l, "\t ")
		if strings.HasPrefix(t, "for ") || strings.HasPrefix(t, "for {") || strings.HasPrefix(t, "switch ") || strings.HasPrefix(t, "switch {") {
			lines[i] = label + ":\n" + l
			return strings.Join(lines, "")
		}
	}
	return text
}

func (e *fileEmitter) stmtUnlabeled(n ast.Node) {
	switch n.Kind() {
	case "inject_into_optional":
		e.stmt(n.Child("sub_expr"))
	case "brace_stmt":
		e.block(n)
		e.w("\n")
	case "pattern_binding_decl":
		e.localBinding(n)
	case "var_decl":
	case "struct_decl", "class_decl", "enum_decl":
		e.hoisted = append(e.hoisted, n)
	case "func_decl":
		e.localFunc(n)
	case "return_stmt":
		e.returnStmt(n)
	case "fail_stmt":
		e.w("return nil\n")
	case "then_stmt":
		if r := n.Child("result"); isVoidGo(e.exprGoType(r)) {
			e.stmt(r)
			if e.resultType == "*struct{}" {
				e.w("return &struct{}{}\n")
			} else {
				e.w("return\n")
			}
		} else {
			e.w("return %s\n", e.expr(r))
		}
	case "if_stmt":
		e.ifStmt(n)
		e.w("\n")
	case "guard_stmt":
		e.guardStmt(n)
	case "while_stmt":
		e.whileStmt(n)
	case "repeat_while_stmt":
		// `continue` must still test the condition, so it is the loop's post statement.
		again := e.newTmp("again")
		saved := e.b
		e.b = strings.Builder{}
		cond := e.expr(n.Child("cond"))
		condPre := e.b.String()
		e.b = saved
		if condPre != "" {
			cond = "func() bool {\n" + condPre + "return " + cond + "\n}()"
		}
		e.w("for %s := true; %s; %s = %s {\n", again, again, again, cond)
		e.stmt(n.Child("body"))
		e.w("}\n")
	case "for_each_stmt":
		e.forEach(n)
	case "switch_stmt":
		e.switchStmt(n)
	case "break_stmt", "continue_stmt":
		jump := strings.TrimSuffix(n.Kind(), "_stmt")
		if label := e.jumpLabel(n); label != "" {
			jump += " " + label
		}
		label := e.jumpLabel(n)
		if d := e.doRet; d != nil && (label == "" && e.jumpDepth == d.depth || label != "" && !d.inner[label]) {
			// The do body runs in a Go closure: leave it and repeat the jump outside.
			flag := d.flag + "_" + strings.ReplaceAll(jump, " ", "_")
			d.jumps[jump] = flag
			e.w("%s = true\nreturn\n", flag)
			break
		}
		e.w("%s\n", jump)
	case "fallthrough_stmt":
		e.w("fallthrough\n")
	case "defer_stmt":
		e.w("defer func() ")
		body := n.Child("body")
		if body == nil {
			body = n.Child("call_expr")
		}
		e.w("{\n")
		fd := n.Child("func_decl")
		if fd == nil {
			fd = n.Child("temp_decl")
		}
		if !e.topLevel[nodeKey(n)] {
			e.m.Unsup("defer-in-nested-scope", e.where, "")
		}
		if fd != nil {
			e.stmts(fd.Child("body").List("elements"))
		} else if body != nil {
			e.stmt(body)
		}
		e.w("}()\n")
	case "assign_expr":
		e.assign(n)
	case "binary_expr":
		if s, ok := e.compoundAssign(n); ok {
			e.w("%s\n", s)
			return
		}
		e.w("_ = %s\n", e.expr(n))
	case "throw_stmt":
		e.w("panic(swiftrt.Throw(%s))\n", e.expr(n.Child("sub_expr")))
	case "do_stmt":
		e.block(n.Child("body"))
		e.w("\n")
	case "do_catch_stmt":
		e.doCatch(n)
	case "discard_assignment_expr":
	case "optional_evaluation_expr":
		s := e.expr(n)
		if strings.HasPrefix(s, "func() {") || isVoidGo(e.exprGoType(n)) && strings.HasSuffix(s, ")") {
			e.w("%s\n", s)
		} else {
			e.w("_ = %s\n", s)
		}
	case "yield_stmt":
		e.w("%s\n", e.m.Unsup("stmt:yield", e.where, ""))
	default:
		if strings.HasSuffix(n.Kind(), "_expr") {
			s := e.expr(n)
			for _, never := range []string{"swiftrt.FatalError(", "swiftrt.PreconditionFailure("} {
				if strings.HasPrefix(s, never) {
					s = "swiftrt.FatalError[struct{}](" + strings.TrimPrefix(s, never)
				}
			}
			if e.isCallLike(n) || isVoidGo(e.exprGoType(n)) && (strings.HasSuffix(s, ")") || strings.HasSuffix(s, "}()")) {
				e.w("%s\n", s)
			} else {
				e.w("_ = %s\n", s)
			}
			return
		}
		e.w("_ = %s\n", e.m.Unsup("stmt:"+n.Kind(), e.where, ""))
	}
}

func (e *fileEmitter) isCallLike(n ast.Node) bool {
	for n != nil {
		switch n.Kind() {
		case "call_expr", "prefix_unary_expr", "postfix_unary_expr", "macro_expansion_expr", "rebind_self_in_constructor_expr":
			return true
		case "paren_expr", "load_expr", "try_expr", "force_try_expr", "dot_syntax_call_expr":
			n = n.Child("sub_expr")
		case "optional_evaluation_expr":
			return false
		default:
			return false
		}
	}
	return false
}

func (e *fileEmitter) localFunc(n ast.Node) {
	saveG := e.generics
	e.generics = e.genericScope(n)
	params, _ := e.paramList(n)
	res := e.resultGo(n)
	name := goSafe(n.Name())
	sig := "func(" + params + ")"
	if res != "" {
		sig += " " + res
	}
	e.w("var %s %s\n%s = %s ", name, sig, name, sig)
	saveInit := e.inInit
	e.inInit = false
	e.body(n, res)
	e.inInit = saveInit
	e.w("\n")
	e.generics = saveG
}

func (e *fileEmitter) returnStmt(n ast.Node) {
	r := n.Child("result")
	if e.doRet != nil {
		if r != nil && e.resultType != "" && !e.inInit {
			savePost := e.post
			e.post = nil
			e.w("%s = %s\n", e.doRet.val, e.expr(r))
			for _, p := range e.post {
				e.w("%s\n", p)
			}
			e.post = savePost
		} else if r != nil && !e.inInit {
			e.stmt(r)
		}
		e.w("%s = true\nreturn\n", e.doRet.flag)
		return
	}
	if e.inInit {
		e.w("return %s\n", e.initRet)
		return
	}
	if r == nil || e.resultType == "" {
		if r != nil {
			e.stmt(r)
		}
		e.w("return\n")
		return
	}
	savePost := e.post
	e.post = nil
	v := e.expr(r)
	if len(e.post) > 0 {
		// Write-backs must run before leaving the function.
		tmp := e.newTmp("res")
		e.w("%s := %s\n", tmp, v)
		for _, p := range e.post {
			e.w("%s\n", p)
		}
		v = tmp
	}
	e.post = savePost
	e.w("return %s\n", v)
}

func (e *fileEmitter) localBinding(n ast.Node) {
	for _, pe := range n.List("pattern_entries") {
		p := pe.Child("pattern")
		init := pe.Child("processed_init")
		if init == nil {
			ast.Walk(p, func(x ast.Node) {
				if x.Kind() == "pattern_named" {
					name := goSafe(x.Child("name").Child("base_name").Str("name"))
					e.w("var %s %s\n_ = %s\n", name, e.m.GoType(e.m.typeOf(x.Type()), e.generics), name)
				}
			})
			continue
		}
		e.bindPattern(p, init, true)
	}
}

// bindPattern declares the variables a pattern introduces, initialised from init.
func (e *fileEmitter) bindPattern(p ast.Node, init ast.Node, declare bool) {
	switch p.Kind() {
	case "pattern_named":
		gt := e.m.GoType(e.m.typeOf(p.Type()), e.generics)
		name := goSafe(p.Name())
		if init == nil {
			e.w("var %s %s\n", name, gt)
			return
		}
		e.w("var %s %s = %s\n", name, gt, e.valueCopy(init))
	case "pattern_typed", "pattern_binding", "pattern":
		e.bindPattern(p.Child("sub_pattern"), init, declare)
	case "pattern_any":
		if init != nil {
			e.w("_ = %s\n", e.expr(init))
		}
	case "pattern_tuple":
		tmp := e.newTmp("tuple")
		src := ""
		if init != nil {
			src = e.expr(init)
		}
		e.w("%s := %s\n", tmp, src)
		for i, el := range p.List("elements") {
			e.bindFromExpr(el, tmp+".F"+itoa(i))
		}
	default:
		e.w("_ = %s\n", e.m.Unsup("pattern:"+p.Kind(), e.where, ""))
	}
}

func (e *fileEmitter) bindFromExpr(p ast.Node, src string) {
	switch p.Kind() {
	case "pattern_named":
		e.w("%s := %s\n_ = %s\n", goSafe(p.Name()), src, goSafe(p.Name()))
	case "pattern_typed", "pattern_binding", "pattern":
		e.bindFromExpr(p.Child("sub_pattern"), src)
	case "pattern_any":
	case "pattern_tuple":
		for i, el := range p.List("elements") {
			e.bindFromExpr(el, derefParen(src)+".F"+itoa(i))
		}
	case "pattern_enum_element":
		e.patternBind(p, src)
	default:
		e.w("_ = %s\n", e.m.Unsup("pattern:"+p.Kind(), e.where, ""))
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// valueCopy emits init, copying slices and maps so Swift value semantics hold on `var` binding.
func (e *fileEmitter) valueCopy(init ast.Node) string {
	s := e.expr(init)
	t := e.m.typeOf(init.Type())
	if isLvalueRead(init) && e.needsCopy(t) {
		return "swiftrt.Copy(" + s + ")"
	}
	return s
}

// needsCopy reports whether assigning a value of type t must copy it to keep Swift value semantics.
func (e *fileEmitter) needsCopy(t *types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case types.Array, types.Dict, types.Tuple:
		return true
	case types.Optional:
		return len(t.Args) == 1 && !e.m.isReferenceGo(t.Args[0], e.m.GoType(t.Args[0], e.generics)) && e.needsCopy(t.Args[0])
	case types.Named:
		switch t.Name {
		case "Swift.Array", "Swift.Dictionary", "Swift.Set", "OrderedCollections.OrderedDictionary", "OrderedCollections.OrderedSet", "DequeModule.Deque", "HeapModule.Heap":
			return true
		}
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			if mod.typeKinds[t.Name] == "struct_decl" {
				return true
			}
		}
	}
	return false
}

func isLvalueRead(n ast.Node) bool {
	for n != nil {
		switch n.Kind() {
		case "load_expr", "paren_expr":
			n = n.Child("sub_expr")
		case "declref_expr", "member_ref_expr":
			return true
		default:
			return false
		}
	}
	return false
}

// conds emits nested checks for an if/while condition list, then calls inner.
func (e *fileEmitter) conds(cs []ast.Node, inner func()) {
	if len(cs) == 0 {
		inner()
		return
	}
	var bools []string
	i := 0
	for i < len(cs) && cs[i].Kind() != "pattern" {
		bools = append(bools, e.expr(cs[i]))
		i++
	}
	if len(bools) > 0 {
		e.w("if %s {\n", strings.Join(bools, " && "))
		e.conds(cs[i:], inner)
		e.w("}\n")
		return
	}
	c := cs[0]
	rest := cs[1:]
	p := c.Child("pattern")
	init := c.Child("initializer")
	switch p.Kind() {
	case "pattern_optional_some":
		tmp := e.newTmp("opt")
		sub := p.Child("sub_pattern")
		if up := unwrapPattern(sub); up != nil && (up.Kind() == "pattern_enum_element" || up.Kind() == "pattern_expr") {
			val := derefParen(e.unwrap(tmp, init))
			var it *types.Type
			if t := e.m.typeOf(init.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 {
				it = t.Args[0]
			}
			e.w("if %s := %s; %s != nil && %s {\n", tmp, e.expr(init), tmp, e.patternTest(up, val, it))
			e.patternBind(up, val)
		} else {
			e.w("if %s := %s; %s != nil {\n", tmp, e.expr(init), tmp)
			e.bindFromExpr(sub, e.unwrap(tmp, init))
		}
		e.conds(rest, inner)
		e.w("}\n")
	case "pattern_binding", "pattern_named", "pattern_typed":
		e.w("{\n")
		e.bindFromExpr(p, e.expr(init))
		e.conds(rest, inner)
		e.w("}\n")
	case "pattern_enum_element", "pattern_expr", "pattern_bool":
		tmp := e.newTmp("subj")
		e.w("if %s := %s; %s {\n", tmp, e.expr(init), e.patternTest(p, tmp, e.m.typeOf(init.Type())))
		e.patternBind(p, tmp)
		e.conds(rest, inner)
		e.w("}\n")
	default:
		e.w("if %s {\n", e.m.Unsup("cond:"+p.Kind(), e.where, "bool"))
		e.conds(rest, inner)
		e.w("}\n")
	}
}

// unwrap dereferences an optional value held in a Go variable.
func (e *fileEmitter) unwrap(v string, optExpr ast.Node) string {
	t := e.m.typeOf(optExpr.Type())
	if t.Kind == types.Optional {
		inner := e.m.GoType(t.Args[0], e.generics)
		if e.m.isReferenceGo(t.Args[0], inner) {
			return v
		}
	}
	return "*" + v
}

func simpleConds(cs []ast.Node) bool {
	for _, c := range cs {
		if c.Kind() == "pattern" {
			return false
		}
	}
	return true
}

func (e *fileEmitter) ifStmt(n ast.Node) {
	cs := n.List("conditions")
	els := n.Child("else_stmt")
	if simpleConds(cs) {
		var parts []string
		for _, c := range cs {
			parts = append(parts, e.expr(c))
		}
		e.w("if %s ", strings.Join(parts, " && "))
		e.block(n.Child("then_stmt"))
		if els != nil {
			e.w(" else ")
			if els.Kind() == "if_stmt" && simpleConds(els.List("conditions")) {
				e.ifStmt(els)
			} else if els.Kind() == "if_stmt" {
				e.w("{\n")
				e.ifStmt(els)
				e.w("\n}")
			} else {
				e.block(els)
			}
		}
		return
	}
	if els == nil {
		e.conds(cs, func() { e.stmts(n.Child("then_stmt").List("elements")) })
		return
	}
	flag := e.newTmp("matched")
	e.w("%s := false\n", flag)
	e.conds(cs, func() {
		e.w("%s = true\n", flag)
		e.stmts(n.Child("then_stmt").List("elements"))
	})
	e.w("if !%s ", flag)
	if els.Kind() == "if_stmt" {
		e.w("{\n")
		e.ifStmt(els)
		e.w("\n}")
	} else {
		e.block(els)
	}
}

func (e *fileEmitter) guardStmt(n ast.Node) {
	elseBody := func() {
		e.block(n.Child("body"))
		e.w("\n")
	}
	for _, c := range n.List("conditions") {
		if c.Kind() != "pattern" {
			e.w("if !(%s) ", e.expr(c))
			elseBody()
			continue
		}
		p := c.Child("pattern")
		init := c.Child("initializer")
		switch p.Kind() {
		case "pattern_optional_some":
			tmp := e.newTmp("opt")
			e.w("%s := %s\nif %s == nil ", tmp, e.expr(init), tmp)
			elseBody()
			e.bindFromExpr(p.Child("sub_pattern"), e.unwrap(tmp, init))
		case "pattern_binding", "pattern_named", "pattern_typed":
			e.bindFromExpr(p, e.expr(init))
		default:
			tmp := e.newTmp("subj")
			e.w("%s := %s\nif !(%s) ", tmp, e.expr(init), e.patternTest(p, tmp, e.m.typeOf(init.Type())))
			elseBody()
			e.patternBind(p, tmp)
		}
	}
}

func (e *fileEmitter) whileStmt(n ast.Node) {
	cs := n.List("conditions")
	if simpleConds(cs) {
		var parts []string
		for _, c := range cs {
			parts = append(parts, e.expr(c))
		}
		e.w("for %s ", strings.Join(parts, " && "))
		e.block(n.Child("body"))
		e.w("\n")
		return
	}
	flag := e.newTmp("ok")
	e.w("for {\n%s := false\n", flag)
	e.conds(cs, func() {
		e.w("%s = true\n", flag)
		e.stmts(n.Child("body").List("elements"))
	})
	e.w("if !%s { break }\n}\n", flag)
}

func (e *fileEmitter) forEach(n ast.Node) {
	seq := n.Child("parsed_sequence")
	p := n.Child("pattern")
	body := func() {
		if w := n.Child("where"); w != nil {
			e.w("if !(%s) { continue }\n", e.expr(w))
		}
		e.stmts(n.Child("body").List("elements"))
	}
	name := func(p ast.Node) string {
		if nm := patternName(p); nm != "" {
			return goSafe(nm)
		}
		return "_"
	}
	st := e.m.typeOf(seq.Type())
	if containsKind(n.Child("desugared_loop"), "current_context_isolation_expr") {
		et := e.m.GoType(e.m.typeOf(p.Type()), e.generics)
		next, tmp := e.newTmp("next"), e.newTmp("el")
		e.w("%s := swiftrt.AsyncIter[%s](%s)\nfor {\n%s := %s()\nif %s == nil {\nbreak\n}\n", next, et, e.expr(seq), tmp, next, tmp)
		if inner := unwrapPattern(p); inner != nil && inner.Kind() == "pattern_tuple" {
			e.bindFromExpr(inner, "(*"+tmp+")")
		} else if nm := name(p); nm != "_" {
			e.w("%s := *%s\n_ = %s\n", nm, tmp, nm)
		}
		body()
		e.w("}\n")
		return
	}
	if inner := unwrapPattern(p); inner != nil && (inner.Kind() == "pattern_is" || inner.Kind() == "pattern_enum_element" || inner.Kind() == "pattern_optional_some" || inner.Kind() == "pattern_expr") {
		et := e.m.typeOf(p.Type())
		src := e.expr(seq)
		if e.m.Recv(st) != "Slice" {
			src = "swiftrt.Iterate[" + e.m.GoType(et, e.generics) + "](" + src + ")"
		}
		tmp := e.newTmp("el")
		e.w("for _, %s := range %s {\n", tmp, src)
		e.w("if !(%s) { continue }\n", e.patternTest(inner, tmp, et))
		e.patternBind(inner, tmp)
		body()
		e.w("}\n")
		return
	}
	// enumerated(): for (i, x) in xs.enumerated()
	if call := callOf(seq); call != nil && call.method == "enumerated" {
		base := e.expr(call.base)
		if e.m.Recv(e.m.typeOf(call.base.Type())) == "String" {
			base = "swiftrt.StringChars(" + base + ")"
		}
		inner := unwrapPattern(p)
		if inner.Kind() == "pattern_tuple" {
			els := inner.List("elements")
			if a, b := name(els[0]), name(els[1]); a == "_" && b == "_" {
				e.w("for range %s {\n", base)
			} else {
				e.w("for %s, %s := range %s {\n", a, b, base)
			}
		} else {
			tmp := e.newTmp("en")
			e.w("for %si, %sv := range %s {\n%s := struct{ F0 int; F1 %s }{%si, %sv}\n", tmp, tmp, base, name(p), e.m.GoType(st, e.generics), tmp, tmp)
		}
		body()
		e.w("}\n")
		return
	}
	switch e.m.Recv(st) {
	case "Range", "ClosedRange":
		v := name(p)
		if v == "_" {
			v = e.newTmp("i")
		}
		r := e.newTmp("r")
		op := "<"
		if e.m.Recv(st) == "ClosedRange" {
			op = "<="
		}
		e.w("{\n%s := %s\n", r, e.expr(seq))
		e.w("for %s := %s.Lo; %s %s %s.Hi; %s++ {\n", v, r, v, op, r, v)
		e.w("_ = %s\n", v)
		body()
		e.w("}\n}\n")
		return
	case "String", "Substring":
		e.w("for _, %s := range swiftrt.StringChars(%s) {\n", name(p), e.stringish(seq))
	case "Map":
		inner := unwrapPattern(p)
		if inner.Kind() == "pattern_tuple" {
			els := inner.List("elements")
			e.w("for %s, %s := range swiftrt.MapOrdered(%s) {\n", "_", "kv", e.expr(seq))
			e.w("%s, %s := kv.F0, kv.F1\n", name(els[0]), name(els[1]))
			for _, el := range els[:2] {
				if nm := name(el); nm != "_" {
					e.w("_ = %s\n", nm)
				}
			}
		} else {
			e.w("for _, %s := range swiftrt.MapOrdered(%s) {\n", name(p), e.expr(seq))
		}
	case "Slice", "Set", "OrderedDictionary", "OrderedSet", "Heap":
		src := e.expr(seq)
		if st.Kind == types.Named && st.Name == "Swift.DefaultIndices" {
			src = "swiftrt.Iterate[int](" + src + ")"
		}
		switch e.m.Recv(st) {
		case "Set":
			src = "swiftrt.SetElements(" + src + ")"
		case "OrderedDictionary":
			src = "swiftrt.OrderedDictionaryElements(" + src + ")"
		case "OrderedSet", "Heap":
			src = src + ".Items"
		}
		inner := unwrapPattern(p)
		if inner.Kind() == "pattern_tuple" {
			tmp := e.newTmp("el")
			e.w("for _, %s := range %s {\n", tmp, src)
			e.bindFromExpr(inner, tmp)
		} else {
			e.w("for _, %s := range %s {\n", name(p), src)
		}
	default:
		if inner := unwrapPattern(p); inner != nil && inner.Kind() == "pattern_tuple" {
			tmp := e.newTmp("el")
			e.w("for _, %s := range swiftrt.Iterate[%s](%s) {\n", tmp, e.m.GoType(e.m.typeOf(p.Type()), e.generics), e.expr(seq))
			e.bindFromExpr(inner, tmp)
			break
		}
		e.w("for _, %s := range swiftrt.Iterate[%s](%s) {\n", name(p), e.m.GoType(e.m.typeOf(p.Type()), e.generics), e.expr(seq))
	}
	body()
	e.w("}\n")
}

func unwrapPattern(p ast.Node) ast.Node {
	for p != nil && (p.Kind() == "pattern_typed" || p.Kind() == "pattern_binding" || p.Kind() == "pattern") {
		p = p.Child("sub_pattern")
	}
	return p
}

type methodCall struct {
	method string
	base   ast.Node
}

func callOf(n ast.Node) *methodCall {
	if n.Kind() != "call_expr" {
		return nil
	}
	fn := stripConv(n.Child("fn"))
	if fn.Kind() != "dot_syntax_call_expr" {
		return nil
	}
	decl := stripConv(fn.Child("fn")).Child("decl")
	args := fn.Child("args").List("args")
	if len(args) == 0 {
		return nil
	}
	return &methodCall{method: decl.Str("base_name"), base: args[0].Child("expr")}
}

func (e *fileEmitter) switchStmt(n ast.Node) {
	subj := n.Child("subject_expr")
	st := e.m.typeOf(subj.Type())
	tmp := e.newTmp("sw")
	e.w("%s := %s\n_ = %s\n", tmp, e.expr(subj), tmp)
	first := true
	for _, c := range n.List("cases") {
		var tests []string
		isDefault := false
		var bindPat ast.Node
		for _, item := range c.List("case_label_items") {
			if item.Bool("default") {
				isDefault = true
				continue
			}
			p := item.Child("pattern")
			if unwrapPattern(p).Kind() == "pattern_any" {
				isDefault = true
				continue
			}
			t := e.patternTest(p, tmp, st)
			if g := item.Child("guard_expr"); g != nil {
				bindPat = p
				t = "(" + t + ") && " + e.withBindings(p, tmp, e.expr(g))
			}
			tests = append(tests, "("+t+")")
			if patternBinds(p) {
				bindPat = p
			}
		}
		cond := strings.Join(tests, " || ")
		if isDefault {
			cond = "true"
		}
		if first {
			e.w("if %s {\n", cond)
			first = false
		} else {
			e.w("} else if %s {\n", cond)
		}
		if bindPat != nil {
			e.patternBind(bindPat, tmp)
		}
		e.caseBody(c.Child("body"))
	}
	if !first {
		e.w("}\n")
	}
}

// caseBody emits a case body; a Swift `break` there exits the switch, which here is an if-chain.
func (e *fileEmitter) caseBody(b ast.Node) {
	els := b.List("elements")
	if len(els) == 1 && els[0].Kind() == "break_stmt" {
		return
	}
	e.w("switch {\ndefault:\n")
	e.stmts(els)
	e.w("}\n")
}

func (e *fileEmitter) withBindings(p ast.Node, subj string, expr string) string {
	if !patternBinds(p) {
		return expr
	}
	saved := e.b
	e.b = strings.Builder{}
	e.patternBind(p, subj)
	binds := e.b.String()
	e.b = saved
	return "func() bool {\n" + binds + "return " + expr + "\n}()"
}

func patternBinds(p ast.Node) bool {
	found := false
	ast.Walk(map[string]any(p), func(n ast.Node) {
		if n.Kind() == "pattern_named" {
			found = true
		}
	})
	return found
}

// patternTest returns a Go boolean testing subj (of Swift type st) against pattern p.
func (e *fileEmitter) patternTest(p ast.Node, subj string, st *types.Type) string {
	p = unwrapPatternKeepSome(p)
	switch p.Kind() {
	case "pattern_any", "pattern_named":
		return "true"
	case "pattern_expr":
		me := p.Child("match_expr")
		args := me.Child("args").List("args")
		if len(args) == 2 {
			lit := args[0].Child("expr")
			lt := e.m.typeOf(lit.Type())
			if r := e.m.Recv(lt); r == "Range" || r == "ClosedRange" || r == "PartialRangeFrom" || r == "PartialRangeUpTo" || r == "PartialRangeThrough" {
				return e.expr(lit) + ".Contains(" + subj + ")"
			}
			if st != nil && e.m.Recv(st) == "Substring" && e.m.Recv(lt) == "String" {
				return derefParen(subj) + ".String() == " + e.expr(lit)
			}
			return subj + " == " + e.expr(lit)
		}
		return e.m.Unsup("pattern_expr-shape", e.where, "bool")
	case "pattern_is":
		target := e.m.GoType(e.m.typeOf(p.Str("cast_to")), e.generics)
		test := "swiftrt.Is[" + target + "](" + subj + ")"
		if sub := p.Child("sub_pattern"); sub != nil {
			if t := e.patternTest(unwrapPattern(sub), "swiftrt.MustAs["+target+"]("+subj+")", e.m.typeOf(p.Str("cast_to"))); t != "true" {
				test += " && " + t
			}
		}
		return test
	case "pattern_enum_element":
		if t, ok := e.optionalCase(p, subj, st); ok {
			return t
		}
		el := e.enumElement(p, st)
		if el == nil {
			if t, ok := e.resultCase(p, subj); ok {
				return t
			}
			return e.m.Unsup("enum-pattern-foreign", e.where, "bool")
		}
		if e.isPayloadCase(el) || e.m.IsPayloadEnum(el.Owner) {
			caseT := el.GoName + e.enumTypeArgs(el, st)
			if st == nil {
				caseT = el.GoName + e.enumTypeArgs(el, e.m.typeOf(p.Type()))
			}
			test := "func() bool { _, ok := " + assertable(subj) + ".(" + caseT + "); return ok }()"
			if sub := p.Child("sub_pattern"); sub != nil {
				inner := e.payloadSubTests(sub, "func() "+caseT+" { v, _ := "+assertable(subj)+".("+caseT+"); return v }()", el)
				if inner != "true" {
					test += " && " + inner
				}
			}
			return test
		}
		return subj + " == " + el.GoName
	case "pattern_optional_some":
		inner := e.patternTest(p.Child("sub_pattern"), "(*"+subj+")", st)
		if inner == "true" {
			return subj + " != nil"
		}
		return subj + " != nil && " + inner
	case "pattern_bool":
		if p.Bool("value") {
			return subj
		}
		return "!" + subj
	case "pattern_tuple":
		var parts []string
		for i, el := range p.List("elements") {
			t := e.patternTest(el, derefParen(subj)+".F"+itoa(i), nil)
			if t != "true" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			return "true"
		}
		return strings.Join(parts, " && ")
	}
	return e.m.Unsup("pattern:"+p.Kind(), e.where, "bool")
}

func unwrapPatternKeepSome(p ast.Node) ast.Node {
	for p != nil && (p.Kind() == "pattern_typed" || p.Kind() == "pattern_binding" || p.Kind() == "pattern" || p.Kind() == "pattern_paren") {
		p = p.Child("sub_pattern")
	}
	return p
}

func (e *fileEmitter) isPayloadCase(el *DeclInfo) bool {
	ps := el.Node.Child("params")
	return ps != nil && len(ps.List("params")) > 0
}

func (e *fileEmitter) enumElementByName(st *types.Type, p ast.Node) *DeclInfo {
	if st == nil {
		return nil
	}
	for st.Kind == types.Optional {
		st = st.Args[0]
	}
	name := p.Child("element").Child("base_name").Str("name")
	for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
		for _, info := range mod.decls {
			if info.Kind == "enum_element_decl" && info.Node.Name() == name && info.Owner == st.Name {
				if mod.Pkg != e.m.Pkg {
					e.m.importPkgs[mod.Pkg] = mod.ImportPath
					c := *info
					c.GoName = mod.Pkg + "." + info.GoName
					return &c
				}
				return info
			}
		}
	}
	return nil
}

func (e *fileEmitter) payloadSubTests(sub ast.Node, val string, el *DeclInfo) string {
	sub = unwrapPatternKeepSome(sub)
	if sub.Kind() == "pattern_tuple" {
		var parts []string
		for i, s := range sub.List("elements") {
			t := e.patternTest(s, val+".F"+itoa(i), nil)
			if t != "true" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			return "true"
		}
		return strings.Join(parts, " && ")
	}
	return e.patternTest(sub, val+".F0", nil)
}

// patternBind declares variables bound by pattern p from subj.
func (e *fileEmitter) patternBind(p ast.Node, subj string) {
	p = unwrapPatternKeepSome(p)
	switch p.Kind() {
	case "pattern_named":
		e.w("%s := %s\n_ = %s\n", goSafe(p.Name()), subj, goSafe(p.Name()))
	case "pattern_optional_some":
		e.patternBind(p.Child("sub_pattern"), "(*"+subj+")")
	case "pattern_tuple":
		for i, el := range p.List("elements") {
			e.patternBind(el, derefParen(subj)+".F"+itoa(i))
		}
	case "pattern_is":
		if sub := p.Child("sub_pattern"); sub != nil && patternBinds(sub) {
			target := e.m.GoType(e.m.typeOf(p.Str("cast_to")), e.generics)
			e.patternBind(unwrapPattern(sub), "swiftrt.MustAs["+target+"]("+subj+")")
		}
	case "pattern_enum_element":
		if inner, val, ok := e.optionalUnwrap(p, subj); ok {
			if inner != nil {
				e.patternBind(inner, val)
			}
			return
		}
		el := e.enumElement(p, e.m.typeOf(p.Type()))
		sub := p.Child("sub_pattern")
		if el == nil && sub != nil && patternBinds(sub) {
			if val, ok := e.resultPayload(p, subj); ok {
				e.patternBind(unwrapSinglePattern(sub), val)
			}
			return
		}
		if el == nil || sub == nil || !patternBinds(sub) {
			return
		}
		v := e.newTmp("payload")
		e.w("%s, _ := %s.(%s)\n", v, assertable(subj), el.GoName+e.enumTypeArgs(el, e.m.typeOf(p.Type())))
		sub = unwrapPatternKeepSome(sub)
		if sub.Kind() == "pattern_tuple" {
			for i, s := range sub.List("elements") {
				e.patternBind(s, v+".F"+itoa(i))
			}
		} else {
			e.patternBind(sub, v+".F0")
		}
	}
}

func (e *fileEmitter) assign(n ast.Node) {
	dest := n.Child("dest")
	src := n.Child("src")
	if dest.Kind() == "discard_assignment_expr" {
		e.w("_ = %s\n", e.expr(src))
		return
	}
	if dest.Kind() == "subscript_expr" {
		if info, _ := e.lookup(dest.Child("decl").Str("decl_usr")); info != nil && info.Kind == "subscript_decl" && !info.FreeSelf() {
			ss, _ := e.argStrings(e.args(dest.Child("args")), false)
			base := dest.Child("base")
			for base.Kind() == "inout_expr" {
				base = base.Child("sub_expr")
			}
			e.w("%s.Set%s(%s)\n", e.lvalue(base), info.GoName, prefixCommaTail(strings.Join(ss, ", "), e.valueCopy(src)))
			return
		}
		if s, ok := e.subscriptAssign(dest, src); ok {
			e.w("%s\n", s)
			return
		}
	}
	if d := stripLoads(dest); d.Kind() == "keypath_application_expr" && strings.HasPrefix(e.exprGoType(d.Child("keypath")), "swiftrt.WritableKeyPath[") {
		base := stripLoads(d.Child("base"))
		recv := "&" + e.lvalue(base)
		if t := e.m.typeOf(base.Type()); t != nil && e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
			recv = "swiftrt.Ref(" + e.expr(base) + ")"
		}
		e.w("%s.Set(%s, %s)\n", e.expr(d.Child("keypath")), recv, e.valueCopy(src))
		return
	}
	if call, ok := e.setterCall(dest, e.valueCopy(src)); ok {
		e.w("%s\n", call)
		return
	}
	if dest.Kind() == "tuple_expr" {
		var ls, rs []string
		tmp := e.newTmp("t")
		e.w("%s := %s\n", tmp, e.expr(src))
		for i, d := range dest.List("elements") {
			ls = append(ls, e.lvalue(d))
			rs = append(rs, tmp+".F"+itoa(i))
		}
		e.w("%s = %s\n", strings.Join(ls, ", "), strings.Join(rs, ", "))
		return
	}
	e.w("%s = %s\n", e.lvalue(dest), e.valueCopy(src))
}

func (e *fileEmitter) doCatch(n ast.Node) {
	ret := e.newTmp("ret")
	done := e.newTmp("returned")
	e.w("{\n")
	if e.resultType != "" && !e.inInit {
		e.w("var %s %s\n_ = %s\n", ret, e.resultType, ret)
	}
	e.w("%s := false\n", done)
	saveDo := e.doRet
	do := &doReturn{val: ret, flag: done, depth: e.jumpDepth, jumps: map[string]string{}, inner: map[string]bool{}}
	saved := e.b
	e.b = strings.Builder{}
	e.doRet = do
	e.block(n.Child("body"))
	e.doRet = saveDo
	body := e.b.String()
	e.b = saved
	for _, jump := range sortedKeys(do.jumps) {
		e.w("%s := false\n", do.jumps[jump])
	}
	e.w("if err := swiftrt.Catch(func() %s", body)
	e.w("); err != nil {\n")
	matched := e.newTmp("caught")
	e.w("%s := false\n", matched)
	for _, c := range n.List("catch_stmts") {
		items := c.List("case_label_items")
		var tests []string
		var bind ast.Node
		for _, it := range items {
			p := it.Child("pattern")
			up := unwrapPattern(p)
			if up == nil || up.Kind() == "pattern_any" || up.Kind() == "pattern_named" {
				tests = append(tests, "true")
				bind = p
				continue
			}
			if up.Kind() == "pattern_is" || up.Kind() == "pattern_enum_element" {
				tests = append(tests, e.patternTest(up, "err", e.m.typeOf(p.Type())))
				bind = up
				continue
			}
			tests = append(tests, e.m.Unsup("catch-pattern:"+up.Kind(), e.where, "bool"))
		}
		if len(tests) == 0 {
			tests = []string{"true"}
		}
		e.w("if !%s && (%s) {\n%s = true\n", matched, strings.Join(tests, " || "), matched)
		if bind != nil {
			e.patternBind(bind, "err")
		} else {
			e.w("error := err\n_ = error\n")
		}
		e.stmts(c.Child("body").List("elements"))
		e.w("}\n")
	}
	e.w("if !%s {\npanic(swiftrt.Throw(err))\n}\n}\n", matched)
	switch {
	case e.inInit:
		e.w("if %s {\nreturn %s\n}\n", done, e.initRet)
	case e.resultType != "":
		e.w("if %s {\nreturn %s\n}\n", done, ret)
	default:
		e.w("if %s {\nreturn\n}\n", done)
	}
	for _, jump := range sortedKeys(do.jumps) {
		e.w("if %s {\n%s\n}\n", do.jumps[jump], jump)
	}
	e.w("}\n")
}

// doReturn redirects `return` inside a do body, which runs in a Go closure, to the enclosing function.
type doReturn struct {
	val, flag string
	depth     int               // jumpDepth when the do body began
	jumps     map[string]string // break/continue flags the body set
	inner     map[string]bool   // labels declared inside the body
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// setterCall renders an assignment to a computed or observed property as a call to its SetX.
func (e *fileEmitter) setterCall(dest ast.Node, value string) (string, bool) {
	for dest.Kind() == "load_expr" || dest.Kind() == "paren_expr" || dest.Kind() == "inout_expr" {
		dest = dest.Child("sub_expr")
	}
	var d, base ast.Node
	switch dest.Kind() {
	case "subscript_expr":
		info, _ := e.lookup(dest.Child("decl").Str("decl_usr"))
		if info == nil || info.Kind != "subscript_decl" || info.FreeSelf() {
			return "", false
		}
		ss, _ := e.argStrings(e.args(dest.Child("args")), false)
		b := dest.Child("base")
		for b.Kind() == "inout_expr" {
			b = b.Child("sub_expr")
		}
		return e.lvalue(b) + ".Set" + info.GoName + "(" + prefixCommaTail(strings.Join(ss, ", "), value) + ")", true
	case "member_ref_expr":
		d, base = dest.Child("decl"), dest.Child("base")
	case "declref_expr":
		d = dest.Child("decl")
	default:
		return "", false
	}
	info, pfx := e.lookup(d.Str("decl_usr"))
	if info == nil || !(info.Settable || info.Observed) || info.Kind != "var_decl" {
		return "", false
	}
	if info.Observed && e.inInit && (base == nil || isSelfRef(base)) {
		return "", false
	}
	name := "Set" + info.GoName
	switch {
	case info.Owner == "" || info.Static:
		return pfx + name + "(" + value + ")", true
	case info.ProtoExt || info.OwnerKind == "foreign":
		recv := "self"
		if base != nil {
			recv = e.lvalue(base)
		}
		return pfx + name + "(&" + recv + ", " + value + ")", true
	}
	recv := "self"
	if base != nil {
		recv = e.lvalue(base)
	}
	return recv + "." + name + "(" + value + ")", true
}

func isSelfRef(n ast.Node) bool {
	for n.Kind() == "load_expr" || n.Kind() == "paren_expr" || n.Kind() == "derived_to_base_expr" {
		n = n.Child("sub_expr")
	}
	return n.Kind() == "declref_expr" && n.Child("decl").Str("base_name") == "self" && n.Child("decl").Str("decl_usr") == ""
}

// enumElement resolves the case an enum pattern names, qualifying it when it lives in a dependency.
func (e *fileEmitter) enumElement(p ast.Node, st *types.Type) *DeclInfo {
	el, pfx := e.lookup(p.Child("element_decl").Str("decl_usr"))
	if el == nil {
		if st == nil || st.Kind == types.Unknown {
			st = e.m.typeOf(p.Type())
		}
		return e.enumElementByName(st, p)
	}
	if pfx != "" {
		c := *el
		c.GoName = pfx + el.GoName
		return &c
	}
	return el
}

func nodeKey(n ast.Node) uintptr { return reflect.ValueOf(n).Pointer() }

// derefParen parenthesizes a dereference so a field selector applies to the pointee.
func derefParen(s string) string {
	if strings.HasPrefix(s, "*") {
		return "(" + s + ")"
	}
	return s
}

// resultCase tests a `.success`/`.failure` pattern against a swiftrt.Result, including any payload pattern.
func (e *fileEmitter) resultCase(p ast.Node, subj string) (string, bool) {
	val, ok := e.resultPayload(p, subj)
	if !ok {
		return "", false
	}
	test := derefParen(subj) + ".Ok"
	if p.Child("element").Child("base_name").Str("name") == "failure" {
		test = "!" + test
	}
	if sub := p.Child("sub_pattern"); sub != nil {
		inner := unwrapSinglePattern(sub)
		if t := e.patternTest(inner, val, e.m.typeOf(inner.Type())); t != "true" {
			test += " && " + t
		}
	}
	return test, true
}

// resultPayload is the Go expression for a Result pattern's payload: the success value, or the failure
// asserted to its Swift type.
func (e *fileEmitter) resultPayload(p ast.Node, subj string) (string, bool) {
	t := e.m.typeOf(p.Type())
	if t == nil || t.Kind != types.Named || t.Name != "Swift.Result" {
		return "", false
	}
	switch p.Child("element").Child("base_name").Str("name") {
	case "success":
		return derefParen(subj) + ".Value", true
	case "failure":
		ft := "any"
		if len(t.Args) == 2 {
			ft = e.m.GoType(t.Args[1], e.generics)
		}
		return "swiftrt.MustAs[" + ft + "](" + derefParen(subj) + ".Err)", true
	}
	return "", false
}

// unwrapSinglePattern strips the one-element tuple Swift wraps around an enum payload pattern.
func unwrapSinglePattern(p ast.Node) ast.Node {
	p = unwrapPatternKeepSome(p)
	if p.Kind() == "pattern_tuple" && len(p.List("elements")) == 1 {
		return unwrapPatternKeepSome(p.List("elements")[0])
	}
	return p
}

// isVoidGo reports a Go type that stands for Swift Void or Void?.
func isVoidGo(gt string) bool {
	return gt == "" || gt == "*" || gt == "struct{}" || gt == "*struct{}"
}

func stripLoads(n ast.Node) ast.Node {
	for n != nil && (n.Kind() == "load_expr" || n.Kind() == "paren_expr" || n.Kind() == "inout_expr") {
		n = n.Child("sub_expr")
	}
	return n
}

// optionalCase tests an enum pattern against an Optional subject: `.none`/`nil`, `.some(p)`, or a case of
// the wrapped enum, which Swift matches through an implicit `.some`.
func (e *fileEmitter) optionalCase(p ast.Node, subj string, st *types.Type) (string, bool) {
	t := st
	if t == nil || t.Kind == types.Unknown {
		t = e.m.typeOf(p.Type())
	}
	if t == nil || t.Kind != types.Optional || len(t.Args) != 1 {
		return "", false
	}
	name := p.Child("element").Child("base_name").Str("name")
	val := e.optionalValue(subj, t)
	ptr := subj
	if strings.HasPrefix(subj, "(*") && strings.HasSuffix(subj, ")") {
		ptr, val = subj[2:len(subj)-1], subj
		if name != "none" && name != "some" {
			return "", false
		}
	}
	switch name {
	case "none":
		return ptr + " == nil", true
	case "some":
		test := ptr + " != nil"
		if sub := p.Child("sub_pattern"); sub != nil {
			if inner := e.patternTest(unwrapSinglePattern(sub), val, t.Args[0]); inner != "true" {
				test += " && " + inner
			}
		}
		return test, true
	}
	if t.Args[0].Kind == types.Optional {
		return "", false
	}
	inner := e.patternTest(p, val, t.Args[0])
	return subj + " != nil && " + inner, true
}

// optionalUnwrap binds the payload of a `.some(p)` pattern, or of a wrapped-enum case matched against an
// Optional subject.
func (e *fileEmitter) optionalUnwrap(p ast.Node, subj string) (ast.Node, string, bool) {
	t := e.m.typeOf(p.Type())
	if t == nil || t.Kind != types.Optional || len(t.Args) != 1 {
		return nil, "", false
	}
	val := e.optionalValue(subj, t)
	switch p.Child("element").Child("base_name").Str("name") {
	case "none":
		return nil, "", true
	case "some":
		if sub := p.Child("sub_pattern"); sub != nil && patternBinds(sub) {
			return unwrapSinglePattern(sub), val, true
		}
		return nil, "", true
	}
	return nil, "", false
}

func (e *fileEmitter) optionalValue(subj string, t *types.Type) string {
	if e.m.isReferenceGo(t.Args[0], e.m.GoType(t.Args[0], e.generics)) {
		return subj
	}
	return "(*" + subj + ")"
}

func containsKind(n ast.Node, kind string) bool {
	found := false
	ast.Walk(n, func(x ast.Node) {
		if x.Kind() == kind {
			found = true
		}
	})
	return found
}

// assertable parenthesizes a dereferenced subject so a type assertion applies to the pointee.
func assertable(subj string) string {
	if strings.HasPrefix(subj, "*") {
		return "(" + subj + ")"
	}
	return subj
}

func rangeStart(n ast.Node) (int, bool) {
	r := n.Child("range")
	if r == nil {
		return 0, false
	}
	f, ok := r["start"].(float64)
	return int(f), ok
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// jumpLabel is the label a `break name` or `continue name` names, read from the source because the
// AST dump leaves labels out.
func (e *fileEmitter) jumpLabel(n ast.Node) string {
	off, ok := rangeStart(n)
	src := e.m.sourceText(e.file.Filename)
	if !ok || off >= len(src) {
		return ""
	}
	i := off
	for i < len(src) && isIdentByte(src[i]) {
		i++
	}
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	j := i
	for j < len(src) && isIdentByte(src[j]) {
		j++
	}
	label := string(src[i:j])
	switch label {
	case "", "case", "default", "return", "if", "else", "for", "while", "let", "var", "guard", "switch", "do", "try":
		return ""
	}
	return label
}

// stmtLabel is the `name:` label written before a loop or switch, if any.
func (e *fileEmitter) stmtLabel(n ast.Node) string {
	off, ok := rangeStart(n)
	src := e.m.sourceText(e.file.Filename)
	if !ok || off > len(src) {
		return ""
	}
	// The statement's range starts at its label: "name: for ...".
	end := off
	for end < len(src) && isIdentByte(src[end]) {
		end++
	}
	if k := end; end > off {
		for k < len(src) && (src[k] == ' ' || src[k] == '\t') {
			k++
		}
		if k < len(src) && src[k] == ':' {
			return string(src[off:end])
		}
	}
	i := off - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i--
	}
	if i < 0 || src[i] != ':' {
		return ""
	}
	i--
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}
	j := i + 1
	for i >= 0 && isIdentByte(src[i]) {
		i--
	}
	return string(src[i+1 : j])
}

// labelUsed reports whether a jump inside n names label.
func (e *fileEmitter) labelUsed(n ast.Node, label string) bool {
	used := false
	ast.Walk(n, func(x ast.Node) {
		if (x.Kind() == "break_stmt" || x.Kind() == "continue_stmt") && e.jumpLabel(x) == label {
			used = true
		}
	})
	return used
}
