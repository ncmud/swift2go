package types

import (
	"strings"
)

// Kind classifies a parsed Swift type.
type Kind int

const (
	Named Kind = iota
	Array
	Dict
	Optional
	Tuple
	Func
	Meta
	Existential
	Generic
	Unknown
)

// Type is a parsed demangled Swift type.
type Type struct {
	Kind   Kind
	Name   string // qualified name for Named/Existential/Generic
	Args   []*Type
	Labels []string // tuple labels; func param labels unused
	Result *Type    // Func result
	Throws bool
	Inout  []bool // Func params marked inout
}

// String renders the type back in Swift-ish notation (for diagnostics).
func (t *Type) String() string {
	if t == nil {
		return "<nil>"
	}
	switch t.Kind {
	case Array:
		return "[" + t.Args[0].String() + "]"
	case Dict:
		return "[" + t.Args[0].String() + " : " + t.Args[1].String() + "]"
	case Optional:
		return t.Args[0].String() + "?"
	case Tuple, Func:
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			l := ""
			if i < len(t.Labels) && t.Labels[i] != "" {
				l = t.Labels[i] + ": "
			}
			parts[i] = l + a.String()
		}
		s := "(" + strings.Join(parts, ", ") + ")"
		if t.Kind == Func {
			s += " -> " + t.Result.String()
		}
		return s
	case Meta:
		return t.Args[0].String() + ".Type"
	case Existential:
		return "any " + t.Name
	}
	s := t.Name
	if len(t.Args) > 0 {
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = a.String()
		}
		s += "<" + strings.Join(parts, ", ") + ">"
	}
	return s
}

type parser struct {
	s   string
	pos int
}

// Parse parses a demangled type string.
func Parse(s string) *Type {
	p := &parser{s: strings.TrimSpace(s)}
	t := p.parseType()
	if p.pos != len(p.s) {
		return &Type{Kind: Unknown, Name: s}
	}
	return t
}

func (p *parser) rest() string {
	if p.pos >= len(p.s) {
		return ""
	}
	return p.s[p.pos:]
}

func (p *parser) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *parser) skipSpace() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) eat(tok string) bool {
	p.skipSpace()
	if strings.HasPrefix(p.rest(), tok) {
		p.pos += len(tok)
		return true
	}
	return false
}

func (p *parser) parseType() *Type {
	p.skipSpace()
	for _, attr := range []string{"@escaping ", "@Sendable ", "@lvalue ", "inout ", "@noescape ", "@autoclosure ", "__owned ", "__shared ", "sending "} {
		if p.eat(attr) {
			return p.parseType()
		}
	}
	var t *Type
	switch {
	case p.eat("some "), p.eat("any "):
		inner := p.parsePrimary()
		if inner.Kind == Generic {
			inner.Kind = Named
		}
		t = &Type{Kind: Existential, Name: inner.Name}
	default:
		t = p.parsePrimary()
	}
	for {
		p.skipSpace()
		switch {
		case p.eat("?"), p.eat("!"):
			t = &Type{Kind: Optional, Args: []*Type{t}}
		case strings.HasPrefix(p.rest(), ".Type"):
			p.pos += len(".Type")
			t = &Type{Kind: Meta, Args: []*Type{t}}
		case strings.HasPrefix(p.rest(), ".Protocol"):
			p.pos += len(".Protocol")
			t = &Type{Kind: Meta, Args: []*Type{t}}
		case p.eat(" & "):
			rhs := p.parsePrimary()
			t = &Type{Kind: Existential, Name: t.Name + "&" + rhs.Name}
		default:
			return t
		}
	}
}

func (p *parser) parsePrimary() *Type {
	p.skipSpace()
	switch p.peek() {
	case '[':
		p.pos++
		k := p.parseType()
		if p.eat(":") {
			v := p.parseType()
			p.eat("]")
			return &Type{Kind: Dict, Args: []*Type{k, v}}
		}
		p.eat("]")
		return &Type{Kind: Array, Args: []*Type{k}}
	case '(':
		p.pos++
		var args []*Type
		var labels []string
		var inout []bool
		for !p.eat(")") {
			if p.pos >= len(p.s) {
				return &Type{Kind: Unknown}
			}
			label := ""
			save := p.pos
			id := p.ident()
			if id != "" && p.eat(":") && p.peek() == ' ' {
				label = id
			} else {
				p.pos = save
			}
			io := p.eat("inout ")
			before := p.pos
			at := p.parseType()
			if p.pos == before {
				p.pos = len(p.s) + 1
				return &Type{Kind: Unknown}
			}
			args = append(args, at)
			labels = append(labels, label)
			inout = append(inout, io)
			p.eat("...")
			p.eat(",")
		}
		p.skipSpace()
		isAsync := p.eat("async ")
		_ = isAsync
		throws := p.eat("throws ")
		if p.eat("->") {
			res := p.parseType()
			return &Type{Kind: Func, Args: args, Labels: labels, Result: res, Throws: throws, Inout: inout}
		}
		if len(args) == 1 && labels[0] == "" {
			return args[0]
		}
		return &Type{Kind: Tuple, Args: args, Labels: labels}
	}
	name, gargs := p.qualifiedName()
	t := &Type{Kind: Named, Name: name, Args: gargs}
	if isGenericParamName(name) {
		t.Kind = Generic
	}
	return t
}

func isGenericParamName(n string) bool {
	if n == "" || strings.Contains(n, ".") {
		return false
	}
	if n[0] < 'A' || n[0] > 'Z' {
		return false
	}
	for _, c := range n[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (p *parser) ident() string {
	p.skipSpace()
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80 {
			p.pos++
			continue
		}
		break
	}
	return p.s[start:p.pos]
}

func (p *parser) qualifiedName() (string, []*Type) {
	var parts []string
	var args []*Type
	for {
		id := p.ident()
		if id == "" {
			break
		}
		if strings.HasPrefix(p.rest(), "<") {
			p.pos++
			for !p.eat(">") {
				if p.pos >= len(p.s) {
					break
				}
				before := p.pos
				args = append(args, p.parseType())
				if p.pos == before {
					p.pos = len(p.s)
					break
				}
				p.eat(",")
			}
		}
		parts = append(parts, id)
		if strings.HasPrefix(p.rest(), ".Type") || strings.HasPrefix(p.rest(), ".Protocol") {
			break
		}
		if p.peek() == '.' {
			p.pos++
			continue
		}
		break
	}
	return strings.Join(parts, "."), args
}
