package swiftrt

import (
	"bytes"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// StringCompareOptions is Foundation `NSString.CompareOptions`.
type StringCompareOptions uint

// NSString.CompareOptions members, under the names the emitter gives imported Objective-C constants.
const (
	NSStringCompareOptionsCENSStringCompareOptionsNSCaseInsensitiveSearch   StringCompareOptions = 1
	NSStringCompareOptionsCENSStringCompareOptionsNSLiteralSearch           StringCompareOptions = 2
	NSStringCompareOptionsCENSStringCompareOptionsNSBackwardsSearch         StringCompareOptions = 4
	NSStringCompareOptionsCENSStringCompareOptionsNSAnchoredSearch          StringCompareOptions = 8
	NSStringCompareOptionsCENSStringCompareOptionsNSRegularExpressionSearch StringCompareOptions = 1024
)

var icuRegexCache sync.Map

// icuClasses are ICU's Unicode meanings of \w, \d and \s (White_Space), written for use inside a bracket expression.
var icuClasses = map[byte]string{
	'w': `\p{L}\p{Nl}\p{M}\p{Nd}\p{Pc}\x{200C}\x{200D}`,
	'd': `\p{Nd}`,
	's': `\t-\r \x{85}\x{A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}`,
}

// icuRegex compiles an ICU pattern with Go's RE2, widening \w, \W, \d, \D, \s and \S to Unicode as ICU does.
// Known differences from ICU: \b uses ASCII word characters, `$` matches only at the very end of the input
// (ICU also matches before a final line terminator), and RE2 has no lookaround or backreferences, so such
// patterns panic.
func icuRegex(pattern string, caseInsensitive bool) *regexp.Regexp {
	re, err := icuRegexCompile(pattern, caseInsensitive)
	if err != nil {
		panic("swiftrt: regular expression not supported by RE2: " + pattern + ": " + err.Error())
	}
	return re
}

// icuRegexCompile is icuRegex returning RE2's compile error instead of panicking.
func icuRegexCompile(pattern string, caseInsensitive bool) (*regexp.Regexp, error) {
	key := pattern
	if caseInsensitive {
		key = "(?i)" + pattern
	}
	if re, ok := icuRegexCache.Load(key); ok {
		return re.(*regexp.Regexp), nil
	}
	var b strings.Builder
	if caseInsensitive {
		b.WriteString("(?i)")
	}
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			n := pattern[i+1]
			i++
			switch n {
			case 'w', 'd', 's':
				if inClass {
					b.WriteString(icuClasses[n])
				} else {
					b.WriteString("[" + icuClasses[n] + "]")
				}
			case 'W', 'D', 'S':
				lower := n + ('a' - 'A')
				if inClass {
					panic("swiftrt: negated ICU class inside a bracket expression is unsupported: " + pattern)
				}
				b.WriteString("[^" + icuClasses[lower] + "]")
			default:
				b.WriteByte('\\')
				b.WriteByte(n)
			}
			continue
		}
		switch {
		case c == '[' && !inClass:
			inClass = true
			b.WriteByte(c)
			if i+1 < len(pattern) && pattern[i+1] == '^' {
				b.WriteByte('^')
				i++
			}
			if i+1 < len(pattern) && pattern[i+1] == ']' {
				b.WriteByte(']')
				i++
			}
		case c == ']' && inClass:
			inClass = false
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, err
	}
	icuRegexCache.Store(key, re)
	return re, nil
}

// icuTemplate converts an ICU replacement template ($1, \$) to Go's Expand syntax.
func icuTemplate(t string) string {
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		c := t[i]
		switch {
		case c == '\\' && i+1 < len(t):
			i++
			if t[i] == '$' {
				b.WriteString("$$")
			} else {
				b.WriteByte(t[i])
			}
		case c == '$' && i+1 < len(t) && t[i+1] >= '0' && t[i+1] <= '9':
			j := i + 1
			for j < len(t) && t[j] >= '0' && t[j] <= '9' {
				j++
			}
			b.WriteString("${" + t[i+1:j] + "}")
			i = j - 1
		case c == '$':
			b.WriteString("$$")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// foldIndex finds needle in s[lo:hi] under Unicode simple case folding; it returns byte offsets into s.
// Foundation's .caseInsensitive uses full case folding, so matches that change length (ß against "SS",
// ŉ against "ʼN") are missed; the standard library has no full-folding table.
func foldIndex(s, needle string, lo, hi int, backwards bool) (int, int, bool) {
	n := utf8.RuneCountInString(needle)
	var starts []int
	for i := lo; i <= hi; {
		starts = append(starts, i)
		if i == hi {
			break
		}
		_, w := utf8.DecodeRuneInString(s[i:hi])
		i += w
	}
	try := func(i int) (int, bool) {
		j := i
		for k := 0; k < n; k++ {
			if j >= hi {
				return 0, false
			}
			_, w := utf8.DecodeRuneInString(s[j:hi])
			j += w
		}
		return j, strings.EqualFold(s[i:j], needle)
	}
	if backwards {
		for k := len(starts) - 1; k >= 0; k-- {
			if j, ok := try(starts[k]); ok {
				return starts[k], j, true
			}
		}
		return 0, 0, false
	}
	for _, i := range starts {
		if j, ok := try(i); ok {
			return i, j, true
		}
	}
	return 0, 0, false
}

func rangeOfOptions(s, needle string, opts StringCompareOptions, lo, hi int) *Range[int] {
	ci := opts&NSStringCompareOptionsCENSStringCompareOptionsNSCaseInsensitiveSearch != 0
	back := opts&NSStringCompareOptionsCENSStringCompareOptionsNSBackwardsSearch != 0
	anchored := opts&NSStringCompareOptionsCENSStringCompareOptionsNSAnchoredSearch != 0
	if needle == "" {
		return nil
	}
	if opts&NSStringCompareOptionsCENSStringCompareOptionsNSRegularExpressionSearch != 0 {
		re := icuRegex(needle, ci)
		all := re.FindAllStringIndex(s[lo:hi], -1)
		if len(all) == 0 {
			return nil
		}
		m := all[0]
		if back {
			m = all[len(all)-1]
		}
		if anchored && ((!back && m[0] != 0) || (back && m[1] != hi-lo)) {
			return nil
		}
		return &Range[int]{lo + m[0], lo + m[1]}
	}
	var i, j int
	var ok bool
	if ci {
		i, j, ok = foldIndex(s, needle, lo, hi, back)
	} else {
		var k int
		if back {
			k = strings.LastIndex(s[lo:hi], needle)
		} else {
			k = strings.Index(s[lo:hi], needle)
		}
		ok = k >= 0
		i, j = lo+k, lo+k+len(needle)
	}
	if !ok || (anchored && ((!back && i != lo) || (back && j != hi))) {
		return nil
	}
	return &Range[int]{i, j}
}

// StringRangeOfOptions is Foundation `range(of:options:)`.
func StringRangeOfOptions(s, needle string, opts StringCompareOptions) *Range[int] {
	return rangeOfOptions(s, needle, opts, 0, len(s))
}

// SubstringRangeOfOptions is Foundation `range(of:options:)` on a Substring; the range indexes the base string.
func SubstringRangeOfOptions(s Substring, needle string, opts StringCompareOptions) *Range[int] {
	return rangeOfOptions(s.Base, needle, opts, s.Lo, s.Hi)
}

// StringReplacingOccurrencesOfWithOptions is Foundation `replacingOccurrences(of:with:options:)`.
func StringReplacingOccurrencesOfWithOptions(s, target, repl string, opts StringCompareOptions) string {
	ci := opts&NSStringCompareOptionsCENSStringCompareOptionsNSCaseInsensitiveSearch != 0
	if opts&NSStringCompareOptionsCENSStringCompareOptionsNSRegularExpressionSearch != 0 {
		return icuRegex(target, ci).ReplaceAllString(s, icuTemplate(repl))
	}
	if !ci {
		return StringReplacingOccurrencesOfWith(s, target, repl)
	}
	var b strings.Builder
	pos := 0
	for {
		i, j, ok := foldIndex(s, target, pos, len(s), false)
		if !ok || target == "" {
			break
		}
		b.WriteString(s[pos:i])
		b.WriteString(repl)
		pos = j
	}
	b.WriteString(s[pos:])
	return b.String()
}

// StringReplacingCharactersInWith is Foundation `replacingCharacters(in:with:)` for a Range or ClosedRange of indices.
func StringReplacingCharactersInWith(s string, r interval[int], repl string) string {
	lo, hi, closed := r.interval()
	if closed {
		hi = StringIndexAfter(s, hi)
	}
	if lo < 0 || hi > len(s) || lo > hi {
		Trap("String index range is out of bounds")
	}
	return s[:lo] + repl + s[hi:]
}

// StringReplaceSubrangeWith is Swift `String.replaceSubrange(_:with:)`.
func StringReplaceSubrangeWith(s *string, r Range[int], repl string) {
	*s = StringReplacingCharactersInWith(*s, r, repl)
}

// SubstringDescription is `Substring.description`.
func SubstringDescription(s Substring) string { return s.String() }

// SubstringRemoveLast is `Substring.removeLast()`.
func SubstringRemoveLast(s *Substring) Character {
	if s.Lo == s.Hi {
		Trap("Can't remove last element from an empty collection")
	}
	c, at := lastGrapheme(s.String())
	s.Hi = s.Lo + at
	return c
}

// SubstringCaseInsensitiveCompare is Foundation `caseInsensitiveCompare(_:)` on a Substring.
func SubstringCaseInsensitiveCompare(a Substring, b string) int {
	return StringCaseInsensitiveCompare(a.String(), b)
}

// IntSamePositionIn is `String.Index.samePosition(in: String)`: nil unless i is on a Character boundary.
func IntSamePositionIn(i int, s string) *int {
	if i == len(s) {
		return &i
	}
	pos := 0
	for _, c := range StringChars(s) {
		if pos == i {
			return &i
		}
		if pos > i {
			break
		}
		pos += len(c)
	}
	return nil
}

// StringOptInitBytesEncoding is Foundation `String(bytes:encoding:)`; invalid UTF-8 gives nil and a leading BOM is dropped.
func StringOptInitBytesEncoding(b []byte, _ StringEncoding) *string {
	b = bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(b) {
		return nil
	}
	s := string(b)
	return &s
}

// CharacterSetControlCharacters is Foundation `.controlCharacters`: general categories Cc and Cf.
var CharacterSetControlCharacters CharacterSet = func(r rune) bool {
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)
}

// CharacterSetUnion is Foundation `CharacterSet.union(_:)`.
func CharacterSetUnion(a, b CharacterSet) CharacterSet {
	return func(r rune) bool { return a(r) || b(r) }
}

// CharacterSetSubtracting is Foundation `CharacterSet.subtracting(_:)`.
func CharacterSetSubtracting(a, b CharacterSet) CharacterSet {
	return func(r rune) bool { return a(r) && !b(r) }
}

// CharacterSetContainsScalar is Foundation `CharacterSet.contains(_: Unicode.Scalar)`.
func CharacterSetContainsScalar(cs CharacterSet, r rune) bool { return cs(r) }

// UnicodeGeneralCategory is Swift `Unicode.GeneralCategory`.
type UnicodeGeneralCategory int

// Unicode.GeneralCategory cases, in Swift's declaration order.
const (
	GeneralCategoryUppercaseLetter UnicodeGeneralCategory = iota
	GeneralCategoryLowercaseLetter
	GeneralCategoryTitlecaseLetter
	GeneralCategoryModifierLetter
	GeneralCategoryOtherLetter
	GeneralCategoryNonspacingMark
	GeneralCategorySpacingMark
	GeneralCategoryEnclosingMark
	GeneralCategoryDecimalNumber
	GeneralCategoryLetterNumber
	GeneralCategoryOtherNumber
	GeneralCategoryConnectorPunctuation
	GeneralCategoryDashPunctuation
	GeneralCategoryOpenPunctuation
	GeneralCategoryClosePunctuation
	GeneralCategoryInitialPunctuation
	GeneralCategoryFinalPunctuation
	GeneralCategoryOtherPunctuation
	GeneralCategoryMathSymbol
	GeneralCategoryCurrencySymbol
	GeneralCategoryModifierSymbol
	GeneralCategoryOtherSymbol
	GeneralCategorySpaceSeparator
	GeneralCategoryLineSeparator
	GeneralCategoryParagraphSeparator
	GeneralCategoryControl
	GeneralCategoryFormat
	GeneralCategorySurrogate
	GeneralCategoryPrivateUse
	GeneralCategoryUnassigned
)

var generalCategoryTables = []*unicode.RangeTable{
	unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lm, unicode.Lo, unicode.Mn, unicode.Mc, unicode.Me,
	unicode.Nd, unicode.Nl, unicode.No, unicode.Pc, unicode.Pd, unicode.Ps, unicode.Pe, unicode.Pi,
	unicode.Pf, unicode.Po, unicode.Sm, unicode.Sc, unicode.Sk, unicode.So, unicode.Zs, unicode.Zl,
	unicode.Zp, unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co,
}

// PropertiesGeneralCategory is Swift `Unicode.Scalar.Properties.generalCategory`.
func PropertiesGeneralCategory(p UnicodeScalarProperties) UnicodeGeneralCategory {
	for i, t := range generalCategoryTables {
		if unicode.Is(t, p.r) {
			return UnicodeGeneralCategory(i)
		}
	}
	return GeneralCategoryUnassigned
}

// swiftFloatDescription renders a float as Swift's `description` does: shortest round-trip digits,
// decimal notation between 1e-4 and 2^significandBits, exponential outside it.
func swiftFloatDescription(f float64, bitSize int) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case f == 0:
		if math.Signbit(f) {
			return "-0.0"
		}
		return "0.0"
	}
	limit, small := math.Ldexp(1, 53), 1e-4
	if bitSize == 32 {
		limit, small = math.Ldexp(1, 24), float64(float32(1e-4))
	}
	if a := math.Abs(f); a < small || a > limit {
		return strconv.FormatFloat(f, 'e', -1, bitSize)
	}
	s := strconv.FormatFloat(f, 'f', -1, bitSize)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// StringInitFloat is Swift `String(_: Float)` / `String(_: Double)`.
func StringInitFloat[F ~float32 | ~float64](f F) string {
	var z F
	if _, ok := any(z).(float32); ok {
		return swiftFloatDescription(float64(f), 32)
	}
	return swiftFloatDescription(float64(f), 64)
}

// Scanner is Foundation `Scanner`, skipping whitespace and newlines.
type Scanner struct {
	s   string
	pos int
}

// NSScannerInitString is Foundation `Scanner(string:)`.
func NSScannerInitString(s string) *Scanner { return &Scanner{s: s} }

var scanFloatPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`)

// NSScannerScanFloat is Foundation `Scanner.scanFloat()`.
func NSScannerScanFloat(sc *Scanner) *float32 {
	i := sc.pos
	for i < len(sc.s) {
		r, w := utf8.DecodeRuneInString(sc.s[i:])
		if !CharacterSetWhitespacesAndNewlines(r) {
			break
		}
		i += w
	}
	m := scanFloatPattern.FindString(sc.s[i:])
	if m == "" {
		return nil
	}
	v, err := strconv.ParseFloat(m, 32)
	if err != nil && !strings.Contains(err.Error(), "range") {
		return nil
	}
	sc.pos = i + len(m)
	f := float32(v)
	return &f
}

// StringOptInitString is `Optional<String>(_:)`: the string wrapped as a non-nil optional.
func StringOptInitString(s string) *string { return &s }
