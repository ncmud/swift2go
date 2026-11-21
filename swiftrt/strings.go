package swiftrt

import (
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Character is a Swift Character: one extended grapheme cluster.
type Character = string

// Substring is a Swift Substring: a view into Base whose indices are Base's byte offsets.
type Substring struct {
	Base   string
	Lo, Hi int
}

func (s Substring) String() string { return s.Base[s.Lo:s.Hi] }

// StringChars splits s into grapheme clusters, Swift's Characters.
func StringChars(s string) []Character {
	out := make([]Character, 0, len(s))
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		out = append(out, g.Str())
	}
	return out
}

func graphemeLen(s string) int {
	_, rest, _, _ := uniseg.FirstGraphemeClusterInString(s, -1)
	return len(s) - len(rest)
}

func lastGrapheme(s string) (Character, int) {
	if s == "" {
		return "", 0
	}
	chars := StringChars(s)
	last := chars[len(chars)-1]
	return last, len(s) - len(last)
}

// StringCount is String.count (grapheme clusters).
func StringCount(s string) int { return uniseg.GraphemeClusterCount(s) }

// StringIsEmpty is String.isEmpty.
func StringIsEmpty(s string) bool { return s == "" }

// StringStartIndex is String.startIndex.
func StringStartIndex(s string) int { return 0 }

// StringEndIndex is String.endIndex.
func StringEndIndex(s string) int { return len(s) }

// StringIndexAfter is String.index(after:).
func StringIndexAfter(s string, i int) int {
	if i >= len(s) {
		Trap("String index is out of bounds")
	}
	return i + graphemeLen(s[i:])
}

// StringIndexBefore is String.index(before:).
func StringIndexBefore(s string, i int) int {
	if i <= 0 {
		Trap("String index is out of bounds")
	}
	_, off := lastGrapheme(s[:i])
	return off
}

// StringSubscriptInt is String[index].
func StringSubscriptInt(s string, i int) Character {
	if i < 0 || i >= len(s) {
		Trap("String index is out of bounds")
	}
	return s[i : i+graphemeLen(s[i:])]
}

// StringSubscriptPartialRangeFrom is String[i...].
func StringSubscriptPartialRangeFrom(s string, r PartialRangeFrom[int]) Substring {
	return Substring{s, r.Lo, len(s)}
}

// StringSubscriptRange is String[a..<b].
func StringSubscriptRange(s string, r Range[int]) Substring { return Substring{s, r.Lo, r.Hi} }

// SubstringIndices is Substring.indices: the base-string offsets of each Character.
func SubstringIndices(s Substring) []int {
	var out []int
	for i := s.Lo; i < s.Hi; {
		out = append(out, i)
		i += graphemeLen(s.Base[i:s.Hi])
	}
	return out
}

// StringIndices is String.indices.
func StringIndices(s string) []int { return SubstringIndices(Substring{s, 0, len(s)}) }

// StringContains is String.contains(_:) for a Character.
func StringContains(s string, c Character) bool {
	if graphemeLen(c) != len(c) {
		return strings.Contains(s, c)
	}
	for _, ch := range StringChars(s) {
		if ch == c {
			return true
		}
	}
	return false
}

// StringContainsString is the Foundation/StringProtocol String.contains(_:) for a substring.
func StringContainsString(s, sub string) bool { return strings.Contains(s, sub) }

// StringHasPrefix is String.hasPrefix(_:).
func StringHasPrefix[P ~string | Substring](s string, p P) bool {
	return strings.HasPrefix(s, stringOf(p))
}

// stringOf is a String or Substring argument as a Go string.
func stringOf[P ~string | Substring](p P) string {
	if sub, ok := any(p).(Substring); ok {
		return sub.String()
	}
	return reflect.ValueOf(p).String()
}

// StringHasSuffix is String.hasSuffix(_:).
func StringHasSuffix(s, p string) bool { return strings.HasSuffix(s, p) }

// StringUppercased is String.uppercased().
func StringUppercased(s string) string { return strings.ToUpper(s) }

// StringLowercased is String.lowercased().
func StringLowercased(s string) string { return strings.ToLower(s) }

// StringReserveCapacity is String.reserveCapacity(_:); a no-op.
func StringReserveCapacityInt(s *string, n int) {}

// StringAppendCharacter is String.append(_: Character).
func StringAppendCharacter(s *string, c Character) { *s += c }

// StringAppendString is String.append(_: String).
func StringAppendString(s *string, t string) { *s += t }

// StringAppendContentsOf is String.append(contentsOf:).
func StringAppendContentsOf[T []Character | string | Substring](s *string, cs T) {
	switch v := any(cs).(type) {
	case string:
		*s += v
	case Substring:
		*s += v.String()
	case []Character:
		*s += strings.Join(v, "")
	}
}

// StringLast is String.last.
func StringLast(s string) *Character {
	if s == "" {
		return nil
	}
	c, _ := lastGrapheme(s)
	return &c
}

// StringFirst is String.first.
func StringFirst(s string) *Character {
	if s == "" {
		return nil
	}
	c := s[:graphemeLen(s)]
	return &c
}

// StringRemoveLast is String.removeLast().
func StringRemoveLast(s *string) Character {
	if *s == "" {
		Trap("Can't remove last element from an empty collection")
	}
	c, off := lastGrapheme(*s)
	*s = (*s)[:off]
	return c
}

// StringDropFirst is String.dropFirst(_:).
func StringDropFirst(s string, ns ...int) Substring {
	n := 1
	if len(ns) > 0 {
		n = ns[0]
	}
	i := 0
	for k := 0; k < n && i < len(s); k++ {
		i += graphemeLen(s[i:])
	}
	return Substring{s, i, len(s)}
}

// StringDropLast is String.dropLast(_:).
func StringDropLast(s string, ns ...int) Substring {
	n := 1
	if len(ns) > 0 {
		n = ns[0]
	}
	end := len(s)
	for k := 0; k < n && end > 0; k++ {
		_, end = lastGrapheme(s[:end])
	}
	return Substring{s, 0, end}
}

// StringLastIndexOf is String.lastIndex(of:).
func StringLastIndexOf(s string, c Character) *int {
	idx := StringIndices(s)
	for k := len(idx) - 1; k >= 0; k-- {
		if StringSubscriptInt(s, idx[k]) == c {
			v := idx[k]
			return &v
		}
	}
	return nil
}

// StringFirstIndexOf is String.firstIndex(of:).
func StringFirstIndexOf(s string, c Character) *int {
	for _, i := range StringIndices(s) {
		if StringSubscriptInt(s, i) == c {
			v := i
			return &v
		}
	}
	return nil
}

// StringSplitSeparatorOmittingEmptySubsequences is String.split(separator:omittingEmptySubsequences:).
func StringSplitSeparatorOmittingEmptySubsequences(s string, sep Character, omit bool) []Substring {
	return StringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s, sep, int(^uint(0)>>1), omit)
}

// StringSplitSeparator is String.split(separator:).
func StringSplitSeparator(s string, sep Character) []Substring {
	return StringSplitSeparatorOmittingEmptySubsequences(s, sep, true)
}

// StringSplitSeparatorMaxSplitsOmittingEmptySubsequences is the full String.split.
func StringSplitSeparatorMaxSplitsOmittingEmptySubsequences(s string, sep Character, maxSplits int, omit bool) []Substring {
	var out []Substring
	start := 0
	splits := 0
	for _, i := range StringIndices(s) {
		if splits >= maxSplits {
			break
		}
		c := StringSubscriptInt(s, i)
		if c == sep {
			if !(omit && i == start) {
				out = append(out, Substring{s, start, i})
				splits++
			}
			start = i + len(c)
		}
	}
	if !(omit && start == len(s)) {
		out = append(out, Substring{s, start, len(s)})
	}
	return out
}

// StringUnicodeScalars is String.unicodeScalars.
func StringUnicodeScalars(s string) []rune { return []rune(s) }

// StringUtf8 is String.utf8.
func StringUtf8(s string) []byte { return []byte(s) }

// StringInitCharacter is String(_: Character).
func StringInitCharacter(c Character) string { return c }

// StringInitSubstring is String(_: Substring).
func StringInitSubstring(s Substring) string { return s.String() }

// StringInitSlice is String(_: [Character]) or String(String.UnicodeScalarView).
func StringInitSlice[T Character | rune](cs []T) string {
	var b strings.Builder
	for _, c := range cs {
		switch v := any(c).(type) {
		case string:
			b.WriteString(v)
		case rune:
			b.WriteRune(v)
		}
	}
	return b.String()
}

// StringInitRepeatingCount is String(repeating:count:).
func StringInitRepeatingCount[T string](s T, n int) string {
	if n < 0 {
		Trap("Negative count not allowed")
	}
	return strings.Repeat(string(s), n)
}

// StringInitDescribing is String(describing:).
func StringInitDescribing(v any) string { return Str(v) }

// StringInitInt is String(_: Int).
func StringInitInt[T Integer](v T) string { return Str(int(v)) }

// StringInitString is String(_: String).
func StringInitString(s string) string { return s }

// SubstringCount is Substring.count.
func SubstringCount(s Substring) int { return StringCount(s.String()) }

// SubstringIsEmpty is Substring.isEmpty.
func SubstringIsEmpty(s Substring) bool { return s.Lo >= s.Hi }

// CharacterIsASCII is Character.isASCII.
func CharacterIsASCII(c Character) bool { return len(c) == 1 && c[0] < 0x80 || c == "\r\n" }

func firstRune(c Character) rune { r, _ := utf8.DecodeRuneInString(c); return r }

// CharacterIsLetter is Character.isLetter.
func CharacterIsLetter(c Character) bool { return unicode.IsLetter(firstRune(c)) }

// CharacterIsNumber is Character.isNumber.
func CharacterIsNumber(c Character) bool { return unicode.IsNumber(firstRune(c)) }

// CharacterIsWhitespace is Character.isWhitespace.
func CharacterIsWhitespace(c Character) bool { return unicode.IsSpace(firstRune(c)) || c == "\r\n" }

// CharacterIsPunctuation is Character.isPunctuation.
func CharacterIsPunctuation(c Character) bool { return unicode.IsPunct(firstRune(c)) }

// CharacterIsUppercase is Character.isUppercase.
func CharacterIsUppercase(c Character) bool { return unicode.IsUpper(firstRune(c)) }

// CharacterIsLowercase is Character.isLowercase.
func CharacterIsLowercase(c Character) bool { return unicode.IsLower(firstRune(c)) }

// ScalarValue is Unicode.Scalar.value.
func ScalarValue(r rune) uint32 { return uint32(r) }

// StringHasPrefixString is String.hasPrefix(_:).
func StringHasPrefixString(s, p string) bool { return strings.HasPrefix(s, p) }

// StringHasSuffixString is String.hasSuffix(_:).
func StringHasSuffixString(s, p string) bool { return strings.HasSuffix(s, p) }

// StringDropFirstInt is String.dropFirst(_:).
func StringDropFirstInt(s string, n int) Substring { return StringDropFirst(s, n) }

// StringDropLastInt is String.dropLast(_:).
func StringDropLastInt(s string, n int) Substring { return StringDropLast(s, n) }

// SubstringContainsString is StringProtocol.contains(_:) on a Substring.
func SubstringContainsString(s Substring, sub string) bool { return strings.Contains(s.String(), sub) }

// StringAllSatisfy is String.allSatisfy(_:) over Characters.
func StringAllSatisfy(s string, f func(Character) bool) bool {
	return SliceAllSatisfy(StringChars(s), f)
}

// StringMap is String.map(_:) over Characters.
func StringMap[U any](s string, f func(Character) U) []U { return SliceMap(StringChars(s), f) }

// StringFilter is String.filter(_:).
func StringFilter(s string, f func(Character) bool) string {
	return strings.Join(SliceFilter(StringChars(s), f), "")
}
