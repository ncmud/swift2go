package emit

import (
	"fmt"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// A Swift ArraySlice keeps the indices of the array it was cut from; its Go slice starts at zero. The
// emitter therefore works out each slice's start index where it can (from the expression that made
// it, or from the `let` that holds it) and shifts every index that crosses between the two.

func isArraySlice(t *types.Type) bool {
	return t != nil && t.Kind == types.Named && t.Name == "Swift.ArraySlice"
}

// sliceBase is a Go expression for the Swift start index of n, an Array or ArraySlice expression. It
// fails when the start cannot be told from the code: a parameter, a property, a `var`.
func (e *fileEmitter) sliceBase(n ast.Node) (string, bool) {
	for n != nil && (n.Kind() == "load_expr" || n.Kind() == "paren_expr" || n.Kind() == "inout_expr") {
		n = n.Child("sub_expr")
	}
	if n == nil {
		return "", false
	}
	t := e.m.typeOf(n.Type())
	if !isArraySlice(t) {
		if t != nil && (t.Kind == types.Array || t.Kind == types.Named && t.Name == "Swift.Array") {
			return "0", true
		}
		return "", false
	}
	switch n.Kind() {
	case "declref_expr":
		lo, ok := e.sliceLos[e.sliceKey(n)]
		return lo, ok
	case "subscript_expr":
		args := n.Child("args").List("args")
		if len(args) != 1 {
			return "", false
		}
		a := args[0].Child("expr")
		switch e.m.Recv(e.m.typeOf(a.Type())) {
		case "Range", "ClosedRange", "PartialRangeFrom":
			if lo, ok := e.rangeLower(a); ok {
				return lo, true
			}
		case "PartialRangeUpTo", "PartialRangeThrough":
			return e.sliceBase(n.Child("base"))
		}
		if strings.Contains(a.Type(), "UnboundedRange_") {
			// `array[...]`, the whole collection.
			return e.sliceBase(n.Child("base"))
		}
	case "call_expr":
		fn := stripConv(n.Child("fn"))
		if fn == nil || fn.Kind() != "dot_syntax_call_expr" {
			return "", false
		}
		d := stripConv(fn.Child("fn")).Child("decl")
		if info, _ := e.lookup(d.Str("decl_usr")); info != nil {
			return "", false
		}
		self := fn.Child("args").List("args")
		args := n.Child("args").List("args")
		if len(self) != 1 || len(args) > 1 {
			return "", false
		}
		recv := self[0].Child("expr")
		lo, ok := e.sliceBase(recv)
		if !ok {
			return "", false
		}
		count := "1"
		label := ""
		if len(args) == 1 && args[0].Child("expr").Kind() != "default_argument_expr" {
			label = args[0].Str("label")
			if !e.isPure(stripLoads(args[0].Child("expr"))) {
				return "", false
			}
			count = e.sliceExpr(args[0].Child("expr"))
		}
		switch d.Str("base_name") {
		case "prefix", "dropLast":
			if label == "" || label == "upTo" || label == "through" || label == "while" {
				return lo, true
			}
		case "dropFirst":
			if e.isPure(stripLoads(recv)) {
				return fmt.Sprintf("(%s + min(%s, len(%s)))", lo, count, e.sliceExpr(recv)), true
			}
		case "suffix":
			if label == "from" {
				return count, true
			}
			if label == "" && e.isPure(stripLoads(recv)) {
				return fmt.Sprintf("(%s + max(len(%s)-%s, 0))", lo, e.sliceExpr(recv), count), true
			}
		}
	}
	return "", false
}

// rangeLower is the lower bound of a range expression whose operands can be evaluated twice.
func (e *fileEmitter) rangeLower(r ast.Node) (string, bool) {
	n := stripLoads(r)
	for n != nil && n.Kind() == "paren_expr" {
		n = stripLoads(n.Child("sub_expr"))
	}
	if n == nil {
		return "", false
	}
	if !e.pureOperand(n) {
		return "", false
	}
	return "(" + e.sliceExpr(r) + ").Lo", true
}

// sliceExpr renders n for a start index, or nothing while sliceVarTrackable only asks whether the
// start can be told.
func (e *fileEmitter) sliceExpr(n ast.Node) string {
	if e.sliceDry {
		return ""
	}
	return e.expr(n)
}

// pureOperand reports whether n, an operand or an operator applied to operands, can be evaluated
// twice.
func (e *fileEmitter) pureOperand(n ast.Node) bool {
	n = stripLoads(n)
	for n != nil && n.Kind() == "paren_expr" {
		n = stripLoads(n.Child("sub_expr"))
	}
	if n == nil {
		return false
	}
	switch n.Kind() {
	case "binary_expr", "postfix_unary_expr", "prefix_unary_expr":
		for _, a := range n.Child("args").List("args") {
			if !e.pureOperand(a.Child("expr")) {
				return false
			}
		}
		return true
	}
	return e.isPure(n)
}

// sliceKey identifies the local variable a reference names.
func (e *fileEmitter) sliceKey(n ast.Node) string {
	if e.scopes == nil {
		return ""
	}
	ref, ok := e.scopes.refs[nodeKey(n)]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%p|%s", ref.frame, ref.name)
}

// bindSliceBase records the start index of the ArraySlice a `let` or `var` statement binds, in a
// variable declared next to it. A `var` is tracked only when every write to it is an assignment,
// which updates the variable; one passed inout or mutated in place is not.
func (e *fileEmitter) bindSliceBase(p, init ast.Node) {
	if e.scopes == nil || !isArraySlice(e.m.typeOf(p.Type())) {
		return
	}
	f := e.scopes.patFrames[nodeKey(p)]
	if f == nil {
		return
	}
	isVar := e.isVarPattern(p)
	if !isVar && f.lets[p.Name()] != 1 || isVar && !e.sliceVarTrackable(f, p.Name()) {
		return
	}
	lo, ok := e.sliceBase(init)
	if !ok {
		return
	}
	v := e.newTmp("lo")
	e.w("%s := %s\n_ = %s\n", v, lo, v)
	if e.sliceLos == nil {
		e.sliceLos = map[string]string{}
	}
	e.sliceLos[fmt.Sprintf("%p|%s", f, p.Name())] = v
}

// sliceVarTrackable reports whether the ArraySlice variable name, declared once in f, is written only
// by plain assignments in f's own body.
func (e *fileEmitter) sliceVarTrackable(f *valueFrame, name string) bool {
	if f.fn == nil || f.written[name] {
		return false
	}
	decls := 0
	ok := true
	// A subscript reaches its base through an inout access; reading or setting an element keeps the
	// slice's indices.
	subscripted := map[uintptr]bool{}
	ast.Walk(f.fn, func(n ast.Node) {
		if n.Kind() == "subscript_expr" {
			subscripted[nodeKey(n.Child("base"))] = true
		}
	})
	ast.Walk(f.fn, func(n ast.Node) {
		switch n.Kind() {
		case "pattern_named":
			if n.Name() == name {
				decls++
			}
		case "inout_expr":
			if d := stripLoads(n.Child("sub_expr")); !subscripted[nodeKey(n)] && d != nil && d.Kind() == "declref_expr" && e.namesLocal(d, f, name) {
				ok = false
			}
		case "assign_expr":
			if d := stripLoads(n.Child("dest")); d != nil && d.Kind() == "declref_expr" && e.namesLocal(d, f, name) && !e.sliceBaseKnown(n.Child("src"), f, name) {
				ok = false
			}
		}
	})
	return ok && decls == 1
}

// sliceBaseKnown reports whether the start index of src, assigned to the variable name of f, can be
// told from the code.
func (e *fileEmitter) sliceBaseKnown(src ast.Node, f *valueFrame, name string) bool {
	key := fmt.Sprintf("%p|%s", f, name)
	_, had := e.sliceLos[key]
	if e.sliceLos == nil {
		e.sliceLos = map[string]string{}
	}
	// The variable itself is tracked if the rest of its writes allow it.
	e.sliceLos[key] = "lo"
	save := e.sliceDry
	e.sliceDry = true
	_, ok := e.sliceBase(src)
	e.sliceDry = save
	if !had {
		delete(e.sliceLos, key)
	}
	return ok
}

// namesLocal reports whether the reference n names the local name declared in f.
func (e *fileEmitter) namesLocal(n ast.Node, f *valueFrame, name string) bool {
	ref, ok := e.scopes.refs[nodeKey(n)]
	return ok && ref.frame == f && ref.name == name
}

// assignSliceBase updates the start index of a tracked ArraySlice variable that dest names, once the
// assignment of src to it has been emitted.
func (e *fileEmitter) assignSliceBase(dest, src ast.Node) func() {
	d := stripLoads(dest)
	for d != nil && d.Kind() == "paren_expr" {
		d = stripLoads(d.Child("sub_expr"))
	}
	if d == nil || d.Kind() != "declref_expr" || !isArraySlice(e.m.typeOf(d.Type())) {
		return func() {}
	}
	lo, tracked := e.sliceLos[e.sliceKey(d)]
	if !tracked {
		return func() {}
	}
	next, ok := e.sliceBase(src)
	if !ok {
		next = e.m.Unsup("arrayslice-index-of-unknown-start", e.where, "int")
	}
	v := e.newTmp("lo")
	e.w("%s := %s\n", v, next)
	return func() { e.w("%s = %s\n", lo, v) }
}

// sliceOffset is the start index to subtract from a Swift index into the ArraySlice base, "" when it
// is zero or base is not an ArraySlice. An ArraySlice whose start is unknown is reported, and indexed
// from zero.
func (e *fileEmitter) sliceOffset(base ast.Node) string {
	b := base
	for b != nil && (b.Kind() == "load_expr" || b.Kind() == "paren_expr" || b.Kind() == "inout_expr") {
		b = b.Child("sub_expr")
	}
	if b == nil || !isArraySlice(e.m.typeOf(b.Type())) {
		return ""
	}
	lo, ok := e.sliceBase(b)
	if !ok {
		e.m.Unsup("arrayslice-index-of-unknown-start", e.where, "")
		return ""
	}
	if lo == "0" {
		return ""
	}
	return lo
}
