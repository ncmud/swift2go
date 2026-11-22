package swiftrt

import (
	"bytes"
	"fmt"
	"hash/maphash"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func sub(s string) Substring { return Substring{s, 0, len(s)} }

// StringIndexOffsetBy is Swift `String.index(_:offsetBy:)` over grapheme clusters; it traps past either end.
func StringIndexOffsetBy(s string, i, n int) int { return SubstringIndexOffsetBy(sub(s), i, n) }

// SubstringIndexOffsetBy is Swift `Substring.index(_:offsetBy:)`.
func SubstringIndexOffsetBy(s Substring, i, n int) int {
	for ; n > 0; n-- {
		if i >= s.Hi {
			Trap("String index is out of bounds")
		}
		i = SubstringIndexAfter(s, i)
	}
	for ; n < 0; n++ {
		if i <= s.Lo {
			Trap("String index is out of bounds")
		}
		i = SubstringIndexBefore(s, i)
	}
	return i
}

// StringIndexOffsetByLimitedBy is Swift `String.index(_:offsetBy:limitedBy:)`.
func StringIndexOffsetByLimitedBy(s string, i, n, limit int) *int {
	for ; n > 0; n-- {
		if i == limit || i >= len(s) {
			return nil
		}
		i = StringIndexAfter(s, i)
	}
	for ; n < 0; n++ {
		if i == limit || i <= 0 {
			return nil
		}
		i = StringIndexBefore(s, i)
	}
	return &i
}

// SubstringIndexAfter is Swift `Substring.index(after:)`.
func SubstringIndexAfter(s Substring, i int) int {
	if i >= s.Hi {
		Trap("String index is out of bounds")
	}
	return i + graphemeLen(s.Base[i:s.Hi])
}

// SubstringIndexBefore is Swift `Substring.index(before:)`.
func SubstringIndexBefore(s Substring, i int) int {
	if i <= s.Lo {
		Trap("String index is out of bounds")
	}
	_, n := lastGrapheme(s.Base[s.Lo:i])
	return s.Lo + n
}

// StringDistanceFromTo is Swift `String.distance(from:to:)` in grapheme clusters.
func StringDistanceFromTo(s string, a, b int) int {
	if a <= b {
		return graphemeCount(s[a:b])
	}
	return -graphemeCount(s[b:a])
}

// SubstringDistanceFromTo is Swift `Substring.distance(from:to:)`.
func SubstringDistanceFromTo(s Substring, a, b int) int { return StringDistanceFromTo(s.Base, a, b) }

// SubstringStartIndex is Swift `Substring.startIndex`.
func SubstringStartIndex(s Substring) int { return s.Lo }

// SubstringEndIndex is Swift `Substring.endIndex`.
func SubstringEndIndex(s Substring) int { return s.Hi }

// SubstringSubscriptInt is Swift `Substring[i]`.
func SubstringSubscriptInt(s Substring, i int) Character {
	if i < s.Lo || i >= s.Hi {
		Trap("String index is out of bounds")
	}
	return s.Base[i : i+graphemeLen(s.Base[i:s.Hi])]
}

// SubstringSubscriptRange is Swift `Substring[a..<b]`.
func SubstringSubscriptRange(s Substring, r Range[int]) Substring {
	if r.Lo < s.Lo || r.Hi > s.Hi || r.Lo > r.Hi {
		Trap("Substring range is out of bounds")
	}
	return Substring{s.Base, r.Lo, r.Hi}
}

// SubstringSubscriptClosedRange is Swift `Substring[a...b]`.
func SubstringSubscriptClosedRange(s Substring, r ClosedRange[int]) Substring {
	return SubstringSubscriptRange(s, Range[int]{r.Lo, SubstringIndexAfter(s, r.Hi)})
}

// SubstringSubscriptPartialRangeFrom is Swift `Substring[a...]`.
func SubstringSubscriptPartialRangeFrom(s Substring, r PartialRangeFrom[int]) Substring {
	return SubstringSubscriptRange(s, Range[int]{r.Lo, s.Hi})
}

// SubstringSubscriptPartialRangeUpTo is Swift `Substring[..<b]`.
func SubstringSubscriptPartialRangeUpTo(s Substring, r PartialRangeUpTo[int]) Substring {
	return SubstringSubscriptRange(s, Range[int]{s.Lo, r.Hi})
}

// SubstringSubscriptPartialRangeThrough is Swift `Substring[...b]`.
func SubstringSubscriptPartialRangeThrough(s Substring, r PartialRangeThrough[int]) Substring {
	return SubstringSubscriptRange(s, Range[int]{s.Lo, SubstringIndexAfter(s, r.Hi)})
}

// StringSubscriptPartialRangeUpTo is Swift `String[..<b]`.
func StringSubscriptPartialRangeUpTo(s string, r PartialRangeUpTo[int]) Substring {
	return SubstringSubscriptPartialRangeUpTo(sub(s), r)
}

// StringSubscriptPartialRangeThrough is Swift `String[...b]`.
func StringSubscriptPartialRangeThrough(s string, r PartialRangeThrough[int]) Substring {
	return SubstringSubscriptPartialRangeThrough(sub(s), r)
}

// StringSubscriptClosedRange is Swift `String[a...b]`.
func StringSubscriptClosedRange(s string, r ClosedRange[int]) Substring {
	return SubstringSubscriptClosedRange(sub(s), r)
}

// StringSubscriptAll is Swift `String[...]`.
func StringSubscriptAll(s string) Substring { return sub(s) }

// SubstringSubscriptAll is Swift `Substring[...]`.
func SubstringSubscriptAll(s Substring) Substring { return s }

// SubstringInitSubstring is Swift `Substring(Substring)`.
func SubstringInitSubstring(s Substring) Substring { return s }

// SubstringInitString is Swift `Substring(String)`.
func SubstringInitString(s string) Substring { return sub(s) }

// SubstringHasPrefix is Swift `Substring.hasPrefix`.
func SubstringHasPrefix(s Substring, p string) bool { return hasPrefixChars(s.String(), p) }

// SubstringHasSuffix is Swift `Substring.hasSuffix`.
func SubstringHasSuffix(s Substring, p string) bool { return hasSuffixChars(s.String(), p) }

// SubstringUppercased is Swift `Substring.uppercased()`.
func SubstringUppercased(s Substring) string { return StringUppercased(s.String()) }

// SubstringLowercased is Swift `Substring.lowercased()`.
func SubstringLowercased(s Substring) string { return StringLowercased(s.String()) }

// CharacterLowercased is Swift `Character.lowercased()`.
func CharacterLowercased(c Character) string { return StringLowercased(c) }

// CharacterUppercased is Swift `Character.uppercased()`.
func CharacterUppercased(c Character) string { return StringUppercased(c) }

// SubstringFirst is Swift `Substring.first`.
func SubstringFirst(s Substring) *Character { return StringFirst(s.String()) }

// SubstringLast is Swift `Substring.last`.
func SubstringLast(s Substring) *Character { return StringLast(s.String()) }

// SubstringDropFirst is Swift `Substring.dropFirst()`.
func SubstringDropFirst(s Substring) Substring { return SubstringDropFirstInt(s, 1) }

// SubstringDropFirstInt is Swift `Substring.dropFirst(n)`.
func SubstringDropFirstInt(s Substring, n int) Substring {
	i := s.Lo
	for ; n > 0 && i < s.Hi; n-- {
		i = SubstringIndexAfter(s, i)
	}
	return Substring{s.Base, i, s.Hi}
}

// SubstringDropLast is Swift `Substring.dropLast()`.
func SubstringDropLast(s Substring) Substring { return SubstringDropLastInt(s, 1) }

// SubstringDropLastInt is Swift `Substring.dropLast(n)`.
func SubstringDropLastInt(s Substring, n int) Substring {
	i := s.Hi
	for ; n > 0 && i > s.Lo; n-- {
		i = SubstringIndexBefore(s, i)
	}
	return Substring{s.Base, s.Lo, i}
}

// StringDropFirst0 is Swift `String.dropFirst()` with its default count.
func StringDropFirst0(s string) Substring { return StringDropFirst(s, 1) }

// StringPrefixInt is Swift `String.prefix(n)`.
func StringPrefixInt(s string, n int) Substring { return SubstringPrefixInt(sub(s), n) }

// SubstringPrefixInt is Swift `Substring.prefix(n)`.
func SubstringPrefixInt(s Substring, n int) Substring {
	if n < 0 {
		Trap("Can't take a prefix of negative length from a collection")
	}
	i := s.Lo
	for ; n > 0 && i < s.Hi; n-- {
		i = SubstringIndexAfter(s, i)
	}
	return Substring{s.Base, s.Lo, i}
}

// StringSuffixInt is Swift `String.suffix(n)`.
func StringSuffixInt(s string, n int) Substring { return SubstringSuffixInt(sub(s), n) }

// SubstringSuffixInt is Swift `Substring.suffix(n)`.
func SubstringSuffixInt(s Substring, n int) Substring {
	if n < 0 {
		Trap("Can't take a suffix of negative length from a collection")
	}
	i := s.Hi
	for ; n > 0 && i > s.Lo; n-- {
		i = SubstringIndexBefore(s, i)
	}
	return Substring{s.Base, i, s.Hi}
}

// StringPrefixWhile is Swift `String.prefix(while:)`.
func StringPrefixWhile(s string, f func(Character) bool) Substring {
	return SubstringPrefixWhile(sub(s), f)
}

// SubstringPrefixWhile is Swift `Substring.prefix(while:)`.
func SubstringPrefixWhile(s Substring, f func(Character) bool) Substring {
	i := s.Lo
	for i < s.Hi {
		n := graphemeLen(s.Base[i:s.Hi])
		if !f(s.Base[i : i+n]) {
			break
		}
		i += n
	}
	return Substring{s.Base, s.Lo, i}
}

// StringDropWhile is Swift `String.drop(while:)`.
func StringDropWhile(s string, f func(Character) bool) Substring {
	p := StringPrefixWhile(s, f)
	return Substring{s, p.Hi, len(s)}
}

// SubstringFirstIndexOf is Swift `Substring.firstIndex(of:)`.
func SubstringFirstIndexOf(s Substring, c Character) *int {
	for i := s.Lo; i < s.Hi; {
		n := graphemeLen(s.Base[i:s.Hi])
		if s.Base[i:i+n] == c {
			return &i
		}
		i += n
	}
	return nil
}

// SubstringLastIndexOf is Swift `Substring.lastIndex(of:)`.
func SubstringLastIndexOf(s Substring, c Character) *int {
	var found *int
	for i := s.Lo; i < s.Hi; {
		n := graphemeLen(s.Base[i:s.Hi])
		if s.Base[i:i+n] == c {
			j := i
			found = &j
		}
		i += n
	}
	return found
}

// StringFirstIndexWhere is Swift `String.firstIndex(where:)`.
func StringFirstIndexWhere(s string, f func(Character) bool) *int {
	for i := 0; i < len(s); {
		n := graphemeLen(s[i:])
		if f(s[i : i+n]) {
			return &i
		}
		i += n
	}
	return nil
}

// SubstringFirstIndexWhere is Swift `Substring.firstIndex(where:)`.
func SubstringFirstIndexWhere(s Substring, f func(Character) bool) *int {
	for i := s.Lo; i < s.Hi; {
		n := graphemeLen(s.Base[i:s.Hi])
		if f(s.Base[i : i+n]) {
			return &i
		}
		i += n
	}
	return nil
}

// StringContainsWhere is Swift `String.contains(where:)`.
func StringContainsWhere(s string, f func(Character) bool) bool {
	return StringFirstIndexWhere(s, f) != nil
}

// SubstringContainsWhere is Swift `Substring.contains(where:)`.
func SubstringContainsWhere(s Substring, f func(Character) bool) bool {
	return SubstringFirstIndexWhere(s, f) != nil
}

// SubstringContains is Swift `Substring.contains(Character)`.
func SubstringContains(s Substring, c Character) bool { return SubstringFirstIndexOf(s, c) != nil }

// SubstringAllSatisfy is Swift `Substring.allSatisfy`.
func SubstringAllSatisfy(s Substring, f func(Character) bool) bool {
	return !SubstringContainsWhere(s, func(c Character) bool { return !f(c) })
}

// SubstringSplitSeparator is Swift `Substring.split(separator:)`.
func SubstringSplitSeparator(s Substring, sep Character) []Substring {
	return SubstringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s, sep, IntMax, true)
}

// SubstringSplitSeparatorOmittingEmptySubsequences is Swift `Substring.split(separator:omittingEmptySubsequences:)`.
func SubstringSplitSeparatorOmittingEmptySubsequences(s Substring, sep Character, omit bool) []Substring {
	return SubstringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s, sep, IntMax, omit)
}

// SubstringSplitSeparatorMaxSplitsOmittingEmptySubsequences is the general Substring split.
func SubstringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s Substring, sep Character, maxSplits int, omit bool) []Substring {
	var out []Substring
	start := s.Lo
	for i := s.Lo; i < s.Hi; {
		n := graphemeLen(s.Base[i:s.Hi])
		if s.Base[i:i+n] == sep && len(out) < maxSplits {
			if !(omit && start == i) {
				out = append(out, Substring{s.Base, start, i})
			}
			start = i + n
		}
		i += n
	}
	if !(omit && start == s.Hi) {
		out = append(out, Substring{s.Base, start, s.Hi})
	}
	if out == nil {
		out = []Substring{}
	}
	return out
}

// StringSplitWhereSeparator is Swift `String.split(whereSeparator:)`.
func StringSplitWhereSeparator(s string, f func(Character) bool) []Substring {
	var out []Substring
	start := 0
	for i := 0; i < len(s); {
		n := graphemeLen(s[i:])
		if f(s[i : i+n]) {
			if start != i {
				out = append(out, Substring{s, start, i})
			}
			start = i + n
		}
		i += n
	}
	if start != len(s) {
		out = append(out, Substring{s, start, len(s)})
	}
	if out == nil {
		out = []Substring{}
	}
	return out
}

// SubstringUnicodeScalars is Swift `Substring.unicodeScalars`.
func SubstringUnicodeScalars(s Substring) []rune { return []rune(s.String()) }

// StringReplacingOccurrencesOfWith is Foundation `replacingOccurrences(of:with:)`.
func StringReplacingOccurrencesOfWith(s, old, new string) string {
	if old == "" {
		return s
	}
	return strings.ReplaceAll(s, old, new)
}

// SubstringReplacingOccurrencesOfWith is Foundation `replacingOccurrences(of:with:)` on a Substring.
func SubstringReplacingOccurrencesOfWith(s Substring, old, new string) string {
	return StringReplacingOccurrencesOfWith(s.String(), old, new)
}

// StringCaseInsensitiveCompare is Foundation `caseInsensitiveCompare`, returning -1, 0 or 1.
func StringCaseInsensitiveCompare(a, b string) int {
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}

// StringRangeOf is Foundation `range(of:)`.
func StringRangeOf(s, needle string) *Range[int] {
	i := strings.Index(s, needle)
	if i < 0 {
		return nil
	}
	return &Range[int]{i, i + len(needle)}
}

// StringRangeOfRange is Foundation `range(of:range:)`.
func StringRangeOfRange(s, needle string, within *Range[int]) *Range[int] {
	lo, hi := 0, len(s)
	if within != nil {
		lo, hi = within.Lo, within.Hi
	}
	i := strings.Index(s[lo:hi], needle)
	if i < 0 {
		return nil
	}
	return &Range[int]{lo + i, lo + i + len(needle)}
}

// SubstringRangeOf is Foundation `range(of:)` on a Substring.
func SubstringRangeOf(s Substring, needle string) *Range[int] {
	r := StringRangeOfRange(s.Base, needle, &Range[int]{s.Lo, s.Hi})
	return r
}

// CharacterSet is Foundation `CharacterSet` as a scalar predicate.
type CharacterSet func(rune) bool

// CharacterSetWhitespacesAndNewlines is Foundation `.whitespacesAndNewlines`.
var CharacterSetWhitespacesAndNewlines CharacterSet = func(r rune) bool {
	return unicode.IsSpace(r) || r == 0x85 || r == 0x2028 || r == 0x2029
}

// CharacterSetWhitespaces is Foundation `.whitespaces`.
var CharacterSetWhitespaces CharacterSet = func(r rune) bool {
	return r == ' ' || r == '\t' || unicode.Is(unicode.Zs, r)
}

// CharacterSetNewlines is Foundation `.newlines`.
var CharacterSetNewlines CharacterSet = func(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x0B || r == 0x0C || r == 0x85 || r == 0x2028 || r == 0x2029
}

// CharacterSetDecimalDigits is Foundation `.decimalDigits`.
var CharacterSetDecimalDigits CharacterSet = func(r rune) bool { return unicode.IsDigit(r) }

// CharacterSetLetters is Foundation `.letters`.
var CharacterSetLetters CharacterSet = func(r rune) bool { return unicode.IsLetter(r) || unicode.IsMark(r) }

// CharacterSetAlphanumerics is Foundation `.alphanumerics`.
var CharacterSetAlphanumerics CharacterSet = func(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r)
}

// CharacterSetPunctuationCharacters is Foundation `.punctuationCharacters`.
var CharacterSetPunctuationCharacters CharacterSet = func(r rune) bool { return unicode.IsPunct(r) }

// CharacterSetInitCharactersIn is Foundation `CharacterSet(charactersIn:)`.
func CharacterSetInitCharactersIn(s string) CharacterSet {
	return func(r rune) bool { return strings.ContainsRune(s, r) }
}

// CharacterSetContains is Foundation `CharacterSet.contains`.
func CharacterSetContains(cs CharacterSet, r rune) bool { return cs(r) }

// StringTrimmingCharactersIn is Foundation `trimmingCharacters(in:)`.
func StringTrimmingCharactersIn(s string, cs CharacterSet) string { return strings.TrimFunc(s, cs) }

// SubstringTrimmingCharactersIn is Foundation `trimmingCharacters(in:)` on a Substring.
func SubstringTrimmingCharactersIn(s Substring, cs CharacterSet) string {
	return strings.TrimFunc(s.String(), cs)
}

// StringComponentsSeparatedBy is Foundation `components(separatedBy:)`. With a String separator it
// splits at each occurrence, an empty separator leaving s whole; with a CharacterSet it splits at
// every scalar in the set, so adjacent separators such as "\r\n" yield an empty component.
func StringComponentsSeparatedBy[S ~string | CharacterSet](s string, sep S) []string {
	switch sep := any(sep).(type) {
	case CharacterSet:
		parts := []string{}
		start := 0
		for i, r := range s {
			if sep(r) {
				parts = append(parts, s[start:i])
				start = i + utf8.RuneLen(r)
			}
		}
		return append(parts, s[start:])
	}
	str := reflect.ValueOf(sep).String()
	if str == "" {
		return []string{s}
	}
	return strings.Split(s, str)
}

// String is Foundation `NSString`, which bridges from a Swift String unchanged.
type String = string

// NSStringAppendingPathComponent is Foundation `NSString.appendingPathComponent(_:)` as on Linux:
// the component joined with one `/` unless the path is empty or already ends in `/`; an empty
// component leaves the path unchanged.
func NSStringAppendingPathComponent(path, component string) string {
	switch {
	case component == "":
		return path
	case path == "":
		return component
	case strings.HasSuffix(path, "/"):
		return path + component
	}
	return path + "/" + component
}

// StringInitDecodingAs is Swift `String(decoding:as: UTF8.self)`: each maximal ill-formed subpart of
// the bytes becomes one U+FFFD, so `[0xE2, 0x82]` decodes to one replacement and `[0xFF, 0xFF]` to two.
func StringInitDecodingAs(b []byte, _ any) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for i := 0; i < len(b); {
		r, n := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || n > 1 {
			sb.Write(b[i : i+n])
			i += n
			continue
		}
		sb.WriteRune(utf8.RuneError)
		i += utf8MaximalSubpart(b[i:])
	}
	return sb.String()
}

// utf8MaximalSubpart is the length of the maximal ill-formed subpart at the start of b, which does
// not begin a well-formed scalar: the lead byte and the continuation bytes that could still have
// completed it under Unicode Table 3-7, and at least one byte.
func utf8MaximalSubpart(b []byte) int {
	lead := b[0]
	var need int
	lo, hi := byte(0x80), byte(0xBF)
	switch {
	case lead >= 0xC2 && lead <= 0xDF:
		need = 1
	case lead == 0xE0:
		need, lo = 2, 0xA0
	case lead >= 0xE1 && lead <= 0xEC, lead == 0xEE, lead == 0xEF:
		need = 2
	case lead == 0xED:
		need, hi = 2, 0x9F
	case lead == 0xF0:
		need, lo = 3, 0x90
	case lead >= 0xF1 && lead <= 0xF3:
		need = 3
	case lead == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	n := 1
	for ; n <= need && n < len(b); n++ {
		if b[n] < lo || b[n] > hi {
			break
		}
		lo, hi = 0x80, 0xBF
	}
	return n
}

// StringInitCString is Swift `String(cString:)`. b holds the C string's bytes, as a byte or CChar
// slice or a string, and the result stops at the first NUL.
func StringInitCString(b any) string {
	var raw []byte
	switch v := b.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	case []int8:
		raw = make([]byte, len(v))
		for i, c := range v {
			raw[i] = byte(c)
		}
	case *any:
		return StringInitCString(*v)
	default:
		panic(fmt.Sprintf("swiftrt.StringInitCString: %T is not a C string", b))
	}
	if i := bytes.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	return StringInitDecodingAs(raw, nil)
}

// UnicodeScalarProperties is Swift `Unicode.Scalar.Properties`.
type UnicodeScalarProperties struct{ r rune }

// ScalarProperties is Swift `Unicode.Scalar.properties`.
func ScalarProperties(r rune) UnicodeScalarProperties { return UnicodeScalarProperties{r} }

// PropertiesIsAlphabetic is Swift `Unicode.Scalar.Properties.isAlphabetic`.
func PropertiesIsAlphabetic(p UnicodeScalarProperties) bool {
	return unicode.IsLetter(p.r) || unicode.Is(unicode.Other_Alphabetic, p.r) || unicode.Is(unicode.Nl, p.r)
}

// PropertiesIsWhitespace is Swift `Unicode.Scalar.Properties.isWhitespace`.
func PropertiesIsWhitespace(p UnicodeScalarProperties) bool {
	return unicode.Is(unicode.White_Space, p.r)
}

// PropertiesIsEmoji is Swift `Unicode.Scalar.Properties.isEmoji`.
func PropertiesIsEmoji(p UnicodeScalarProperties) bool {
	return p.r >= 0x1F000 || (p.r >= 0x2600 && p.r <= 0x27BF)
}

// ScalarIsASCII is Swift `Unicode.Scalar.isASCII`.
func ScalarIsASCII(r rune) bool { return r < 0x80 }

// ScalarInitUInt32 is Swift `Unicode.Scalar(UInt32)`.
func ScalarInitUInt32[T Integer](v T) *rune {
	r := rune(v)
	if int64(v) < 0 || int64(v) > utf8.MaxRune || (r >= 0xD800 && r <= 0xDFFF) {
		return nil
	}
	return &r
}

// CharacterInitScalar is Swift `Character(Unicode.Scalar)`.
func CharacterInitScalar(r rune) Character { return string(r) }

// CharacterInitString is Swift `Character(String)`; it traps unless s is one grapheme cluster.
func CharacterInitString(s string) Character {
	if graphemeCount(s) != 1 {
		Trap("Can't form a Character from a String containing more than one extended grapheme cluster")
	}
	return s
}

// CharacterUnicodeScalars is Swift `Character.unicodeScalars`.
func CharacterUnicodeScalars(c Character) []rune { return []rune(c) }

// CharacterAsciiValue is Swift `Character.asciiValue`.
func CharacterAsciiValue(c Character) *uint8 {
	if c == "\r\n" {
		v := uint8('\n')
		return &v
	}
	if len(c) != 1 || c[0] >= 0x80 {
		return nil
	}
	v := c[0]
	return &v
}

// CharacterWholeNumberValue is Swift `Character.wholeNumberValue`.
func CharacterWholeNumberValue(c Character) *int {
	r := firstRune(c)
	if utf8.RuneCountInString(c) != 1 || !unicode.IsDigit(r) {
		return nil
	}
	v := int(r - '0')
	if r > 0x7F {
		v = int(unicode.ToLower(r)) % 10
	}
	return &v
}

// CharacterIsHexDigit is Swift `Character.isHexDigit`.
func CharacterIsHexDigit(c Character) bool {
	return len(c) == 1 && strings.ContainsRune("0123456789abcdefABCDEF", rune(c[0]))
}

// CharacterIsSymbol is Swift `Character.isSymbol`.
func CharacterIsSymbol(c Character) bool { return unicode.IsSymbol(firstRune(c)) }

// CharacterIsNewline is Swift `Character.isNewline`.
func CharacterIsNewline(c Character) bool { return c == "\r\n" || CharacterSetNewlines(firstRune(c)) }

// CharacterIsCased is Swift `Character.isCased`.
func CharacterIsCased(c Character) bool {
	r := firstRune(c)
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
}

// SubstringFilter is `Substring.filter`, which returns a String.
func SubstringFilter(s Substring, f func(Character) bool) string { return StringFilter(s.String(), f) }

// StringReversed is `String.reversed()`.
func StringReversed(s string) []Character { return SliceReversed(StringChars(s)) }

// ReversedCollectionDropWhile is `reversed().drop(while:)`.
func ReversedCollectionDropWhile[T any](xs []T, f func(T) bool) []T { return SliceDropWhile(xs, f) }

// StringInitReversedCollection is `String(reversedCharacters)`.
func StringInitReversedCollection(cs []Character) string { return StringInitSlice(cs) }

// StringInitIntRadixUppercase is `String(n, radix:uppercase:)`.
func StringInitIntRadixUppercase[T Integer](n T, radix int, upper bool) string {
	s := strconv.FormatInt(int64(n), radix)
	if upper {
		s = strings.ToUpper(s)
	}
	return s
}

// StringInitIntRadix is `String(n, radix:)`.
func StringInitIntRadix[T Integer](n T, radix int) string { return strconv.FormatInt(int64(n), radix) }

// CharacterIsWholeNumber reports `Character.wholeNumberValue != nil`.
func CharacterIsWholeNumber(c Character) bool { return CharacterWholeNumberValue(c) != nil }

// Print is Swift `print(_:)`.
func Print(items []any) { PrintSeparatorTerminator(items, " ", "\n") }

// PrintSeparator is Swift `print(_:separator:)`.
func PrintSeparator(items []any, separator string) {
	PrintSeparatorTerminator(items, separator, "\n")
}

// PrintTerminator is Swift `print(_:terminator:)`.
func PrintTerminator(items []any, terminator string) {
	PrintSeparatorTerminator(items, " ", terminator)
}

// PrintSeparatorTerminator is Swift `print(_:separator:terminator:)`.
func PrintSeparatorTerminator(items []any, separator, terminator string) {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = Str(it)
	}
	fmt.Print(strings.Join(parts, separator) + terminator)
}

// CFCeil is C `ceil`.
func CFCeil(x float64) float64 { return math.Ceil(x) }

// CFFloor is C `floor`.
func CFFloor(x float64) float64 { return math.Floor(x) }

// CFPow is C `pow`.
func CFPow(x, y float64) float64 { return math.Pow(x, y) }

// CFLog10 is C `log10`.
func CFLog10(x float64) float64 { return math.Log10(x) }

// CFSqrt is C `sqrt`.
func CFSqrt(x float64) float64 { return math.Sqrt(x) }

// splitWhere is Swift `split(maxSplits:omittingEmptySubsequences:whereSeparator:)` over the
// Characters of s: after maxSplits splits the rest of s is one last piece.
func splitWhere(s Substring, maxSplits int, omit bool, f func(Character) bool) []Substring {
	if maxSplits < 0 {
		Trap("Must take zero or more splits")
	}
	out := []Substring{}
	start := s.Lo
	for i := s.Lo; i < s.Hi; {
		n := graphemeLen(s.Base[i:s.Hi])
		if len(out) < maxSplits && f(s.Base[i:i+n]) {
			if !(omit && start == i) {
				out = append(out, Substring{s.Base, start, i})
			}
			start = i + n
		}
		i += n
	}
	if !(omit && start == s.Hi) {
		out = append(out, Substring{s.Base, start, s.Hi})
	}
	return out
}

// SubstringSplitWhereSeparator is Swift `Substring.split(whereSeparator:)`.
func SubstringSplitWhereSeparator(s Substring, f func(Character) bool) []Substring {
	return splitWhere(s, IntMax, true, f)
}

// StringSplitMaxSplitsWhereSeparator is Swift `String.split(maxSplits:whereSeparator:)`.
func StringSplitMaxSplitsWhereSeparator(s string, maxSplits int, f func(Character) bool) []Substring {
	return splitWhere(sub(s), maxSplits, true, f)
}

// StringSplitOmittingEmptySubsequencesWhereSeparator is Swift
// `String.split(omittingEmptySubsequences:whereSeparator:)`.
func StringSplitOmittingEmptySubsequencesWhereSeparator(s string, omit bool, f func(Character) bool) []Substring {
	return splitWhere(sub(s), IntMax, omit, f)
}

// StringSplitMaxSplitsOmittingEmptySubsequencesWhereSeparator is Swift
// `String.split(maxSplits:omittingEmptySubsequences:whereSeparator:)`.
func StringSplitMaxSplitsOmittingEmptySubsequencesWhereSeparator(s string, maxSplits int, omit bool, f func(Character) bool) []Substring {
	return splitWhere(sub(s), maxSplits, omit, f)
}

// SubstringSplitSeparatorMaxSplits is Swift `Substring.split(separator:maxSplits:)`.
func SubstringSplitSeparatorMaxSplits(s Substring, sep Character, maxSplits int) []Substring {
	if maxSplits < 0 {
		Trap("Must take zero or more splits")
	}
	return SubstringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s, sep, maxSplits, true)
}

// SubstringComponentsSeparatedBy is Foundation `components(separatedBy:)` on a Substring.
func SubstringComponentsSeparatedBy[S ~string | CharacterSet](s Substring, sep S) []string {
	return StringComponentsSeparatedBy(s.String(), sep)
}

// StringRemoveFirst is Swift `String.removeFirst()`: it removes and returns the first Character,
// trapping on an empty string.
func StringRemoveFirst(s *string) Character {
	if *s == "" {
		Trap("Can't remove first element from an empty collection")
	}
	n := graphemeLen(*s)
	c := (*s)[:n]
	*s = (*s)[n:]
	return c
}

// StringRemoveFirstInt is Swift `String.removeFirst(_:)`: it removes the first n Characters,
// trapping when n is negative or more than the string holds.
func StringRemoveFirstInt(s *string, n int) {
	if n < 0 {
		Trap("Number of elements to remove should be non-negative")
	}
	i := 0
	for range n {
		if i >= len(*s) {
			Trap("Can't remove more items from a collection than it has")
		}
		i += graphemeLen((*s)[i:])
	}
	*s = (*s)[i:]
}

// StringRemoveLastInt is Swift `String.removeLast(_:)`: it removes the last n Characters, trapping
// when n is negative or more than the string holds.
func StringRemoveLastInt(s *string, n int) {
	if n < 0 {
		Trap("Number of elements to remove should be non-negative")
	}
	if n == 0 {
		return
	}
	chars := StringChars(*s)
	if n > len(chars) {
		Trap("Can't remove more items from a collection than it contains")
	}
	end := len(*s)
	for _, c := range chars[len(chars)-n:] {
		end -= len(c)
	}
	*s = (*s)[:end]
}

// StringInsertContentsOfAt is Swift `String.insert(contentsOf:at:)`.
func StringInsertContentsOfAt(s *string, inserted string, at int) {
	if at < 0 || at > len(*s) {
		Trap("String index is out of bounds")
	}
	*s = (*s)[:at] + inserted + (*s)[at:]
}

// StringLastIndexWhere is Swift `String.lastIndex(where:)`.
func StringLastIndexWhere(s string, f func(Character) bool) *int {
	for end := len(s); end > 0; {
		c, start := lastGrapheme(s[:end])
		if f(c) {
			return &start
		}
		end = start
	}
	return nil
}

// StringFirstRangeOf is Swift `Collection.firstRange(of:)` on a String: the range of the first run
// of Characters equal to needle's, comparing each by canonical equivalence. An empty needle
// matches at the start.
func StringFirstRangeOf[S ~string | Substring](s string, needleText S) *Range[int] {
	needle := stringOf(needleText)
	want := StringChars(needle)
	chars := StringChars(s)
	for i, at := 0, 0; i+len(want) <= len(chars); i++ {
		if charsEquivalent(chars[i:i+len(want)], want) {
			end := at
			for _, c := range chars[i : i+len(want)] {
				end += len(c)
			}
			return &Range[int]{at, end}
		}
		at += len(chars[i])
	}
	return nil
}

// StringReplacingOccurrencesOfWithRange is Foundation
// `replacingOccurrences(of:with:options:range:)` with no options: it replaces the occurrences
// that lie inside the range, or everywhere when the range is nil.
func StringReplacingOccurrencesOfWithRange(s, target, repl string, r *Range[int]) string {
	if r == nil {
		return StringReplacingOccurrencesOfWith(s, target, repl)
	}
	if r.Lo < 0 || r.Hi > len(s) || r.Lo > r.Hi {
		Trap("String index is out of bounds")
	}
	return s[:r.Lo] + StringReplacingOccurrencesOfWith(s[r.Lo:r.Hi], target, repl) + s[r.Hi:]
}

// StringLocalizedCaseInsensitiveContains is Foundation `localizedCaseInsensitiveContains(_:)`:
// whether other occurs in s ignoring case. An empty other is never contained.
func StringLocalizedCaseInsensitiveContains(s, other string) bool {
	if other == "" {
		return false
	}
	_, _, ok := foldIndex(s, other, 0, len(s), false)
	return ok
}

// StringDebugDescription is Swift `String.debugDescription`: the string as a quoted literal.
func StringDebugDescription(s string) string { return swiftQuote(s) }

// SubstringDebugDescription is Swift `Substring.debugDescription`.
func SubstringDebugDescription(s Substring) string { return swiftQuote(s.String()) }

var stringHashSeed = maphash.MakeSeed()

// StringHashValue is Swift `String.hashValue`: equal for canonically equivalent strings, and
// seeded per process, so it differs from run to run as Swift's does.
func StringHashValue(s string) int {
	return int(maphash.String(stringHashSeed, StringDecomposedStringWithCanonicalMapping(s)))
}

// ScalarInitInt is Swift `Unicode.Scalar(_: UInt8)`: the scalar with that value.
func ScalarInitInt(v uint8) rune { return rune(v) }

// StringInitScalar is Swift `String(_: Unicode.Scalar)`.
func StringInitScalar(r rune) string { return string(r) }

// StringScalarAt is `s.unicodeScalars[i]` for a String index i: the scalar that starts at that
// byte offset.
func StringScalarAt(s string, i int) rune {
	if i < 0 || i >= len(s) {
		Trap("String index is out of bounds")
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

// StringScalarIndexAfter is `s.unicodeScalars.index(after: i)`: the index of the next scalar.
func StringScalarIndexAfter(s string, i int) int {
	if i < 0 || i >= len(s) {
		Trap("String index is out of bounds")
	}
	_, n := utf8.DecodeRuneInString(s[i:])
	return i + n
}

// StringScalarIndicesFrom is `s.unicodeScalars[r].indices` for `r = i...`: the String index of
// each scalar from i to the end.
func StringScalarIndicesFrom(s string, r PartialRangeFrom[int]) []int {
	if r.Lo < 0 || r.Lo > len(s) {
		Trap("String index is out of bounds")
	}
	out := []int{}
	for i := r.Lo; i < len(s); {
		out = append(out, i)
		_, n := utf8.DecodeRuneInString(s[i:])
		i += n
	}
	return out
}

// StringScalarsFrom is `s.unicodeScalars[i...]` as the scalars themselves.
func StringScalarsFrom(s string, r PartialRangeFrom[int]) []rune {
	if r.Lo < 0 || r.Lo > len(s) {
		Trap("String index is out of bounds")
	}
	return []rune(s[r.Lo:])
}
