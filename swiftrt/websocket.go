package swiftrt

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TimeAmount is SwiftNIO `TimeAmount`, a signed count of nanoseconds.
type TimeAmount struct{ Nanoseconds int64 }

func cappedNanoseconds(amount, multiplier int64) TimeAmount {
	r := amount * multiplier
	if amount != 0 && r/multiplier != amount {
		if amount > 0 {
			return TimeAmount{1<<63 - 1}
		}
		return TimeAmount{-1 << 63}
	}
	return TimeAmount{r}
}

// TimeAmountMillisecondsInt is SwiftNIO `TimeAmount.milliseconds(_:)`, capped at the Int64 range
// instead of overflowing.
func TimeAmountMillisecondsInt[T Integer](ms T) TimeAmount { return cappedNanoseconds(int64(ms), 1e6) }

// TimeAmountSecondsInt is SwiftNIO `TimeAmount.seconds(_:)`, capped at the Int64 range.
func TimeAmountSecondsInt[T Integer](s T) TimeAmount { return cappedNanoseconds(int64(s), 1e9) }

// MultiThreadedEventLoopGroup is SwiftNIO `MultiThreadedEventLoopGroup`. Connections run on
// goroutines, so the group carries no state.
type MultiThreadedEventLoopGroup struct{}

// MultiThreadedEventLoopGroupSingleton is SwiftNIO `MultiThreadedEventLoopGroup.singleton`.
var MultiThreadedEventLoopGroupSingleton = &MultiThreadedEventLoopGroup{}

// CertificateVerification is NIOSSL `CertificateVerification`.
type CertificateVerification int

// CertificateVerification cases.
const (
	CertificateVerificationNone CertificateVerification = iota
	CertificateVerificationNoHostnameVerification
	CertificateVerificationFullVerification
)

// TLSConfiguration is NIOSSL `TLSConfiguration`, with the fields a client connection honours. Peers
// are verified against the system trust roots.
type TLSConfiguration struct {
	CertificateVerification CertificateVerification
	ShutdownTimeout         TimeAmount
}

// TLSConfigurationMakeClientConfiguration is NIOSSL `TLSConfiguration.makeClientConfiguration()`: full
// certificate verification and a five-second shutdown timeout.
func TLSConfigurationMakeClientConfiguration() TLSConfiguration {
	return TLSConfiguration{
		CertificateVerification: CertificateVerificationFullVerification,
		ShutdownTimeout:         TimeAmount{5e9},
	}
}

func (c TLSConfiguration) goConfig(serverName string) *tls.Config {
	cfg := &tls.Config{ServerName: serverName}
	switch c.CertificateVerification {
	case CertificateVerificationNone:
		cfg.InsecureSkipVerify = true
	case CertificateVerificationNoHostnameVerification:
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("tls: no peer certificate")
			}
			certs := make([]*x509.Certificate, len(raw))
			for i, r := range raw {
				c, err := x509.ParseCertificate(r)
				if err != nil {
					return err
				}
				certs[i] = c
			}
			inter := x509.NewCertPool()
			for _, c := range certs[1:] {
				inter.AddCert(c)
			}
			_, err := certs[0].Verify(x509.VerifyOptions{Intermediates: inter})
			return err
		}
	}
	return cfg
}

// WebSocketErrorCode is NIOWebSocket `WebSocketErrorCode`, held as its wire value: each named case
// is its code number and `.unknown(n)` is any other n, as `WebSocketErrorCode(codeNumber:)` maps them.
type WebSocketErrorCode uint16

// WebSocketErrorCode cases.
const (
	WebSocketErrorCodeNormalClosure               WebSocketErrorCode = 1000
	WebSocketErrorCodeGoingAway                   WebSocketErrorCode = 1001
	WebSocketErrorCodeProtocolError               WebSocketErrorCode = 1002
	WebSocketErrorCodeUnacceptableData            WebSocketErrorCode = 1003
	WebSocketErrorCodeDataInconsistentWithMessage WebSocketErrorCode = 1007
	WebSocketErrorCodePolicyViolation             WebSocketErrorCode = 1008
	WebSocketErrorCodeMessageTooLarge             WebSocketErrorCode = 1009
	WebSocketErrorCodeMissingExtension            WebSocketErrorCode = 1010
	WebSocketErrorCodeUnexpectedServerError       WebSocketErrorCode = 1011
)

func (c WebSocketErrorCode) known() bool {
	return c >= 1000 && c <= 1003 || c >= 1007 && c <= 1011
}

// IntInitWebSocketErrorCode is NIOWebSocket `UInt16(webSocketErrorCode:)`: the code's wire value.
func IntInitWebSocketErrorCode[T Integer](c WebSocketErrorCode) T { return IntInitInt[T](int(c)) }

// WebSocketCloseFrame is WSCore `WebSocketCloseFrame`: the code and reason of a close frame.
type WebSocketCloseFrame struct {
	CloseCode WebSocketErrorCode
	Reason    *string
}

// WebSocketCloseFrameCloseCode is WSCore `WebSocketCloseFrame.closeCode`.
func WebSocketCloseFrameCloseCode(f WebSocketCloseFrame) WebSocketErrorCode { return f.CloseCode }

// WebSocketMessage is WSCore `WebSocketMessage`: WebSocketMessageText or WebSocketMessageBinary.
type WebSocketMessage interface{ isWebSocketMessage() }

// WebSocketMessageText is WSCore `WebSocketMessage.text(_:)`.
type WebSocketMessageText struct{ F0 string }

// WebSocketMessageBinary is WSCore `WebSocketMessage.binary(_:)`.
type WebSocketMessageBinary struct{ F0 []byte }

func (WebSocketMessageText) isWebSocketMessage()   {}
func (WebSocketMessageBinary) isWebSocketMessage() {}

// NIOWebSocketError is NIOWebSocket `NIOWebSocketError`, thrown when a received frame breaks the
// frame-level rules.
type NIOWebSocketError int

// NIOWebSocketError cases.
const (
	NIOWebSocketErrorInvalidFrameLength NIOWebSocketError = iota
	NIOWebSocketErrorFragmentedControlFrame
	NIOWebSocketErrorMultiByteControlFrameLength
)

func (e NIOWebSocketError) Error() string {
	return [...]string{"invalidFrameLength", "fragmentedControlFrame", "multiByteControlFrameLength"}[e]
}

// WebSocketClientError is WSClient `WebSocketClientError`.
type WebSocketClientError int

// WebSocketClientError values.
const (
	WebSocketClientErrorInvalidURL WebSocketClientError = iota
	WebSocketClientErrorWebSocketUpgradeFailed
	WebSocketClientErrorServerProtocolError
	WebSocketClientErrorServerSentMessageTooLarge
	WebSocketClientErrorServerSentDataInconsistentWithMessage
)

func (e WebSocketClientError) Error() string {
	return [...]string{
		"Invalid URL",
		"WebSocket upgrade failed",
		"Received unexpected data from the server",
		"Server sent a message that was too large",
		"Server sent a message with data inconsistent with the message type",
	}[e]
}

// wsInternalClose is WSCore `WebSocketHandler.InternalError.close(_:)`: the handler closes the
// connection with the code.
type wsInternalClose struct{ code WebSocketErrorCode }

func (e wsInternalClose) Error() string { return "close(" + strconv.Itoa(int(e.code)) + ")" }

// wsIOOnClosedChannel is NIOCore `ChannelError.ioOnClosedChannel`.
var wsIOOnClosedChannel = errors.New("ioOnClosedChannel")

const (
	wsOpContinuation byte = 0x0
	wsOpText         byte = 0x1
	wsOpBinary       byte = 0x2
	wsOpClose        byte = 0x8
	wsOpPing         byte = 0x9
	wsOpPong         byte = 0xA
)

type wsFrame struct {
	fin    bool
	rsv    byte
	opcode byte
	data   []byte
}

// WebSocketDataFrame is WSCore `WebSocketDataFrame`: a text, binary or continuation frame.
type WebSocketDataFrame struct {
	Opcode string
	Data   []byte
	Fin    bool
}

const (
	wsStateOpen = iota
	wsStateClosing
	wsStateClosed
)

// wsHandler is WSCore `WebSocketHandler` for a client connection: the socket, the close state
// machine and the frames the read goroutine has decoded.
type wsHandler struct {
	conn         net.Conn
	frames       chan wsFrame
	readErr      error
	quit         chan struct{}
	inputClosed  chan struct{}
	closeInput   sync.Once
	writeMu      sync.Mutex
	mu           sync.Mutex
	state        int
	closeFrame   *WebSocketCloseFrame
	iterated     bool
	terminated   bool
	validateUTF8 bool
	maxFrameSize int
}

func (h *wsHandler) readLoop(br *bufio.Reader) {
	defer close(h.frames)
	for {
		f, err := readWSFrame(br, h.maxFrameSize)
		if err != nil {
			var we NIOWebSocketError
			if errors.As(err, &we) {
				code := WebSocketErrorCodeProtocolError
				if we == NIOWebSocketErrorInvalidFrameLength {
					code = WebSocketErrorCodeMessageTooLarge
				}
				_ = h.writeFrame(wsFrame{fin: true, opcode: wsOpClose, data: binary.BigEndian.AppendUint16(nil, uint16(code))})
				h.conn.Close()
				h.readErr = err
			} else if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, net.ErrClosed) {
				h.readErr = err
			}
			return
		}
		select {
		case h.frames <- f:
		case <-h.quit:
			return
		}
	}
}

func readWSFrame(br *bufio.Reader, maxFrameSize int) (wsFrame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		return wsFrame{}, err
	}
	f := wsFrame{fin: hdr[0]&0x80 != 0, rsv: hdr[0] & 0x70, opcode: hdr[0] & 0x0F}
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(br, b[:]); err != nil {
			return wsFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(br, b[:]); err != nil {
			return wsFrame{}, err
		}
		length = binary.BigEndian.Uint64(b[:])
		if length > 1<<63-1 {
			return wsFrame{}, NIOWebSocketErrorInvalidFrameLength
		}
	}
	if length > uint64(maxFrameSize) {
		return wsFrame{}, NIOWebSocketErrorInvalidFrameLength
	}
	if f.opcode&0x08 != 0 {
		if !f.fin {
			return wsFrame{}, NIOWebSocketErrorFragmentedControlFrame
		}
		if length > 125 {
			return wsFrame{}, NIOWebSocketErrorMultiByteControlFrameLength
		}
	}
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(br, key[:]); err != nil {
			return wsFrame{}, err
		}
	}
	f.data = make([]byte, length)
	if _, err := io.ReadFull(br, f.data); err != nil {
		return wsFrame{}, err
	}
	if masked {
		for i := range f.data {
			f.data[i] ^= key[i%4]
		}
	}
	return f, nil
}

// writeFrame masks and sends one frame, as a client must.
func (h *wsHandler) writeFrame(f wsFrame) error {
	b0 := f.opcode | f.rsv
	if f.fin {
		b0 |= 0x80
	}
	buf := []byte{b0}
	switch n := len(f.data); {
	case n < 126:
		buf = append(buf, 0x80|byte(n))
	case n <= 0xFFFF:
		buf = binary.BigEndian.AppendUint16(append(buf, 0x80|126), uint16(n))
	default:
		buf = binary.BigEndian.AppendUint64(append(buf, 0x80|127), uint64(n))
	}
	var key [4]byte
	if _, err := rand.Read(key[:]); err != nil {
		return err
	}
	buf = append(buf, key[:]...)
	for i, c := range f.data {
		buf = append(buf, c^key[i%4])
	}
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if _, err := h.conn.Write(buf); err != nil {
		return wsIOOnClosedChannel
	}
	return nil
}

func (h *wsHandler) sendClose(code WebSocketErrorCode, reason *string) error {
	data := binary.BigEndian.AppendUint16(nil, uint16(code))
	if reason != nil {
		data = append(data, *reason...)
	}
	return h.writeFrame(wsFrame{fin: true, opcode: wsOpClose, data: data})
}

// close is `WebSocketHandler.close(code:reason:)`: it sends a close frame unless one was already
// sent or received.
func (h *wsHandler) close(code WebSocketErrorCode, reason *string) error {
	h.mu.Lock()
	open := h.state == wsStateOpen
	if open {
		h.state = wsStateClosing
	}
	h.mu.Unlock()
	if !open {
		return nil
	}
	return h.sendClose(code, reason)
}

// receivedClose is `WebSocketHandler.receivedClose(_:)`: it records the peer's close frame and,
// when the connection was still open, echoes a close code.
func (h *wsHandler) receivedClose(f wsFrame) {
	if f.rsv != 0 {
		_ = h.close(WebSocketErrorCodeProtocolError, nil)
		return
	}
	data := f.data
	var frame *WebSocketCloseFrame
	var code *WebSocketErrorCode
	if len(data) >= 2 {
		c := WebSocketErrorCode(binary.BigEndian.Uint16(data))
		code = &c
		data = data[2:]
	}
	hasReason := len(data) > 0
	var reason *string
	if hasReason {
		if s, ok := wsDecodeText(data, h.validateUTF8); ok {
			reason = &s
		}
	}
	if code != nil {
		frame = &WebSocketCloseFrame{CloseCode: *code, Reason: reason}
	}
	h.mu.Lock()
	prev := h.state
	if prev != wsStateClosed {
		h.state = wsStateClosed
		h.closeFrame = frame
	}
	h.mu.Unlock()
	if prev != wsStateOpen {
		return
	}
	reply := WebSocketErrorCodeProtocolError
	switch {
	case hasReason && reason == nil:
	case len(f.data) == 0:
		reply = WebSocketErrorCodeNormalClosure
	case code != nil:
		if c := *code; c.known() || c >= 3000 && c <= 4999 {
			reply = WebSocketErrorCodeNormalClosure
		}
	}
	_ = h.sendClose(reply, nil)
}

func wsDecodeText(b []byte, validate bool) (string, bool) {
	s := StringInitDecodingAs(b, nil)
	if validate && s != string(b) {
		return "", false
	}
	return s, true
}

// nextFrame waits for the next decoded frame. It reports false when the input has ended or the
// calling task is cancelled, which ends the sequence for good.
func (h *wsHandler) nextFrame() (wsFrame, bool) {
	h.mu.Lock()
	done := h.terminated
	h.mu.Unlock()
	if done {
		return wsFrame{}, false
	}
	var cancelled <-chan struct{}
	if s := currentScope(); s != nil {
		if s.isCancelled() {
			h.terminate()
			return wsFrame{}, false
		}
		cancelled = s.done
	}
	select {
	case f, ok := <-h.frames:
		if ok {
			return f, true
		}
		h.terminate()
		if err := h.readErr; err != nil {
			h.readErr = nil
			panic(Throw(err))
		}
	case <-cancelled:
		h.terminate()
	case <-h.inputClosed:
		h.terminate()
	}
	return wsFrame{}, false
}

func (h *wsHandler) terminate() {
	h.mu.Lock()
	h.terminated = true
	h.mu.Unlock()
}

// WebSocketInboundStream is WSCore `WebSocketInboundStream`, the frames a connection receives.
type WebSocketInboundStream struct{ h *wsHandler }

// WebSocketInboundStreamAsyncIterator is WSCore `WebSocketInboundStream.AsyncIterator`.
type WebSocketInboundStreamAsyncIterator struct {
	h      *wsHandler
	closed bool
}

// WebSocketInboundStreamMakeAsyncIterator is `WebSocketInboundStream.makeAsyncIterator()`. A stream is
// iterated once; a second iterator finishes at once.
func WebSocketInboundStreamMakeAsyncIterator(s WebSocketInboundStream) WebSocketInboundStreamAsyncIterator {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	done := s.h.iterated
	s.h.iterated = true
	return WebSocketInboundStreamAsyncIterator{h: s.h, closed: done}
}

// WebSocketInboundStreamAsyncIteratorNext is `WebSocketInboundStream.AsyncIterator.next()`: the next
// text, binary or continuation frame, or nil once the peer's close frame arrives or the input ends.
// It answers pings, and closes the connection with a protocol error on a frame with reserved bits
// or an unknown opcode.
func WebSocketInboundStreamAsyncIteratorNext(it *WebSocketInboundStreamAsyncIterator) *WebSocketDataFrame {
	if it.closed {
		return nil
	}
	h := it.h
	for {
		f, ok := h.nextFrame()
		if !ok {
			return nil
		}
		switch f.opcode {
		case wsOpClose:
			h.receivedClose(f)
			return nil
		case wsOpPing:
			h.mu.Lock()
			open := h.state == wsStateOpen
			h.mu.Unlock()
			if open && h.writeFrame(wsFrame{fin: true, opcode: wsOpPong, data: f.data}) != nil {
				_ = h.close(WebSocketErrorCodeUnexpectedServerError, nil)
			}
		case wsOpPong:
		case wsOpText, wsOpBinary, wsOpContinuation:
			if f.rsv != 0 {
				_ = h.close(WebSocketErrorCodeProtocolError, nil)
				continue
			}
			op := map[byte]string{wsOpText: "text", wsOpBinary: "binary", wsOpContinuation: "continuation"}[f.opcode]
			return &WebSocketDataFrame{Opcode: op, Data: f.data, Fin: f.fin}
		default:
			_ = h.close(WebSocketErrorCodeProtocolError, nil)
		}
	}
}

func (it *WebSocketInboundStreamAsyncIterator) message(first WebSocketDataFrame, data []byte) WebSocketMessage {
	if first.Opcode == "binary" {
		return WebSocketMessageBinary{data}
	}
	s, ok := wsDecodeText(data, it.h.validateUTF8)
	if !ok {
		panic(Throw(wsInternalClose{WebSocketErrorCodeDataInconsistentWithMessage}))
	}
	return WebSocketMessageText{s}
}

// WebSocketInboundStreamAsyncIteratorNextMessageMaxSize is
// `WebSocketInboundStream.AsyncIterator.nextMessage(maxSize:)`: the next message, joining a
// fragmented one, or nil when the frames end. Continuation frames that take the message past
// maxSize, and frames out of order, throw and close the connection.
func WebSocketInboundStreamAsyncIteratorNextMessageMaxSize(it *WebSocketInboundStreamAsyncIterator, maxSize int) *WebSocketMessage {
	first := WebSocketInboundStreamAsyncIteratorNext(it)
	if first == nil {
		return nil
	}
	if first.Opcode == "continuation" {
		panic(Throw(wsInternalClose{WebSocketErrorCodeProtocolError}))
	}
	data := first.Data
	if first.Fin {
		m := it.message(*first, data)
		return &m
	}
	for {
		f := WebSocketInboundStreamAsyncIteratorNext(it)
		if f == nil {
			return nil
		}
		if f.Opcode != "continuation" {
			panic(Throw(wsInternalClose{WebSocketErrorCodeProtocolError}))
		}
		if len(data)+len(f.Data) > maxSize {
			panic(Throw(wsInternalClose{WebSocketErrorCodeMessageTooLarge}))
		}
		data = append(data[:len(data):len(data)], f.Data...)
		if f.Fin {
			m := it.message(*first, data)
			return &m
		}
	}
}

// WebSocketInboundMessageStream is WSCore `WebSocketInboundMessageStream`.
type WebSocketInboundMessageStream struct {
	stream  WebSocketInboundStream
	maxSize int
}

// WebSocketInboundStreamMessagesMaxSize is `WebSocketInboundStream.messages(maxSize:)`: the stream's
// frames joined into messages of at most maxSize bytes.
func WebSocketInboundStreamMessagesMaxSize(s WebSocketInboundStream, maxSize int) WebSocketInboundMessageStream {
	return WebSocketInboundMessageStream{s, maxSize}
}

// WebSocketInboundMessageStreamAsyncIterator is `WebSocketInboundMessageStream.AsyncIterator`.
type WebSocketInboundMessageStreamAsyncIterator struct {
	frames  WebSocketInboundStreamAsyncIterator
	maxSize int
}

// WebSocketInboundMessageStreamMakeAsyncIterator is `WebSocketInboundMessageStream.makeAsyncIterator()`.
func WebSocketInboundMessageStreamMakeAsyncIterator(s WebSocketInboundMessageStream) WebSocketInboundMessageStreamAsyncIterator {
	return WebSocketInboundMessageStreamAsyncIterator{WebSocketInboundStreamMakeAsyncIterator(s.stream), s.maxSize}
}

// WebSocketInboundMessageStreamAsyncIteratorNext is `WebSocketInboundMessageStream.AsyncIterator.next()`.
func WebSocketInboundMessageStreamAsyncIteratorNext(it *WebSocketInboundMessageStreamAsyncIterator) *WebSocketMessage {
	return WebSocketInboundStreamAsyncIteratorNextMessageMaxSize(&it.frames, it.maxSize)
}

// OutboundFrame is WSCore `WebSocketOutboundWriter.OutboundFrame`.
type OutboundFrame struct {
	opcode byte
	data   []byte
}

// OutboundFrameText is `WebSocketOutboundWriter.OutboundFrame.text(_:)`.
func OutboundFrameText(s string) OutboundFrame { return OutboundFrame{wsOpText, []byte(s)} }

// OutboundFrameBinary is `WebSocketOutboundWriter.OutboundFrame.binary(_:)`.
func OutboundFrameBinary(b []byte) OutboundFrame { return OutboundFrame{wsOpBinary, b} }

// OutboundFramePong is `WebSocketOutboundWriter.OutboundFrame.pong`.
var OutboundFramePong = OutboundFrame{opcode: wsOpPong}

// WebSocketOutboundWriter is WSCore `WebSocketOutboundWriter`.
type WebSocketOutboundWriter struct {
	h            *wsHandler
	maxFrameSize int
}

// WebSocketOutboundWriterWrite is `WebSocketOutboundWriter.write(_:)`: it sends the frame as one
// final frame, throwing CancellationError if the current task is cancelled and
// `ChannelError.ioOnClosedChannel` once the connection is closed.
func WebSocketOutboundWriterWrite(w WebSocketOutboundWriter, f OutboundFrame) {
	TaskCheckCancellation()
	if err := w.h.writeFrame(wsFrame{fin: true, opcode: f.opcode, data: f.data}); err != nil {
		panic(Throw(err))
	}
}

// WebSocketOutboundWriterCloseReason is `WebSocketOutboundWriter.close(_:reason:)`: it sends a close
// frame unless one was already sent or received.
func WebSocketOutboundWriterCloseReason(w WebSocketOutboundWriter, code WebSocketErrorCode, reason *string) {
	if err := w.h.close(code, reason); err != nil {
		panic(Throw(err))
	}
}

// WebSocketClientContext is WSClient `WebSocketClient.Context`.
type WebSocketClientContext struct{ Logger Logger }

// WebSocketClientConfiguration is WSClient `WebSocketClientConfiguration`, with the fields a
// connection without extensions, proxies or automatic pings honours.
type WebSocketClientConfiguration struct {
	MaxFrameSize int
	CloseTimeout Duration
	ValidateUTF8 bool
	SniHostname  *string
}

// WebSocketClientConfigurationInit is `WebSocketClientConfiguration()`: 16 KiB frames and a
// 15-second close timeout.
func WebSocketClientConfigurationInit() WebSocketClientConfiguration {
	return WebSocketClientConfigurationInitMaxFrameSize(1 << 14)
}

// WebSocketClientConfigurationInitMaxFrameSize is `WebSocketClientConfiguration(maxFrameSize:)`.
func WebSocketClientConfigurationInitMaxFrameSize(n int) WebSocketClientConfiguration {
	return WebSocketClientConfiguration{MaxFrameSize: n, CloseTimeout: DurationSeconds(15)}
}

// WebSocketClientConnectUrlConfigurationTlsConfigurationEventLoopGroupLoggerHandler is WSClient
// `WebSocketClient.connect(url:configuration:tlsConfiguration:eventLoopGroup:logger:handler:)`.
//
// It opens the connection, over TLS for a wss or https URL, and runs handler with the inbound frames
// and an outbound writer. When handler returns it sends a normal close, or 1011 when handler threw,
// then reads until the peer closes or the close timeout passes. It returns the peer's close frame,
// or nil when none arrived, and rethrows handler's error, mapping the runtime's own protocol closes
// to WebSocketClientError. Cancelling the calling task ends the inbound frames.
func WebSocketClientConnectUrlConfigurationTlsConfigurationEventLoopGroupLoggerHandler(
	rawURL string,
	cfg WebSocketClientConfiguration,
	tlsConfiguration *TLSConfiguration,
	_ any,
	logger Logger,
	handler func(WebSocketInboundStream, WebSocketOutboundWriter, WebSocketClientContext),
) *WebSocketCloseFrame {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		panic(Throw(WebSocketClientErrorInvalidURL))
	}
	host := u.Hostname()
	scheme := strings.ToLower(u.Scheme)
	requiresTLS := scheme == "wss" || scheme == "https"
	port := u.Port()
	if port == "" {
		port = "80"
		if requiresTLS {
			port = "443"
		}
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		panic(Throw(err))
	}
	defer conn.Close()
	if requiresTLS {
		tc := TLSConfigurationMakeClientConfiguration()
		if tlsConfiguration != nil {
			tc = *tlsConfiguration
		}
		serverName := host
		if cfg.SniHostname != nil {
			serverName = *cfg.SniHostname
		}
		tconn := tls.Client(conn, tc.goConfig(serverName))
		if err := tconn.Handshake(); err != nil {
			panic(Throw(err))
		}
		conn = tconn
	}
	br := wsUpgrade(conn, u, scheme, host)

	h := &wsHandler{
		conn:         conn,
		frames:       make(chan wsFrame),
		quit:         make(chan struct{}),
		inputClosed:  make(chan struct{}),
		validateUTF8: cfg.ValidateUTF8,
		maxFrameSize: cfg.MaxFrameSize,
	}
	defer close(h.quit)
	go h.readLoop(br)
	if s := currentScope(); s != nil {
		defer s.onCancel(func() { h.closeInput.Do(func() { close(h.inputClosed) }) })()
	}

	ctx := WebSocketClientContext{Logger: logger}
	inbound := WebSocketInboundStream{h}
	outbound := WebSocketOutboundWriter{h, cfg.MaxFrameSize}
	closeCode := WebSocketErrorCodeNormalClosure
	clientErr := Catch(func() { handler(inbound, outbound, ctx) })
	if clientErr != nil {
		var ic wsInternalClose
		if errors.As(clientErr, &ic) {
			closeCode = ic.code
		} else {
			closeCode = WebSocketErrorCodeUnexpectedServerError
		}
	}
	if err := h.close(closeCode, nil); err != nil && !errors.Is(err, wsIOOnClosedChannel) {
		panic(Throw(err))
	}
	h.mu.Lock()
	closing := h.state == wsStateClosing
	h.mu.Unlock()
	if closing {
		timeout := time.NewTimer(time.Duration(cfg.CloseTimeout))
		defer timeout.Stop()
	drain:
		for {
			select {
			case f, ok := <-h.frames:
				if !ok {
					if h.readErr != nil {
						panic(Throw(h.readErr))
					}
					break drain
				}
				if f.opcode == wsOpClose {
					h.receivedClose(f)
				}
			case <-timeout.C:
				break drain
			}
		}
	}
	if clientErr != nil {
		var ic wsInternalClose
		if errors.As(clientErr, &ic) {
			switch ic.code {
			case WebSocketErrorCodeMessageTooLarge:
				panic(Throw(WebSocketClientErrorServerSentMessageTooLarge))
			case WebSocketErrorCodeDataInconsistentWithMessage:
				panic(Throw(WebSocketClientErrorServerSentDataInconsistentWithMessage))
			default:
				panic(Throw(WebSocketClientErrorServerProtocolError))
			}
		}
		panic(Throw(clientErr))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == wsStateClosed {
		return h.closeFrame
	}
	return nil
}

// wsUpgrade performs the opening handshake, throwing WebSocketClientError.webSocketUpgradeFailed
// unless the server switches protocols with the expected accept key. It returns the reader that
// holds any frame bytes sent after the response.
func wsUpgrade(conn net.Conn, u *url.URL, scheme, host string) *bufio.Reader {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic(Throw(err))
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	target := u.EscapedPath()
	if target == "" {
		target = "/"
	}
	if u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	hostHeader := host
	if strings.Contains(host, ":") {
		hostHeader = "[" + host + "]"
	}
	if u.Port() != "" {
		hostHeader += ":" + u.Port()
	}
	req := "GET " + target + " HTTP/1.1\r\n" +
		"Host: " + hostHeader + "\r\n" +
		"Origin: " + scheme + "://" + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Content-Length: 0\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		panic(Throw(err))
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		panic(Throw(err))
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.StatusCode != http.StatusSwitchingProtocols ||
		!strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		panic(Throw(WebSocketClientErrorWebSocketUpgradeFailed))
	}
	return br
}
