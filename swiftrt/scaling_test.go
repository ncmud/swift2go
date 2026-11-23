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
// any machine, and a workload whose quadratic failure would be a copy is judged by it alone. Time
// catches quadratic work that allocates nothing. A timed round repeats the workload until it takes
// milliseconds at n, and the time at each size is the median of several rounds with the collector
// held off, so a preemption or page-fault burst on a busy machine slows one round without moving
// the median. A workload fails on time only when every attempt exceeds the bound. The maps and sets
// stay small enough at 10n to stay in the CPU's caches: a lookup in a larger one waits on memory,
// whose latency grows with the table and with what else the machine runs.

const (
	// scalingTimeBound is the largest time ratio between 10n and n that passes.
	scalingTimeBound = 35
	// scalingBytesBound is the largest ratio of bytes allocated that passes. Slices and maps grow
	// by doubling, so a linear workload can allocate up to about twenty times as much at 10n.
	scalingBytesBound = 30
	// scalingMinBytes is the allocation at 10n below which the byte ratio is not checked.
	scalingMinBytes = 64 << 10
	// scalingRoundTime is the least time a round at n takes: the workload repeats until it does.
	scalingRoundTime = 2 * time.Millisecond
	scalingMaxReps   = 10_000
	scalingHeapLimit = 1 << 30
	scalingRounds    = 7
	scalingAttempts  = 3
)

// scalingCase is one workload. prepare builds its input of size n outside the timing and returns
// the work to time, which a round runs repeatedly.
type scalingCase struct {
	name    string
	n       int
	prepare func(n int) func()
	// consumes marks work that uses up or changes its input, so each run needs a fresh prepare.
	consumes bool
	// bytesOnly judges the workload by its allocation alone, for an operation whose quadratic
	// failure would copy its storage on every call. Growing an array to 10n spends most of its time
	// faulting in the fresh pages its backing arrays take, which a shared CI runner charges
	// unevenly, while at n the pages from earlier runs are reused.
	bytesOnly bool
}

type scalingCost struct {
	elapsed time.Duration // the median time of a round
	bytes   uint64        // the least allocation of one run
}

// scalingReps is how many runs of the workload at n make a round of at least scalingRoundTime.
func scalingReps(c scalingCase) int {
	once := time.Duration(1<<63 - 1)
	for range 3 {
		once = min(once, scalingRound(c, c.n, 1))
	}
	return int(min(scalingMaxReps, scalingRoundTime/max(once, 1)+1))
}

// scalingRound times reps runs of the workload at size n, preparing its input outside the timing.
func scalingRound(c scalingCase, n, reps int) time.Duration {
	if !c.consumes {
		work := c.prepare(n)
		runtime.GC()
		start := time.Now()
		for range reps {
			work()
		}
		return time.Since(start)
	}
	runtime.GC()
	var total time.Duration
	for range reps {
		work := c.prepare(n)
		start := time.Now()
		work()
		total += time.Since(start)
	}
	return total
}

// measureScaling is the median time of a round of reps runs at size n, and the least allocation of
// one run. It stops after a round that takes longer than limit, which only a workload failing its
// bound does, and judges by the rounds it has.
func measureScaling(c scalingCase, n, reps int, limit time.Duration) scalingCost {
	cost := scalingCost{bytes: 1<<64 - 1}
	var before, after runtime.MemStats
	for range 2 {
		work := c.prepare(n)
		runtime.GC()
		runtime.ReadMemStats(&before)
		work()
		runtime.ReadMemStats(&after)
		cost.bytes = min(cost.bytes, after.TotalAlloc-before.TotalAlloc)
	}
	if c.bytesOnly {
		return cost
	}
	rounds := make([]time.Duration, 0, scalingRounds)
	for range scalingRounds {
		rounds = append(rounds, scalingRound(c, n, reps))
		if rounds[len(rounds)-1] > limit {
			break
		}
	}
	slices.Sort(rounds)
	cost.elapsed = rounds[len(rounds)/2]
	return cost
}

// scalingFailure describes why a workload's growth from small to big exceeds the bounds, or is
// empty when it does not.
func scalingFailure(small, big scalingCost, bytesOnly bool) string {
	var reasons []string
	if r := float64(big.elapsed) / float64(max(small.elapsed, 1)); r > scalingTimeBound && !bytesOnly {
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
			reps := 1
			if !c.bytesOnly {
				reps = scalingReps(c)
			}
			var failure string
			for range scalingAttempts {
				small := measureScaling(c, c.n, reps, time.Hour)
				big := measureScaling(c, 10*c.n, reps, 2*scalingTimeBound*small.elapsed)
				if failure = scalingFailure(small, big, c.bytesOnly); failure == "" {
					t.Logf("n=%d, %d runs a round: %v, %d bytes; 10n: %v, %d bytes", c.n, reps, small.elapsed, small.bytes, big.elapsed, big.bytes)
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
		{name: kind + "/removeLast until empty", n: 2_000, consumes: true, prepare: func(n int) func() {
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
		{name: "removeLast until empty", n: 50_000, consumes: true, prepare: func(n int) func() {
			xs := make([]int, n)
			return func() {
				for len(xs) > 0 {
					_ = SliceRemoveLast(&xs)
				}
			}
		}},
		{name: "insert(at: 0) a fixed number of times", n: 2_000, consumes: true, prepare: func(n int) func() {
			xs := make([]int, n)
			return func() {
				for i := range fixed {
					SliceInsertAt(&xs, i, 0)
				}
			}
		}},
		{name: "removeFirst a fixed number of times", n: 2_000, consumes: true, prepare: func(n int) func() {
			xs := make([]int, n)
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
		{name: "updateValue(_:forKey:)", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			return func() {
				var d map[string]int
				for i, k := range keys {
					_ = MapUpdateValueForKey(&d, i, k)
				}
			}
		}},
		{name: "lookup", n: 2_000, prepare: func(n int) func() {
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
		{name: "removeValue(forKey:) until empty", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			d := make(map[string]int, n)
			return func() {
				for i, k := range keys {
					d[k] = i
				}
				for _, k := range keys {
					_ = MapRemoveValueForKey(&d, k)
				}
			}
		}},
		{name: "keys and values", n: 2_000, prepare: func(n int) func() {
			d := map[string]int{}
			for i, k := range scalingKeys(n) {
				d[k] = i
			}
			return func() {
				_ = MapKeys(d)
				_ = MapValues(d)
			}
		}},
		{name: "HMap set and get", n: 1_000, prepare: func(n int) func() {
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
		{name: "insert", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			return func() {
				var s map[string]struct{}
				for _, k := range keys {
					_ = SetInsert(&s, k)
				}
			}
		}},
		{name: "contains", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			s := scalingSet(keys)
			return func() {
				for _, k := range keys {
					_ = SetContains(s, k)
				}
			}
		}},
		{name: "remove until empty", n: 2_000, consumes: true, prepare: func(n int) func() {
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
		{name: "union", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			a, b := scalingSet(keys[:n/2]), scalingSet(keys[n/2:])
			return func() {
				_ = SetUnion(a, keys[n/2:])
				_ = SetUnionSet(a, b)
			}
		}},
		{name: "intersection", n: 2_000, prepare: func(n int) func() {
			keys := scalingKeys(n)
			a, b := scalingSet(keys[:2*n/3]), scalingSet(keys[n/3:])
			return func() {
				_ = SetIntersection(a, keys[n/3:])
				_ = SetIntersectionSet(a, b)
			}
		}},
		{name: "iteration", n: 2_000, prepare: func(n int) func() {
			s := scalingSet(scalingKeys(n))
			return func() { _ = SetElements(s) }
		}},
		{name: "HSet insert and contains", n: 1_000, prepare: func(n int) func() {
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
		{name: "Copy of nested values", n: 1_000, prepare: func(n int) func() {
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
		{name: "Copy of a growing array element by element", n: 1_000, prepare: func(n int) func() {
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

// castRoot, castMid and castLeaf are a class hierarchy as the emitter writes one: each subclass
// embeds its superclass, the root records the most-derived object, and a subclass lists its
// superclass parts.
type castRoot struct{ SwiftSub any }

func (c *castRoot) SwiftDynamic() any {
	if c != nil && c.SwiftSub != nil {
		return c.SwiftSub
	}
	return c
}

type castMid struct{ castRoot }

func (c *castMid) SwiftBase(i int) any {
	switch i {
	case 0:
		return &c.castRoot
	}
	return nil
}

type castLeaf struct{ castMid }

func (c *castLeaf) SwiftBase(i int) any {
	switch i {
	case 0:
		return &c.castMid
	case 1:
		return &c.castRoot
	}
	return nil
}

type castOther struct{ castRoot }

func (c *castOther) SwiftBase(i int) any {
	switch i {
	case 0:
		return &c.castRoot
	}
	return nil
}

// TestCastAllocations checks that a cast between class references allocates nothing, whether it
// succeeds or fails, and that `as?` to a class through AsRef does not box its result.
func TestCastAllocations(t *testing.T) {
	leaf := &castLeaf{}
	leaf.SwiftSub = leaf
	other := &castOther{}
	other.SwiftSub = other
	var root, otherRoot any = &leaf.castRoot, &other.castRoot
	plainRoot := any(&castRoot{})
	if AsRef[*castLeaf](root) != leaf || AsRef[*castMid](root) != &leaf.castMid || AsRef[*castOther](root) != nil {
		t.Fatal("AsRef cast a class reference wrongly")
	}
	if !Is[*castMid](root) || Is[*castMid](otherRoot) || As[*castOther](root) != nil {
		t.Fatal("Is or As cast a class reference wrongly")
	}
	var nilRoot *castRoot
	if AsRef[*castLeaf](nilRoot) != nil || Is[*castLeaf](nilRoot) {
		t.Fatal("a nil class reference cast to a subclass")
	}
	for name, cast := range map[string]func(){
		"AsRef to the dynamic class":  func() { _ = AsRef[*castLeaf](root) },
		"AsRef to a middle class":     func() { _ = AsRef[*castMid](root) },
		"AsRef to an unrelated class": func() { _ = AsRef[*castOther](root) },
		"As to an unrelated class":    func() { _ = As[*castOther](root) },
		"Is to an unrelated class":    func() { _ = Is[*castLeaf](otherRoot) },
		"Is from a plain root object": func() { _ = Is[*castLeaf](plainRoot) },
		"MustAs to the dynamic class": func() { _ = MustAs[*castLeaf](root) },
	} {
		if n := testing.AllocsPerRun(1000, cast); n != 0 {
			t.Errorf("%s: %v allocations, want 0", name, n)
		}
	}
}

// castFixtures are class references for the cast tests: a leaf seen through its root, an
// unrelated subclass seen through the root, and a plain root object, which has no superclass
// parts, as an NPC is a plain GameCharacter in ncmud.
func castFixtures() (root, otherRoot, plainRoot any) {
	leaf := &castLeaf{}
	leaf.SwiftSub = leaf
	other := &castOther{}
	other.SwiftSub = other
	return &leaf.castRoot, &other.castRoot, &castRoot{}
}

// TestCastFailureCost checks that a failed cast between class references costs about what a
// successful one does, with no lookup on the failing path: a plain root object fails at once, and a
// subclass only walks its own superclass parts.
func TestCastFailureCost(t *testing.T) {
	if raceDetector {
		t.Skip("the race detector's instrumentation dominates the timings; CI runs these tests without -race")
	}
	if testing.Short() {
		t.Skip("timing tests are skipped in short mode")
	}
	root, otherRoot, plainRoot := castFixtures()
	const n = 200_000
	timed := func(cast func()) time.Duration {
		start := time.Now()
		for range n {
			cast()
		}
		return time.Since(start)
	}
	var sink bool
	succeed := func() { sink = sink != Is[*castLeaf](root) }
	for _, c := range []struct {
		name  string
		bound float64 // in successful casts
		cast  func()
	}{
		{"a plain root to a subclass", 3, func() { sink = sink != Is[*castLeaf](plainRoot) }},
		{"a plain root to a subclass through AsRef", 3, func() { sink = sink != (AsRef[*castMid](plainRoot) != nil) }},
		// The cast walks the subclass's superclass parts, one method call each.
		{"a subclass to an unrelated subclass", 6, func() { sink = sink != Is[*castLeaf](otherRoot) }},
	} {
		// The failing and the successful casts are timed in alternation, and the ratio is the median
		// of the pairs', so a busy machine slows both sides of a pair or only one pair.
		const pairs = 9
		ratios := make([]float64, pairs)
		var success, failure time.Duration
		for i := range ratios {
			success, failure = timed(succeed), timed(c.cast)
			ratios[i] = float64(failure) / float64(max(success, 1))
		}
		slices.Sort(ratios)
		if r := ratios[pairs/2]; r > c.bound {
			t.Errorf("casting %s failed in %.1fx the time of a successful cast (%v against %v per cast)", c.name, r, failure/n, success/n)
		}
	}
}

// BenchmarkClassCast measures the casts the game server runs most: isPC on an NPC, which fails
// on a plain root object, and on a player, which succeeds.
func BenchmarkClassCast(b *testing.B) {
	root, otherRoot, plainRoot := castFixtures()
	for _, c := range []struct {
		name string
		v    any
	}{{"success", root}, {"plain-root-failure", plainRoot}, {"sibling-failure", otherRoot}} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = Is[*castLeaf](c.v)
			}
		})
	}
}

// scalingDepth reads tl at each of depth nested calls, so a read that walks the stack costs
// quadratic time in depth.
func scalingDepth(tl *TaskLocal[int], depth int) int {
	if depth == 0 {
		return 0
	}
	return TaskLocalGet(tl) + scalingDepth(tl, depth-1)
}

func TestScalingTaskLocal(t *testing.T) {
	runScaling(t, []scalingCase{
		{name: "TaskLocal.get at every level of a deepening stack", n: 1_000, prepare: func(n int) func() {
			tl := TaskLocalInitWrappedValue(0)
			return func() {
				TaskLocalWithValueOperation(tl, 1, func() int { return scalingDepth(tl, n) })
			}
		}},
	})
}
