package emit

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
)

// Swift evaluates operands strictly left to right, and a variable operand is read when it is
// reached. Go orders only calls and receives: `swiftrt.Add(x, f())` may read x after f returns.
// When a later operand may write what an earlier operand reads, the earlier operand is wrapped in
// swiftrt.Eval, a call that Go evaluates, with its argument, before the later call.

// pinOperands marks each operand that a later operand could change.
func (e *fileEmitter) pinOperands(ops ...ast.Node) {
	for i := len(ops) - 2; i >= 0; i-- {
		if e.needsPin(ops[i], ops[i+1:]) {
			if e.pinned == nil {
				e.pinned = map[uintptr]bool{}
			}
			e.pinned[nodeKey(ops[i])] = true
		}
	}
}

// needsPin reports whether evaluating later could change the value of op.
func (e *fileEmitter) needsPin(op ast.Node, later []ast.Node) bool {
	op = stripConv(op)
	switch op.Kind() {
	case "integer_literal_expr", "float_literal_expr", "boolean_literal_expr", "string_literal_expr", "nil_literal_expr",
		"type_expr", "dot_syntax_call_expr", "closure_expr", "autoclosure_expr", "default_argument_expr", "call_expr":
		// A call's result is computed when Go makes the call, which it orders.
		return false
	case "erasure_expr", "inject_into_optional", "derived_to_base_expr", "paren_expr", "load_expr", "force_value_expr":
		return e.needsPin(op.Child("sub_expr"), later)
	case "array_expr", "tuple_expr":
		for _, el := range op.List("elements") {
			if e.needsPin(el, later) {
				return true
			}
		}
		return false
	case "dictionary_expr":
		for _, el := range op.List("elements") {
			if e.needsPin(el, later) {
				return true
			}
		}
		return false
	case "binary_expr", "prefix_unary_expr", "postfix_unary_expr":
		if e.m.ownDecl(e.opDecl(op).Str("decl_usr")) != nil {
			break
		}
		for _, a := range e.args(op.Child("args")) {
			if e.needsPin(a.node, later) {
				return true
			}
		}
		return false
	case "declref_expr", "member_ref_expr", "subscript_expr", "tuple_element_expr":
		if e.isConstantRead(op) {
			return false
		}
		if name, ok := e.localRoot(op); ok {
			return e.anyLater(later, func(n ast.Node) bool { return e.mayWriteLocal(n, name) })
		}
	}
	return e.anyLater(later, e.mayWrite)
}

func (e *fileEmitter) anyLater(ops []ast.Node, f func(ast.Node) bool) bool {
	for _, op := range ops {
		if op != nil && f(op) {
			return true
		}
	}
	return false
}

// isConstantRead reports whether n reads an enum case or a global or static `let`.
func (e *fileEmitter) isConstantRead(n ast.Node) bool {
	if n.Kind() != "declref_expr" && n.Kind() != "member_ref_expr" {
		return false
	}
	info := e.m.ownDecl(n.Child("decl").Str("decl_usr"))
	return info != nil && (info.Kind == "enum_element_decl" || info.Kind == "var_decl" && info.Node.Bool("let") && (info.Owner == "" || info.Static))
}

// localRoot returns the local variable or parameter that n reads, itself or a part of it held by
// value. Only an inout argument or a closure capturing it can change such a read.
func (e *fileEmitter) localRoot(n ast.Node) (string, bool) {
	for {
		switch n.Kind() {
		case "load_expr", "paren_expr", "force_value_expr", "bind_optional_expr", "optional_evaluation_expr", "inject_into_optional":
			n = n.Child("sub_expr")
		case "tuple_element_expr":
			n = n.Child("base")
		case "member_ref_expr", "subscript_expr":
			if info := e.m.ownDecl(n.Child("decl").Str("decl_usr")); info != nil && (info.Computed || info.Kind == "subscript_decl") {
				return "", false
			}
			base := n.Child("base")
			if t := e.m.typeOf(base.Type()); t == nil || e.m.isReferenceGo(t, e.m.GoType(t, e.generics)) {
				return "", false
			}
			n = base
		case "declref_expr":
			d := n.Child("decl")
			return d.Str("base_name"), d.Str("decl_usr") == ""
		default:
			return "", false
		}
	}
}

// mayWriteLocal reports whether evaluating n could assign the local variable name: it passes it
// inout, assigns it in a closure, or calls a closure or local function, which can assign the locals
// it captures.
func (e *fileEmitter) mayWriteLocal(n ast.Node, name string) bool {
	found := false
	walkEvaluated(n, func(x ast.Node) {
		switch {
		case found:
		case x.Kind() == "inout_expr" || x.Kind() == "assign_expr":
			target := x.Child("sub_expr")
			if x.Kind() == "assign_expr" {
				target = x.Child("dest")
			}
			root, local := e.localRoot(target)
			found = target == nil || local && root == name
		case x.Kind() == "call_expr":
			usr := e.calleeUSR(x)
			found = usr == "" || e.m.ownsUSR(usr) && localEntity.MatchString(usr)
		}
	})
	return found
}

// walkEvaluated calls f for n and the nodes under it that run when n is evaluated: it does not
// enter closures, whose bodies run only when called.
func walkEvaluated(n any, f func(ast.Node)) {
	switch v := n.(type) {
	case map[string]any:
		walkEvaluated(ast.Node(v), f)
	case ast.Node:
		if k := v.Kind(); k == "closure_expr" || k == "autoclosure_expr" {
			return
		}
		f(v)
		for _, c := range v {
			walkEvaluated(c, f)
		}
	case []any:
		for _, c := range v {
			walkEvaluated(c, f)
		}
	}
}

// localEntity matches the mangling of a declaration local to a function, such as a nested func.
var localEntity = regexp.MustCompile(`L\d*_`)

// pin wraps out, the Go for n, in swiftrt.Eval when n was marked by pinOperands and Go would not
// already evaluate it in order.
func (e *fileEmitter) pin(n ast.Node, out string) string {
	if !e.pinned[nodeKey(n)] || isOrderedGo(out) {
		return out
	}
	return "swiftrt.Eval(" + out + ")"
}

// goConversions are the predeclared types, whose call syntax is a conversion and not ordered.
var goConversions = map[string]bool{"bool": true, "string": true, "int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true, "byte": true, "rune": true,
	"float32": true, "float64": true, "any": true}

// isOrderedGo reports whether Go evaluates the expression src as a call, which it orders among the
// other calls of the enclosing expression, or as a constant.
func isOrderedGo(src string) bool {
	x, err := parser.ParseExpr(src)
	if err != nil {
		return false
	}
	for {
		p, ok := x.(*goast.ParenExpr)
		if !ok {
			break
		}
		x = p.X
	}
	switch v := x.(type) {
	case *goast.BasicLit:
		return true
	case *goast.CallExpr:
		switch f := v.Fun.(type) {
		case *goast.Ident:
			return !goConversions[f.Name]
		case *goast.SelectorExpr, *goast.IndexExpr, *goast.IndexListExpr, *goast.FuncLit:
			return true
		}
	}
	return false
}

// mayWrite reports whether evaluating n could assign a variable that another operand reads: it
// calls code of this package or its dependencies, calls a closure, passes something inout, or runs
// a computed property or subscript defined here. Standard library calls without inout arguments
// only read.
func (e *fileEmitter) mayWrite(n ast.Node) bool {
	key := nodeKey(n)
	if v, ok := e.writes[key]; ok {
		return v
	}
	found := false
	walkEvaluated(n, func(x ast.Node) {
		if found {
			return
		}
		switch x.Kind() {
		case "inout_expr", "assign_expr":
			found = true
		case "call_expr":
			found = e.callMayWrite(x)
		case "member_ref_expr", "subscript_expr", "declref_expr":
			if info := e.m.ownDecl(x.Child("decl").Str("decl_usr")); info != nil && (info.Computed || info.Lazy || info.Kind == "subscript_decl") {
				found = true
			}
		case "binary_expr", "prefix_unary_expr", "postfix_unary_expr":
			if e.m.ownDecl(e.opDecl(x).Str("decl_usr")) != nil {
				found = true
			}
		}
	})
	if e.writes == nil {
		e.writes = map[uintptr]bool{}
	}
	e.writes[key] = found
	return found
}

// callMayWrite reports whether a call runs code that is not the standard library's, other than
// building an enum case.
func (e *fileEmitter) callMayWrite(call ast.Node) bool {
	usr := e.calleeUSR(call)
	if info := e.m.ownDecl(usr); info != nil {
		return info.Kind != "enum_element_decl"
	}
	return usr == "" || e.m.ownsUSR(usr)
}

// calleeUSR returns the USR of the function a call names, or "" for a closure value.
func (e *fileEmitter) calleeUSR(call ast.Node) string {
	fn := stripConv(call.Child("fn"))
	switch fn.Kind() {
	case "dot_syntax_call_expr", "constructor_ref_call_expr":
		fn = stripConv(fn.Child("fn"))
	}
	if fn.Kind() != "declref_expr" {
		return ""
	}
	return fn.Child("decl").Str("decl_usr")
}

// ownDecl returns the declaration usr names in this module or a dependency being transpiled.
func (m *Module) ownDecl(usr string) *DeclInfo {
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

// ownsUSR reports whether usr names a declaration nested in this module or a dependency being
// transpiled, such as a local function.
func (m *Module) ownsUSR(usr string) bool {
	for _, name := range append([]string{m.Name}, depNames(m)...) {
		if strings.HasPrefix(usr, "s:"+strconv.Itoa(len(name))+name) {
			return true
		}
	}
	return false
}

func depNames(m *Module) []string {
	names := make([]string, len(m.Deps))
	for i, d := range m.Deps {
		names[i] = d.Name
	}
	return names
}

// goBinaryOps are the Go binary operators the emitter joins operands with.
var goBinaryOps = map[string]token.Token{"+": token.ADD, "-": token.SUB, "*": token.MUL, "/": token.QUO, "%": token.REM,
	"&": token.AND, "|": token.OR, "^": token.XOR, "<<": token.SHL, ">>": token.SHR, "&^": token.AND_NOT,
	"==": token.EQL, "!=": token.NEQ, "<": token.LSS, "<=": token.LEQ, ">": token.GTR, ">=": token.GEQ,
	"&&": token.LAND, "||": token.LOR}

// infix joins l and r with the Go binary operator op, parenthesizing an operand Go would otherwise
// split: Swift's tree already grouped each operand, and Go's precedences differ from Swift's (a shift
// binds tighter than & in Swift, not in Go) and from those of the operators a lowering emits.
func infix(l, op, r string) string {
	return operand(l, op, false) + " " + op + " " + operand(r, op, true)
}

// assocInfix is infix for an associative operator, such as && or String's +, which leaves a right
// operand joined by the same operator unparenthesized.
func assocInfix(l, op, r string) string {
	if b, ok := parseBinary(r); ok && b != nil && b.Op == goBinaryOps[op] {
		return operand(l, op, false) + " " + op + " " + r
	}
	return infix(l, op, r)
}

// operand parenthesizes src, the left or right operand of the Go operator op, when it is a binary
// expression that would not stay whole: one of lower precedence, or on the right one of the same
// precedence, as Go's binary operators associate to the left.
func operand(src, op string, right bool) string {
	tok, ok := goBinaryOps[op]
	if !ok || !strings.ContainsAny(src, "+-*/%&|^<>=!") {
		return src
	}
	b, ok := parseBinary(src)
	if !ok {
		return "(" + src + ")"
	}
	if b != nil && (b.Op.Precedence() < tok.Precedence() || right && b.Op.Precedence() == tok.Precedence()) {
		return "(" + src + ")"
	}
	return src
}

// unaryOperand parenthesizes src, the operand of a Go unary operator, when it is a binary expression.
func unaryOperand(src string) string {
	if !strings.ContainsAny(src, "+-*/%&|^<>=!") {
		return src
	}
	if b, ok := parseBinary(src); !ok || b != nil {
		return "(" + src + ")"
	}
	return src
}

// parseBinary parses src, a Go expression, and returns it if it is a binary expression. It reports
// false when src does not parse.
func parseBinary(src string) (*goast.BinaryExpr, bool) {
	x, err := parser.ParseExpr(sanitizeRuntimeNames(src))
	if err != nil {
		return nil, false
	}
	b, _ := x.(*goast.BinaryExpr)
	return b, true
}
