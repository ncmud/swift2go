package emit

import (
	"maps"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// Swift arrays, dictionaries, sets, structs, optionals and existentials of them are values: every
// variable, property, element and parameter holds its own. Their Go forms (slices, maps, pointers,
// interfaces) share storage when assigned.
//
// The emitter keeps one invariant: no two places share storage. A mutation through a place can then
// write in place. To keep it, each value expression has an origin (where its storage lives) and each
// use a sink (where the value goes), and origin.copies says when the two need a copy between them:
//
//   - a value put into a place (a var, a property, an element, an enum case) is copied unless it is a
//     temporary;
//   - a value returned is copied unless it is a temporary or a local of the function, which moves;
//   - a value bound to a `let`, passed as an argument or iterated by a loop is lent, and copied only
//     if the place it was read from can change before the loan ends.
//
// The last point is where the copies Swift never makes are saved. A place can change during a loan
// only if it is shared (a class property, a global) and the code that runs meanwhile can write shared
// places (effects.go), or if it is a local variable that the loop body or a closure writes.

// origin says where the storage of a value expression lives.
type origin int

const (
	// originFresh is a temporary nothing else refers to: a literal, or the result of a call.
	originFresh origin = iota
	// originOwnedLet is a `let` of the current function that owns its storage.
	originOwnedLet
	// originBorrowedLet cannot change while the current function runs, but belongs to someone else: a
	// parameter, `self` in a non-mutating method, a captured or global `let`, a `let` bound to part of
	// one of those.
	originBorrowedLet
	// originLocalVar is a `var` of the current function that no closure captures.
	originLocalVar
	// originExclusive is an inout parameter or a mutating method's `self`: only the current function
	// can reach it until it returns.
	originExclusive
	// originCaptured is a `var` that a closure and the function declaring it both use.
	originCaptured
	// originShared is a place other code can change during any call: a property of a class instance,
	// a global or static `var`.
	originShared
)

// sink says where a value is going.
type sink int

const (
	// sinkLet binds an immutable local.
	sinkLet sink = iota
	// sinkStore puts the value into a place that outlives the expression: a `var`, an assignment, an
	// element of a collection or aggregate, a mutating call's argument.
	sinkStore
	// sinkReturn hands the value to the caller.
	sinkReturn
	// sinkBorrow lends the value for the length of a call or a loop: an argument, a `for … in` source.
	sinkBorrow
	// sinkNone is a value used where it is written, such as an operand or a condition.
	sinkNone
)

// copies reports whether a value of origin o must be copied on its way into s.
func (o origin) copies(s sink) bool {
	switch s {
	case sinkLet:
		return o >= originLocalVar
	case sinkStore:
		return o != originFresh
	case sinkReturn:
		// A local the function owns dies with the function, so its storage moves to the caller.
		return o != originFresh && o != originOwnedLet && o != originLocalVar
	case sinkBorrow:
		return o >= originCaptured
	case sinkNone:
		return false
	}
	return true
}

// valueFrame is the local scope of one function, accessor or closure.
type valueFrame struct {
	fn       ast.Node
	quiet    int // memo of quietFrame on the outermost frame: 1 yes, 2 no
	parent   *valueFrame
	params   map[string]bool // parameter name → inout
	lets     map[string]int  // declarations of the name as `let` in the body
	vars     map[string]bool // names declared as `var` in the body
	inits    map[string]ast.Node
	captured map[string]bool // names nested functions and closures refer to
	written  map[string]bool // names nested functions and closures write
	escapes  map[string]bool // names an escaping closure or a nested function refers to
	owned    map[string]int  // memo of ownsLet: 1 owned, 2 borrowed, 3 being decided
}

// localRef resolves one reference to a local name.
type localRef struct {
	frame *valueFrame // where the name is declared
	name  string
	cross bool // declared outside the function or closure that refers to it
}

// valueScopes is what resolveLocals learns about a file.
type valueScopes struct {
	refs      map[uintptr]localRef    // declref_expr → the local it names
	within    map[uintptr]*valueFrame // member_ref_expr and declref_expr → the function it is in
	varPats   map[uintptr]bool        // pattern_named nodes under a `var` pattern
	stmtPats  map[uintptr]bool        // pattern_named nodes of `let`/`var` statements
	patFrames map[uintptr]*valueFrame // pattern_named → the function that declares it
	bodies    map[uintptr]*valueFrame // function-like node → its frame
}

func isFunctionLike(kind string) bool {
	switch kind {
	case "func_decl", "constructor_decl", "destructor_decl", "accessor_decl", "closure_expr":
		return true
	}
	return false
}

// resolveLocals walks a file once and resolves every reference to a local variable to its declaration,
// by Swift's lexical scoping over functions and closures. The dump names locals without a USR.
func resolveLocals(items []ast.Node) *valueScopes {
	vs := newValueScopes()
	for _, it := range items {
		vs.walk(map[string]any(it), nil, false)
	}
	return vs
}

func newValueScopes() *valueScopes {
	return &valueScopes{refs: map[uintptr]localRef{}, within: map[uintptr]*valueFrame{}, varPats: map[uintptr]bool{}, stmtPats: map[uintptr]bool{},
		patFrames: map[uintptr]*valueFrame{}, bodies: map[uintptr]*valueFrame{}}
}

// resolveLocalsParallel is [resolveLocals] over the items of many files, which resolve
// independently: each item's scopes start at its top level.
func resolveLocalsParallel(files []ast.File) *valueScopes {
	shards := make([]*valueScopes, len(files))
	parallel(len(files), func(i int) { shards[i] = resolveLocals(files[i].Items) })
	var n [6]int
	for _, sh := range shards {
		n[0] += len(sh.refs)
		n[1] += len(sh.within)
		n[2] += len(sh.varPats)
		n[3] += len(sh.stmtPats)
		n[4] += len(sh.patFrames)
		n[5] += len(sh.bodies)
	}
	vs := &valueScopes{refs: make(map[uintptr]localRef, n[0]), within: make(map[uintptr]*valueFrame, n[1]),
		varPats: make(map[uintptr]bool, n[2]), stmtPats: make(map[uintptr]bool, n[3]),
		patFrames: make(map[uintptr]*valueFrame, n[4]), bodies: make(map[uintptr]*valueFrame, n[5])}
	for _, sh := range shards {
		maps.Copy(vs.refs, sh.refs)
		maps.Copy(vs.within, sh.within)
		maps.Copy(vs.varPats, sh.varPats)
		maps.Copy(vs.stmtPats, sh.stmtPats)
		maps.Copy(vs.patFrames, sh.patFrames)
		maps.Copy(vs.bodies, sh.bodies)
	}
	return vs
}

// walk resolves the references under n, which is inside f; loaded says n is only read (it sits under
// a load_expr).
func (vs *valueScopes) walk(n any, f *valueFrame, loaded bool) {
	switch v := n.(type) {
	case []any:
		for _, c := range v {
			vs.walk(c, f, loaded)
		}
	case map[string]any:
		node := ast.Node(v)
		kind := node.Kind()
		switch {
		case isFunctionLike(kind):
			f = vs.newFrame(node, f)
			loaded = false
		case kind == "load_expr":
			loaded = true
		case kind == "argument_list":
			loaded = false
		}
		if f != nil && (kind == "declref_expr" || kind == "member_ref_expr") {
			vs.within[nodeKey(node)] = f
		}
		if kind == "declref_expr" {
			if d := node.Child("decl"); d.Str("decl_usr") == "" {
				name := d.Str("base_name")
				for g := f; g != nil; g = g.parent {
					_, isParam := g.params[name]
					if !isParam && g.lets[name] == 0 && !g.vars[name] {
						continue
					}
					vs.refs[nodeKey(node)] = localRef{frame: g, name: name, cross: g != f}
					if g != f {
						g.captured[name] = true
						if !loaded && g.vars[name] {
							g.written[name] = true
						}
						for h := f; h != g; h = h.parent {
							if h.fn.Kind() != "closure_expr" || h.fn.Bool("escaping") {
								g.escapes[name] = true
							}
						}
					}
					break
				}
			}
		}
		for _, c := range v {
			switch c.(type) {
			case map[string]any, []any:
				vs.walk(c, f, loaded)
			}
		}
	}
}

// newFrame collects the parameters and local declarations of one function-like node.
func (vs *valueScopes) newFrame(fn ast.Node, parent *valueFrame) *valueFrame {
	f := &valueFrame{fn: fn, parent: parent, params: map[string]bool{}, lets: map[string]int{}, vars: map[string]bool{},
		inits: map[string]ast.Node{}, captured: map[string]bool{}, written: map[string]bool{}, escapes: map[string]bool{}, owned: map[string]int{}}
	vs.bodies[nodeKey(fn)] = f
	for _, p := range fn.Child("params").List("params") {
		f.params[p.Name()] = p.Bool("inout")
	}
	var collect func(n any, inVar bool)
	collect = func(n any, inVar bool) {
		switch v := n.(type) {
		case []any:
			for _, c := range v {
				collect(c, inVar)
			}
		case map[string]any:
			node := ast.Node(v)
			switch kind := node.Kind(); {
			case isFunctionLike(kind):
				return
			case kind == "var_decl":
				if node.Bool("let") {
					f.lets[node.Name()]++
				} else {
					f.vars[node.Name()] = true
				}
				return
			case kind == "pattern_binding_decl":
				for _, pe := range node.List("pattern_entries") {
					if p := unwrapPattern(pe.Child("pattern")); p != nil && p.Kind() == "pattern_named" {
						f.inits[p.Name()] = pe.Child("processed_init")
					}
					ast.Walk(pe.Child("pattern"), func(x ast.Node) {
						if x.Kind() == "pattern_named" {
							vs.stmtPats[nodeKey(x)] = true
						}
					})
				}
			case kind == "pattern_binding":
				inVar = node.Str("kind") == "var"
			case kind == "pattern_named":
				vs.patFrames[nodeKey(node)] = f
				if inVar {
					vs.varPats[nodeKey(node)] = true
					f.vars[node.Name()] = true
				}
			}
			for _, c := range v {
				switch c.(type) {
				case map[string]any, []any:
					collect(c, inVar)
				}
			}
		}
	}
	for k, c := range fn {
		if k != "params" {
			collect(c, false)
		}
	}
	// A pattern that binds without `var` declares a `let`; var_decl nodes exist only for statements.
	var lets func(n any)
	lets = func(n any) {
		switch v := n.(type) {
		case []any:
			for _, c := range v {
				lets(c)
			}
		case map[string]any:
			node := ast.Node(v)
			if isFunctionLike(node.Kind()) {
				return
			}
			if node.Kind() == "pattern_named" && !vs.stmtPats[nodeKey(node)] {
				// Bound by a condition, a case or a loop: it names part of its subject.
				f.lets[node.Name()] += 2
			}
			for _, c := range v {
				switch c.(type) {
				case map[string]any, []any:
					lets(c)
				}
			}
		}
	}
	for k, c := range fn {
		if k != "params" {
			lets(c)
		}
	}
	return f
}

// isVarPattern reports whether a pattern_named node binds a mutable variable.
func (e *fileEmitter) isVarPattern(p ast.Node) bool {
	if e.scopes == nil {
		return true
	}
	if e.scopes.varPats[nodeKey(p)] {
		return true
	}
	f := e.scopes.patFrames[nodeKey(p)]
	return f == nil || f.vars[p.Name()]
}

// localOrigin classifies a reference to a local name; loaded says the reference is read as a
// mutable place (it sits under a load_expr).
func (e *fileEmitter) localOrigin(n ast.Node, loaded bool) origin {
	name := n.Child("decl").Str("base_name")
	if e.scopes == nil {
		return originShared
	}
	ref, ok := e.scopes.refs[nodeKey(n)]
	if !loaded {
		if !ok || ref.cross {
			return originBorrowedLet
		}
		if _, isParam := ref.frame.params[name]; isParam {
			return originBorrowedLet
		}
		if e.ownsLet(ref.frame, name) && !ref.frame.escapes[name] {
			return originOwnedLet
		}
		// Borrowed, or read by a closure that may outlive the function: its storage cannot move away.
		return originBorrowedLet
	}
	if name == "self" {
		return originExclusive
	}
	if !ok {
		return originCaptured
	}
	if ref.frame.written[name] {
		return originCaptured
	}
	if ref.cross {
		// No closure writes it, and the function that declares it is not running: it holds still.
		return originBorrowedLet
	}
	if inout, isParam := ref.frame.params[name]; isParam && inout && !ref.frame.vars[name] {
		return originExclusive
	}
	if ref.frame.escapes[name] {
		// A closure that reads it may outlive the function, so its storage cannot move to the caller.
		return originExclusive
	}
	return originLocalVar
}

// sharedPlace is the origin of a read of the shared place n. Inside a function that cannot write
// shared places, directly or through what it calls, the place holds still for as long as the function
// runs, like a constant the function does not own.
func (e *fileEmitter) sharedPlace(n ast.Node) origin {
	if e.scopes == nil {
		return originShared
	}
	f := e.scopes.within[nodeKey(n)]
	if f == nil {
		return originShared
	}
	for f.parent != nil {
		f = f.parent
	}
	if f.quiet == 0 {
		f.quiet = 2
		if !e.m.regionWrites(map[string]any(f.fn)) {
			f.quiet = 1
		}
	}
	if f.quiet == 1 {
		return originBorrowedLet
	}
	return originShared
}

// ownsLet reports whether the `let` named name in f holds storage of its own: it is declared once, by a
// statement, and its initializer was fresh or was copied when bound.
func (e *fileEmitter) ownsLet(f *valueFrame, name string) bool {
	switch f.owned[name] {
	case 1:
		return true
	case 2, 3:
		return false
	}
	f.owned[name] = 3
	owned := false
	if init := f.inits[name]; init != nil && f.lets[name] == 1 && !f.vars[name] {
		owned = e.origin(init) != originBorrowedLet
	}
	if owned {
		f.owned[name] = 1
	} else {
		f.owned[name] = 2
	}
	return owned
}

func worse(a, b origin) origin {
	if b > a {
		return b
	}
	return a
}

// origin classifies where the storage of the value n evaluates to lives.
func (e *fileEmitter) origin(n ast.Node) origin { return e.originOf(n, false) }

func (e *fileEmitter) originOf(n ast.Node, loaded bool) origin {
	if n == nil {
		return originFresh
	}
	switch n.Kind() {
	case "load_expr":
		return e.originOf(n.Child("sub_expr"), true)
	case "paren_expr", "inout_expr", "try_expr", "force_try_expr", "optional_try_expr", "await_expr", "dot_self_expr", "coerce_expr",
		"derived_to_base_expr", "bind_optional_expr", "force_value_expr", "optional_evaluation_expr", "function_conversion_expr",
		"covariant_return_conversion_expr", "underlying_to_opaque_expr", "abi_safe_conversion_expr", "collection_upcast_expr",
		"unsafe_expr", "borrow_expr", "copy_expr", "consume_expr", "any_hashable_erasure_expr", "conditional_checked_cast_expr",
		"forced_checked_cast_expr", "archetype_to_super_expr", "vararg_expansion_expr", "unresolved_type_conversion_expr":
		return e.originOf(n.Child("sub_expr"), loaded)
	case "erasure_expr", "inject_into_optional":
		// The wrapper is new; what it wraps is shared only when the wrapped value has storage to share.
		sub := n.Child("sub_expr")
		if !e.needsCopy(e.m.typeOf(sub.Type())) {
			return originFresh
		}
		return e.originOf(sub, loaded)
	case "open_existential_expr":
		if e.opened == nil {
			e.opened = map[string]ast.Node{}
		}
		e.opened[n.Child("opaque_value").Str("identity")] = n.Child("existential_value")
		return e.originOf(n.Child("sub_expr"), loaded)
	case "opaque_value_expr":
		if v, ok := e.opened[n.Str("identity")]; ok {
			return e.originOf(v, loaded)
		}
		return originShared
	case "dot_syntax_base_ignored":
		return e.originOf(n.Child("rhs"), loaded)
	case "declref_expr":
		d := n.Child("decl")
		if d.Str("decl_usr") == "" {
			return e.localOrigin(n, loaded)
		}
		if info, _ := e.lookup(d.Str("decl_usr")); info != nil && info.Kind == "var_decl" && info.Computed {
			return originFresh
		}
		if loaded {
			return e.sharedPlace(n)
		}
		return originBorrowedLet
	case "member_ref_expr":
		base := n.Child("base")
		info, _ := e.lookup(n.Child("decl").Str("decl_usr"))
		if info == nil {
			if usr := n.Child("decl").Str("decl_usr"); strings.HasPrefix(usr, "c:") {
				return e.originOf(base, loaded)
			}
			// A property of a library type (`first`, `values`, `keys`): part of its base, or built from it.
			return e.derived(n, []ast.Node{base})
		}
		if info.Kind != "var_decl" || info.Computed {
			return originFresh
		}
		bt := e.m.typeOf(base.Type())
		if info.Static || bt == nil || bt.Kind == types.Meta || e.m.isReferenceGo(bt, e.m.GoType(bt, e.generics)) {
			if loaded {
				return e.sharedPlace(n)
			}
			return originBorrowedLet
		}
		return e.originOf(base, loaded)
	case "tuple_element_expr", "keypath_application_expr":
		return e.originOf(n.Child("base"), loaded)
	case "subscript_expr":
		if info, _ := e.lookup(n.Child("decl").Str("decl_usr")); info != nil {
			return originFresh
		}
		if t := e.m.typeOf(n.Type()); t != nil && t.Kind == types.Optional && len(t.Args) == 1 && !e.needsCopy(t.Args[0]) && e.m.Recv(e.m.typeOf(n.Child("base").Type())) != "Slice" {
			// A dictionary lookup makes a new optional.
			return originFresh
		}
		o := e.originOf(n.Child("base"), loaded)
		for _, a := range n.Child("args").List("args") {
			if a.Str("label") == "default" && e.needsCopy(e.m.typeOf(a.Child("expr").Type())) {
				o = worse(o, e.origin(a.Child("expr")))
			}
		}
		return o
	case "ternary_expr":
		return worse(e.origin(n.Child("then_expr")), e.origin(n.Child("else_expr")))
	case "tuple_expr", "array_expr":
		// An aggregate built from parts is as shared as its parts are.
		return e.derived(nil, n.List("elements"))
	case "dictionary_expr":
		o := originFresh
		for _, el := range n.List("elements") {
			o = worse(o, e.derived(nil, el.List("elements")))
		}
		return o
	case "call_expr", "binary_expr", "prefix_unary_expr", "postfix_unary_expr":
		return e.callOrigin(n)
	case "autoclosure_expr":
		if len(n.Child("params").List("params")) == 0 {
			return e.origin(n.Child("single_expr_body"))
		}
	}
	return originFresh
}

// freshResults names library functions that build their result's own storage: a new array, set or
// dictionary, whose elements are still the receiver's.
var freshResults = map[string]bool{
	"filter": true, "sorted": true, "reversed": true, "shuffled": true, "dropFirst": true, "dropLast": true,
	"split": true, "components": true, "union": true, "intersection": true, "subtracting": true,
	"symmetricDifference": true, "merging": true, "enumerated": true, "keys": true, "values": true, "init": true,
	"+": true, "indices": true, "sorted(by:)": true,
}

// producedByClosure names library functions whose result holds what their closure argument returned.
var producedByClosure = map[string]bool{
	"map": true, "compactMap": true, "flatMap": true, "mapValues": true, "compactMapValues": true, "reduce": true,
}

// callOrigin classifies a call's result. A function of this program returns storage the caller owns
// (see sinkReturn). A library function may return its receiver, an argument, or a part of one.
func (e *fileEmitter) callOrigin(n ast.Node) origin {
	fn := stripConv(n.Child("fn"))
	var operands []ast.Node
	for _, a := range n.Child("args").List("args") {
		operands = append(operands, a.Child("expr"))
	}
	if n.Kind() != "call_expr" {
		// An operator applied to a tuple of operands.
		operands = nil
		if args := n.Child("args"); args != nil {
			for _, a := range args.List("args") {
				operands = append(operands, a.Child("expr"))
			}
		}
		if len(operands) == 0 {
			if sub := n.Child("sub_expr"); sub != nil {
				operands = append(operands, sub.List("elements")...)
			}
		}
	}
	var d ast.Node
	switch fn.Kind() {
	case "dot_syntax_call_expr":
		d = stripConv(fn.Child("fn")).Child("decl")
		for _, a := range fn.Child("args").List("args") {
			operands = append([]ast.Node{a.Child("expr")}, operands...)
		}
	case "constructor_ref_call_expr":
		d = stripConv(fn.Child("fn")).Child("decl")
	case "declref_expr":
		d = fn.Child("decl")
	default:
		// A closure or function value: the program's own code.
		return originFresh
	}
	usr := d.Str("decl_usr")
	if usr == "" {
		return originFresh
	}
	if info, _ := e.lookup(usr); info != nil {
		if info.Kind == "enum_element_decl" || memberwise(info) {
			// An enum case or a memberwise-initialized struct holds its arguments as they were passed.
			var args []ast.Node
			for _, a := range n.Child("args").List("args") {
				args = append(args, a.Child("expr"))
			}
			return e.derived(nil, args)
		}
		return originFresh
	}
	if strings.Contains(e.m.demangle(usr), "(in ") {
		return originFresh
	}
	name := d.Str("base_name")
	if producedByClosure[name] {
		return originFresh
	}
	for _, op := range operands {
		if op.Kind() == "inout_expr" {
			// A mutating library call returns what it took out of the collection.
			return originFresh
		}
	}
	if freshResults[name] && !e.elementsNeedCopy(e.m.typeOf(n.Type())) {
		return originFresh
	}
	return e.derived(n, operands)
}

// derived is the origin of a library result built from operands: the worst of those it could share
// storage with.
func (e *fileEmitter) derived(n ast.Node, operands []ast.Node) origin {
	if t := e.m.typeOf(n.Type()); n != nil && t != nil && t.Kind == types.Optional && len(t.Args) == 1 && !e.needsCopy(t.Args[0]) && n.Kind() != "binary_expr" {
		// The library hands back a new optional (`first`, `d[k]`); an operator like ?? passes one through.
		return originFresh
	}
	o := originFresh
	for _, op := range operands {
		if op == nil || !e.needsCopy(e.m.typeOf(op.Type())) {
			continue
		}
		o = worse(o, e.origin(op))
	}
	return o
}

// elementsNeedCopy reports whether a collection's elements have storage of their own to share.
func (e *fileEmitter) elementsNeedCopy(t *types.Type) bool {
	if t == nil {
		return true
	}
	switch t.Kind {
	case types.Array, types.Dict:
		for _, a := range t.Args {
			if e.needsCopy(a) {
				return true
			}
		}
		return false
	case types.Named:
		switch t.Name {
		case "Swift.Array", "Swift.Dictionary", "Swift.Set", "Swift.ArraySlice", "Swift.ContiguousArray":
			for _, a := range t.Args {
				if e.needsCopy(a) {
					return true
				}
			}
			return false
		}
	}
	return true
}

// owned emits n for a sink, copying it when the sink must not share n's storage.
func (e *fileEmitter) owned(n ast.Node, s sink) string {
	if n == nil {
		return e.expr(n)
	}
	if n.Kind() == "call_expr" && (s == sinkStore || s == sinkReturn) {
		e.keeps++
		defer func() { e.keeps-- }()
	}
	t := e.m.typeOf(n.Type())
	if !e.needsCopy(t) {
		return e.expr(n)
	}
	switch n.Kind() {
	case "paren_expr":
		return "(" + e.owned(n.Child("sub_expr"), s) + ")"
	case "ternary_expr":
		if gt := e.exprGoType(n); !isVoidGo(gt) {
			// Copy only the branch that reads a place.
			return "func() " + gt + " { if " + e.expr(n.Child("cond_expr")) + " { return " + e.owned(n.Child("then_expr"), s) + " }; return " + e.owned(n.Child("else_expr"), s) + " }()"
		}
	case "tuple_expr":
		return e.tupleFor(n, s)
	case "array_expr", "dictionary_expr":
		// The literal is new; its elements go where it goes.
		e.partsOf, e.partSink = nodeKey(n), s
		return e.expr(n)
	case "call_expr":
		if info := e.calleeInfo(n); info != nil && (info.Kind == "enum_element_decl" || memberwise(info)) {
			e.partsOf, e.partSink = nodeKey(n), s
			return e.expr(n)
		}
	case "inject_into_optional":
		// The optional is new; only the value put into it can be shared.
		sub := n.Child("sub_expr")
		if _, done := e.snapshots[nodeKey(sub)]; done || !e.needsCopy(e.m.typeOf(sub.Type())) || !e.origin(sub).copies(s) {
			return e.expr(n)
		}
		if e.snapshots == nil {
			e.snapshots = map[uintptr]string{}
		}
		e.snapshots[nodeKey(sub)] = "swiftrt.Copy(" + e.expr(sub) + ")"
		defer delete(e.snapshots, nodeKey(sub))
		return e.expr(n)
	}
	if e.origin(n).copies(s) {
		return "swiftrt.Copy(" + e.expr(n) + ")"
	}
	return e.expr(n)
}

// stored emits an argument a mutating library call keeps in a collection of type into (nil when not
// known). An argument that is itself a sequence of the elements to keep, as in append(contentsOf:),
// is copied only for its elements' sake.
func (e *fileEmitter) stored(n ast.Node, into *types.Type) string {
	t := e.m.typeOf(n.Type())
	if !e.needsCopy(t) {
		return e.expr(n)
	}
	if into != nil && e.isCollection(t) && !e.elementsNeedCopy(t) {
		whole := false
		for _, a := range into.Args {
			whole = whole || a.String() == t.String()
		}
		if !whole {
			return e.expr(n)
		}
	}
	return e.owned(n, sinkStore)
}

func (e *fileEmitter) isCollection(t *types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case types.Array, types.Dict:
		return true
	case types.Named:
		switch t.Name {
		case "Swift.Array", "Swift.Dictionary", "Swift.Set", "Swift.ArraySlice", "Swift.ContiguousArray":
			return true
		}
	}
	return false
}

// lendReceiver prepares the receiver of a call to one of the program's methods or getters: a struct
// read from a shared place is copied first when the callee can change that place while it runs. The
// returned function ends the loan.
func (e *fileEmitter) lendReceiver(base ast.Node, callee *DeclInfo) func() {
	if base == nil || callee == nil || callee.Node == nil || callee.Static || base.Kind() == "inout_expr" {
		return func() {}
	}
	if _, done := e.snapshots[nodeKey(base)]; done || !e.needsCopy(e.m.typeOf(base.Type())) || e.origin(base) < originCaptured || !e.m.mayWrite(callee.Node.Str("usr")) {
		return func() {}
	}
	if e.snapshots == nil {
		e.snapshots = map[uintptr]string{}
	}
	e.snapshots[nodeKey(base)] = "swiftrt.Copy(" + e.expr(base) + ")"
	return func() { delete(e.snapshots, nodeKey(base)) }
}

// lendClosures marks the closures passed to the library call n that live only inside its result, as a
// lazy sequence's do: while the result stays in the function, so do they, and they capture without
// copying. A call that returns nothing or a task runs its closure later, and a result owned stores
// or returns takes its closures with it.
func (e *fileEmitter) lendClosures(n ast.Node) {
	if e.keeps > 0 {
		return
	}
	fn := stripConv(n.Child("fn"))
	if fn == nil {
		return
	}
	switch fn.Kind() {
	case "dot_syntax_call_expr", "constructor_ref_call_expr":
		fn = stripConv(fn.Child("fn"))
	}
	if fn == nil || fn.Kind() != "declref_expr" {
		return
	}
	usr := fn.Child("decl").Str("decl_usr")
	if info, _ := e.lookup(usr); usr == "" || info != nil || strings.Contains(e.m.demangle(usr), "(in ") {
		return
	}
	t := e.m.typeOf(n.Type())
	if t == nil || t.Kind == types.Unknown || isVoidGo(e.m.GoType(t, e.generics)) || strings.HasPrefix(t.Name, "Swift.Task") {
		return
	}
	for _, a := range n.Child("args").List("args") {
		if cl := stripConv(a.Child("expr")); cl != nil && cl.Kind() == "closure_expr" && cl.Bool("escaping") {
			if e.lent == nil {
				e.lent = map[uintptr]bool{}
			}
			e.lent[nodeKey(cl)] = true
		}
	}
}

// captureValues lists the copies an escaping closure makes of what it captures. Swift captures an
// immutable value when the closure is made; a constant that only borrows its storage (a parameter,
// `self`, a `let` bound to part of one) is copied then, since the closure can run after the lender has
// changed it. Each statement redeclares the captured name, for a scope wrapped around the closure.
func (e *fileEmitter) captureValues(closure ast.Node) []string {
	if !closure.Bool("escaping") || e.scopes == nil || e.lent[nodeKey(closure)] {
		return nil
	}
	inner := e.scopes.bodies[nodeKey(closure)]
	if inner == nil {
		return nil
	}
	var copies []string
	for _, c := range closure.List("captures") {
		d := c.Child("decl")
		name := d.Str("base_name")
		t := e.m.typeOf(d.Str("type_usr"))
		if d.Str("decl_usr") != "" || name == "" || t == nil || !e.needsCopy(t) || e.inoutParams[name] {
			continue
		}
		if name != "self" {
			// A `var` is captured as the variable itself, and a constant that owns its storage keeps
			// it for the closure.
			var decl *valueFrame
			for f := inner.parent; f != nil && decl == nil; f = f.parent {
				if _, isParam := f.params[name]; isParam || f.lets[name] > 0 || f.vars[name] {
					decl = f
				}
			}
			if decl == nil || decl.vars[name] {
				continue
			}
			if _, isParam := decl.params[name]; !isParam && e.ownsLet(decl, name) {
				continue
			}
		}
		v := e.declRef(d, nil)
		if strings.ContainsAny(v, "(*. ") {
			continue
		}
		copies = append(copies, v+" := swiftrt.Copy("+v+")\n_ = "+v+"\n")
	}
	return copies
}

// memberwise reports whether info is a synthesized initializer, which assigns each parameter to its
// property.
func memberwise(info *DeclInfo) bool {
	return info != nil && info.Kind == "constructor_decl" && info.OwnerKind == "struct_decl" && info.Node != nil && (info.Node.Implicit() || info.Node.Child("body") == nil)
}

// calleeInfo is the declaration a call expression calls, when it is one of the program's.
func (e *fileEmitter) calleeInfo(n ast.Node) *DeclInfo {
	fn := stripConv(n.Child("fn"))
	if fn == nil {
		return nil
	}
	switch fn.Kind() {
	case "dot_syntax_call_expr", "constructor_ref_call_expr":
		fn = stripConv(fn.Child("fn"))
	}
	if fn == nil || fn.Kind() != "declref_expr" {
		return nil
	}
	info, _ := e.lookup(fn.Child("decl").Str("decl_usr"))
	return info
}

// partsSink is the sink for the parts of the aggregate n (a literal, an enum case, a memberwise
// initializer call): the one owned chose for n, or none when n is only read where it is written.
func (e *fileEmitter) partsSink(n ast.Node) *sink {
	s := sinkNone
	if e.partsOf != 0 && e.partsOf == nodeKey(n) {
		s = e.partSink
	}
	e.partsOf = 0
	return &s
}

// part emits one part of an aggregate whose parts go into s.
func (e *fileEmitter) part(n ast.Node, s sink) string {
	if s == sinkNone {
		return e.expr(n)
	}
	return e.owned(n, s)
}

// bound is the Go expression a pattern variable p is initialised with from src, which names part of
// a subject of origin e.bindOrigin.
func (e *fileEmitter) bound(p ast.Node, src string) string {
	s := sinkLet
	if e.isVarPattern(p) {
		s = sinkStore
	}
	if e.bindOrigin.copies(s) && e.needsCopy(e.m.typeOf(p.Type())) {
		return "swiftrt.Copy(" + src + ")"
	}
	return src
}

// mutatesLocal reports whether body writes the local variable name: it refers to it other than by
// loading its value.
func mutatesLocal(body any, name string) bool {
	found := false
	var walk func(n any, loaded bool)
	walk = func(n any, loaded bool) {
		if found {
			return
		}
		switch v := n.(type) {
		case []any:
			for _, c := range v {
				walk(c, loaded)
			}
		case map[string]any:
			node := ast.Node(v)
			switch node.Kind() {
			case "load_expr":
				loaded = true
			case "declref_expr":
				if d := node.Child("decl"); d.Str("decl_usr") == "" && d.Str("base_name") == name && !loaded {
					found = true
				}
				return
			case "argument_list":
				// Arguments are evaluated on their own, whatever reads the call's result.
				loaded = false
			}
			for _, c := range v {
				switch c.(type) {
				case map[string]any, []any:
					walk(c, loaded)
				}
			}
		}
	}
	walk(body, false)
	return found
}

// loopSource emits the collection a loop or a library call with a closure iterates. Swift iterates
// the value the collection had when the loop started, so a place the body can change is copied first.
func (e *fileEmitter) loopSource(seq ast.Node, body ...any) string {
	if e.snapshotNeeded(seq, body...) {
		return "swiftrt.Copy(" + e.expr(seq) + ")"
	}
	return e.expr(seq)
}

func (e *fileEmitter) snapshotNeeded(seq ast.Node, body ...any) bool {
	if !e.needsCopy(e.m.typeOf(seq.Type())) {
		return false
	}
	switch e.origin(seq) {
	case originShared:
		return e.m.regionWrites(body...)
	case originCaptured:
		// A closure writes this variable: it changes if the body does so itself, or runs code that
		// could reach that closure.
		root := placeRoot(seq)
		return root == "" || mutatesLocal(body, root) || e.m.regionCalls(body...)
	case originLocalVar, originExclusive:
		root := placeRoot(seq)
		return root != "" && mutatesLocal(body, root)
	}
	return false
}

// placeRoot is the local variable a place expression is rooted at, or "".
func placeRoot(n ast.Node) string {
	for n != nil {
		switch n.Kind() {
		case "declref_expr":
			if n.Child("decl").Str("decl_usr") == "" {
				return n.Child("decl").Str("base_name")
			}
			return ""
		case "member_ref_expr", "subscript_expr", "tuple_element_expr":
			n = n.Child("base")
		case "call_expr":
			// A library call on a place (`xs.reversed()`, `d.values`): follow its receiver.
			fn := stripConv(n.Child("fn"))
			if fn.Kind() != "dot_syntax_call_expr" {
				return ""
			}
			args := fn.Child("args").List("args")
			if len(args) == 0 {
				return ""
			}
			n = args[0].Child("expr")
		default:
			sub := n.Child("sub_expr")
			if sub == nil {
				return ""
			}
			n = sub
		}
	}
	return ""
}
