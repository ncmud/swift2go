//go:build !wasip1

package swiftrt

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestSocketpairCarriesBytesBothWays(t *testing.T) {
	fds := []int32{0, 0}
	if CFSocketpair(CMacroAF_UNIX, CMacroSOCK_STREAM, 0, &fds) != 0 {
		t.Fatalf("socketpair failed: errno %d", CErrno())
	}
	defer CFClose(fds[0])
	defer CFClose(fds[1])
	flags := FcntlIntIntInt(fds[1], CMacroF_GETFL, 0)
	if FcntlIntIntInt(fds[1], CMacroF_SETFL, flags|CMacroO_NONBLOCK) != 0 {
		t.Fatal("fcntl O_NONBLOCK")
	}
	buf := make([]byte, 8)
	if n := CFRead(fds[1], &buf, len(buf)); n != -1 {
		t.Fatalf("read of an empty non-blocking socket = %d", n)
	}
	if CFWrite(fds[0], "ping", 4) != 4 {
		t.Fatal("write")
	}
	if n := CFRead(fds[1], &buf, len(buf)); n != 4 || string(buf[:4]) != "ping" {
		t.Fatalf("read %d %q", n, buf[:n])
	}
	if CX__socket_typeRawValue(CE__socket_typeSOCK_STREAM) != uint32(CMacroSOCK_STREAM) {
		t.Fatal("glibc's __socket_type disagrees with SOCK_STREAM")
	}
}

func TestJSONFoundationCollections(t *testing.T) {
	obj := NSJSONSerializationJsonObjectWith([]byte(`{"b": [1, 2], "a": {}}`)).(Dictionary)
	if NSDictionaryCount(obj) != 2 || !slices.Equal(NSDictionaryAllKeys(obj), []any{"a", "b"}) {
		t.Fatalf("dictionary %v", obj)
	}
	arr := (*NSDictionarySubscriptAny(obj, "b")).(Array)
	if NSArrayCount(arr) != 2 || (*NSArrayFirstObject(arr)).(NSNumber).Description() != "1" || NSArrayFirstObject(Array{}) != nil {
		t.Fatalf("array %v", arr)
	}
}

func TestGapsEleven(t *testing.T) {
	if got := Tupled5(func(a int, b string, c bool, d int, e string) string { return b + e })(struct {
		F0 int
		F1 string
		F2 bool
		F3 int
		F4 string
	}{1, "x", true, 2, "y"}); got != "xy" {
		t.Fatal(got)
	}
	if e := StringEnumerated("a😀b"); len(e) != 3 || e[1].F0 != 1 || e[1].F1 != "😀" {
		t.Fatalf("%v", e)
	}
	s := "a😀b"
	if IntUtf16OffsetIn(len("a😀"), s) != 3 {
		t.Fatal("utf16Offset")
	}
	if !StringContains("the dark", Substring{"a dark wall", 2, 6}) || StringContains("light", Substring{"dark", 0, 4}) {
		t.Fatal("contains Substring")
	}
	if ClosedRangeCount(ClosedRange[uint64]{Lo: 1, Hi: 500}) != 500 {
		t.Fatal("ClosedRange<UInt64>.count")
	}
	if got := ClosedRangeFlatMap(ClosedRange[int]{Lo: 1, Hi: 3}, func(i int) []int { return []int{i, i} }); !slices.Equal(got, []int{1, 1, 2, 2, 3, 3}) {
		t.Fatal(got)
	}
	if AtomicWrappingAddOrdering(AtomicInit[int64](7), 1, AtomicUpdateOrderingRelaxed).F1 != 8 {
		t.Fatal("wrappingAdd newValue")
	}
}

func TestSplitOnASeparatorOfSeveralCharacters(t *testing.T) {
	got := SliceMap(StringSplitSeparator("a\n\rb\n\r\n\rc\n\r", "\n\r"), func(s Substring) string { return s.String() })
	// "\r\n" is one Character, so the second separator is not on Character boundaries, as in Swift.
	if !slices.Equal(got, []string{"a", "b\n\r\n\rc"}) {
		t.Fatalf("%q", got)
	}
	got = SliceMap(StringSplitSeparatorOmittingEmptySubsequences("a--b----c", "--", false), func(s Substring) string { return s.String() })
	if !slices.Equal(got, []string{"a", "b", "", "c"}) {
		t.Fatalf("%q", got)
	}
}

func TestFoundationGapsEleven(t *testing.T) {
	s := "hello, world"
	StringRemoveSubrangeRange(&s, Range[int]{Lo: 5, Hi: 7})
	if s != "helloworld" {
		t.Fatal(s)
	}
	if string(DataInitRepeatingCount(0x5A, 3)) != "ZZZ" {
		t.Fatal("Data(repeating:count:)")
	}
	if f := URLFragment(URL{Raw: "https://x/y#frag"}); f == nil || *f != "frag" || URLFragment(URL{Raw: "https://x"}) != nil {
		t.Fatal("fragment")
	}
	if c := DateComponentsInitYearMonthDayHour(Some(2026), Some(7), Some(4), Some(12)); *c.Hour != 12 || *c.Day != 4 {
		t.Fatal("DateComponents")
	}
	if !strings.HasSuffix(CFNSTemporaryDirectory(), "/") {
		t.Fatal("NSTemporaryDirectory")
	}
	path := t.TempDir() + "/f"
	if err := os.WriteFile(path, []byte("abc"), 0o640); err != nil {
		t.Fatal(err)
	}
	when := Date(1_000_000)
	NSFileManagerSetAttributesOfItemAtPath(FileManager{}, map[FileAttributeKey]any{NSFileAttributeKeyCNSFileModificationDate: when}, path)
	attrs := NSFileManagerAttributesOfItemAtPath(FileManager{}, path)
	if attrs[NSFileAttributeKeyCNSFilePosixPermissions] != 0o640 || attrs[NSFileAttributeKeyCNSFileModificationDate] != when || attrs[NSFileAttributeKeyCNSFileSize] != int64(3) {
		t.Fatalf("%v", attrs)
	}
}

func TestIterateUnsignedRanges(t *testing.T) {
	if got := Iterate[uint64](Range[uint64]{Lo: 2, Hi: 5}); !slices.Equal(got, []uint64{2, 3, 4}) {
		t.Fatal(got)
	}
	if got := Iterate[uint8](ClosedRange[uint8]{Lo: 254, Hi: 255}); !slices.Equal(got, []uint8{254, 255}) {
		t.Fatal(got)
	}
	if got := Iterate[int8](Range[int8]{Lo: -1, Hi: -1}); len(got) != 0 {
		t.Fatal(got)
	}
}

type castWord struct{ Value string }

func TestCastAValueToItsOptional(t *testing.T) {
	var v any = castWord{"hi"}
	if p := MustAs[*castWord](v); p == nil || p.Value != "hi" {
		t.Fatal("as! Word? of a Word")
	}
	if As[*int]("x") != nil {
		t.Fatal("as? Int? of a String")
	}
}

func TestHMapEqualsNSDictionary(t *testing.T) {
	parsed := NSJSONSerializationJsonObjectWith([]byte(`[{"nowhere": {}}, {"a": {"b": 1}}]`))
	lit := []Dictionary{{"nowhere": HMap[any, any]{}}, {"a": HMap[any, any]{M: map[string]HEntry[any, any]{"b": {Key: "b", Value: 1}}}}}
	if !Equal(any(lit), parsed) {
		t.Fatal("HMap literal vs parsed JSON")
	}
}
