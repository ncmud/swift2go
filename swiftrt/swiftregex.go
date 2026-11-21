package swiftrt

import (
	"reflect"
	"regexp"
)

// SwiftRegex is _StringProcessing `Regex<Output>`. Output is Substring for a regex without captures,
// or a tuple struct whose F0 is the whole match and whose later fields are the captures (Substring,
// or *Substring for a capture that may not participate). Matching runs on RE2 after the same ICU
// class widening NSRegularExpression gets, which is scalar-based: grapheme-cluster semantics (the
// Swift default) are approximated by Unicode scalar matching.
type SwiftRegex[O any] struct {
	pattern    string
	ignoreCase bool
	re         *regexp.Regexp
}

// RegexMatch is `Regex<Output>.Match`.
type RegexMatch[O any] struct {
	Output O
	Range  Range[int]
}

// RegexSemanticLevel is `RegexSemanticLevel`.
type RegexSemanticLevel int

const (
	// RegexSemanticLevelGraphemeCluster is `.graphemeCluster`.
	RegexSemanticLevelGraphemeCluster RegexSemanticLevel = iota
	// RegexSemanticLevelUnicodeScalar is `.unicodeScalar`.
	RegexSemanticLevelUnicodeScalar
)

func compileSwiftRegex[O any](pattern string, ignoreCase bool) SwiftRegex[O] {
	re, err := icuRegexCompile(pattern, ignoreCase)
	if err != nil {
		panic("swiftrt: regex literal not supported by RE2: " + pattern + ": " + err.Error())
	}
	return SwiftRegex[O]{pattern: pattern, ignoreCase: ignoreCase, re: re}
}

// RegexLiteral is a regex literal (`/…/` or `#/…/#`).
func RegexLiteral[O any](pattern string) SwiftRegex[O] { return compileSwiftRegex[O](pattern, false) }

// RegexIgnoresCase is `Regex.ignoresCase(_:)`.
func RegexIgnoresCase[O any](r SwiftRegex[O], on ...bool) SwiftRegex[O] {
	ignore := len(on) == 0 || on[0]
	return compileSwiftRegex[O](r.pattern, ignore)
}

// RegexMatchingSemantics is `Regex.matchingSemantics(_:)`. Matching is always scalar-based here.
func RegexMatchingSemantics[O any](r SwiftRegex[O], _ RegexSemanticLevel) SwiftRegex[O] { return r }

// MatchOutput is `Regex.Match.output`.
func MatchOutput[O any](m RegexMatch[O]) O { return m.Output }

// MatchRange is `Regex.Match.range`.
func MatchRange[O any](m RegexMatch[O]) Range[int] { return m.Range }

// MatchSubscriptDynamicMemberKeyPath is `match.1`: a member of the match's output.
func MatchSubscriptDynamicMemberKeyPath[O, V any](m RegexMatch[O], get func(O) V) V {
	return get(m.Output)
}

func regexMatchAt[O any](s string, loc []int) RegexMatch[O] {
	var out O
	v := reflect.ValueOf(&out).Elem()
	if v.Type() == substringType {
		v.Set(reflect.ValueOf(Substring{s, loc[0], loc[1]}))
	} else if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField() && 2*i+1 < len(loc); i++ {
			lo, hi := loc[2*i], loc[2*i+1]
			f := v.Field(i)
			switch {
			case f.Type() == substringType && lo >= 0:
				f.Set(reflect.ValueOf(Substring{s, lo, hi}))
			case f.Type() == reflect.PointerTo(substringType) && lo >= 0:
				f.Set(reflect.ValueOf(&Substring{s, lo, hi}))
			}
		}
	}
	return RegexMatch[O]{Output: out, Range: Range[int]{Lo: loc[0], Hi: loc[1]}}
}

// StringFirstMatchOf is `String.firstMatch(of:)`.
func StringFirstMatchOf[O any](s string, r SwiftRegex[O]) *RegexMatch[O] {
	loc := r.re.FindStringSubmatchIndex(s)
	if loc == nil {
		return nil
	}
	m := regexMatchAt[O](s, loc)
	return &m
}

// StringWholeMatchOf is `String.wholeMatch(of:)`: a match of the entire string.
func StringWholeMatchOf[O any](s string, r SwiftRegex[O]) *RegexMatch[O] {
	whole := compileSwiftRegex[O](`\A(?:`+r.pattern+`)\z`, r.ignoreCase)
	return StringFirstMatchOf(s, whole)
}

// StringMatchesOf is `String.matches(of:)`.
func StringMatchesOf[O any](s string, r SwiftRegex[O]) []RegexMatch[O] {
	var out []RegexMatch[O]
	for _, loc := range r.re.FindAllStringSubmatchIndex(s, -1) {
		out = append(out, regexMatchAt[O](s, loc))
	}
	return out
}

// StringContainsRegex is `String.contains(_:)` with a regex.
func StringContainsRegex[O any](s string, r SwiftRegex[O]) bool { return r.re.MatchString(s) }

// StringReplacingWith is `String.replacing(_:with:)`, where with is the replacement String or a
// closure from each match to its replacement. Every non-overlapping match is replaced, left to right.
func StringReplacingWith[O any](s string, r SwiftRegex[O], with any) string {
	var b []byte
	last := 0
	for _, loc := range r.re.FindAllStringSubmatchIndex(s, -1) {
		b = append(b, s[last:loc[0]]...)
		switch w := with.(type) {
		case string:
			b = append(b, w...)
		case func(RegexMatch[O]) string:
			b = append(b, w(regexMatchAt[O](s, loc))...)
		default:
			b = append(b, Str(reflect.ValueOf(with).Call([]reflect.Value{reflect.ValueOf(regexMatchAt[O](s, loc))})[0].Interface())...)
		}
		last = loc[1]
	}
	return string(append(b, s[last:]...))
}
