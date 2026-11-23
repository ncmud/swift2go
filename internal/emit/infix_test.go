package emit

import "testing"

func TestInfix(t *testing.T) {
	cases := []struct{ l, op, r, want string }{
		{"a", "&", "b << 2", "a & (b << 2)"},
		{"a", "*", "b >> 1", "a * (b >> 1)"},
		{"a << 2", "&", "b", "a << 2 & b"},
		{"a", "|", "b & c", "a | b & c"},
		{"a", "|", "b ^ c", "a | (b ^ c)"},
		{"a | b", "^", "c", "a | b ^ c"},
		{"a", "-", "b - c", "a - (b - c)"},
		{"a + b", "*", "c", "(a + b) * c"},
		{"a", "&&", "b || c", "a && (b || c)"},
		{"a", "||", "b && c", "a || b && c"},
		{"f", "==", "len(s) == 0", "f == (len(s) == 0)"},
		{"x", "+", "-y", "x + -y"},
		{"f(a + b)", "*", "c[i-1]", "f(a + b) * c[i-1]"},
		{"func() int { return a + b }()", "&", "c", "func() int { return a + b }() & c"},
	}
	for _, c := range cases {
		if got := infix(c.l, c.op, c.r); got != c.want {
			t.Errorf("infix(%q, %q, %q) = %q, want %q", c.l, c.op, c.r, got, c.want)
		}
	}
}

func TestUnaryOperand(t *testing.T) {
	cases := map[string]string{
		"x":               "x",
		"len(s) == 0":     "(len(s) == 0)",
		"a - b":           "(a - b)",
		"f(a - b)":        "f(a - b)",
		"swiftrt.Neg(x)":  "swiftrt.Neg(x)",
		"*p":              "*p",
		"s.items[i+1].ok": "s.items[i+1].ok",
	}
	for src, want := range cases {
		if got := unaryOperand(src); got != want {
			t.Errorf("unaryOperand(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestAssocInfix(t *testing.T) {
	cases := []struct{ l, op, r, want string }{
		{"a", "&&", "b && c", "a && b && c"},
		{"a", "&&", "b || c", "a && (b || c)"},
		{`s`, "+", `"x" + t`, `s + "x" + t`},
		{`s`, "+", `a - b`, `s + (a - b)`},
	}
	for _, c := range cases {
		if got := assocInfix(c.l, c.op, c.r); got != c.want {
			t.Errorf("assocInfix(%q, %q, %q) = %q, want %q", c.l, c.op, c.r, got, c.want)
		}
	}
}
