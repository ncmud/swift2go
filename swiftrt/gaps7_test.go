package swiftrt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestCryptTraditionalDES(t *testing.T) {
	c := func(k, s string) string {
		var a, b any = k, s
		return StringInitCString(CFCrypt(&a, &b))
	}
	cases := []struct{ key, salt, want string }{
		{"password", "ab", "abJnggxhB/yWI"},
		{"", "..", "..X8NBuQ4l6uQ"},
		{"abcdefghijk", "zZ", "zZx8AiUhD/THE"},
		{"\xc3\xa9t\xc3\xa9", "9/", "9/w1K15ZDSdDM"},
		{"iti", "9/2JHYAWFUKAs", "9/2JHYAWFUKAs"},
		{"\x80", "ab", "abmF1QH4PEr.E"},
		{"x", "a!", "*0"},
		{"x", "a", "*0"},
		{"x", "*0", "*1"},
		{"x", "abc$", "abiQ6Ep3EYTHc"},
		{"x", "ab:", "*0"},
		{"x", "ab ", "*0"},
		{"x", "9/2JHYAWFUKAs!", "*0"},
	}
	for _, tc := range cases {
		if got := c(tc.key, tc.salt); got != tc.want {
			t.Errorf("crypt(%q, %q) = %q, want %q", tc.key, tc.salt, got, tc.want)
		}
	}
	got := StringWithCString("password", func(p *any) string {
		var salt any = "ab"
		return StringInitCString(CFCrypt(Some[any](p), &salt))
	})
	if got != "abJnggxhB/yWI" {
		t.Errorf("crypt through withCString = %q", got)
	}
}

func TestNSNumberObjCTypeAndValues(t *testing.T) {
	cases := []struct {
		n      NSNumber
		typ    byte
		double float64
		u64    uint64
	}{
		{NSNumberInt64(5), 'i', 5, 5},
		{NSNumberInt64(-1), 'i', -1, 1<<64 - 1},
		{NSNumberInt64(12), 'i', 12, 12},
		{NSNumberInt64(13), 'q', 13, 13},
		{NSNumberInt64(-2), 'q', -2, 1<<64 - 2},
		{NSNumberUInt64(5), 'i', 5, 5},
		{NSNumberUInt64(1 << 63), 'Q', 9.223372036854776e18, 1 << 63},
		{NSNumberBool(true), 'c', 1, 1},
		{NSNumberDouble(-2.5), 'd', -2.5, 18446744073709551613},
		{NSNumberDouble(1e30), 'd', 1e30, 5076964154930102272},
		{NSNumberDouble(-2.5e20), 'd', -2.5e20, 8254417031933722624},
		{NSNumberDouble(2e19), 'd', 2e19, 1553255926290448384},
		{NSNumberDouble(1.71e38), 'd', 1.71e38, 1<<64 - 1},
		{NSNumberDouble(-1e40), 'd', -1e40, 0},
	}
	for _, tc := range cases {
		if got := UnsafePointerPointee(NSValueObjCType(tc.n)); got != int8(tc.typ) {
			t.Errorf("%v objCType = %c, want %c", tc.n, got, tc.typ)
		}
		if got := NSNumberDoubleValue(tc.n); got != tc.double {
			t.Errorf("%v doubleValue = %v", tc.n, got)
		}
		if got := NSNumberUint64Value(tc.n); got != tc.u64 {
			t.Errorf("%v uint64Value = %d, want %d", tc.n, got, tc.u64)
		}
	}
}

func TestComponentsAndPathComponents(t *testing.T) {
	if got := StringComponentsSeparatedBy("a\r\nb\nc d", CharacterSetNewlines); !reflect.DeepEqual(got, []string{"a", "", "b", "c", "d"}) {
		t.Errorf("components(separatedBy: .newlines) = %q", got)
	}
	if got := StringComponentsSeparatedBy("", CharacterSetNewlines); !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("empty components = %q", got)
	}
	if got := StringComponentsSeparatedBy("abc", ""); !reflect.DeepEqual(got, []string{"abc"}) {
		t.Errorf("empty separator = %q", got)
	}
	if got := StringComponentsSeparatedBy("a,b", ","); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("comma separator = %q", got)
	}
	for _, tc := range [][3]string{{"/tmp/x", ".env", "/tmp/x/.env"}, {"/tmp/", "/a/", "/tmp//a/"}, {"", "b", "b"}, {"a//", "b", "a//b"}, {"/", ".env", "/.env"}, {"a", "", "a"}} {
		if got := NSStringAppendingPathComponent(tc[0], tc[1]); got != tc[2] {
			t.Errorf("%q.appendingPathComponent(%q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}

func TestDecomposedStringWithCanonicalMapping(t *testing.T) {
	var got []string
	for _, r := range StringDecomposedStringWithCanonicalMapping("Ångström café ﬁ ǅ 한") {
		got = append(got, fmt.Sprintf("%x", r))
	}
	want := []string{"41", "30a", "6e", "67", "73", "74", "72", "6f", "308", "6d", "20", "63", "61", "66", "65", "301", "20", "fb01", "20", "1c5", "20", "1112", "1161", "11ab"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NFD = %v", got)
	}
	if got := StringDecomposedStringWithCanonicalMapping("ạ́"); got != "ạ́" {
		t.Errorf("combining marks not reordered: %q", got)
	}
}

func TestNSRegularExpressionMatches(t *testing.T) {
	re := NSRegularExpressionInitPattern("b+")
	s := "abbcb🎉bb"
	full := NSRangeInitPartialRangeFromIn(PartialRangeFrom[int]{Lo: 0}, s)
	var got []Range[int]
	for _, m := range NSRegularExpressionMatchesInRange(re, s, full) {
		got = append(got, NSTextCheckingResultRange(m))
	}
	if want := []Range[int]{{1, 3}, {4, 5}, {7, 9}}; !reflect.DeepEqual(got, want) {
		t.Errorf("matches = %v", got)
	}
	if m := NSRegularExpressionFirstMatchInRange(re, s, full); m == nil || NSTextCheckingResultRange(*m) != (Range[int]{1, 3}) {
		t.Errorf("firstMatch = %v", m)
	}
	if m := NSRegularExpressionFirstMatchInRange(re, "xyz", Range[int]{0, 3}); m != nil {
		t.Errorf("firstMatch on xyz = %v", m)
	}
	anchored := NSRegularExpressionInitPattern("^b")
	if ms := NSRegularExpressionMatchesInRange(anchored, "abab", Range[int]{1, 4}); len(ms) != 1 || NSTextCheckingResultRange(ms[0]) != (Range[int]{1, 2}) {
		t.Errorf("anchored at range start = %v", ms)
	}
}

func TestFileManagerAndHandleGaps(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"b", ".hidden", "a"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names := NSFileManagerContentsOfDirectoryAtPath(NSFileManagerDefault, dir)
	slices.Sort(names)
	if !reflect.DeepEqual(names, []string{".hidden", "a", "b"}) {
		t.Errorf("contentsOfDirectory = %q", names)
	}
	err := Catch(func() { NSFileManagerContentsOfDirectoryAtPath(NSFileManagerDefault, filepath.Join(dir, "missing")) })
	var ns NSError
	if !errors.As(err, &ns) || ns.Code != 260 {
		t.Errorf("missing directory threw %v", err)
	}
	err = Catch(func() { NSFileManagerContentsOfDirectoryAtPath(NSFileManagerDefault, filepath.Join(dir, "a")) })
	if !errors.As(err, &ns) || ns.Code != 256 {
		t.Errorf("file as directory threw %v", err)
	}
	if p := NSFileManagerContentsAtPath(NSFileManagerDefault, filepath.Join(dir, "a")); p == nil || string(*p) != "a" {
		t.Errorf("contents(atPath:) = %v", p)
	}
	if NSFileManagerContentsAtPath(NSFileManagerDefault, dir) != nil || NSFileManagerContentsAtPath(NSFileManagerDefault, filepath.Join(dir, "missing")) != nil {
		t.Error("contents(atPath:) of a directory or missing file is not nil")
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	if !NSFileManagerFileExistsAtPath(NSFileManagerDefault, dir) || NSFileManagerFileExistsAtPath(NSFileManagerDefault, "") || NSFileManagerFileExistsAtPath(NSFileManagerDefault, filepath.Join(dir, "dangling")) {
		t.Error("fileExists(atPath:) mismatch")
	}
	f, err2 := os.Create(filepath.Join(dir, "out"))
	if err2 != nil {
		t.Fatal(err2)
	}
	h := FileHandle{File: f}
	NSFileHandleWrite(h, []byte("hello"))
	NSFileHandleClose(h)
	if b, _ := os.ReadFile(filepath.Join(dir, "out")); string(b) != "hello" {
		t.Errorf("written %q", b)
	}
}

type gaps7LocalizedError struct{}

func (gaps7LocalizedError) ErrorDescription() *string { return Some("custom") }

type gaps7PlainError struct{}

func TestErrorLocalizedDescriptionAndProcessInfo(t *testing.T) {
	cases := []struct {
		err  any
		want string
	}{
		{Throw(gaps7LocalizedError{}).Err, "custom"},
		{Throw(gaps7PlainError{}).Err, "The operation could not be completed. (gaps7PlainError error 1.)"},
		{CancellationErrorInit(), "The operation could not be completed. (Swift.CancellationError error 1.)"},
		{NSError{Domain: "NSCocoaErrorDomain", Code: 260, Msg: "The operation could not be completed. The file doesn’t exist."}, "The operation could not be completed. The file doesn’t exist."},
	}
	for _, tc := range cases {
		if got := ErrorLocalizedDescription(tc.err); got != tc.want {
			t.Errorf("localizedDescription(%T) = %q, want %q", tc.err, got, tc.want)
		}
	}
	t.Setenv("SWIFTRT_GAPS7", "a=b")
	if got := NSProcessInfoEnvironment(NSProcessInfoProcessInfo)["SWIFTRT_GAPS7"]; got != "a=b" {
		t.Errorf("environment = %q", got)
	}
	if a, b := NSProcessInfoSystemUptime(NSProcessInfoProcessInfo), NSProcessInfoSystemUptime(NSProcessInfoProcessInfo); a <= 0 || b < a {
		t.Errorf("systemUptime %v then %v", a, b)
	}
}

func TestTaskGroupTypedNextAndExecutorRefs(t *testing.T) {
	got := WithThrowingTaskGroupOfBody(TypeOf[*int](), func(race *any) int {
		ThrowingTaskGroupAddTaskOperation(race, func() *int {
			v := 41
			return &v
		})
		first := OptAnyAsRef[*int](ThrowingTaskGroupNext(race))
		if OptAnyAsRef[*int](ThrowingTaskGroupNext(race)) != nil {
			t.Error("drained group returned a result")
		}
		return *first + 1
	})
	if MustAs[int](got) != 42 {
		t.Errorf("group result = %v", got)
	}
	WithTaskGroupOfBody(TypeOf[string](), func(group *any) {
		TaskGroupAddTaskOperation(group, func() string { return "a" })
		if s := OptAnyAs[string](TaskGroupNext(group)); s == nil || *s != "a" {
			t.Errorf("TaskGroup.next() = %v", s)
		}
		if OptAnyAs[string](TaskGroupNext(group)) != nil {
			t.Error("drained TaskGroup returned a result")
		}
		TaskGroupCancelAll(*group)
	})
	var nilChild any = (*int)(nil)
	if OptAnyAsRef[*int](&nilChild) != nil {
		t.Error("a child's nil result")
	}
	executor := &struct{ name string }{"serial"}
	if TaskExecutorAsUnownedTaskExecutor(executor).Executor != executor || SerialExecutorAsUnownedSerialExecutor(executor).Executor != executor {
		t.Error("unowned executor references")
	}
}
