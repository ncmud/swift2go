package swiftrt

import "testing"

type twoCaps = struct {
	F0 Substring
	F1 Substring
}

func TestSwiftRegexReplacing(t *testing.T) {
	re := RegexLiteral[twoCaps](`<@!?(\p{Nd}+)>`)
	got := StringReplacingWith(s0, re, func(m RegexMatch[twoCaps]) string { return "@" + m.Output.F1.String() })
	if got != "hi @123 and @45!" {
		t.Fatalf("got %q", got)
	}
	if StringReplacingWith("a{x}b{y}", RegexLiteral[Substring](`\{[^}]*\}`), "") != "ab" {
		t.Fatal("string replacement")
	}
}

const s0 = "hi <@123> and <@!45>!"

func TestSwiftRegexWholeMatch(t *testing.T) {
	re := RegexLiteral[Substring](`[a-z][a-z0-9_]{0,59}`)
	if StringWholeMatchOf("fire_bolt2", re) == nil || StringWholeMatchOf("Fire", re) != nil || StringWholeMatchOf("fire bolt", re) != nil {
		t.Fatal("wholeMatch")
	}
	if StringWholeMatchOf("FIRE", RegexIgnoresCase(re)) == nil {
		t.Fatal("ignoresCase")
	}
}
