package swiftrt

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		var msg string
		switch v := r.(type) {
		case string:
			msg = v
		case error:
			msg = v.Error()
		}
		if !strings.Contains(msg, want) {
			t.Fatalf("panic %q, want %q", msg, want)
		}
	}()
	f()
}

// Expected values come from swift-corelibs-foundation on Linux.
func TestNSRangeConversions(t *testing.T) {
	s := "ae\u0301\U0001F600b"
	cases := []struct {
		r    Range[int]
		want string
		ok   bool
	}{
		{Range[int]{1, 2}, "", false},
		{Range[int]{1, 3}, "e\u0301", true},
		{Range[int]{3, 4}, "", false},
		{Range[int]{3, 5}, "😀", true},
		{Range[int]{6, 6}, "", true},
		{Range[int]{6, 7}, "", false},
		{Range[int]{IntMax, IntMax}, "", false},
	}
	for _, c := range cases {
		got := RangeOptInitNSRangeIn(c.r, s)
		if (got != nil) != c.ok || got != nil && s[got.Lo:got.Hi] != c.want {
			t.Errorf("Range(%v) = %v", c.r, got)
		}
	}
	if got := NSRangeInitPartialRangeFromIn(PartialRangeFrom[int]{Lo: 1}, s); got != (Range[int]{1, 6}) {
		t.Errorf("NSRange(1...) = %v, want {1, 5}", got)
	}
}

func TestNSRegularExpressionInitPattern(t *testing.T) {
	if NSRegularExpressionInitPattern(`undead|\w+`) == nil {
		t.Fatal("nil regex")
	}
	err := Catch(func() { NSRegularExpressionInitPattern("a(b") })
	var ns NSError
	if !errors.As(err, &ns) || ns.Code != 2048 {
		t.Fatalf("a(b threw %v, want NSCocoaErrorDomain 2048", err)
	}
	whole := func(re *Regex, s string) []string {
		var out []string
		for _, m := range NSRegularExpressionMatchesInRange(re, s, Range[int]{0, utf16Offset(s, len(s))}) {
			r := RangeOptInitNSRangeIn(m.Ranges[0], s)
			out = append(out, s[r.Lo:r.Hi])
		}
		return out
	}
	if got := whole(NSRegularExpressionInitPattern(`a(?![a-z])`), "ab a1 a"); strings.Join(got, ",") != "a,a" {
		t.Errorf("lookahead matched %q", got)
	}
	if got := whole(NSRegularExpressionInitPattern(`(\w)\1`), "a😀😀bbc"); strings.Join(got, ",") != "bb" {
		t.Errorf("backreference matched %q", got)
	}
	if got := whole(NSRegularExpressionInitPattern(`(?<!x)y`), "xy zy"); strings.Join(got, ",") != "y" {
		t.Errorf("lookbehind matched %q", got)
	}
	mustPanic(t, "not supported by RE2 or regexp2", func() { NSRegularExpressionInitPattern(`\N{DIGIT ONE}`) })
}

func TestSHA256AndPBKDF2(t *testing.T) {
	d := SHA256HashData([]byte("abc"))
	if hex.EncodeToString(SHA256DigestPrefixInt(d, 4)) != "ba7816bf" || len(SHA256DigestPrefixInt(d, 40)) != 32 {
		t.Fatalf("prefix wrong")
	}
	if got := SHA256DigestMap(d, func(b uint8) uint8 { return b })[31]; got != 0xad {
		t.Fatalf("map last byte %x", got)
	}
	mustPanic(t, "negative length", func() { SHA256DigestPrefixInt(d, -1) })

	key := PBKDF2DeriveKeyFromSaltUsingOutputByteCountUnsafeUncheckedRounds([]byte("password"), []byte("salt"), HashFunctionSha256, 32, 1)
	got := SymmetricKeyWithUnsafeBytes(key, func(p any) []uint8 { return SliceInitUnsafeRawBufferPointer[[]uint8](p) })
	if hex.EncodeToString(got) != "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b" {
		t.Fatalf("pbkdf2 = %x", got)
	}
	if Catch(func() {
		PBKDF2DeriveKeyFromSaltUsingOutputByteCountUnsafeUncheckedRounds(nil, nil, HashFunctionSha256, 32, 0)
	}) == nil {
		t.Fatal("zero rounds did not throw")
	}
}

func TestTaskAndContinuation(t *testing.T) {
	task := TaskInitOperation(func() int { return 7 })
	if TaskValue(task) != 7 {
		t.Fatal("task value")
	}
	thrower := TaskInitOperation(func() { panic(Throw(CancellationErrorInit())) })
	if err := Catch(func() { TaskValue(thrower) }); !errors.As(err, new(CancellationError)) {
		t.Fatalf("task value threw %v", err)
	}
	v := WithCheckedThrowingContinuation(func(c any) {
		go CheckedContinuationResumeReturning(c, "done")
	})
	if v != "done" {
		t.Fatalf("continuation value %v", v)
	}
	err := Catch(func() {
		WithCheckedThrowingContinuation(func(c any) {
			CheckedContinuationResumeThrowing(c, CancellationErrorInit())
			mustPanic(t, "more than once", func() { CheckedContinuationResumeReturning(c, 1) })
		})
	})
	if err == nil || err.Error() != "CancellationError()" {
		t.Fatalf("continuation threw %v", err)
	}
}

func TestIndexingIteratorAndHasher(t *testing.T) {
	xs := []int{1, 2}
	it := SliceMakeIterator(xs)
	xs[0] = 9
	if *IndexingIteratorNext(&it) != 1 || *IndexingIteratorNext(&it) != 2 || IndexingIteratorNext(&it) != nil {
		t.Fatal("iterator")
	}
	var a, b any
	HasherCombine(&a, 42)
	HasherCombine(&b, 42)
	if HasherFinalize(a) != HasherFinalize(b) {
		t.Fatal("equal values hash differently")
	}
}

func TestReduceIntoPassesInoutAccumulator(t *testing.T) {
	sum := SliceReduceInto([]int{1, 2, 3}, 0, func(acc *int, x int) { *acc += x })
	counts := MapReduceInto(map[string]int{"a": 1, "b": 2}, map[int]int{}, func(acc *map[int]int, kv struct {
		F0 string
		F1 int
	}) {
		(*acc)[kv.F1]++
	})
	if sum != 6 || len(counts) != 2 {
		t.Fatalf("sum %d counts %v", sum, counts)
	}
}
