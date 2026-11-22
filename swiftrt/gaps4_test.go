package swiftrt

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestSetToSetOperations(t *testing.T) {
	a := SetInitSlice[map[int]struct{}]([]int{1, 2, 3})
	b := SetInitSlice[map[int]struct{}]([]int{2, 3, 4})
	sorted := func(s map[int]struct{}) []int { return SetSorted(s) }
	if got := sorted(SetSubtractingSetSet(a, b)); !slices.Equal(got, []int{1}) {
		t.Errorf("subtracting = %v", got)
	}
	if got := sorted(SetIntersectionSetSet(a, b)); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("intersection = %v", got)
	}
	if got := sorted(SetSymmetricDifferenceSet(a, b)); !slices.Equal(got, []int{1, 4}) {
		t.Errorf("symmetricDifference = %v", got)
	}
	c := SetInitSlice[map[int]struct{}]([]int{1, 2, 3})
	SetSubtractSetSet(&c, b)
	if got := sorted(c); !slices.Equal(got, []int{1}) {
		t.Errorf("subtract = %v", got)
	}
	d := SetInitSlice[map[int]struct{}]([]int{1, 2, 3})
	SetFormIntersectionSet(&d, b)
	if got := sorted(d); !slices.Equal(got, []int{2, 3}) {
		t.Errorf("formIntersection = %v", got)
	}
	if !SetIsSubsetSet(d, a) || SetIsSubsetSet(a, d) || !SetIsSupersetSet(a, d) || SetIsSupersetSet(d, a) {
		t.Error("isSubset/isSuperset")
	}
	if SetIsDisjointSet(a, b) || !SetIsDisjointSet(a, SetInitSlice[map[int]struct{}]([]int{9})) {
		t.Error("isDisjoint")
	}
	if len(SetSubtractingSetSet(a, a)) != 0 || len(a) != 3 {
		t.Error("subtracting itself changed the receiver or kept elements")
	}
}

func TestMutexInitOptional(t *testing.T) {
	m := MutexInitOpt[*int](nil)
	if MutexWithLock(m, func(p **int) *int { return *p }) != nil {
		t.Error("initial state not nil")
	}
	r := MutexInitRef[func(string)](nil)
	if MutexWithLock(r, func(p *func(string)) func(string) { return *p }) != nil {
		t.Error("initial closure not nil")
	}
}

func TestDispatchGlobalQueue(t *testing.T) {
	q := OS_dispatch_queueGlobalQos(DispatchQoSQoSClassUtility)
	if q != OS_dispatch_queueGlobalQos(DispatchQoSQoSClassUtility) || q.Label != "com.apple.root.utility-qos" {
		t.Errorf("utility queue %q not shared", q.Label)
	}
	if OS_dispatch_queueGlobalQos(DispatchQoSQoSClassUnspecified) != OS_dispatch_queueGlobalQos(DispatchQoSQoSClassDefault) {
		t.Error(".unspecified is not the default queue")
	}
	// A concurrent queue runs the second job while the first is still blocked.
	release := make(chan struct{})
	done := make(chan struct{})
	OS_dispatch_queueAsyncExecute(q, func() { <-release })
	OS_dispatch_queueAsyncExecute(q, func() { close(done) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("global queue ran jobs serially")
	}
	close(release)
	if OS_dispatch_queueSyncExecute(q, func() int { return 4 }) != 4 {
		t.Error("sync on a global queue")
	}
}

// Expected strings come from swift-yyjson 0.6 (YYJSONEncoder, YYJSONSerialization) on macOS.
func TestYYJSONOptions(t *testing.T) {
	type P struct {
		B []int          `swift:"b"`
		A string         `swift:"a"`
		C map[string]int `swift:"c"`
	}
	v := P{B: []int{1, 2}, A: "x/y", C: map[string]int{}}
	enc := func(o YYJSONWriteOptions) string {
		e := YYJSONEncoderInit()
		YYJSONWriteOptionsFormUnion(&e.WriteOptions, o)
		return string(YYJSONEncoderEncode(e, v))
	}
	cases := []struct{ got, want string }{
		{enc(YYJSONWriteOptionsDefault), `{"b":[1,2],"a":"x/y","c":{}}`},
		{enc(YYJSONWriteOptionsPrettyPrinted), "{\n    \"b\": [\n        1,\n        2\n    ],\n    \"a\": \"x/y\",\n    \"c\": {}\n}"},
		{enc(YYJSONWriteOptionsIndentationTwoSpaces), "{\n  \"b\": [\n    1,\n    2\n  ],\n  \"a\": \"x/y\",\n  \"c\": {}\n}"},
		{enc(OptionSetUnion(YYJSONWriteOptionsEscapeSlashes, OptionSetUnion(YYJSONWriteOptionsSortedKeys, YYJSONWriteOptionsNewlineAtEnd))), "{\"a\":\"x\\/y\",\"b\":[1,2],\"c\":{}}\n"},
		{string(YYJSONSerializationDataWithJSONObjectOptions("a/b", WritingOptionsFragmentsAllowed)), `"a\/b"`},
		{string(YYJSONSerializationDataWithJSONObjectOptions(map[string]any{"k": []any{1, 2}}, OptionSetUnion(WritingOptionsPrettyPrinted, WritingOptionsNewlineAtEnd))), "{\n    \"k\": [\n        1,\n        2\n    ]\n}\n"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("%d: got %q, want %q", i, c.got, c.want)
		}
	}
	e := YYJSONEncoderInit()
	if r := YYJSONWriteOptionsInsert(&e.WriteOptions, YYJSONWriteOptionsSortedKeys); !r.F0 || YYJSONWriteOptionsInsert(&e.WriteOptions, YYJSONWriteOptionsSortedKeys).F0 {
		t.Error("insert reports membership wrongly")
	}

	if n, ok := YYJSONSerializationJsonObjectWithOptions([]byte("3"), ReadingOptionsFragmentsAllowed).(NSNumber); !ok || n.Description() != "3" {
		t.Errorf("fragment = %#v", n)
	}
	for _, f := range []func(){
		func() { YYJSONSerializationJsonObjectWithOptions([]byte("3"), ReadingOptionsMutableContainers) },
		func() { YYJSONSerializationDataWithJSONObjectOptions("a", YYJSONSerializationWritingOptions{}) },
	} {
		if err := Catch(f); err == nil || err.Error() != "Top-level JSON value must be an array or dictionary" {
			t.Errorf("fragment without .fragmentsAllowed: %v", err)
		}
	}
	err := Catch(func() { YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeOf[P](), nil) })
	if !errors.As(err, new(YYJSONError)) || err.Error() != "Empty content" {
		t.Errorf("empty input: %v", err)
	}
	got := YYJSONDecoderDecodeFrom(YYJSONDecoderInit(), TypeOf[P](), []byte(`{"b":[3],"a":"q","c":{"z":1}}`))
	if got.A != "q" || !slices.Equal(got.B, []int{3}) || got.C["z"] != 1 {
		t.Errorf("decoded %+v", got)
	}
}

func TestAsyncStream(t *testing.T) {
	pair := AsyncStreamMakeStreamOf(TypeOf[int]())
	stream, cont := pair.F0, pair.F1
	ContinuationYield(cont, 1)
	ContinuationYield(cont, 2)
	it := AsyncStreamMakeAsyncIterator(stream)
	if v := AsyncStreamIteratorNext(&it); v == nil || *v != 1 {
		t.Fatalf("first = %v", v)
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		ContinuationYield(cont, 3)
		ContinuationFinish(cont)
		ContinuationYield(cont, 4)
	}()
	var got []int
	for v := AsyncStreamIteratorNext(&it); v != nil; v = AsyncStreamIteratorNext(&it) {
		got = append(got, *v)
	}
	if !slices.Equal(got, []int{2, 3}) || AsyncStreamIteratorNext(&it) != nil {
		t.Errorf("drained %v", got)
	}

	// Cancelling the task waiting in next() ends the stream.
	pair2 := AsyncStreamMakeStreamOf(TypeOf[int]())
	waiting := make(chan struct{})
	task := TaskInitOperation(func() *int {
		it := AsyncStreamMakeAsyncIterator(pair2.F0)
		close(waiting)
		return AsyncStreamIteratorNext(&it)
	})
	<-waiting
	time.Sleep(10 * time.Millisecond)
	TaskCancel(task)
	if v := TaskValue(task).(*int); v != nil {
		t.Errorf("cancelled next = %v", *v)
	}
	ContinuationYield(pair2.F1, 5)
	it2 := AsyncStreamMakeAsyncIterator(pair2.F0)
	if AsyncStreamIteratorNext(&it2) != nil {
		t.Error("stream accepted a yield after cancellation finished it")
	}
}

type recordingLogHandler struct {
	name     string
	LogLevel LoggerLevel `swift:"logLevel"`
	mu       *sync.Mutex
	out      *[]string
}

func (h recordingLogHandler) Log(e LogEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.out = append(*h.out, h.name+" "+Str(e.Level)+" "+e.Message+" "+LogEventSource(e))
}

// Expected lines come from swift-log 1.16.1 on macOS.
func TestLogging(t *testing.T) {
	var mu sync.Mutex
	var out []string
	a := recordingLogHandler{name: "a", LogLevel: LevelDebug, mu: &mu, out: &out}
	b := recordingLogHandler{name: "b", LogLevel: LevelWarning, mu: &mu, out: &out}
	l := LoggerInitLabelFactory("x", func(string) LogHandler { return MultiplexLogHandlerInitSlice([]LogHandler{a, b}) })
	if l.LogLevel != LevelDebug {
		t.Errorf("multiplex level = %v", Str(l.LogLevel))
	}
	LoggerTraceFileLine(l, "t", "main/main.swift", 13)
	LoggerDebugFileLine(l, "d", "main/main.swift", 13)
	LoggerWarningFileLine(l, "w", "main/main.swift", 13)
	l.LogLevel = LevelError
	LoggerWarningFileLine(l, "w2", "main/main.swift", 16)
	LoggerErrorFileLine(l, "e2", "nofile", 16)
	want := []string{"a debug d main", "a warning w main", "b warning w main", "a error e2 n/a", "b error e2 n/a"}
	if !slices.Equal(out, want) {
		t.Errorf("logged %q, want %q", out, want)
	}
	if a.LogLevel != LevelDebug {
		t.Error("setting the logger's level changed the caller's handler value")
	}

	n := LoggerInitLabelFactory("n", func(string) LogHandler { return SwiftLogNoOpLogHandlerInit() })
	if Str(n.LogLevel) != "critical" || LevelRawValue(LevelNotice) != "notice" || *LevelOptInitRawValue("error") != LevelError || LevelOptInitRawValue("Error") != nil {
		t.Error("level names")
	}

	mv := MetadataValueDictionary(LoggerMetadata{
		"k": MetadataValueArray([]LoggerMetadataValue{MetadataValueString("a\"b"), MetadataValueStringConvertible(3)}),
		"z": MetadataValueString("q"),
	})
	if got := Str(mv); got != `["k": ["a\"b", "3"], "z": "q"]` {
		t.Errorf("metadata description %s", got)
	}
	if Str(MetadataValueDictionary(nil)) != "[:]" || Str(MetadataValueArray(nil)) != "[]" {
		t.Error("empty metadata descriptions")
	}
	if got := swiftDebugQuote("it's\t\x01\x7fé"); got != `"it\'s\t\u{01}\u{7F}é"` {
		t.Errorf("debug quote %s", got)
	}

	if err := Catch(func() {
		defer func() {
			if r := recover(); r != nil {
				panic(Throw(errors.New("trapped")))
			}
		}()
		loggingSystem.mu.Lock()
		factory, initialized := loggingSystem.factory, loggingSystem.initialized
		loggingSystem.mu.Unlock()
		defer func() {
			loggingSystem.mu.Lock()
			loggingSystem.factory, loggingSystem.initialized = factory, initialized
			loggingSystem.mu.Unlock()
		}()
		LoggingSystemBootstrap(func(string) LogHandler { return a })
		if LoggerInitLabel("y").LogLevel != LevelDebug {
			t.Error("bootstrapped factory not used")
		}
		LoggingSystemBootstrap(func(string) LogHandler { return b })
	}); err == nil || err.Error() != "trapped" {
		t.Errorf("second bootstrap: %v", err)
	}
}
