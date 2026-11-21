package swiftrt

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Expected strings come from Rainbow 4 on macOS with outputTarget = .console.
func TestRainbowApply(t *testing.T) {
	esc := func(s string) string { return strings.ReplaceAll(s, "\x1b", "ESC") }
	red := func(s string) string { return rainbowApply(s, []byte{31}, true, true) }
	plainRed := func(s string) string { return rainbowApply(s, []byte{31}, true, false) }
	cases := []struct{ got, want string }{
		{red("hello"), "ESC[31mhelloESC[0m"},
		{rainbowApply(red("hello"), []byte{1}, true, true), "ESC[31;1mhelloESC[0m"},
		{rainbowApply("hello", []byte{90}, true, true), "ESC[90mhelloESC[0m"},
		{red("a") + "b", "ESC[31maESC[0mb"},
		{rainbowApply(red("a")+"b", []byte{33}, true, true), "ESC[31maESC[0mESC[33mbESC[0m"},
		{rainbowApply(red("a")+"b", []byte{1}, true, true), "ESC[31;1maESC[0mESC[1mbESC[0m"},
		{rainbowApply("x\x1b[0mY", []byte{36}, true, true), "ESC[36mxESC[0mESC[36mYESC[0m"},
		{red(""), ""},
		{red("\x1b[38;5;200mz"), "ESC[31mzESC[0m"},
		{rainbowApply("\x1b[1;31mz\x1b[0m", []byte{33}, true, true), "ESC[33;1mzESC[0m"},
		{plainRed("a") + "b", "ab"},
		{rainbowApply("\x1b[31mq\x1b[0m", []byte{1}, true, false), "q"},
		{rainbowApply("\x1b[31mq\x1b[0m", []byte{1}, false, true), "ESC[31mqESC[0m"},
	}
	for i, c := range cases {
		if esc(c.got) != c.want {
			t.Errorf("%d: got %q, want %q", i, esc(c.got), c.want)
		}
	}
}

// Expected output comes from swift-yyjson's YYJSONSerialization.
func TestYYJSONSerializationData(t *testing.T) {
	obj := map[string]any{"s": "a/b\"\\\n\x01é", "i": 3, "d": 1.0, "f": 0.1, "big": 1e21, "small": 1e-7,
		"neg": -2.5, "arr": []any{1, true, NSNull{}}, "u": uint64(18446744073709551615), "e": map[string]any{}}
	want := `{"arr":[1,true,null],"big":1e21,"d":1.0,"e":{},"f":0.1,"i":3,"neg":-2.5,"s":"a\/b\"\\\n\u0001é","small":1e-7,"u":18446744073709551615}`
	if got := string(YYJSONSerializationDataWithJSONObject(obj)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	doubles := map[float64]string{1e20: "100000000000000000000.0", 123: "123.0", 0.000001: "0.000001",
		1.5e300: "1.5e300", 9007199254740993: "9007199254740992.0", 5e-324: "5e-324", 1234.5678: "1234.5678"}
	for f, want := range doubles {
		if got := yyjsonDouble(f); got != want {
			t.Errorf("yyjsonDouble(%v) = %s, want %s", f, got, want)
		}
	}
	mustPanic(t, "Top-level", func() { YYJSONSerializationDataWithJSONObject("x") })
	mustPanic(t, "NaN", func() { YYJSONSerializationDataWithJSONObject([]any{zeroFloat() / zeroFloat()}) })
}

func zeroFloat() float64 { return 0 }

// Expected casts come from swift-yyjson's jsonObject(with:) on macOS.
func TestYYJSONSerializationJsonObject(t *testing.T) {
	v := YYJSONSerializationJsonObjectWith([]byte(`{"a":1,"b":0.1,"c":true,"d":null,"e":[{"x":1}],"f":18446744073709551615,"g":1.0}`))
	m := *As[map[string]any](v)
	type row struct {
		i    *int
		d    *float64
		b    *bool
		u    *uint64
		desc string
	}
	got := func(k string) row {
		x := m[k]
		return row{As[int](x), As[float64](x), As[bool](x), As[uint64](x), Str(x)}
	}
	check := func(k string, i, d, b, u bool, desc string) {
		t.Helper()
		r := got(k)
		if (r.i != nil) != i || (r.d != nil) != d || (r.b != nil) != b || (r.u != nil) != u || r.desc != desc {
			t.Errorf("%s: int %v double %v bool %v uint64 %v desc %q", k, r.i != nil, r.d != nil, r.b != nil, r.u != nil, r.desc)
		}
	}
	check("a", true, true, true, true, "1")
	check("b", false, true, false, false, "0.1")
	check("c", true, true, true, true, "1")
	check("d", false, false, false, false, "<null>")
	check("f", false, false, false, true, "18446744073709551615")
	check("g", true, true, true, true, "1")
	if xs := As[[]map[string]int](m["e"]); xs == nil || (*xs)[0]["x"] != 1 {
		t.Errorf("[[String: Int]] cast = %v", xs)
	}
	if As[[]int]([]any{NSNumberInt64(1), "a"}) != nil {
		t.Error("[1, \"a\"] as? [Int] should be nil")
	}
	for _, bad := range []string{"{", "[1,]", "1", "[1] x", "[NaN]"} {
		if TryFlat(func() *any { return Some(YYJSONSerializationJsonObjectWith([]byte(bad))) }) != nil {
			t.Errorf("%s parsed", bad)
		}
	}
}

func TestGameJSONCoders(t *testing.T) {
	type P struct {
		A []int          `swift:"a"`
		B string         `swift:"b"`
		C map[string]int `swift:"c"`
	}
	e := GameJSONEncoderInit()
	e.OutputFormatting = GameJSONEncoderOutputFormatting{1 | 2}
	want := "{\n  \"a\": [\n    1,\n    2\n  ],\n  \"b\": \"x\\/y\",\n  \"c\": {}\n}"
	if got := string(GameJSONEncoderEncode(e, P{[]int{1, 2}, "x/y", map[string]int{}})); got != want {
		t.Errorf("got %s", got)
	}
	e.OutputFormatting = GameJSONEncoderOutputFormatting{4}
	if got := string(GameJSONEncoderEncode(e, P{nil, "x/y", nil})); got != `{"a":[],"b":"x/y","c":{}}` {
		t.Errorf("got %s", got)
	}
	p := GameJSONDecoderDecodeFrom(GameJSONDecoderInit(), TypeOf[P](), []byte(`{"a":[3],"b":"q","c":{"k":2}}`))
	if p.B != "q" || p.A[0] != 3 || p.C["k"] != 2 {
		t.Errorf("decoded %+v", p)
	}
	mustPanic(t, "dataCorrupted", func() {
		GameJSONDecoderDecodeFrom(GameJSONDecoderInit(), TypeOf[map[string]float64](), []byte(`{"a":1,}`))
	})
}

func TestURLSessionDataFor(t *testing.T) {
	var seen *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("X-RateLimit-Remaining", "4")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"retry_after":1.5}`)
	}))
	defer srv.Close()
	req := URLRequestInitUrl(*URLOptInitString(srv.URL + "/api/x"))
	copyBefore := req
	req.HttpMethod = Some("POST")
	URLRequestSetValueForHTTPHeaderField(&req, Some("application/json"), "Content-Type")
	URLRequestSetValueForHTTPHeaderField(&req, Some("Bot t"), "authorization")
	URLRequestSetValueForHTTPHeaderField(&req, Some("Bot u"), "Authorization")
	req.HttpBody = Some([]byte(`{"a":1}`))
	if URLRequestValueForHTTPHeaderField(copyBefore, "Authorization") != nil {
		t.Error("header write leaked into a copy")
	}
	config := NSURLSessionConfigurationCObjc
	config.TimeoutIntervalForRequest = 5
	session := NSURLSessionInitConfiguration(config)
	defer NSURLSessionCObjc(session)
	res := NSURLSessionDataFor(session, req)
	http := *As[HTTPURLResponse](res.F1)
	if NSHTTPURLResponseCObjc(http) != 429 || string(res.F0) != `{"retry_after":1.5}` {
		t.Errorf("status %d body %s", http.StatusCode, res.F0)
	}
	if v := HTTPURLResponseValueForHTTPHeaderField(http, "x-ratelimit-remaining"); v == nil || *v != "4" {
		t.Errorf("header %v", v)
	}
	if seen.Method != "POST" || seen.Header.Get("Authorization") != "Bot u" || seen.Header.Get("Content-Type") != "application/json" || body != `{"a":1}` {
		t.Errorf("request %s %v %q", seen.Method, seen.Header, body)
	}
	srv.Close()
	err := Catch(func() { NSURLSessionDataFor(NSURLSessionInitConfiguration(config), req) })
	var urlErr URLError
	if !errors.As(err, &urlErr) || urlErr.Code != URLErrorCannotConnectToHost {
		t.Errorf("closed server error = %v", err)
	}
}

func TestURLComponents(t *testing.T) {
	// Expected values come from Foundation's URLComponents(string:) on macOS.
	cases := []struct {
		in   string
		ok   bool
		host *string
		path string
	}{
		{"https://Discord.com/api/webhooks/1/a%20b?x=1", true, Some("Discord.com"), "/api/webhooks/1/a b"},
		{"discord.com/api", true, nil, "discord.com/api"},
		{"http://[::1]:80/x", true, Some("[::1]"), "/x"},
		{"not a url", true, nil, "not a url"},
		{"", true, nil, ""},
		{"http:///p", true, Some(""), "/p"},
		{"https://user@h.com:8/p", true, Some("h.com"), "/p"},
		{"mailto:a@b", true, nil, "a@b"},
		{"https://h.com/%zz", true, Some("h.com"), "/%zz"},
		{"http://exa mple.com/x", false, nil, ""},
		{"ht tp://x/y", false, nil, ""},
		{"http://h.com:abc/", false, nil, ""},
		{"://x", true, nil, "://x"},
	}
	for _, c := range cases {
		got := URLComponentsOptInitString(c.in)
		if (got != nil) != c.ok {
			t.Errorf("%q: ok = %v", c.in, got != nil)
			continue
		}
		if got == nil {
			continue
		}
		h := URLComponentsHost(*got)
		if (h == nil) != (c.host == nil) || h != nil && *h != *c.host || URLComponentsPath(*got) != c.path {
			t.Errorf("%q: host %v path %q", c.in, h, URLComponentsPath(*got))
		}
	}
}

func TestTaskCancellationAndGroups(t *testing.T) {
	if TaskIsCancelled() {
		t.Fatal("cancelled outside a task")
	}
	started := make(chan struct{})
	var sawCancel, handlerRan atomic.Bool
	task := TaskInitOperation(func() {
		WithTaskCancellationHandlerOperationOnCancel(func() struct{} {
			close(started)
			err := Catch(func() { TaskSleepFor(DurationSecondsInt(10)) })
			sawCancel.Store(errors.As(err, new(CancellationError)) && TaskIsCancelled())
			return struct{}{}
		}, func() { handlerRan.Store(true) })
	})
	<-started
	TaskCancel(task)
	TaskValue(task)
	if !sawCancel.Load() || !handlerRan.Load() {
		t.Errorf("sleep cancelled %v, handler ran %v", sawCancel.Load(), handlerRan.Load())
	}

	var finished atomic.Int32
	result := WithThrowingTaskGroupOfBody(TypeOf[int](), func(group *any) int {
		ThrowingTaskGroupAddTaskOperation(group, func() int { return 7 })
		ThrowingTaskGroupAddTaskOperation(group, func() int {
			err := Catch(func() { TaskSleepFor(DurationSecondsInt(10)) })
			if err != nil {
				finished.Add(1)
			}
			return 0
		})
		first := ThrowingTaskGroupNext(group)
		ThrowingTaskGroupCancelAll(*group)
		return (*first).(int)
	})
	if result != 7 || finished.Load() != 1 {
		t.Errorf("group result %v, cancelled children %d", result, finished.Load())
	}

	err := Catch(func() {
		WithThrowingTaskGroupOfBody(TypeOf[struct{}](), func(group *any) {
			ThrowingTaskGroupAddTaskOperation(group, func() { panic(Throw(errors.New("boom"))) })
			ThrowingTaskGroupNext(group)
		})
	})
	if err == nil || err.Error() != "boom" {
		t.Errorf("child error = %v", err)
	}
}

func TestTaskSleepOutsideTask(t *testing.T) {
	start := time.Now()
	TaskSleepFor(DurationMillisecondsInt(5))
	if time.Since(start) < 5*time.Millisecond {
		t.Error("slept too short")
	}
	mustPanic(t, "not a Swift Duration", func() { TaskSleepFor(5) })
}
