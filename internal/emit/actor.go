package emit

import (
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
)

// actorEnter is the statement that runs the rest of a function on self's actor; see docs/ACTORS.md.
const actorEnter = "defer swiftrt.ActorEnter(self.SwiftActor())()\n"

// isActor reports whether the type declaration info is an actor.
func isActor(info *DeclInfo) bool {
	return info != nil && info.Kind == "class_decl" && info.Node.Bool("actor")
}

// actorIsolated reports whether member, a method or the property or subscript an accessor belongs to,
// runs isolated to its owner's actor. Statics and `nonisolated` members do not.
func (e *fileEmitter) actorIsolated(member ast.Node, info *DeclInfo) bool {
	if info == nil || info.Static || member.Bool("nonisolated") {
		return false
	}
	return isActor(e.m.typeDecl(info.Owner))
}

// isolatedMember starts the body of an actor's member on its actor when it is isolated, and marks self
// as on the actor while the body is emitted; the returned function restores the marks.
func (e *fileEmitter) isolatedMember(member ast.Node, info *DeclInfo) func() {
	saveIn, saveOff := e.inActor, e.selfOffActor
	e.inActor, e.selfOffActor = false, false
	if e.actorIsolated(member, info) {
		e.preamble += actorEnter
		e.inActor = true
	}
	return func() { e.inActor, e.selfOffActor = saveIn, saveOff }
}

// isolatedClosure is the start of closure n's body when the closure runs on the actor of the method it
// is written in, and on another goroutine: an escaping closure the AST marks actor_isolated. It marks
// self as off the actor inside an escaping closure that is not isolated.
func (e *fileEmitter) isolatedClosure(n ast.Node) string {
	if !n.Bool("escaping") {
		return ""
	}
	if _, isolated := n["actor_isolated"]; isolated && e.inActor {
		e.selfOffActor = false
		return actorEnter
	}
	e.selfOffActor = true
	return ""
}

// actorStoredRead is the read of stored property info through base, when the property is an actor's
// isolated state and the read is not on the actor already: a read through another reference, or through
// self from a closure that runs off the actor.
func (e *fileEmitter) actorStoredRead(base ast.Node, info *DeclInfo, read func(recv string) string) (string, bool) {
	if info.Static || info.Computed || info.Node.Bool("let") || info.Node.Bool("nonisolated") || !isActor(e.m.typeDecl(info.Owner)) {
		return "", false
	}
	if isSelfRef(base) && !e.selfOffActor {
		return "", false
	}
	recvT := e.exprGoType(base)
	if !strings.HasPrefix(recvT, "*") {
		return "", false
	}
	t := e.m.typeOf(info.Node.Str("interface_type"))
	value := read("self")
	if e.needsCopy(t) {
		// The value leaves the actor, whose later writes it must not see.
		value = "swiftrt.Copy(" + value + ")"
	}
	return "swiftrt.ActorRead(" + e.expr(base) + ", func(self " + recvT + ") " + e.m.GoType(t, e.generics) + " {\nreturn " + value + "\n})", true
}
