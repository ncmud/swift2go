package swiftrt

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

var graphemeSamples = []string{
	"", "a", "ab", "a\r\nb", "\r\n", "\n\r", "e\u0301x", "x\u0301",
	"\U0001F1FA\U0001F1F8\U0001F1EB\U0001F1F7", "\U0001F469\u200d\U0001F469\u200d\U0001F467!",
	"\u0600a b", "{Rred{r} \u2551 box \u255a\u2550\u255d", "a\u0308\u0301ab", "\r\r\n", "\ud55c\uad6d\uc5b4", "a\u200db",
}

func TestFirstGraphemeMatchesUniseg(t *testing.T) {
	for _, s := range graphemeSamples {
		var want []string
		g := uniseg.NewGraphemes(s)
		for g.Next() {
			want = append(want, g.Str())
		}
		got := StringChars(s)
		if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
			t.Errorf("StringChars(%q) = %q, want %q", s, got, want)
		}
		if n := StringCount(s); n != len(want) {
			t.Errorf("StringCount(%q) = %d, want %d", s, n, len(want))
		}
	}
}

func TestHasPrefixAndSuffixChars(t *testing.T) {
	cases := []struct {
		s, p           string
		prefix, suffix bool
	}{
		{"abc", "ab", true, false},
		{"abc", "bc", false, true},
		{"éx", "e", false, false},
		{"éx", "é", true, false},
		{"xé", "é", false, true},
		{"\r\nb", "\r", false, false},
		{"{R║x", "{R", true, false},
		{"║x{r}", "{r}", false, true},
		{"║", "{", false, false},
		{"ab", "abc", false, false},
		{"", "", true, true},
		{"a", "", true, true},
	}
	for _, c := range cases {
		if got := hasPrefixChars(c.s, c.p); got != c.prefix {
			t.Errorf("hasPrefixChars(%q, %q) = %v, want %v", c.s, c.p, got, c.prefix)
		}
		if got := hasSuffixChars(c.s, c.p); got != c.suffix {
			t.Errorf("hasSuffixChars(%q, %q) = %v, want %v", c.s, c.p, got, c.suffix)
		}
	}
}

// BenchmarkSubstringHasPrefixScan is a colour parser's scan: hasPrefix at every offset of a page of text
// with box-drawing characters in it. Each call must cost the prefix's length, not the rest of the page.
func BenchmarkSubstringHasPrefixScan(b *testing.B) {
	page := strings.Repeat("|   HP  50,000/50,000 {G+57{g  ║ Mana  8,550/8,550 ╚═╝  \r\n", 60)
	for b.Loop() {
		for i := 0; i < len(page); i++ {
			_ = SubstringHasPrefix(Substring{Base: page, Lo: i, Hi: len(page)}, "{G")
		}
	}
}

func BenchmarkStringCount(b *testing.B) {
	page := strings.Repeat("|   HP  50,000/50,000 {G+57{g  Mana  8,550/8,550   \r\n", 60)
	for b.Loop() {
		_ = StringCount(page)
	}
}

func TestLastGraphemeMatchesStringChars(t *testing.T) {
	for _, s := range graphemeSamples {
		if s == "" {
			continue
		}
		chars := StringChars(s)
		want := chars[len(chars)-1]
		got, at := lastGrapheme(s)
		if got != want || at != len(s)-len(want) {
			t.Errorf("lastGrapheme(%q) = %q at %d, want %q at %d", s, got, at, want, len(s)-len(want))
		}
	}
}

// BenchmarkStringDropLastLoop walks a page backwards a Character at a time, as index(before:) loops do.
func BenchmarkStringDropLastLoop(b *testing.B) {
	page := strings.Repeat("|   HP  50,000/50,000 {G+57{g  Mana  8,550/8,550   \r\n", 20)
	for b.Loop() {
		for end := len(page); end > 0; {
			_, end = lastGrapheme(page[:end])
		}
	}
}
