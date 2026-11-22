package emit

import (
	"fmt"

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
			count = e.expr(args[0].Child("expr"))
		}
		switch d.Str("base_name") {
		case "prefix", "dropLast":
			if label == "" || label == "upTo" || label == "through" || label == "while" {
				return lo, true
			}
		case "dropFirst":
			if e.isPure(stripLoads(recv)) {
				return fmt.Sprintf("(%s + min(%s, len(%s)))", lo, count, e.expr(recv)), true
			}
		case "suffix":
			if label == "from" {
				return count, true
			}
			if label == "" && e.isPure(stripLoads(recv)) {
				return fmt.Sprintf("(%s + max(len(%s)-%s, 0))", lo, e.expr(recv), count), true
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
	switch n.Kind() {
	case "binary_expr", "postfix_unary_expr", "prefix_unary_expr":
		for _, a := range n.Child("args").List("args") {
			if !e.isPure(stripLoads(a.Child("expr"))) {
				return "", false
			}
		}
	default:
		if !e.isPure(n) {
			return "", false
		}
	}
	return "(" + e.expr(r) + ").Lo", true
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

// bindSliceBase records the start index of the ArraySlice a `let` statement binds, in a variable
// declared next to it.
func (e *fileEmitter) bindSliceBase(p, init ast.Node) {
	if e.scopes == nil || !isArraySlice(e.m.typeOf(p.Type())) || e.isVarPattern(p) {
		return
	}
	f := e.scopes.patFrames[nodeKey(p)]
	if f == nil || f.lets[p.Name()] != 1 {
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
