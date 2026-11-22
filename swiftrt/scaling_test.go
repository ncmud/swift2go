package swiftrt

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The scaling tests run each operation's workload at n and at 10n and check how the cost grows.
// Every workload is linear in n when the operations in it have the complexity Swift documents
// (an O(n) operation called once, or an O(1) operation called n times), so the cost should grow
// about tenfold, or a little more for an O(n log n) sort. A quadratic implementation grows it a
// hundredfold.
//
// Two costs are checked. Bytes allocated is nearly deterministic, so it catches a quadratic copy on
// any machine. Time catches quadratic work that allocates nothing; it is the minimum of several
// runs with the collector held off, and a workload fails only when every attempt exceeds the bound,
// so a busy machine slows a run without failing it.

const (
	// scalingTimeBound is the largest time ratio between 10n and n that passes.
	scalingTimeBound = 35
	// scalingBytesBound is the largest ratio of bytes allocated that passes. Slices and maps grow
	// by doubling, so a linear workload can allocate up to about twenty times as much at 10n.
	scalingBytesBound = 30
	// scalingMinBytes is the allocation at 10n below which the byte ratio is not checked.
	scalingMinBytes = 64 << 10
	// scalingMinTime is the least time at n the time ratio is taken against, so a workload too
	// quick to time reliably at n is judged by its time at 10n.
	scalingMinTime   = 200 * time.Microsecond
	scalingHeapLimit = 1 << 30
	scalingRuns      = 5
	scalingAttempts  = 3
)

// scalingCase is one workload. prepare builds its input of size n outside the timing and returns
// the work to time; it is called again for every run, so the work may consume its input.
type scalingCase struct {
	name    string
	n       int
	prepare func(n int) func()
	// bytesOnly judges the workload by its allocation alone. Growing an array to 10n spends most of
	// its time faulting in the fresh pages its backing arrays take, which a shared CI runner charges
	// unevenly, while at n the pages from earlier runs are reused; a quadratic copy shows in the
	// bytes allocated.
	bytesOnly bool
}

type scalingCost struct {
	elapsed time.Duration
	bytes   uint64
}

// measureScaling is the least time and allocation of several runs at size n. It stops after a run
// that takes longer than limit, which only a workload failing its bound does.
func measureScaling(prepare func(int) func(), n int, limit time.Duration) scalingCost {
	best := scalingCost{elapsed: time.Duration(1<<63 - 1), bytes: 1<<64 - 1}
	var before, after runtime.MemStats
	for range scalingRuns {
		work := prepare(n)
		runtime.GC()
		runtime.ReadMemStats(&before)
		start := time.Now()
		work()
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		best.elapsed = min(best.elapsed, elapsed)
		best.bytes = min(best.bytes, after.TotalAlloc-before.TotalAlloc)
		if elapsed > limit {
			break
		}
	}
	return best
}

// scalingFailure describes why a workload's growth from small to big exceeds the bounds, or is
// empty when it does not.
func scalingFailure(small, big scalingCost, bytesOnly bool) string {
	var reasons []string
	if r := float64(big.elapsed) / float64(max(small.elapsed, scalingMinTime)); r > scalingTimeBound && !bytesOnly {
		reasons = append(reasons, fmt.Sprintf("time grew %.0fx (%v to %v)", r, small.elapsed, big.elapsed))
	}
	if big.bytes >= scalingMinBytes {
		if r := float64(big.bytes) / float64(max(small.bytes, 1)); r > scalingBytesBound {
			reasons = append(reasons, fmt.Sprintf("allocation grew %.0fx (%d to %d bytes)", r, small.bytes, big.bytes))
		}
	}
	return strings.Join(reasons, "; ")
}

func runScaling(t *testing.T, cases []scalingCase) {
	t.Helper()
	if raceDetector {
		t.Skip("the race detector's instrumentation dominates the timings; CI runs these tests without -race")
	}
	if testing.Short() {
		t.Skip("scaling tests are skipped in short mode")
	}
	// Collect only when the heap nears the limit, so a run's time is its own work and not the
	// collector's, and later runs reuse the pages the first one faulted in.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	defer debug.SetMemoryLimit(debug.SetMemoryLimit(scalingHeapLimit))
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var failure string
			for range scalingAttempts {
				small := measureScaling(c.prepare, c.n, time.Hour)
				big := measureScaling(c.prepare, 10*c.n, 2*scalingTimeBound*small.elapsed)
				if failure = scalingFailure(small, big, c.bytesOnly); failure == "" {
					t.Logf("n=%d: %v, %d bytes; 10n: %v, %d bytes", c.n, small.elapsed, small.bytes, big.elapsed, big.bytes)
					return
				}
				if strings.Contains(failure, "allocation") {
					break // the allocation is the same on every attempt
				}
			}
			t.Errorf("from n=%d to 10n, %s; the workload should be linear in n", c.n, failure)
		})
	}
}

const (
	scalingASCII   = "the quick brown fox jumps over the lazy dog |{G+57{g| 8,550/8,550\r\n"
	scalingUnicode = "héllo wörld ║ Mana ╚═╝ 👍🏽 é 🇳🇿 ✓ \r\n"
)

// scalingText is about n bytes of text made of whole repetitions of unit.
func scalingText(unit string, n int) string {
	return strings.Repeat(unit, max(1, n/len(unit)))
}

func stringCases(kind, unit string) []scalingCase {
	text := func(n int) string { return scalingText(unit, n) }
	return []scalingCase{
		{name: kind + "/count", n: 20_000, prepare: func(n int) func() {
			s := text(n)
			return func() { _ = StringCount(s) }
		}},
		{name: kind + "/index(after:) walk", n: 20_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for i := StringStartIndex(s); i < StringEndIndex(s); i = StringIndexAfter(s, i) {
				}
			}
		}},
		{name: kind + "/index(before:) walk", n: 2_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for i := StringEndIndex(s); i > StringStartIndex(s); i = StringIndexBefore(s, i) {
				}
			}
		}},
		{name: kind + "/index(_:offsetBy:)", n: 20_000, prepare: func(n int) func() {
			s := text(n)
			chars := StringCount(s)
			return func() { _ = StringIndexOffsetBy(s, 0, chars) }
		}},
		{name: kind + "/hasPrefix at every index", n: 2_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for i := 0; i < len(s); i = StringIndexAfter(s, i) {
					_ = StringHasPrefix(s, "{G")
					_ = SubstringHasPrefix(Substring{s, i, len(s)}, "{G")
					_ = SubstringHasPrefix(Substring{s, i, len(s)}, "wörld")
				}
			}
		}},
		{name: kind + "/hasSuffix at every index", n: 2_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for i := len(s); i > 0; i = StringIndexBefore(s, i) {
					_ = StringHasSuffix(s, "\r\n")
					_ = SubstringHasSuffix(Substring{s, 0, i}, "{g")
					_ = SubstringHasSuffix(Substring{s, 0, i}, "wörld")
				}
			}
		}},
		{name: kind + "/append(Character)", n: 20_000, prepare: func(n int) func() {
			chars := StringChars(text(n))
			return func() {
				s := ""
				for _, c := range chars {
					StringAppendCharacter(&s, c)
				}
			}
		}},
		{name: kind + "/append(String)", n: 20_000, prepare: func(n int) func() {
			count := len(text(n)) / len(unit)
			return func() {
				s := ""
				for range count {
					StringAppendString(&s, unit)
				}
			}
		}},
		{name: kind + "/+= one word at a time", n: 20_000, prepare: func(n int) func() {
			words := strings.Fields(text(n))
			return func() {
				s := ""
				for _, w := range words {
					StringAppend(&s, w)
					StringAppend(&s, " ")
				}
			}
		}},
		{name: kind + "/append(contentsOf:) Substring", n: 20_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				out := ""
				for i := 0; i < len(s); {
					j := StringIndexAfter(s, i)
					StringAppendContentsOf(&out, Substring{s, i, j})
					i = j
				}
			}
		}},
		{name: kind + "/dropFirst until empty", n: 20_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for sub := StringDropFirst(s); sub.Lo < sub.Hi; sub = SubstringDropFirst(sub) {
				}
			}
		}},
		{name: kind + "/dropLast until empty", n: 2_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for sub := StringDropLast(s); sub.Lo < sub.Hi; sub = SubstringDropLast(sub) {
				}
			}
		}},
		{name: kind + "/removeLast until empty", n: 2_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for s != "" {
					_ = StringRemoveLast(&s)
				}
			}
		}},
		{name: kind + "/split", n: 200_000, prepare: func(n int) func() {
			s := text(n)
			return func() { _ = StringSplitSeparator(s, " ") }
		}},
		{name: kind + "/contains(Character)", n: 50_000, prepare: func(n int) func() {
			s := text(n)
			return func() { _ = StringContains(s, "Z") }
		}},
		{name: kind + "/contains(String)", n: 500_000, prepare: func(n int) func() {
			s := text(n)
			return func() { _ = StringContainsString(s, "absent") }
		}},
		{name: kind + "/Substring slicing at every index", n: 20_000, prepare: func(n int) func() {
			s := text(n)
			return func() {
				for i := 0; i < len(s); {
					j := StringIndexAfter(s, i)
					_ = StringSubscriptRange(s, Range[int]{i, j})
					_ = StringSubscriptPartialRangeFrom(s, PartialRangeFrom[int]{i})
					_ = StringSubscriptPartialRangeUpTo(s, PartialRangeUpTo[int]{j})
					i = j
				}
			}
		}},
	}
}

func TestScalingStringASCII(t *testing.T) { runScaling(t, stringCases("ascii", scalingASCII)) }

func TestScalingStringUnicode(t *testing.T) { runScaling(t, stringCases("unicode", scalingUnicode)) }

func shuffledInts(n int) []int {
	xs := make([]int, n)
	for i := range xs {
		xs[i] = i
	}
	rand.New(rand.NewPCG(1, uint64(n))).Shuffle(n, func(i, j int) { xs[i], xs[j] = xs[j], xs[i] })
	return xs
}

func TestScalingArray(t *testing.T) {
	const fixed = 200 // calls of an O(n) operation per workload
	runScaling(t, []scalingCase{
		{name: "append", n: 50_000, bytesOnly: true, prepare: func(n int) func() {
			return func() {
				var xs []int
				for i := range n {
					SliceAppend(&xs, i)
				}
			}
		}},
		{name: "insert(at: endIndex)", n: 50_000, prepare: func(n int) func() {
			return func() {
				var xs []int
				for i := range n {
					SliceInsertAt(&xs, i, len(xs))
				}
			}
		}},
		{name: "removeLast until empty", n: 50_000, prepare: func(n int) func() {
			xs := shuffledInts(n)
			return func() {
				for len(xs) > 0 {
					_ = SliceRemoveLast(&xs)
				}
			}
		}},
		{name: "insert(at: 0) a fixed number of times", n: 2_000, prepare: func(n int) func() {
			xs := shuffledInts(n)
			return func() {
				for i := range fixed {
					SliceInsertAt(&xs, i, 0)
				}
			}
		}},
		{name: "removeFirst a fixed number of times", n: 2_000, prepare: func(n int) func() {
			xs := shuffledInts(n)
			return func() {
				for range fixed {
					_ = SliceRemoveFirst(&xs)
				}
			}
		}},
		{name: "contains", n: 500_000, prepare: func(n int) func() {
			xs := shuffledInts(n)
			return func() { _ = SliceContains(xs, -1) }
		}},
		{name: "sorted", n: 20_000, prepare: func(n int) func() {
			xs := shuffledInts(n)
			return func() { _ = SliceSorted(xs) }
		}},
		{name: "slicing at every index", n: 200_000, prepare: func(n int) func() {
			xs := shuffledInts(n)
			return func() {
				for i := range xs {
					_ = SliceSubscriptRange(xs, Range[int]{i, len(xs)})
					_ = SliceSubscriptPartialRangeFrom(xs, PartialRangeFrom[int]{i})
				}
			}
		}},
	})
}

func scalingKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "key" + strconv.Itoa(i)
	}
	return keys
}

func scalingSet(keys []string) map[string]struct{} {
	s := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		s[k] = struct{}{}
	}
	return s
}

func TestScalingDictionary(t *testing.T) {
	runScaling(t, []scalingCase{
		{name: "updateValue(_:forKey:)", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			return func() {
				var d map[string]int
				for i, k := range keys {
					_ = MapUpdateValueForKey(&d, i, k)
				}
			}
		}},
		{name: "lookup", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			d := map[string]int{}
			for i, k := range keys {
				d[k] = i
			}
			return func() {
				for _, k := range keys {
					_ = MapSubscriptDefault(d, k, 0)
				}
			}
		}},
		{name: "removeValue(forKey:) until empty", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			d := map[string]int{}
			for i, k := range keys {
				d[k] = i
			}
			return func() {
				for _, k := range keys {
					_ = MapRemoveValueForKey(&d, k)
				}
			}
		}},
		{name: "keys and values", n: 20_000, prepare: func(n int) func() {
			d := map[string]int{}
			for i, k := range scalingKeys(n) {
				d[k] = i
			}
			return func() {
				_ = MapKeys(d)
				_ = MapValues(d)
			}
		}},
		{name: "HMap set and get", n: 5_000, prepare: func(n int) func() {
			keys := make([][]int, n)
			for i := range keys {
				keys[i] = []int{i, i + 1}
			}
			return func() {
				var d HMap[[]int, int]
				for i, k := range keys {
					HMapSet(&d, k, &i)
				}
				for _, k := range keys {
					_ = HMapGet(d, k)
				}
			}
		}},
	})
}

func TestScalingSet(t *testing.T) {
	runScaling(t, []scalingCase{
		{name: "insert", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			return func() {
				var s map[string]struct{}
				for _, k := range keys {
					_ = SetInsert(&s, k)
				}
			}
		}},
		{name: "contains", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			s := scalingSet(keys)
			return func() {
				for _, k := range keys {
					_ = SetContains(s, k)
				}
			}
		}},
		{name: "remove until empty", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			s := scalingSet(keys)
			return func() {
				for _, k := range keys {
					_ = SetRemove(&s, k)
				}
			}
		}},
		{name: "formUnion two members at a time", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			return func() {
				var s map[string]struct{}
				for i := 0; i+1 < len(keys); i += 2 {
					SetFormUnion(&s, keys[i:i+2])
				}
			}
		}},
		{name: "formUnion(Set) two members at a time", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			parts := make([]map[string]struct{}, 0, n/2)
			for i := 0; i+1 < len(keys); i += 2 {
				parts = append(parts, scalingSet(keys[i:i+2]))
			}
			return func() {
				var s map[string]struct{}
				for _, p := range parts {
					SetFormUnionSet(&s, p)
				}
			}
		}},
		{name: "union", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			a, b := scalingSet(keys[:n/2]), scalingSet(keys[n/2:])
			return func() {
				_ = SetUnion(a, keys[n/2:])
				_ = SetUnionSet(a, b)
			}
		}},
		{name: "intersection", n: 20_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			a, b := scalingSet(keys[:2*n/3]), scalingSet(keys[n/3:])
			return func() {
				_ = SetIntersection(a, keys[n/3:])
				_ = SetIntersectionSet(a, b)
			}
		}},
		{name: "iteration", n: 20_000, prepare: func(n int) func() {
			s := scalingSet(scalingKeys(n))
			return func() { _ = SetElements(s) }
		}},
		{name: "HSet insert and contains", n: 5_000, prepare: func(n int) func() {
			keys := make([][]int, n)
			for i := range keys {
				keys[i] = []int{i, i + 1}
			}
			return func() {
				var s HSet[[]int]
				for _, k := range keys {
					_ = HSetInsert(&s, k)
				}
				for _, k := range keys {
					_ = HSetContains(s, k)
				}
			}
		}},
	})
}

type scalingLeaf struct {
	Name string
	Tags []string
	Opt  *int
}

type scalingTree struct {
	Leaves []scalingLeaf
	ByName map[string]scalingLeaf
	Grid   [][]int
}

func TestScalingCopy(t *testing.T) {
	runScaling(t, []scalingCase{
		{name: "Copy of nested values", n: 5_000, prepare: func(n int) func() {
			v := scalingTree{ByName: map[string]scalingLeaf{}}
			for i := range n {
				one := i
				leaf := scalingLeaf{Name: strconv.Itoa(i), Tags: []string{"a", "b"}, Opt: &one}
				v.Leaves = append(v.Leaves, leaf)
				v.ByName[leaf.Name] = leaf
				v.Grid = append(v.Grid, []int{i, i})
			}
			return func() { _ = Copy(v) }
		}},
		{name: "Copy of a growing array element by element", n: 5_000, prepare: func(n int) func() {
			return func() {
				var xs []scalingLeaf
				for i := range n {
					xs = append(xs, Copy(scalingLeaf{Name: "x", Tags: []string{strconv.Itoa(i)}}))
				}
				_ = slices.Clip(xs)
			}
		}},
	})
}
