package swiftrt

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
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

// Regex is Foundation `NSRegularExpression`, compiled with Go's RE2 after ICU class widening.
type Regex struct {
	Pattern string
	re      *regexp.Regexp
}

// NSRegularExpressionInitPattern is Foundation `NSRegularExpression(pattern:)`. An invalid pattern
// throws NSCocoaErrorDomain 2048 as on Linux; a valid ICU pattern RE2 cannot run (lookaround,
// backreferences, possessive quantifiers, ICU-only escapes) panics as unsupported.
func NSRegularExpressionInitPattern(pattern string) *Regex {
	re, err := icuRegexCompile(pattern, false)
	if err != nil {
		if icuOnlySyntax(pattern, err) {
			panic("swiftrt: regular expression not supported by RE2: " + pattern + ": " + err.Error())
		}
		panic(Throw(NSError{Domain: "NSCocoaErrorDomain", Code: 2048, Msg: "The operation could not be completed. The value is invalid."}))
	}
	return &Regex{Pattern: pattern, re: re}
}

// icuOnlySyntax reports whether RE2 rejected pattern for a construct ICU accepts.
func icuOnlySyntax(pattern string, err error) bool {
	var se *syntax.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code {
	case syntax.ErrInvalidPerlOp:
		return true
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

// NSRegularExpressionMatchesInRange is Foundation `NSRegularExpression.matches(in:range:)`: the
// non-overlapping matches inside the UTF-16 range r of s, which bounds anchors and lookaround as
// the default (non-transparent, anchoring) bounds do. A range outside s traps. Unlike ICU, an empty
// match directly after a non-empty match is not reported (RE2's iteration rule).
func NSRegularExpressionMatchesInRange(re *Regex, s string, r Range[int]) []TextCheckingResult {
	lo, sub := nsRegexSearchRange(s, r)
	out := []TextCheckingResult{}
	for _, loc := range re.re.FindAllStringSubmatchIndex(sub, -1) {
		out = append(out, textCheckingResult(s, lo, loc))
	}
	return out
}

// NSRegularExpressionFirstMatchInRange is Foundation `NSRegularExpression.firstMatch(in:range:)`:
// the first match inside the UTF-16 range r of s, or nil.
func NSRegularExpressionFirstMatchInRange(re *Regex, s string, r Range[int]) *TextCheckingResult {
	lo, sub := nsRegexSearchRange(s, r)
	loc := re.re.FindStringSubmatchIndex(sub)
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
