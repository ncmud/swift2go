package emit

import (
	"maps"
	"reflect"
	"slices"
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
	if e.nestedDefers(list) {
		// Swift runs a block's defer when the block exits, Go's at function return: give the block a
		// function of its own.
		e.exitingClosure(func() {
			e.w("{\n")
			e.stmts(list)
			e.w("}")
		}, func(body string) { e.w("func() %s()\n", body) })
		return
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
		// A statement that opened scopes for its own rebinding (a guard binding a name twice) leaves
		// them open for the statements after it.
		opened += e.pendingOpens
		e.pendingOpens = 0
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
		if n.Kind() != "switch_stmt" {
			e.loopDepth++
			defer func() { e.loopDepth-- }()
		}
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
		if e.initFail != "" {
			e.w("return %s\n", e.initFail)
		} else {
			e.w("return nil\n")
		}
	case "then_stmt":
		if r := n.Child("result"); isVoidGo(e.exprGoType(r)) {
			e.stmt(r)
			if e.resultType == "*struct{}" {
				e.w("return &struct{}{}\n")
			} else {
				e.w("return\n")
			}
		} else {
			e.w("return %s\n", e.owned(r, sinkStore))
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
		e.jump(strings.TrimSuffix(n.Kind(), "_stmt"), e.jumpLabel(n))
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
				if after, ok := strings.CutPrefix(s, never); ok {
					s = "swiftrt.FatalError[struct{}](" + after
				}
			}
			if e.isCallLike(n) || isVoidGo(e.exprGoType(n)) && (strings.HasSuffix(s, ")") || strings.HasSuffix(s, "}()")) {
				e.w("%s\n", s)
			} else {
				e.w("_ = %s\n", s)
			}
			return
		}
		if n.Kind() == "typealias" {
			// A local type alias only names a type, which each use already resolved.
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
	key := n.Str("usr")
	if key == "" {
		key = "local:" + n.Name()
	}
	e.m.localFuncs[key] = n
	saveG, saveIO := e.generics, e.inoutParams
	e.inoutParams = maps.Clone(e.inoutParams)
	defer func() { e.inoutParams = saveIO }()
	e.generics = e.genericScope(n)
	params, _ := e.paramList(n)
	res := e.resultGo(n)
	name := goSafe(n.Name())
	if len(e.typeParams) > 0 {
		// Go function literals cannot be generic: hoist to a package-level generic function that
		// takes the captured variables by pointer.
		e.hoistGenericLocal(n, name, params, res)
		e.generics = saveG
		return
	}
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
			savePost := e.post
			e.post = nil
			e.stmt(r)
			for _, p := range e.post {
				e.w("%s\n", p)
			}
			e.post = savePost
		}
		e.w("return\n")
		return
	}
	savePost := e.post
	e.post = nil
	v := e.owned(r, sinkReturn)
	if e.resultType == "*struct{}" && strings.HasPrefix(v, "func() {") && strings.HasSuffix(v, "}()") {
		// An optional chain ending in a Void call (`x?.f()`) returned as `()?`: run it, then return
		// a value. Whether the chain reached the call is not kept.
		e.w("%s\n", v)
		v = "&struct{}{}"
	}
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
		s := sinkLet
		if e.isVarPattern(p) {
			s = sinkStore
		}
		e.w("var %s %s = %s\n", name, gt, e.owned(init, s))
		e.bindSliceBase(p, init)
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
		e.bindOrigin = e.origin(init)
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
		e.w("%s := %s\n_ = %s\n", goSafe(p.Name()), e.bound(p, src), goSafe(p.Name()))
	case "pattern_typed", "pattern_binding", "pattern":
		e.bindFromExpr(p.Child("sub_pattern"), src)
	case "pattern_any":
	case "pattern_tuple":
		for i, el := range p.List("elements") {
			e.bindFromExpr(el, derefParen(src)+".F"+itoa(i))
		}
	case "pattern_enum_element":
		e.patternBind(p, src)
	case "pattern_optional_some":
		// The caller has tested src for nil; a class reference's Optional is the pointer itself.
		sub := p.Child("sub_pattern")
		if t := e.m.typeOf(sub.Type()); t != nil && t.Kind != types.Optional && e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
			e.bindFromExpr(sub, src)
		} else {
			e.bindFromExpr(sub, "(*"+derefParen(src)+")")
		}
	default:
		e.w("_ = %s\n", e.m.Unsup("pattern:"+p.Kind(), e.where, ""))
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// needsCopy reports whether a value of type t has storage a Go assignment would share: a slice, a map,
// a pointer to a value, or an interface that can hold one of those.
func (e *fileEmitter) needsCopy(t *types.Type) bool {
	return e.m.sharesStorage(t, map[string]bool{})
}

// sharesStorage is needsCopy; visiting holds the structs being examined, for recursive types.
func (m *Module) sharesStorage(t *types.Type, visiting map[string]bool) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case types.Array, types.Dict, types.Generic, types.Unknown:
		return true
	case types.Tuple:
		for _, a := range t.Args {
			if m.sharesStorage(a, visiting) {
				return true
			}
		}
		return false
	case types.Optional:
		if len(t.Args) != 1 {
			return false
		}
		if isGenericParamType(t.Args[0]) || !m.isReferenceGo(t.Args[0], m.GoType(t.Args[0], nil)) {
			// T? is a pointer to its own T.
			return true
		}
		return m.sharesStorage(t.Args[0], visiting)
	case types.Existential:
		return t.Name != "Swift.AnyObject" && t.Name != "Swift.Error"
	case types.Named:
		switch t.Name {
		case "Swift.Array", "Swift.Dictionary", "Swift.Set", "Swift.ArraySlice", "Swift.ContiguousArray", "Swift.Any",
			"OrderedCollections.OrderedDictionary", "OrderedCollections.OrderedSet", "DequeModule.Deque", "HeapModule.Heap":
			return true
		}
		if !m.isTranspiledType(t.Name) {
			// A library type shares storage when its Go form is a slice, a map or a runtime collection.
			gt := m.GoType(t, nil)
			for _, shape := range []string{"[]", "map[", "swiftrt.HSet[", "swiftrt.HMap[", "swiftrt.Ordered", "swiftrt.Deque[", "swiftrt.Heap["} {
				if strings.HasPrefix(gt, shape) {
					return true
				}
			}
			return false
		}
		if m.shares == nil {
			m.shares = map[string]bool{}
		}
		if shares, ok := m.shares[t.Name]; ok {
			return shares
		}
		if visiting[t.Name] {
			return false
		}
		visiting[t.Name] = true
		defer delete(visiting, t.Name)
		shares := false
		for _, mod := range append([]*Module{m}, m.Deps...) {
			kind := mod.typeKinds[t.Name]
			switch kind {
			case "protocol":
				shares = !m.classOnly(t.Name, map[string]bool{})
			case "struct_decl", "enum_decl":
				if len(t.Args) > 0 || len(mod.typeParamNames[t.Name]) > 0 {
					shares = true
					break
				}
				for _, d := range mod.decls {
					if d.Owner != t.Name || d.Node == nil || shares {
						continue
					}
					switch {
					case d.Kind == "var_decl" && !d.Static && !d.Computed:
						shares = m.sharesStorage(m.typeOf(d.Node.Str("interface_type")), visiting)
					case d.Kind == "enum_element_decl":
						for _, p := range d.Node.Child("params").List("params") {
							shares = shares || m.sharesStorage(m.typeOf(p.Str("interface_type")), visiting)
						}
					}
				}
			}
			if kind != "" {
				break
			}
		}
		if len(visiting) == 1 {
			// Only a finished answer is kept: one reached inside a cycle may be too optimistic.
			m.shares[t.Name] = shares
		}
		return shares
	}
	return false
}

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
		e.w("if %s {\n", andJoin(bools))
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
			e.bindOrigin = e.origin(init)
			e.patternBind(up, val)
		} else {
			e.w("if %s := %s; %s != nil {\n", tmp, e.expr(init), tmp)
			e.bindOrigin = e.origin(init)
			e.bindFromExpr(sub, e.unwrap(tmp, init))
		}
		e.conds(rest, inner)
		e.w("}\n")
	case "pattern_binding", "pattern_named", "pattern_typed":
		if refutable(p) {
			// `case let (a?, b?) = pair`: test the value once, then bind from it.
			tmp := e.newTmp("subj")
			e.w("{\n%s := %s\nif %s {\n", tmp, e.expr(init), e.patternTest(unwrapPatternKeepSome(p), tmp, e.m.typeOf(init.Type())))
			e.bindOrigin = e.origin(init)
			e.bindFromExpr(p, tmp)
			e.conds(rest, inner)
			e.w("}\n}\n")
			break
		}
		e.w("{\n")
		src := e.expr(init)
		e.bindOrigin = e.origin(init)
		e.bindFromExpr(p, src)
		e.conds(rest, inner)
		e.w("}\n")
	case "pattern_enum_element", "pattern_expr", "pattern_bool":
		tmp := e.newTmp("subj")
		e.w("if %s := %s; %s {\n", tmp, e.expr(init), e.patternTest(p, tmp, e.m.typeOf(init.Type())))
		e.bindOrigin = e.origin(init)
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
		e.w("if %s ", andJoin(parts))
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
	// The else body cannot see the guard's bindings, so a name it shares with one is the outer
	// variable: keep that value reachable after the binding shadows it.
	outer := map[string]string{}
	referenced := map[string]bool{}
	declaredInside := map[string]bool{}
	ast.Walk(n.Child("body"), func(x ast.Node) {
		switch x.Kind() {
		case "declref_expr":
			if x.Child("decl").Str("decl_usr") == "" {
				referenced[x.Child("decl").Str("base_name")] = true
			}
		case "pattern_named":
			declaredInside[x.Child("name").Child("base_name").Str("name")] = true
		case "parameter":
			declaredInside[x.Name()] = true
		}
	})
	// `if let made` inside the else body reads the outer `made` before declaring its own.
	shorthand := map[string]bool{}
	ast.Walk(n.Child("body"), func(x ast.Node) {
		if x.Kind() != "pattern" || x.Child("initializer") == nil {
			return
		}
		init := stripLoads(x.Child("initializer"))
		if init.Kind() == "declref_expr" && init.Child("decl").Str("decl_usr") == "" {
			shorthand[init.Child("decl").Str("base_name")] = true
		}
	})
	for name := range declaredInside {
		if !shorthand[name] {
			delete(referenced, name)
		}
	}
	for _, name := range declaredNames(n) {
		if referenced[name] {
			alias := e.newTmp("outer")
			e.w("%s := %s\n_ = %s\n", alias, e.declRef(ast.Node{"base_name": name, "decl_usr": ""}, nil), alias)
			outer[name] = alias
		}
	}
	shadowed := map[string]string{}
	boundHere := map[string]bool{}
	// Scopes opened for rebinding stay open after the guard; report them once it is done, so the
	// else bodies' own statement lists don't close them.
	opens := 0
	defer func() { e.pendingOpens += opens }()
	elseBody := func() {
		saved := e.renames
		if len(shadowed) > 0 {
			e.renames = maps.Clone(e.renames)
			if e.renames == nil {
				e.renames = map[string]string{}
			}
			maps.Copy(e.renames, shadowed)
		}
		e.block(n.Child("body"))
		e.renames = saved
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
		// `guard let (a, far) = x, let far = far`: the second binding needs a new Go scope.
		for _, name := range patternNames(p) {
			if boundHere[name] {
				e.w("{\n")
				opens++
				boundHere = map[string]bool{}
				break
			}
		}
		for _, name := range patternNames(p) {
			boundHere[name] = true
		}
		switch p.Kind() {
		case "pattern_optional_some":
			tmp := e.newTmp("opt")
			e.w("%s := %s\nif %s == nil ", tmp, e.expr(init), tmp)
			elseBody()
			e.bindOrigin = e.origin(init)
			e.bindFromExpr(p.Child("sub_pattern"), e.unwrap(tmp, init))
		case "pattern_binding", "pattern_named", "pattern_typed":
			if refutable(p) {
				tmp := e.newTmp("subj")
				e.w("%s := %s\nif !(%s) ", tmp, e.expr(init), e.patternTest(unwrapPatternKeepSome(p), tmp, e.m.typeOf(init.Type())))
				elseBody()
				e.bindOrigin = e.origin(init)
				e.bindFromExpr(p, tmp)
				break
			}
			src := e.expr(init)
			e.bindOrigin = e.origin(init)
			e.bindFromExpr(p, src)
		default:
			tmp := e.newTmp("subj")
			e.w("%s := %s\nif !(%s) ", tmp, e.expr(init), e.patternTest(p, tmp, e.m.typeOf(init.Type())))
			elseBody()
			e.bindOrigin = e.origin(init)
			e.patternBind(p, tmp)
		}
		ast.Walk(p, func(x ast.Node) {
			if x.Kind() == "pattern_named" {
				if alias, ok := outer[x.Child("name").Child("base_name").Str("name")]; ok {
					shadowed[x.Child("name").Child("base_name").Str("name")] = alias
				}
			}
		})
	}
}

func (e *fileEmitter) whileStmt(n ast.Node) {
	cs := n.List("conditions")
	if simpleConds(cs) {
		var parts []string
		for _, c := range cs {
			parts = append(parts, e.expr(c))
		}
		e.w("for %s ", andJoin(parts))
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
		// The loop's own bindings are in this scope: a body declaration of the same name shadows them.
		e.paramNames = map[string]bool{}
		ast.Walk(p, func(x ast.Node) {
			if x.Kind() == "pattern_named" {
				e.paramNames[goSafe(x.Child("name").Child("base_name").Str("name"))] = true
			}
		})
		e.stmts(n.Child("body").List("elements"))
	}
	name := func(p ast.Node) string {
		if nm := patternName(p); nm != "" {
			return goSafe(nm)
		}
		return "_"
	}
	st := e.m.typeOf(seq.Type())
	// Each iteration binds part of the value the loop iterates, which the loop keeps unchanged.
	e.bindOrigin = originBorrowedLet
	source := func(s ast.Node) string {
		return e.loopSource(s, map[string]any(n.Child("body")), map[string]any(n.Child("where")))
	}
	// A `for var` element is the loop body's own copy.
	ownElement := func(p ast.Node, nm string) {
		if named := unwrapPattern(p); nm != "_" && named != nil && named.Kind() == "pattern_named" && e.isVarPattern(named) && e.needsCopy(e.m.typeOf(named.Type())) {
			e.w("%s = swiftrt.Copy(%s)\n", nm, nm)
		}
	}
	if containsKind(n.Child("desugared_loop"), "current_context_isolation_expr") {
		et := e.m.GoType(e.m.typeOf(p.Type()), e.generics)
		if et == "" {
			et = "struct{}"
		}
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
		src := source(seq)
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
		base := source(call.base)
		if e.m.Recv(e.m.typeOf(call.base.Type())) == "String" {
			base = "swiftrt.StringChars(" + base + ")"
		}
		inner := unwrapPattern(p)
		if inner.Kind() == "pattern_tuple" {
			els := inner.List("elements")
			if second := unwrapPattern(els[1]); second != nil && second.Kind() == "pattern_tuple" {
				// for (i, (a, b)) in pairs.enumerated(): bind the element's fields.
				el := e.newTmp("el")
				e.w("for %s, %s := range %s {\n", name(els[0]), el, base)
				e.bindFromExpr(second, el)
			} else if a, b := name(els[0]), name(els[1]); a == "_" && b == "_" {
				e.w("for range %s {\n", base)
			} else {
				e.w("for %s, %s := range %s {\n", a, b, base)
				ownElement(els[1], b)
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
		if isVarPattern(p) && v != "_" {
			// The body may change a `for var` binding; the iteration must not see it.
			i := e.newTmp("i")
			e.w("for %s := %s.Lo; %s %s %s.Hi; %s++ {\n%s := %s\n", i, r, i, op, r, i, v, i)
		} else {
			e.w("for %s := %s.Lo; %s %s %s.Hi; %s++ {\n", v, r, v, op, r, v)
		}
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
			e.w("for %s, %s := range swiftrt.MapOrdered(%s) {\n", "_", "kv", source(seq))
			e.w("%s, %s := kv.F0, kv.F1\n", name(els[0]), name(els[1]))
			for _, el := range els[:2] {
				if nm := name(el); nm != "_" {
					e.w("_ = %s\n", nm)
				}
			}
			ownElement(els[1], name(els[1]))
		} else {
			e.w("for _, %s := range swiftrt.MapOrdered(%s) {\n", name(p), source(seq))
		}
	case "Slice", "Set", "OrderedDictionary", "HMap", "OrderedSet", "Heap":
		src := source(seq)
		if st.Kind == types.Named && st.Name == "Swift.DefaultIndices" {
			src = "swiftrt.Iterate[int](" + src + ")"
		}
		switch e.m.Recv(st) {
		case "Set":
			src = "swiftrt.SetElements(" + src + ")"
		case "OrderedDictionary", "HMap":
			src = "swiftrt." + e.m.Recv(st) + "Elements(" + src + ")"
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
			ownElement(p, name(p))
		}
	default:
		if src := e.expr(seq); e.m.mappedCall(src) {
			// A mapped package's sequence is ranged over directly, so it can be a lazy iter.Seq.
			if inner := unwrapPattern(p); inner != nil && inner.Kind() == "pattern_tuple" {
				tmp := e.newTmp("el")
				e.w("for %s := range %s {\n", tmp, src)
				e.bindFromExpr(inner, tmp)
			} else {
				e.w("for %s := range %s {\n", name(p), src)
			}
			break
		}
		if inner := unwrapPattern(p); inner != nil && inner.Kind() == "pattern_tuple" {
			tmp := e.newTmp("el")
			e.w("for _, %s := range swiftrt.Iterate[%s](%s) {\n", tmp, e.m.GoType(e.m.typeOf(p.Type()), e.generics), source(seq))
			e.bindFromExpr(inner, tmp)
			break
		}
		e.w("for _, %s := range swiftrt.Iterate[%s](%s) {\n", name(p), e.m.GoType(e.m.typeOf(p.Type()), e.generics), source(seq))
		ownElement(p, name(p))
	}
	body()
	e.w("}\n")
}

// isVarPattern reports whether p binds with `var`.
func isVarPattern(p ast.Node) bool {
	for p != nil && (p.Kind() == "pattern_typed" || p.Kind() == "pattern") {
		p = p.Child("sub_pattern")
	}
	return p != nil && p.Kind() == "pattern_binding" && p.Str("kind") == "var"
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
	// A case that falls through needs Go's switch; otherwise an if-chain reads better.
	goSwitch := false
	for _, c := range n.List("cases") {
		ast.Walk(c.Child("body"), func(x ast.Node) {
			if x.Kind() == "fallthrough_stmt" {
				goSwitch = true
			}
		})
	}
	if goSwitch {
		e.w("switch {\n")
	}
	first := true
	for _, c := range n.List("cases") {
		var tests []string
		isDefault := false
		var bindPat ast.Node
		var bindPats []ast.Node
		var bindTestStrs []string
		guarded := false
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
			if g := whereExpr(item); g != nil {
				bindPat = p
				// The guard only reads its bindings.
				e.bindOrigin = originFresh
				t = "(" + t + ") && " + e.withBindings(p, tmp, e.expr(g))
			}
			tests = append(tests, "("+t+")")
			if whereExpr(item) != nil {
				guarded = true
			}
			if patternBinds(p) {
				bindPat = p
				bindPats = append(bindPats, p)
				bindTestStrs = append(bindTestStrs, t)
			}
		}
		cond := strings.Join(tests, " || ")
		if isDefault {
			cond = "true"
		}
		switch {
		case goSwitch:
			e.w("case %s:\n", cond)
		case first:
			e.w("if %s {\n", cond)
		default:
			e.w("} else if %s {\n", cond)
		}
		first = false
		if len(bindPats) > 1 && !guarded {
			// `case .a(let x), .b(let x):` binds x from whichever pattern matched.
			e.bindOrigin = e.origin(subj)
			e.alternativeBind(bindPats, bindTestStrs, tmp)
		} else if bindPat != nil {
			e.bindOrigin = e.origin(subj)
			e.patternBind(bindPat, tmp)
		}
		if goSwitch {
			if els := c.Child("body").List("elements"); !(len(els) == 1 && els[0].Kind() == "break_stmt") {
				e.stmts(els)
			}
		} else {
			e.caseBody(c.Child("body"))
		}
	}
	if !first || goSwitch {
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

// alternativeBind declares the variables a case's alternative patterns share, each taken from the
// first pattern whose test holds.
func (e *fileEmitter) alternativeBind(pats []ast.Node, tests []string, subj string) {
	type binding struct{ name, slot string }
	var names []binding
	ast.Walk(map[string]any(pats[0]), func(n ast.Node) {
		if n.Kind() == "pattern_named" {
			b := binding{goSafe(n.Name()), e.newTmp("alt")}
			names = append(names, b)
			e.w("var %s %s\n", b.slot, e.m.GoType(e.m.typeOf(n.Type()), e.generics))
		}
	})
	for i, p := range pats {
		if i == 0 {
			e.w("if %s {\n", tests[i])
		} else {
			e.w("} else if %s {\n", tests[i])
		}
		e.patternBind(p, subj)
		for _, b := range names {
			e.w("%s = %s\n", b.slot, b.name)
		}
	}
	e.w("}\n")
	for _, b := range names {
		e.w("%s := %s\n_ = %s\n", b.name, b.slot, b.name)
	}
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
			if lg := e.m.GoType(lt, e.generics); st != nil && e.m.GoType(st, e.generics) == "error" && lg != "error" && lg != "" {
				// A caught error matched against a value of a concrete error type (`catch E.empty`).
				return "func() bool { v := swiftrt.As[" + lg + "](" + subj + "); return v != nil && *v == " + e.expr(lit) + " }()"
			}
			return subj + " == " + e.expr(lit)
		}
		return e.m.Unsup("pattern_expr-shape", e.where, "bool")
	case "pattern_is":
		target := e.m.GoType(e.m.typeOf(p.Str("cast_to")), e.generics)
		test := "swiftrt.Is[" + target + "](" + subj + ")"
		if sub := p.Child("sub_pattern"); sub != nil {
			if t := e.patternTest(unwrapPattern(sub), "swiftrt.MustAs["+target+"]("+subj+")", e.m.typeOf(p.Str("cast_to"))); t != "true" {
				test = andJoin([]string{test, t})
			}
		}
		return test
	case "pattern_enum_element":
		if t, ok := e.optionalCase(p, subj, st, false); ok {
			return t
		}
		el := e.enumElement(p, st)
		if el == nil {
			if t, ok := e.resultCase(p, subj); ok {
				return t
			}
			if c := e.mappedCase(p, st); c != "" {
				return subj + " == " + c
			}
			pt := st
			if pt == nil {
				pt = e.m.typeOf(p.Type())
			}
			if caseT, ok := e.foreignPayloadCase(p, pt); ok {
				test := "func() bool { _, ok := " + assertable(subj) + ".(" + caseT + "); return ok }()"
				val := "func() " + caseT + " { v, _ := " + assertable(subj) + ".(" + caseT + "); return v }()"
				if inner := e.payloadSubTests(p.Child("sub_pattern"), val, nil); inner != "true" {
					test = andJoin([]string{test, inner})
				}
				return test
			}
			if pt != nil && pt.Kind == types.Named && p.Child("sub_pattern") == nil && len(pt.Args) == 0 {
				// A case without a payload of an enum the runtime models: its constant, named as
				// an expression naming the case is.
				if gt := e.m.GoType(pt, e.generics); strings.HasPrefix(gt, "swiftrt.") && !strings.ContainsAny(gt, "[* /") {
					return subj + " == " + gt + capitalize(p.Child("element").Child("base_name").Str("name"))
				}
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
					test = andJoin([]string{test, inner})
				}
			}
			return test
		}
		return subj + " == " + el.GoName
	case "pattern_optional_some":
		val := "(*" + subj + ")"
		if t := e.m.typeOf(p.Child("sub_pattern").Type()); t != nil && t.Kind != types.Optional && e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
			val = subj
		}
		inner, derefed := "", val != subj
		if up := unwrapPattern(p.Child("sub_pattern")); derefed && up != nil && up.Kind() == "pattern_enum_element" {
			if t, ok := e.optionalCase(up, val, st, true); ok {
				inner = t
			}
		}
		if inner == "" {
			it := st
			if derefed && st != nil && st.Kind == types.Optional && len(st.Args) == 1 {
				it = st.Args[0]
			}
			inner = e.patternTest(p.Child("sub_pattern"), val, it)
		}
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
		return andJoin(parts)
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
		return andJoin(parts)
	}
	return e.patternTest(sub, val+".F0", nil)
}

// patternBind declares variables bound by pattern p from subj.
func (e *fileEmitter) patternBind(p ast.Node, subj string) {
	p = unwrapPatternKeepSome(p)
	switch p.Kind() {
	case "pattern_named":
		e.w("%s := %s\n_ = %s\n", goSafe(p.Name()), e.bound(p, subj), goSafe(p.Name()))
	case "pattern_optional_some":
		sub := p.Child("sub_pattern")
		if t := e.m.typeOf(sub.Type()); t != nil && t.Kind != types.Optional && e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
			// A class reference's Optional is the pointer itself.
			e.patternBind(sub, subj)
		} else {
			e.patternBind(sub, "(*"+subj+")")
		}
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
			} else if caseT, ok := e.foreignPayloadCase(p, e.m.typeOf(p.Type())); ok {
				v := e.newTmp("payload")
				e.w("%s, _ := %s.(%s)\n", v, assertable(subj), caseT)
				e.payloadBind(sub, v)
			}
			return
		}
		if el == nil || sub == nil || !patternBinds(sub) {
			return
		}
		v := e.newTmp("payload")
		e.w("%s, _ := %s.(%s)\n", v, assertable(subj), el.GoName+e.enumTypeArgs(el, e.m.typeOf(p.Type())))
		e.payloadBind(sub, v)
	}
}

// payloadBind declares the variables an enum payload pattern binds from the case struct v.
func (e *fileEmitter) payloadBind(sub ast.Node, v string) {
	sub = unwrapPatternKeepSome(sub)
	if sub.Kind() == "pattern_tuple" {
		for i, s := range sub.List("elements") {
			e.patternBind(s, v+".F"+itoa(i))
		}
	} else {
		e.patternBind(sub, v+".F0")
	}
}

// foreignPayloadCase is the runtime case struct for a payload pattern on an enum from another Swift
// module, which swiftrt models as an interface implemented by one struct per case, named after the
// enum and the case (`WebSocketMessage.text(_:)` is swiftrt.WebSocketMessageText).
func (e *fileEmitter) foreignPayloadCase(p ast.Node, st *types.Type) (string, bool) {
	if p.Child("sub_pattern") == nil {
		return "", false
	}
	for st != nil && st.Kind == types.Optional && len(st.Args) == 1 {
		st = st.Args[0]
	}
	if st == nil || st.Kind != types.Named || len(st.Args) > 0 {
		return "", false
	}
	name := p.Child("element").Child("base_name").Str("name")
	if name == "" {
		return "", false
	}
	if mp := e.m.mappedTypeModule(st.Name); mp != nil {
		// A mapped package declares the same shape: one struct per payload case, fields F0, F1, ….
		return mp.Alias() + "." + mappedOwner(st.Name) + capitalize(name), true
	}
	gt := e.m.GoType(st, e.generics)
	if !strings.HasPrefix(gt, "swiftrt.") || strings.ContainsAny(gt, "[* /") {
		return "", false
	}
	return gt + capitalize(name), true
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
			e.w("%s.Set%s(%s)\n", e.lvalue(base), info.GoName, prefixCommaTail(strings.Join(ss, ", "), e.owned(src, sinkStore)))
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
		e.w("%s.Set(%s, %s)\n", e.expr(d.Child("keypath")), recv, e.owned(src, sinkStore))
		return
	}
	setterSink := sinkStore
	if d := stripLoads(dest); d.Kind() == "member_ref_expr" {
		if info, _ := e.lookup(d.Child("decl").Str("decl_usr")); info != nil && info.Computed && info.Settable && !info.Observed {
			// The setter's body copies what it keeps.
			setterSink = sinkBorrow
		}
	}
	if call, ok := e.setterCall(dest, e.owned(src, setterSink)); ok {
		e.w("%s\n", call)
		return
	}
	if dest.Kind() == "tuple_expr" {
		var ls, rs []string
		tmp := e.newTmp("t")
		e.w("%s := %s\n", tmp, e.owned(src, sinkStore))
		for i, d := range dest.List("elements") {
			ls = append(ls, e.lvalue(d))
			rs = append(rs, tmp+".F"+itoa(i))
		}
		e.w("%s = %s\n", strings.Join(ls, ", "), strings.Join(rs, ", "))
		return
	}
	e.w("%s = %s\n", e.lvalue(dest), e.owned(src, sinkStore))
}

// jump writes a break or continue. Inside a block that runs as a Go closure, one that leaves the
// block sets the block's flag for it and returns, and the code after the closure repeats it.
func (e *fileEmitter) jump(kind, label string) {
	text := kind
	if label != "" {
		text += " " + label
	}
	d := e.doRet
	leaves := false
	if d != nil {
		switch {
		case label != "":
			leaves = !d.inner[label]
		case kind == "continue":
			leaves = e.loopDepth == d.loops
		default:
			leaves = e.jumpDepth == d.depth
		}
	}
	if !leaves {
		e.w("%s\n", text)
		return
	}
	flag := d.flag + "_" + strings.ReplaceAll(text, " ", "_")
	d.jumps[text] = flag
	e.w("%s = true\nreturn\n", flag)
}

// nestedDefers reports whether list, a block other than a function body, holds a defer.
func (e *fileEmitter) nestedDefers(list []ast.Node) bool {
	found := false
	for _, st := range list {
		if st.Kind() == "defer_stmt" && !e.topLevel[nodeKey(st)] {
			found = true
		}
	}
	if !found {
		return false
	}
	if e.topLevel == nil {
		e.topLevel = map[uintptr]bool{}
	}
	for _, st := range list {
		e.topLevel[nodeKey(st)] = true
	}
	return true
}

// exitingClosure emits a block that runs as a Go function literal. emit writes the literal's body,
// braces included; wrap places it. A return, break or continue that leaves the block sets a flag
// inside the literal, and the enclosing function repeats it after the literal returns.
func (e *fileEmitter) exitingClosure(emit func(), wrap func(body string)) {
	ret := e.newTmp("ret")
	done := e.newTmp("returned")
	e.w("{\n")
	if e.resultType != "" && !e.inInit {
		e.w("var %s %s\n_ = %s\n", ret, e.resultType, ret)
	}
	e.w("%s := false\n", done)
	saveDo := e.doRet
	do := &doReturn{val: ret, flag: done, depth: e.jumpDepth, loops: e.loopDepth, jumps: map[string]string{}, inner: map[string]bool{}}
	saved := e.b
	e.b = strings.Builder{}
	e.doRet = do
	emit()
	e.doRet = saveDo
	body := e.b.String()
	e.b = saved
	for _, jump := range sortedKeys(do.jumps) {
		e.w("%s := false\n", do.jumps[jump])
	}
	wrap(body)
	e.w("if %s {\n", done)
	switch {
	case saveDo != nil:
		// This block is itself inside a closure: leave that one too.
		if e.resultType != "" && !e.inInit {
			e.w("%s = %s\n", saveDo.val, ret)
		}
		e.w("%s = true\nreturn\n", saveDo.flag)
	case e.inInit:
		e.w("return %s\n", e.initRet)
	case e.resultType != "":
		e.w("return %s\n", ret)
	default:
		e.w("return\n")
	}
	e.w("}\n")
	for _, text := range sortedKeys(do.jumps) {
		e.w("if %s {\n", do.jumps[text])
		kind, label, _ := strings.Cut(text, " ")
		e.jump(kind, label)
		e.w("}\n")
	}
	e.w("}\n")
}

func (e *fileEmitter) doCatch(n ast.Node) {
	e.exitingClosure(func() { e.block(n.Child("body")) }, func(body string) {
		e.w("if err := swiftrt.Catch(func() %s", body)
		e.catchClauses(n)
	})
}

// catchClauses writes the rest of the if statement that doCatch opens: it tests err against each
// catch clause in order and rethrows when none matches.
func (e *fileEmitter) catchClauses(n ast.Node) {
	e.w("); err != nil {\n")
	matched := e.newTmp("caught")
	e.w("%s := false\n", matched)
	for _, c := range n.List("catch_stmts") {
		items := c.List("case_label_items")
		var tests []string
		var bind ast.Node
		bindSrc := func(b ast.Node) string {
			// A typed throw (`do throws(ParseError)`) binds error at the concrete type.
			if gt := e.m.GoType(e.m.typeOf(b.Type()), e.generics); b.Kind() != "pattern_is" && gt != "" && gt != "error" && gt != "any" && !strings.HasPrefix(gt, "any ") {
				return "swiftrt.MustAs[" + gt + "](err)"
			}
			return "err"
		}
		for _, it := range items {
			p := it.Child("pattern")
			up := unwrapPattern(p)
			var t string
			var b ast.Node
			switch {
			case up == nil || up.Kind() == "pattern_any" || up.Kind() == "pattern_named":
				t, b = "true", p
			case up.Kind() == "pattern_is" || up.Kind() == "pattern_enum_element":
				if up.Kind() == "pattern_is" {
					if ct := e.m.typeOf(up.Str("cast_to")); ct != nil && strings.HasPrefix(e.m.GoType(ct, e.generics), "any") {
						t = e.m.Unsup("catch-unmapped-error-type", e.where, "bool")
						break
					}
				}
				subj := "err"
				if up.Kind() == "pattern_enum_element" {
					// The caught error holds the thrown enum value, which need not be a Go error itself.
					subj = "swiftrt.Dynamic(err)"
				}
				t, b = e.patternTest(up, subj, e.m.typeOf(p.Type())), up
			default:
				t = e.m.Unsup("catch-pattern:"+up.Kind(), e.where, "bool")
			}
			if g := whereExpr(it); g != nil && b != nil {
				// `catch … where cond`: the clause matches only when cond holds for its bindings.
				t = "(" + t + ") && " + e.withBindings(b, bindSrc(b), e.expr(g))
			}
			if b != nil {
				bind = b
			}
			tests = append(tests, t)
		}
		if len(tests) == 0 {
			tests = []string{"true"}
		}
		e.w("if !%s && (%s) {\n%s = true\n", matched, strings.Join(tests, " || "), matched)
		if bind != nil {
			e.patternBind(bind, bindSrc(bind))
		} else {
			e.w("error := err\n_ = error\n")
		}
		e.stmts(c.Child("body").List("elements"))
		e.w("}\n")
	}
	e.w("if !%s {\npanic(swiftrt.Throw(err))\n}\n}\n", matched)
}

// doReturn redirects `return` inside a do body, which runs in a Go closure, to the enclosing function.
type doReturn struct {
	val, flag string
	depth     int               // jumpDepth when the do body began
	loops     int               // loopDepth when the do body began
	jumps     map[string]string // break/continue flags the body set
	inner     map[string]bool   // labels declared inside the body
}

func sortedKeys(m map[string]string) []string {
	keys := slices.Sorted(maps.Keys(m))
	return keys
}

// setterCall renders an assignment to a computed or observed property as a call to its SetX.
func (e *fileEmitter) setterCall(dest ast.Node, value string) (string, bool) {
	for dest.Kind() == "load_expr" || dest.Kind() == "paren_expr" || dest.Kind() == "inout_expr" {
		dest = dest.Child("sub_expr")
	}
	if dest.Bool("direct_to_storage") {
		// Inside a property's own observers or accessors, Swift writes its storage directly.
		return "", false
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
	if mp := e.m.mappedDecl(d.Str("decl_usr")); mp != nil {
		return e.mappedSetter(mp, d, base, value), true
	}
	info, pfx := e.lookup(d.Str("decl_usr"))
	if info == nil || !(info.Settable || info.Observed) || info.Kind != "var_decl" {
		return "", false
	}
	if info.Observed && e.initializing(info) && (base == nil || isSelfRef(base)) {
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
			test = andJoin([]string{test, t})
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
// the wrapped enum, which Swift matches through an implicit `.some`. derefed marks a subj of the form
// `(*ptr)` that an enclosing `?` pattern already unwrapped from st, so ptr is the Optional tested.
func (e *fileEmitter) optionalCase(p ast.Node, subj string, st *types.Type, derefed bool) (string, bool) {
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
	if derefed {
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
				test = andJoin([]string{test, inner})
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

// andJoin joins a Swift condition list with &&, parenthesizing conditions that contain ||, which
// binds looser than && in Go.
func andJoin(conds []string) string {
	out := make([]string, len(conds))
	for i, c := range conds {
		if strings.Contains(c, "||") && !(strings.HasPrefix(c, "(") && strings.HasSuffix(c, ")") && balancedParens(c[1:len(c)-1])) {
			c = "(" + c + ")"
		}
		out[i] = c
	}
	return strings.Join(out, " && ")
}

// balancedParens reports whether s never closes a parenthesis it did not open.
func balancedParens(s string) bool {
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// localGeneric is a generic local function hoisted to package level.
type localGeneric struct {
	goName   string
	captures []string
}

func (e *fileEmitter) hoistGenericLocal(n ast.Node, name, params, res string) {
	goName := "local" + capitalize(name) + itoa(e.m.nextHoist())
	var caps, capParams []string
	io := map[string]bool{}
	for _, c := range n.List("captures") {
		d := c.Child("decl")
		cn := d.Str("base_name")
		if cn == "" || cn == "self" {
			continue
		}
		caps = append(caps, goSafe(cn))
		capParams = append(capParams, goSafe(cn)+" *"+e.m.GoType(e.m.typeOf(d.Str("type_usr")), e.generics))
		io[cn] = true
	}
	all := strings.Join(capParams, ", ")
	if params != "" {
		all = strings.Join(append(capParams, params), ", ")
	}
	saved, saveIO := e.b, e.inoutParams
	e.b = strings.Builder{}
	e.inoutParams = io
	e.w("func %s%s(%s) %s ", goName, e.typeParamList(), all, res)
	saveInit := e.inInit
	e.inInit = false
	e.body(n, res)
	e.inInit = saveInit
	e.w("\n\n")
	e.extraDecls.WriteString(e.b.String())
	e.b, e.inoutParams = saved, saveIO
	if e.localGenerics == nil {
		e.localGenerics = map[string]localGeneric{}
	}
	e.localGenerics[n.Name()] = localGeneric{goName: goName, captures: caps}
}

// whereExpr is a case item's `where` clause. Swift 6.4 dumps it as where_expr.
func whereExpr(item ast.Node) ast.Node {
	if w := item.Child("where_expr"); w != nil {
		return w
	}
	return item.Child("guard_expr")
}

// refutable reports whether a pattern can fail to match.
func refutable(p ast.Node) bool {
	r := false
	ast.Walk(p, func(x ast.Node) {
		switch x.Kind() {
		case "pattern_optional_some", "pattern_enum_element", "pattern_expr", "pattern_is", "pattern_bool":
			r = true
		}
	})
	return r
}

// patternNames lists the names a pattern binds.
func patternNames(p ast.Node) []string {
	var out []string
	ast.Walk(p, func(x ast.Node) {
		if x.Kind() == "pattern_named" {
			out = append(out, goSafe(x.Child("name").Child("base_name").Str("name")))
		}
	})
	return out
}
