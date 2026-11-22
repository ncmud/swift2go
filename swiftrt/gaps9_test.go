package swiftrt

import (
	"reflect"
	"testing"
	"time"
)

func subs(xs []Substring) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.String()
	}
	return out
}

func TestSplitWhereSeparator(t *testing.T) {
	space := func(c Character) bool { return c == " " || c == "\t" }
	cases := []struct {
		name string
		got  []Substring
		want []string
	}{
		{"substring", SubstringSplitWhereSeparator(Substring{"x  a b ", 1, 7}, space), []string{"a", "b"}},
		{"maxSplits", StringSplitMaxSplitsWhereSeparator(" 3  the rest  ", 1, space), []string{"3", " the rest  "}},
		{"maxSplits zero", StringSplitMaxSplitsWhereSeparator("a b", 0, space), []string{"a b"}},
		{"keeping empties", StringSplitOmittingEmptySubsequencesWhereSeparator("a  b ", false, space), []string{"a", "", "b", ""}},
		{"keeping empties of empty", StringSplitOmittingEmptySubsequencesWhereSeparator("", false, space), []string{""}},
		{"omitting empties of empty", StringSplitOmittingEmptySubsequencesWhereSeparator("", true, space), []string{}},
		{"both", StringSplitMaxSplitsOmittingEmptySubsequencesWhereSeparator("a  b c", 1, false, space), []string{"a", " b c"}},
		{"both omitting", StringSplitMaxSplitsOmittingEmptySubsequencesWhereSeparator("  a  b c", 1, true, space), []string{"a", " b c"}},
		{"separator maxSplits", SubstringSplitSeparatorMaxSplits(sub("1 2 3 4 5"), " ", 3), []string{"1", "2", "3", "4 5"}},
	}
	for _, c := range cases {
		if got := subs(c.got); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := SubstringComponentsSeparatedBy(Substring{"{x}a\n\nb", 3, 7}, "\n"); !reflect.DeepEqual(got, []string{"a", "", "b"}) {
		t.Errorf("components = %q", got)
	}
}

func TestStringRemoveAndInsert(t *testing.T) {
	s := "éa🇫🇷z"
	if c := StringRemoveFirst(&s); c != "é" || s != "a🇫🇷z" {
		t.Errorf("removeFirst() = %q leaving %q", c, s)
	}
	StringRemoveLastInt(&s, 2)
	if s != "a" {
		t.Errorf("removeLast(2) left %q", s)
	}
	s = "🇫🇷bc"
	StringRemoveFirstInt(&s, 2)
	if s != "c" {
		t.Errorf("removeFirst(2) left %q", s)
	}
	StringRemoveFirstInt(&s, 0)
	StringRemoveLastInt(&s, 0)
	StringInsertContentsOfAt(&s, "ab", 0)
	StringInsertContentsOfAt(&s, "!", len(s))
	if s != "abc!" {
		t.Errorf("insert(contentsOf:at:) gave %q", s)
	}
	for name, f := range map[string]func(){
		"removeFirst on empty": func() { e := ""; StringRemoveFirst(&e) },
		"removeFirst too many": func() { e := "ab"; StringRemoveFirstInt(&e, 3) },
		"removeLast too many":  func() { e := "ab"; StringRemoveLastInt(&e, 3) },
		"removeLast negative":  func() { e := "ab"; StringRemoveLastInt(&e, -1) },
	} {
		if !panics(f) {
			t.Errorf("%s did not trap", name)
		}
	}
}

func panics(f func()) (did bool) {
	defer func() { did = recover() != nil }()
	f()
	return false
}

func TestStringSearchHelpers(t *testing.T) {
	newline := func(c Character) bool { return c == "\n" || c == "\r\n" }
	if got := StringLastIndexWhere("a\nb\r\nc", newline); got == nil || *got != 3 {
		t.Errorf("lastIndex(where:) = %v, want 3", got)
	}
	if got := StringLastIndexWhere("abc", newline); got != nil {
		t.Errorf("lastIndex(where:) = %v, want nil", *got)
	}
	// "é" precomposed in the haystack, decomposed in the needle.
	if got := StringFirstRangeOf("café au lait", "fé "); got == nil || *got != (Range[int]{2, 6}) {
		t.Errorf("firstRange(of:) = %v, want 2..<6", got)
	}
	// "e" alone does not match the first Character of "é".
	if got := StringFirstRangeOf("ée", "e"); got == nil || *got != (Range[int]{3, 4}) {
		t.Errorf("firstRange(of:) across a cluster = %v, want 3..<4", got)
	}
	if got := StringFirstRangeOf("abc", "x"); got != nil {
		t.Errorf("firstRange(of:) = %v, want nil", *got)
	}
	if got := StringFirstRangeOf("abc", ""); got == nil || *got != (Range[int]{0, 0}) {
		t.Errorf("firstRange(of: \"\") = %v, want 0..<0", got)
	}
	if got := StringReplacingOccurrencesOfWithRange("aXbXc", "X", "-", StringRangeOf("aXbXc", "X")); got != "a-bXc" {
		t.Errorf("replacing in range = %q", got)
	}
	if got := StringReplacingOccurrencesOfWithRange("aXbXc", "X", "-", nil); got != "a-b-c" {
		t.Errorf("replacing with a nil range = %q", got)
	}
	if !StringLocalizedCaseInsensitiveContains("America/New_York", "new_y") || StringLocalizedCaseInsensitiveContains("Europe/Paris", "") || StringLocalizedCaseInsensitiveContains("Europe/Paris", "rome") {
		t.Error("localizedCaseInsensitiveContains is wrong")
	}
	if got, want := StringDebugDescription("it's \"q\"\n\x01é"), `"it\'s \"q\"\n\u{01}é"`; got != want {
		t.Errorf("debugDescription = %s, want %s", got, want)
	}
	if got := SubstringDebugDescription(Substring{"abcdef", 1, 3}); got != `"bc"` {
		t.Errorf("Substring.debugDescription = %s", got)
	}
	if StringHashValue("café") != StringHashValue("café") || StringHashValue("a") == StringHashValue("b") {
		t.Error("hashValue does not follow String equality")
	}
	if got := StringInitScalar(ScalarInitInt(0xe9)); got != "é" {
		t.Errorf("String(Unicode.Scalar(0xE9)) = %q", got)
	}
}

func TestSetAndSliceGaps(t *testing.T) {
	set := map[int]struct{}{1: {}, 2: {}, 3: {}}
	before := set
	SetFormSymmetricDifference(&set, []int{3, 4, 4})
	if !reflect.DeepEqual(set, map[int]struct{}{1: {}, 2: {}, 4: {}}) || len(before) != 3 {
		t.Errorf("formSymmetricDifference gave %v, the original now %v", set, before)
	}
	SetFormSymmetricDifference(&set, map[int]struct{}{1: {}})
	if !reflect.DeepEqual(set, map[int]struct{}{2: {}, 4: {}}) {
		t.Errorf("formSymmetricDifference with a set gave %v", set)
	}
	if !SetIsSupersetOf(set, []int{2, 2}) || !SetIsSupersetOf(set, map[int]struct{}{}) || SetIsSupersetOf(set, map[int]struct{}{2: {}, 5: {}}) {
		t.Error("isSuperset(of:) is wrong")
	}
	if got := SetInitString[map[Character]struct{}]("abca"); len(got) != 3 {
		t.Errorf("Set(String) = %v", got)
	}
	if got := SetInitClosedRange[map[int]struct{}](ClosedRange[int]{1180, 1199}); len(got) != 20 {
		t.Errorf("Set(1180...1199) has %d elements", len(got))
	}

	queue := []string{"b"}
	SlicePrepend(&queue, "a")
	if !reflect.DeepEqual(queue, []string{"a", "b"}) {
		t.Errorf("prepend gave %v", queue)
	}
	lines := []int{0, 1, 2, 3, 4, 5}
	SliceRemoveSubrangeRange(&lines, Range[int]{1, 3})
	SliceRemoveSubrange(&lines, ClosedRange[int]{2, 3})
	SliceRemoveSubrange(&lines, Range[int]{2, 2})
	if !reflect.DeepEqual(lines, []int{0, 3}) {
		t.Errorf("removeSubrange gave %v", lines)
	}
	if !panics(func() { SliceRemoveSubrange(&lines, ClosedRange[int]{1, 2}) }) {
		t.Error("removeSubrange past the end did not trap")
	}
	if got := SliceInitStrideThrough[[]int](StrideFromThroughBy(5, 20, 5)); !reflect.DeepEqual(got, []int{5, 10, 15, 20}) {
		t.Errorf("Array(stride) = %v", got)
	}
	if got := SliceInitZip2Sequence[[]struct {
		F0 string
		F1 int
	}](Zip([]string{"a", "b"}, []int{1})); len(got) != 1 || got[0].F0 != "a" || got[0].F1 != 1 {
		t.Errorf("Array(zip) = %v", got)
	}
	if got := SliceDistanceFromTo(StringUtf8("héllo"), 0, 3); got != 3 {
		t.Errorf("distance = %d", got)
	}
	if got := RangeLast(Range[int]{0, 3}); got == nil || *got != 2 {
		t.Errorf("Range.last = %v", got)
	}
	if RangeLast(Range[int]{4, 4}) != nil {
		t.Error("empty Range.last is not nil")
	}
	if got := ReversedCollectionJoinedSeparator(SliceReversed([]string{"bag", "chest"}), ", inside "); got != "chest, inside bag" {
		t.Errorf("joined = %q", got)
	}
	if got := LazyMapSequenceSorted([]string{"b", "a"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("sorted = %v", got)
	}
	if got := LazyFilterSequenceFirst(LazyFilterSequenceDropFirstInt([]int{7, 8, 9}, 1)); got == nil || *got != 8 {
		t.Errorf("dropFirst(1).first = %v", got)
	}
	if LazyFilterSequenceFirst(LazyFilterSequenceDropFirstInt([]int{7}, 1)) != nil {
		t.Error("first of an exhausted sequence is not nil")
	}
}

func TestMapAndRangeGaps(t *testing.T) {
	base := map[string]string{"PATH": "/bin", "HOME": "/root"}
	merged := MapMergingUniquingKeysWith(base, map[string]string{"HOME": "/home", "X": "1"}, func(_, replacement string) string { return replacement })
	if !reflect.DeepEqual(merged, map[string]string{"PATH": "/bin", "HOME": "/home", "X": "1"}) || base["HOME"] != "/root" {
		t.Errorf("merging gave %v, the receiver now %v", merged, base)
	}
	type entry = struct {
		F0 int
		F1 float64
	}
	byKey := func(a, b entry) bool { return a.F0 < b.F0 }
	if got := MapMaxBy(map[int]float64{5: 0.1, 15: 0.2, 10: 0.9}, byKey); got == nil || got.F0 != 15 {
		t.Errorf("max(by:) = %v", got)
	}
	if MapMaxBy(map[int]float64{}, byKey) != nil {
		t.Error("max(by:) of an empty dictionary is not nil")
	}
	even := func(v int) bool { return v%2 == 0 }
	if got := ClosedRangeCountWhere(ClosedRange[int]{1, 5}, even); got != 2 {
		t.Errorf("count(where:) = %d", got)
	}
	if got := ClosedRangeCountWhere(ClosedRange[int]{1, 0}, even); got != 0 {
		t.Errorf("count(where:) of an empty range = %d", got)
	}
	refs := ClosedRangeCompactMapRef(ClosedRange[int64]{1, 4}, func(v int64) *int64 {
		if v%2 == 0 {
			return nil
		}
		return &v
	})
	if len(refs) != 2 || *refs[0] != 1 || *refs[1] != 3 {
		t.Errorf("compactMap = %v", refs)
	}
	if got := FloatMagnitude(-0.5); got != 0.5 {
		t.Errorf("magnitude = %v", got)
	}
	now := InstantNow()
	if got := InstantDurationTo(now, InstantAdvancedBy(now, DurationMilliseconds(1500))); time.Duration(got) != 1500*time.Millisecond {
		t.Errorf("advanced(by:) moved %v", time.Duration(got))
	}
	var generator any = SystemRandomNumberGeneratorInit()
	if v := IntRandomInUsing(ClosedRange[int]{3, 4}, &generator); v < 3 || v > 4 {
		t.Errorf("random = %d", v)
	}
}

func TestKeyValuePairs(t *testing.T) {
	type pair = struct {
		F0 string
		F1 any
	}
	var columns any = []pair{{"kind", "state"}, {"kind", 2}, {"negated", false}}
	if got := SliceInitKeyValuePairs[[]pair](columns); len(got) != 3 || got[1].F1 != 2 {
		t.Errorf("Array(KeyValuePairs) = %v", got)
	}
	names := KeyValuePairsMap(columns, func(p pair) string { return p.F0 })
	if !reflect.DeepEqual(names, []string{"kind", "kind", "negated"}) {
		t.Errorf("KeyValuePairs.map = %v", names)
	}
	type named struct {
		F0 string
		F1 any
	}
	if got := KeyValuePairsMap([]named{{"a", 1}}, func(p pair) string { return p.F0 }); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("KeyValuePairs.map over a named tuple = %v", got)
	}
	if got := SliceInitKeyValuePairs[[]pair]([]pair{}); len(got) != 0 {
		t.Errorf("empty KeyValuePairs = %v", got)
	}
}

func TestSymmetricKeyAndHMAC(t *testing.T) {
	// RFC 4231 test case 2.
	key := SymmetricKeyInitData([]byte("Jefe"))
	body := []byte("what do ya want for nothing?")
	codes := []string{
		"5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843",
		"af45d2e376484031617f78d2b58a6b1b9c7ef464f5a01b47e42ec3736322445e8e2240ca5e69e2c78b3239ecfab21649",
		"164b7a7bfcf819e2e395fbe73b56e0a387bd64222e831fd610270cd7ea2505549758bf75c05a994a6d034f65f8f0e6fdcaeab1a34d4a6b4b636e070a38bce737",
	}
	for _, hexCode := range codes {
		code := unhex(t, hexCode)
		if !HMACIsValidAuthenticationCodeAuthenticatingUsing(code, body, key) {
			t.Errorf("HMAC of %d bytes is not valid", len(code))
		}
		code[0] ^= 1
		if HMACIsValidAuthenticationCodeAuthenticatingUsing(code, body, key) {
			t.Errorf("a tampered HMAC of %d bytes is valid", len(code))
		}
	}
	if HMACIsValidAuthenticationCodeAuthenticatingUsing(unhex(t, codes[0])[:31], body, key) {
		t.Error("a truncated HMAC is valid")
	}
	if SHA256ByteCount != 32 {
		t.Errorf("SHA256.byteCount = %d", SHA256ByteCount)
	}
	a, b := SymmetricKeyInitSize(SymmetricKeySizeBits256), SymmetricKeyInitSize(SymmetricKeySizeBits256)
	text := SymmetricKeyWithUnsafeBytes(a, func(p any) string { return DataBase64EncodedString(DataInitUnsafeRawBufferPointer(p)) })
	if len(a.bytes) != 32 || len(text) != 44 || reflect.DeepEqual(a.bytes, b.bytes) {
		t.Errorf("SymmetricKey(size: .bits256) gave %d bytes, base64 %q", len(a.bytes), text)
	}
	source := []byte{1, 2}
	copied := SymmetricKeyInitData(source)
	source[0] = 9
	if copied.bytes[0] != 1 {
		t.Error("SymmetricKey(data:) shares its bytes with the data")
	}
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	out := DataInitCapacity(len(s) / 2)
	for i := 0; i < len(s); i += 2 {
		var v uint8
		for _, c := range []byte(s[i : i+2]) {
			switch {
			case c >= '0' && c <= '9':
				v = v<<4 | (c - '0')
			case c >= 'a' && c <= 'f':
				v = v<<4 | (c - 'a' + 10)
			default:
				t.Fatalf("bad hex %q", s)
			}
		}
		DataAppendInt(&out, v)
	}
	return out
}

func TestDataGaps(t *testing.T) {
	if got := DataBase64EncodedString([]byte{0xfb, 0xff, 0x00, 0x01}); got != "+/8AAQ==" {
		t.Errorf("base64EncodedString = %q", got)
	}
	if got := DataBase64EncodedString(nil); got != "" {
		t.Errorf("base64EncodedString of empty data = %q", got)
	}
	view := SubstringUtf8(Substring{"xéy", 1, 3})
	data := DataInitUTF8View(view)
	view[0] = 0
	if string(data) != "é" {
		t.Errorf("Data(utf8) = %q", data)
	}
	var cstr any = []int8{'h', 'i', 0, 'x'}
	if got := DataInitBytesCount(&cstr, CFStrlen(&cstr)); string(got) != "hi" {
		t.Errorf("Data(bytes:count:) = %q", got)
	}
	if got := DataInitBytesCount(CStringOf("abc"), 2); string(got) != "ab" {
		t.Errorf("Data(bytes:count:) of a C string = %q", got)
	}
	if !panics(func() { DataInitBytesCount([]byte{1}, 2) }) {
		t.Error("Data(bytes:count:) past the buffer did not trap")
	}
}

func TestNSNumberAndNull(t *testing.T) {
	var null any = NSNullInit()
	if !Is[Null](null) || CFCFGetTypeID(null) == CFCFBooleanGetTypeID() {
		t.Error("NSNull() is not an NSNull")
	}
	if CFCFGetTypeID(NSNumberBool(false)) != CFCFBooleanGetTypeID() || CFCFGetTypeID(NSNumberInt64(1)) == CFCFBooleanGetTypeID() {
		t.Error("CFGetTypeID does not tell a Bool NSNumber from an integer one")
	}
	cases := []struct {
		n       NSNumber
		integer int
		boolean bool
	}{
		{NSNumberBool(true), 1, true},
		{NSNumberInt64(-7), -7, true},
		{NSNumberInt64(0), 0, false},
		{NSNumberUInt64(1<<64 - 1), -1, true},
		{NSNumberDouble(-2.9), -2, true},
		{NSNumberDouble(0.5), 0, true},
		{NSNumberDouble(1e300), 1<<63 - 1, true},
		{NSNumberDouble(-1e300), -1 << 63, true},
	}
	for _, c := range cases {
		if got := NSNumberIntValue(c.n); got != c.integer {
			t.Errorf("%v.intValue = %d, want %d", c.n.Description(), got, c.integer)
		}
		if got := NSNumberBoolValue(c.n); got != c.boolean {
			t.Errorf("%v.boolValue = %v, want %v", c.n.Description(), got, c.boolean)
		}
	}
}

func TestRegexReplacingMatchesInRange(t *testing.T) {
	re := NSRegularExpressionInitPattern(`\b12(\d{2})\b`)
	source := "é 1201, 1299 and 12345; 1207"
	whole := NSRangeInitRangeIn(Range[int]{StringStartIndex(source), StringEndIndex(source)}, source)
	if whole != (Range[int]{0, 28}) {
		t.Errorf("NSRange of the whole string = %v, want {0, 28}", whole)
	}
	if got, want := NSRegularExpressionStringByReplacingMatchesInOptionsRangeWithTemplate(re, source, MatchingOptions{}, whole, "34$1"), "é 3401, 3499 and 12345; 3407"; got != want {
		t.Errorf("replacing = %q, want %q", got, want)
	}
	if got, want := NSRegularExpressionStringByReplacingMatchesInOptionsRangeWithTemplate(re, source, MatchingOptions{}, Range[int]{8, 28}, `\$$1`), "é 1201, $99 and 12345; $07"; got != want {
		t.Errorf("replacing in a range = %q, want %q", got, want)
	}
	if !panics(func() {
		NSRegularExpressionStringByReplacingMatchesInOptionsRangeWithTemplate(re, source, MatchingOptions{}, Range[int]{0, 99}, "")
	}) {
		t.Error("a range past the end did not trap")
	}
}
