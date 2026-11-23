package swiftrt

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"

	"github.com/dlclark/regexp2"
)

// NSError is a Foundation `NSError` thrown by runtime APIs that report Cocoa errors.
type NSError struct {
	Domain string
	Code   int
	Msg    string
}

func (e NSError) Error() string { return e.Msg }

// NSRegularExpressionEscapedPatternFor is Foundation `NSRegularExpression.escapedPattern(for:)`: each of
// `*?+[(){}^$|\./` gets a backslash, so the result matches s literally.
func NSRegularExpressionEscapedPatternFor(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`*?+[(){}^$|\./`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Regex is Foundation `NSRegularExpression`, compiled with Go's RE2 after ICU class widening, or with
// a backtracking engine when the pattern uses ICU syntax RE2 lacks.
type Regex struct {
	Pattern string
	re      *regexp.Regexp
	bt      *regexp2.Regexp
}

// NSRegularExpressionInitPattern is Foundation `NSRegularExpression(pattern:)`. An invalid pattern
// throws NSCocoaErrorDomain 2048 as on Linux. A valid ICU pattern RE2 cannot run (lookaround,
// backreferences, possessive quantifiers) runs on regexp2's backtracking engine; one neither engine
// accepts (ICU-only escapes) panics as unsupported.
func NSRegularExpressionInitPattern(pattern string) *Regex {
	r, ok := compileICU(pattern, false)
	if !ok {
		panic(Throw(NSError{Domain: "NSCocoaErrorDomain", Code: 2048, Msg: "The operation could not be completed. The value is invalid."}))
	}
	return r
}

// compileICU compiles an ICU pattern on RE2, or on regexp2's backtracking engine when it uses ICU syntax RE2
// lacks. It reports false for a pattern that is invalid, and panics for one neither engine accepts.
func compileICU(pattern string, caseInsensitive bool) (*Regex, bool) {
	re, err := icuRegexCompile(pattern, caseInsensitive)
	if err == nil {
		return &Regex{Pattern: pattern, re: re}, true
	}
	if !icuOnlySyntax(pattern, err) {
		return nil, false
	}
	opts := regexp2.RegexOptions(regexp2.Unicode)
	if caseInsensitive {
		opts |= regexp2.IgnoreCase
	}
	bt, btErr := regexp2.Compile(pattern, opts)
	if btErr != nil {
		panic("swiftrt: regular expression not supported by RE2 or regexp2: " + pattern + ": " + err.Error())
	}
	return &Regex{Pattern: pattern, bt: bt}, true
}

// replaceAll is s with every match replaced by the ICU template.
func (r *Regex) replaceAll(s, template string) string {
	if r.bt == nil {
		return r.re.ReplaceAllString(s, icuTemplate(template))
	}
	var b strings.Builder
	last := 0
	for _, loc := range r.findAll(s) {
		b.WriteString(s[last:loc[0]])
		expandICUTemplate(&b, template, s, loc)
		last = loc[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// findAll is the submatch byte offsets of every non-overlapping match in s, as
// regexp.FindAllStringSubmatchIndex reports them.
func (r *Regex) findAll(s string) [][]int {
	if r.bt == nil {
		return r.re.FindAllStringSubmatchIndex(s, -1)
	}
	var out [][]int
	runes := runeByteOffsets(s)
	m, err := r.bt.FindStringMatch(s)
	for err == nil && m != nil {
		out = append(out, backtrackLoc(m, runes))
		m, err = r.bt.FindNextMatch(m)
	}
	if err != nil {
		panic("swiftrt: regexp2: " + err.Error())
	}
	return out
}

// find is the submatch byte offsets of the first match in s, or nil.
func (r *Regex) find(s string) []int {
	if r.bt == nil {
		return r.re.FindStringSubmatchIndex(s)
	}
	m, err := r.bt.FindStringMatch(s)
	if err != nil {
		panic("swiftrt: regexp2: " + err.Error())
	}
	if m == nil {
		return nil
	}
	return backtrackLoc(m, runeByteOffsets(s))
}

// runeByteOffsets maps each rune index of s, and its length, to a byte offset.
func runeByteOffsets(s string) []int {
	out := make([]int, 0, len(s)+1)
	for i := range s {
		out = append(out, i)
	}
	return append(out, len(s))
}

// backtrackLoc converts a regexp2 match, whose positions count runes, to byte offsets in the layout
// of regexp.FindStringSubmatchIndex: -1 for a group that did not participate.
func backtrackLoc(m *regexp2.Match, runes []int) []int {
	groups := m.Groups()
	loc := make([]int, 2*len(groups))
	for i, g := range groups {
		if len(g.Captures) == 0 {
			loc[2*i], loc[2*i+1] = -1, -1
			continue
		}
		loc[2*i], loc[2*i+1] = runes[g.Index], runes[g.Index+g.Length]
	}
	return loc
}

// icuOnlySyntax reports whether RE2 rejected pattern for a construct ICU accepts.
func icuOnlySyntax(pattern string, err error) bool {
	se, ok := errors.AsType[*syntax.Error](err)
	if !ok {
		return false
	}
	switch se.Code {
	case syntax.ErrInvalidPerlOp:
		return true
	case syntax.ErrInvalidNamedCapture:
		// RE2 reads lookbehind's `(?<=` and `(?<!` as a malformed named group.
		return strings.HasPrefix(se.Expr, "(?<=") || strings.HasPrefix(se.Expr, "(?<!")
	case syntax.ErrInvalidEscape:
		if len(se.Expr) >= 2 {
			c := se.Expr[1]
			return c >= '0' && c <= '9' || strings.IndexByte("GXRhHvVNkuUZ", c) >= 0
		}
	case syntax.ErrInvalidRepeatOp:
		return strings.HasSuffix(se.Expr, "+") && len(se.Expr) >= 2 && strings.IndexByte("*+?}", se.Expr[len(se.Expr)-2]) >= 0
	}
	return false
}

// utf16Offset is the UTF-16 length of s[:i].
func utf16Offset(s string, i int) int {
	n := 0
	for _, r := range s[:i] {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// utf16ByteIndex is the byte offset of UTF-16 offset u in s; ok is false when u is past the end or
// falls between the two halves of a surrogate pair.
func utf16ByteIndex(s string, u int) (int, bool) {
	if u < 0 {
		return 0, false
	}
	n := 0
	for i, r := range s {
		if n == u {
			return i, true
		}
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if n+w > u {
			return 0, false
		}
		n += w
	}
	if n == u {
		return len(s), true
	}
	return 0, false
}

// NSRangeInitPartialRangeFromIn is Foundation `NSRange(_:in:)` on a `PartialRangeFrom<String.Index>`:
// the UTF-16 location and length of `s[r...]`.
func NSRangeInitPartialRangeFromIn(r PartialRangeFrom[int], s string) Range[int] {
	if r.Lo < 0 || r.Lo > len(s) {
		Trap("String index is out of bounds")
	}
	return Range[int]{Lo: utf16Offset(s, r.Lo), Hi: utf16Offset(s, len(s))}
}

// RangeOptInitNSRangeIn is Foundation `Range<String.Index>(_:in:)`: the string range an NSRange
// (UTF-16 offsets) covers, or nil when it is NSNotFound, out of bounds, or (as on Linux) either end
// is not on a Character boundary.
func RangeOptInitNSRangeIn(r Range[int], s string) *Range[int] {
	if r.Lo == IntMax {
		return nil
	}
	lo, ok := utf16ByteIndex(s, r.Lo)
	if !ok {
		return nil
	}
	hi, ok := utf16ByteIndex(s, r.Hi)
	if !ok || IntSamePositionIn(lo, s) == nil || IntSamePositionIn(hi, s) == nil {
		return nil
	}
	if hi < lo {
		Trap("Range requires lowerBound <= upperBound")
	}
	return &Range[int]{Lo: lo, Hi: hi}
}

// TextCheckingResult is Foundation `NSTextCheckingResult` for a regular-expression match: the UTF-16
// range of the whole match followed by each capture group's, NSNotFound for a group that did not
// participate.
type TextCheckingResult struct{ Ranges []Range[int] }

// NSTextCheckingResultRange is Foundation `NSTextCheckingResult.range`: the whole match's range.
func NSTextCheckingResultRange(m TextCheckingResult) Range[int] { return m.Ranges[0] }

// NSTextCheckingResultRangeAt is Foundation `NSTextCheckingResult.range(at:)`: capture group i's
// range, 0 being the whole match.
func NSTextCheckingResultRangeAt(m TextCheckingResult, i int) Range[int] { return m.Ranges[i] }

// NSRegularExpressionMatchesInRange is Foundation `NSRegularExpression.matches(in:range:)`: the
// non-overlapping matches inside the UTF-16 range r of s, which bounds anchors and lookaround as
// the default (non-transparent, anchoring) bounds do. A range outside s traps. Unlike ICU, an empty
// match directly after a non-empty match is not reported (RE2's iteration rule).
func NSRegularExpressionMatchesInRange(re *Regex, s string, r Range[int]) []TextCheckingResult {
	lo, sub := nsRegexSearchRange(s, r)
	out := []TextCheckingResult{}
	for _, loc := range re.findAll(sub) {
		out = append(out, textCheckingResult(s, lo, loc))
	}
	return out
}

// NSRegularExpressionFirstMatchInRange is Foundation `NSRegularExpression.firstMatch(in:range:)`:
// the first match inside the UTF-16 range r of s, or nil.
func NSRegularExpressionFirstMatchInRange(re *Regex, s string, r Range[int]) *TextCheckingResult {
	lo, sub := nsRegexSearchRange(s, r)
	loc := re.find(sub)
	if loc == nil {
		return nil
	}
	m := textCheckingResult(s, lo, loc)
	return &m
}

// nsRegexSearchRange is the byte offset and text of the UTF-16 range r of s.
func nsRegexSearchRange(s string, r Range[int]) (int, string) {
	lo, ok1 := utf16ByteIndex(s, r.Lo)
	hi, ok2 := utf16ByteIndex(s, r.Hi)
	if !ok1 || !ok2 || r.Hi < r.Lo {
		Trap(fmt.Sprintf("NSRegularExpression: range {%d, %d} out of bounds of the string", r.Lo, r.Hi-r.Lo))
	}
	return lo, s[lo:hi]
}

// textCheckingResult converts submatch byte offsets within s[base:] to UTF-16 ranges of s.
func textCheckingResult(s string, base int, loc []int) TextCheckingResult {
	m := TextCheckingResult{Ranges: make([]Range[int], len(loc)/2)}
	for i := range m.Ranges {
		if loc[2*i] < 0 {
			m.Ranges[i] = Range[int]{Lo: IntMax, Hi: IntMax}
			continue
		}
		m.Ranges[i] = Range[int]{Lo: utf16Offset(s, base+loc[2*i]), Hi: utf16Offset(s, base+loc[2*i+1])}
	}
	return m
}

// MatchingOptions is Foundation `NSRegularExpression.MatchingOptions`.
type MatchingOptions struct{ RawValue uint }

// NSRangeInitRangeIn is Foundation `NSRange(_:in:)` on a `Range<String.Index>`: the UTF-16 location
// and length of `s[r]`.
func NSRangeInitRangeIn(r Range[int], s string) Range[int] {
	if r.Lo < 0 || r.Hi > len(s) || r.Lo > r.Hi {
		Trap("String index is out of bounds")
	}
	return Range[int]{Lo: utf16Offset(s, r.Lo), Hi: utf16Offset(s, r.Hi)}
}

// NSRegularExpressionStringByReplacingMatchesInOptionsRangeWithTemplate is Foundation
// `NSRegularExpression.stringByReplacingMatches(in:options:range:withTemplate:)` with no options:
// s with each match inside the UTF-16 range r replaced by the template, in which `$n` is capture
// group n and a backslash makes the next character literal. A range outside s traps.
func NSRegularExpressionStringByReplacingMatchesInOptionsRangeWithTemplate(re *Regex, s string, options MatchingOptions, r Range[int], template string) string {
	if options.RawValue != 0 {
		panic(fmt.Sprintf("swiftrt: NSRegularExpression.MatchingOptions %#x are not supported", options.RawValue))
	}
	lo, sub := nsRegexSearchRange(s, r)
	return s[:lo] + re.replaceAll(sub, template) + s[lo+len(sub):]
}

// expandICUTemplate writes an ICU replacement template for one match: `$n` is capture group n (as
// many digits as name a group) and a backslash makes the next character literal.
func expandICUTemplate(b *strings.Builder, template, s string, loc []int) {
	groups := len(loc) / 2
	for i := 0; i < len(template); i++ {
		c := template[i]
		switch {
		case c == '\\' && i+1 < len(template):
			i++
			b.WriteByte(template[i])
		case c == '$' && i+1 < len(template) && template[i+1] >= '0' && template[i+1] <= '9':
			n, j := 0, i+1
			for j < len(template) && template[j] >= '0' && template[j] <= '9' && (n*10+int(template[j]-'0')) < groups {
				n = n*10 + int(template[j]-'0')
				j++
			}
			if j == i+1 {
				n, j = int(template[i+1]-'0'), i+2
			}
			if n < groups && loc[2*n] >= 0 {
				b.WriteString(s[loc[2*n]:loc[2*n+1]])
			}
			i = j - 1
		default:
			b.WriteByte(c)
		}
	}
}

// NSTextCheckingResultNumberOfRanges is Foundation `NSTextCheckingResult.numberOfRanges`: the whole match and
// each capture group.
func NSTextCheckingResultNumberOfRanges(m TextCheckingResult) int { return len(m.Ranges) }

// IntUtf16OffsetIn is `String.Index.utf16Offset(in:)`: the UTF-16 code units of s before i.
func IntUtf16OffsetIn(i int, s string) int {
	if i < 0 || i > len(s) {
		Trap("String index is out of bounds")
	}
	return utf16Offset(s, i)
}
