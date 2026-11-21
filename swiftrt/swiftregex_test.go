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

type fourCaps = struct {
	F0, F1, F2, F3 Substring
}

func TestSwiftRegexUnicodeWordBoundary(t *testing.T) {
	re := RegexLiteral[fourCaps](`\$([eE])((?:\s|\{[^}]+\})*(?:(?:simply|suddenly|just|completely|accidentally|finally)\s+)?)([A-Za-z]+(?:['’]t)?)\b`)
	for in, want := range map[string]bool{"$e attack's foo": false, "$e e.g": false, "$e attacks you": true, "$e can't stop": true} {
		if got := StringFirstMatchOf(in, re) != nil; got != want {
			t.Errorf("%q: matched %v, want %v", in, got, want)
		}
	}
	if m := StringFirstMatchOf("$e can't stop", re); m == nil || m.Output.F3.String() != "can't" {
		t.Fatalf("got %+v", m)
	}
}

func TestSwiftRegexEmptyMatchAfterMatch(t *testing.T) {
	if got := StringReplacingWith("baaac", RegexLiteral[Substring](`a*`), "-"); got != "-b--c-" {
		t.Fatalf("got %q", got)
	}
}
