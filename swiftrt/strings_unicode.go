package swiftrt

import (
	"strings"

	"github.com/rivo/uniseg"
)

// mapCase is s with each scalar mapped by Swift's full case mapping: special for the scalars
// whose mapping is not unicode's simple one, simple for the rest.
func mapCase(s string, special map[rune]string, simple func(rune) rune) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteString(mapRune(r, special, simple))
	}
	return b.String()
}

func mapRune(r rune, special map[rune]string, simple func(rune) rune) string {
	if m, ok := special[r]; ok {
		return m
	}
	return string(simple(r))
}

// hasPrefixChars is Swift `hasPrefix`: p's Characters begin s's, compared by canonical equivalence.
func hasPrefixChars(s, p string) bool {
	if strings.HasPrefix(s, p) {
		return graphemeBoundary(s, len(p))
	}
	if isASCII(s) && isASCII(p) {
		return false
	}
	cs, ps := StringChars(s), StringChars(p)
	return len(ps) <= len(cs) && charsEquivalent(cs[:len(ps)], ps)
}

// hasSuffixChars is Swift `hasSuffix`: p's Characters end s's, compared by canonical equivalence.
func hasSuffixChars(s, p string) bool {
	if strings.HasSuffix(s, p) {
		return graphemeBoundary(s, len(s)-len(p))
	}
	if isASCII(s) && isASCII(p) {
		return false
	}
	cs, ps := StringChars(s), StringChars(p)
	return len(ps) <= len(cs) && charsEquivalent(cs[len(cs)-len(ps):], ps)
}

func charsEquivalent(a, b []Character) bool {
	for i := range a {
		if a[i] != b[i] && StringDecomposedStringWithCanonicalMapping(a[i]) != StringDecomposedStringWithCanonicalMapping(b[i]) {
			return false
		}
	}
	return true
}

// graphemeBoundary reports whether byte offset i of s falls between two Characters.
func graphemeBoundary(s string, i int) bool {
	if i == 0 || i == len(s) {
		return true
	}
	state, off := -1, 0
	rest := s
	for off < i {
		var c string
		c, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
		off += len(c)
	}
	return off == i
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
