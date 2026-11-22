package swiftrt

import (
	"strings"
	"sync"
	"testing"
)

func TestStringAppendKeepsEveryValue(t *testing.T) {
	long := strings.Repeat("x", appendMinLen)
	s := long
	StringAppend(&s, "a")
	before := s // shares s's buffer
	StringAppend(&s, "b")
	copied := s
	StringAppend(&s, "c")
	StringAppend(&before, "Z") // before no longer ends at the high-water mark, so it is copied
	StringAppend(&copied, "Y")
	StringAppend(&s, "d")
	if s != long+"abcd" || before != long+"aZ" || copied != long+"abY" {
		t.Fatalf("s=%q before=%q copied=%q", s[appendMinLen:], before[appendMinLen:], copied[appendMinLen:])
	}
	prefix := s[:appendMinLen+1] // a substring at the buffer's start
	StringAppend(&prefix, "Q")
	if s != long+"abcd" || prefix != long+"aQ" {
		t.Fatalf("s=%q prefix=%q", s[appendMinLen:], prefix[appendMinLen:])
	}
}

func TestStringReserveCapacityThenAppend(t *testing.T) {
	s := ""
	StringReserveCapacityInt(&s, 1000)
	keep := s
	for i := 0; i < 300; i++ {
		StringAppend(&s, "ab")
		StringAppendCharacter(&s, "é")
	}
	if want := strings.Repeat("abé", 300); s != want || keep != "" {
		t.Fatalf("got %d bytes, keep %q", len(s), keep)
	}
}

func TestStringAppendManyBuffers(t *testing.T) {
	// More strings than slots, appended in turn, so buffers lose their slots between appends.
	ss := make([]string, appendBufSlots*3)
	for round := 0; round < 50; round++ {
		for i := range ss {
			StringAppend(&ss[i], strings.Repeat(string(rune('a'+i%26)), 3))
		}
	}
	for i, s := range ss {
		if want := strings.Repeat(string(rune('a'+i%26)), 150); s != want {
			t.Fatalf("ss[%d] = %q", i, s)
		}
	}
}

func TestStringAppendConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	shared := strings.Repeat("s", appendMinLen)
	StringAppend(&shared, "!")
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			s := shared // every goroutine starts from the same buffer
			for i := 0; i < 1000; i++ {
				StringAppend(&s, string(rune('0'+g)))
			}
			if want := shared + strings.Repeat(string(rune('0'+g)), 1000); s != want {
				t.Errorf("goroutine %d built a different string", g)
			}
		}(g)
	}
	wg.Wait()
}

// BenchmarkStringAppendCharacters builds a 28 KB string a character at a time, as a parser's output
// loop does. Go's += makes this quadratic.
func BenchmarkStringAppendCharacters(b *testing.B) {
	for b.Loop() {
		s := ""
		for i := 0; i < 28_000; i++ {
			StringAppend(&s, "x")
		}
	}
}
