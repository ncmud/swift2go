package swiftrt

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStringDecodingReplacesMaximalSubparts(t *testing.T) {
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte{0xFF, 0xFF}, "��"},
		{[]byte{0xE2, 0x82}, "�"},
		{[]byte{0xE2, 0x82, 0x41}, "�A"},
		{[]byte{0xF0, 0x80, 0x80}, "���"},
		{[]byte{0xED, 0xA0, 0x80}, "���"},
		{[]byte{0xC0, 0xAF}, "��"},
		{[]byte{0xF4, 0x90, 0x80, 0x80}, "����"},
		{[]byte{0x61, 0xF1, 0x80, 0x80, 0xE1, 0x80, 0xC2, 0x62}, "a���b"},
		{[]byte("héllo"), "héllo"},
	}
	for _, c := range cases {
		if got := StringInitDecodingAs(c.in, nil); got != c.want {
			t.Errorf("decoding % x = %q, want %q", c.in, got, c.want)
		}
	}
	if got := StringInitCString([]byte{'h', 'i', 0, 'x'}); got != "hi" {
		t.Errorf("cString = %q", got)
	}
	var p any = []byte("ab\x00")
	if got := StringInitCString(p); got != "ab" {
		t.Errorf("cString(any) = %q", got)
	}
}

func cArg(s string) *any {
	var v any = s
	return &v
}

func TestPosixEnvironmentAndFiles(t *testing.T) {
	const name = "SWIFTRT_GAPS5_X"
	os.Unsetenv(name)
	defer os.Unsetenv(name)
	if CFGetenv(cArg(name)) != nil {
		t.Fatal("getenv of unset variable")
	}
	if CFSetenv(cArg(name), cArg("a"), 0) != 0 || StringInitCString(*CFGetenv(cArg(name))) != "a" {
		t.Error("setenv a")
	}
	if CFSetenv(cArg(name), cArg("b"), 0) != 0 || StringInitCString(*CFGetenv(cArg(name))) != "a" {
		t.Error("setenv without overwrite replaced the value")
	}
	if CFSetenv(cArg(name), cArg("c"), 1) != 0 || StringInitCString(*CFGetenv(cArg(name))) != "c" {
		t.Error("setenv with overwrite")
	}
	if CFSetenv(cArg(""), cArg("a"), 1) != -1 || CFSetenv(cArg("A=B"), cArg("a"), 1) != -1 {
		t.Error("setenv accepted an invalid name")
	}

	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if CFUnlink(cArg(a)) != -1 || CFRename(cArg(a), cArg(b)) != -1 {
		t.Error("unlink/rename of a missing file succeeded")
	}
	_ = os.WriteFile(a, []byte("x"), 0o600)
	if CFRename(cArg(a), cArg(b)) != 0 {
		t.Error("rename failed")
	}
	if CFUnlink(cArg(b)) != 0 {
		t.Error("unlink failed")
	}
	if CFUnlink(cArg(dir)) != -1 {
		t.Error("unlink removed a directory")
	}
	if got := StringInitCString(*CFStrerror(2)); got != "No such file or directory" {
		t.Errorf("strerror(2) = %q", got)
	}
}

func TestLoggerLevelCasesAndExecutorJobs(t *testing.T) {
	if LoggerLevelInfo != LevelInfo || LevelRawValue(LoggerLevelCritical) != "critical" {
		t.Error("Logger.Level cases")
	}
	ran := false
	var job any = UnownedJobInitExecutorJob(ExecutorJob{run: func() { ran = true }})
	UnownedJobRunSynchronouslyOn(job, nil)
	if !ran {
		t.Error("job did not run")
	}
	if got := TimeAmountMillisecondsInt(250).Nanoseconds; got != 250_000_000 {
		t.Errorf("milliseconds = %d", got)
	}
	if got := TimeAmountMillisecondsInt(int64(1) << 62).Nanoseconds; got != 1<<63-1 {
		t.Errorf("capped milliseconds = %d", got)
	}
	if IntInitWebSocketErrorCode[uint16](WebSocketErrorCodeGoingAway) != 1001 {
		t.Error("UInt16(webSocketErrorCode:)")
	}
}

// wsTestPeer is the server end of an upgraded test connection.
type wsTestPeer struct {
	conn net.Conn
	br   *bufio.Reader
}

func (p wsTestPeer) send(fin bool, op byte, data []byte) {
	b0 := op
	if fin {
		b0 |= 0x80
	}
	buf := []byte{b0}
	if len(data) < 126 {
		buf = append(buf, byte(len(data)))
	} else {
		buf = binary.BigEndian.AppendUint16(append(buf, 126), uint16(len(data)))
	}
	_, _ = p.conn.Write(append(buf, data...))
}

func (p wsTestPeer) sendClose(code uint16, reason string) {
	p.send(true, wsOpClose, append(binary.BigEndian.AppendUint16(nil, code), reason...))
}

// recv reads one client frame, which must be masked.
func (p wsTestPeer) recv(t *testing.T) wsFrame {
	f, err := readWSFrame(p.br, 1<<20)
	if err != nil {
		t.Errorf("server read: %v", err)
	}
	return f
}

func wsTestServer(t *testing.T, serve func(wsTestPeer)) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nope" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Origin") == "" || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			t.Errorf("handshake headers %v", r.Header)
		}
		if r.URL.RawQuery != "v=10" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		conn, rw, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
		rw.Flush()
		serve(wsTestPeer{conn, rw.Reader})
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/gateway?v=10"
}

func wsConnect(url string, handler func(WebSocketInboundStream, WebSocketOutboundWriter, WebSocketClientContext)) (frame *WebSocketCloseFrame, err error) {
	err = Catch(func() {
		frame = WebSocketClientConnectUrlConfigurationTlsConfigurationEventLoopGroupLoggerHandler(
			url, WebSocketClientConfigurationInitMaxFrameSize(1<<20), nil, MultiThreadedEventLoopGroupSingleton, LoggerInitLabel("test"), handler)
	})
	return
}

func TestWebSocketClientExchangesMessagesAndReturnsCloseFrame(t *testing.T) {
	serverGot := make(chan string, 4)
	url := wsTestServer(t, func(p wsTestPeer) {
		p.send(false, wsOpText, []byte("hel"))
		p.send(true, wsOpPing, []byte("pp"))
		p.send(true, wsOpContinuation, []byte("lo"))
		if f := p.recv(t); f.opcode != wsOpPong || string(f.data) != "pp" {
			t.Errorf("pong = %+v", f)
		}
		p.send(true, wsOpBinary, []byte{1, 2})
		f := p.recv(t)
		serverGot <- string(f.data)
		p.sendClose(4014, "Disallowed intent(s).")
		if f := p.recv(t); f.opcode != wsOpClose || binary.BigEndian.Uint16(f.data) != 1000 {
			t.Errorf("close reply = %+v", f)
		}
	})
	var got []WebSocketMessage
	frame, err := wsConnect(url, func(in WebSocketInboundStream, out WebSocketOutboundWriter, _ WebSocketClientContext) {
		it := WebSocketInboundMessageStreamMakeAsyncIterator(WebSocketInboundStreamMessagesMaxSize(in, 1<<20))
		for m := WebSocketInboundMessageStreamAsyncIteratorNext(&it); m != nil; m = WebSocketInboundMessageStreamAsyncIteratorNext(&it) {
			got = append(got, *m)
			if _, ok := (*m).(WebSocketMessageBinary); ok {
				WebSocketOutboundWriterWrite(out, OutboundFrameText(`{"op":1}`))
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (WebSocketMessageText{"hello"}) {
		t.Errorf("messages = %#v", got)
	}
	if s := <-serverGot; s != `{"op":1}` {
		t.Errorf("server got %q", s)
	}
	if frame == nil || IntInitWebSocketErrorCode[uint16](WebSocketCloseFrameCloseCode(*frame)) != 4014 || *frame.Reason != "Disallowed intent(s)." {
		t.Errorf("close frame = %+v", frame)
	}
}

func TestWebSocketClientClosesWith1011AndRethrowsHandlerError(t *testing.T) {
	boom := errors.New("boom")
	url := wsTestServer(t, func(p wsTestPeer) {
		f := p.recv(t)
		if f.opcode != wsOpClose || binary.BigEndian.Uint16(f.data) != 1011 {
			t.Errorf("close = %+v", f)
		}
		p.sendClose(1011, "")
	})
	_, err := wsConnect(url, func(WebSocketInboundStream, WebSocketOutboundWriter, WebSocketClientContext) {
		panic(Throw(boom))
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

func TestWebSocketClientRejectsOversizedMessage(t *testing.T) {
	url := wsTestServer(t, func(p wsTestPeer) {
		p.send(false, wsOpText, []byte("abc"))
		p.send(true, wsOpContinuation, []byte("def"))
		f := p.recv(t)
		if f.opcode != wsOpClose || binary.BigEndian.Uint16(f.data) != 1009 {
			t.Errorf("close = %+v", f)
		}
		p.sendClose(1009, "")
	})
	_, err := wsConnect(url, func(in WebSocketInboundStream, _ WebSocketOutboundWriter, _ WebSocketClientContext) {
		it := WebSocketInboundMessageStreamMakeAsyncIterator(WebSocketInboundStreamMessagesMaxSize(in, 4))
		WebSocketInboundMessageStreamAsyncIteratorNext(&it)
	})
	if err != WebSocketClientErrorServerSentMessageTooLarge {
		t.Errorf("err = %v", err)
	}
}

func TestWebSocketClientUpgradeFailureAndCancellation(t *testing.T) {
	url := wsTestServer(t, func(p wsTestPeer) {
		if f := p.recv(t); f.opcode != wsOpClose {
			t.Errorf("close = %+v", f)
		}
	})
	if _, err := wsConnect(strings.Replace(url, "/gateway", "/nope", 1), nil); err != WebSocketClientErrorWebSocketUpgradeFailed {
		t.Errorf("upgrade err = %v", err)
	}
	if _, err := wsConnect("not a url", nil); err != WebSocketClientErrorInvalidURL {
		t.Errorf("invalid url err = %v", err)
	}

	ended := make(chan *WebSocketMessage, 1)
	task := TaskInitOperation(func() {
		cfg := WebSocketClientConfigurationInitMaxFrameSize(1 << 20)
		cfg.CloseTimeout = DurationMilliseconds(100)
		WebSocketClientConnectUrlConfigurationTlsConfigurationEventLoopGroupLoggerHandler(url, cfg, nil, nil, LoggerInitLabel("test"),
			func(in WebSocketInboundStream, _ WebSocketOutboundWriter, _ WebSocketClientContext) {
				it := WebSocketInboundMessageStreamMakeAsyncIterator(WebSocketInboundStreamMessagesMaxSize(in, 1<<20))
				ended <- WebSocketInboundMessageStreamAsyncIteratorNext(&it)
			})
	})
	time.Sleep(50 * time.Millisecond)
	TaskCancel(task)
	select {
	case m := <-ended:
		if m != nil {
			t.Errorf("message after cancel = %v", *m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not end the inbound stream")
	}
	TaskValue(task)
}
