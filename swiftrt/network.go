package swiftrt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// URLRequest is Foundation `URLRequest`. Its headers are copied on write, so assigning a request
// copies it as the Swift value type does.
type URLRequest struct {
	Url             *URL
	HttpMethod      *string
	HttpBody        *[]byte
	TimeoutInterval float64
	headers         http.Header
}

// URLRequestInitUrl is Foundation `URLRequest(url:)`: a GET request with a 60-second timeout.
func URLRequestInitUrl(u URL) URLRequest {
	return URLRequest{Url: &u, HttpMethod: Some("GET"), TimeoutInterval: 60}
}

// URLRequestSetValueForHTTPHeaderField is `URLRequest.setValue(_:forHTTPHeaderField:)`: it replaces the
// field's value, matching the name case-insensitively, or removes the field when value is nil.
func URLRequestSetValueForHTTPHeaderField(r *URLRequest, value *string, field string) {
	h := r.headers.Clone()
	if h == nil {
		h = http.Header{}
	}
	if value == nil {
		h.Del(field)
	} else {
		h.Set(field, *value)
	}
	r.headers = h
}

// URLRequestValueForHTTPHeaderField is `URLRequest.value(forHTTPHeaderField:)`.
func URLRequestValueForHTTPHeaderField(r URLRequest, field string) *string {
	if v := r.headers.Values(field); len(v) > 0 {
		return Some(strings.Join(v, ","))
	}
	return nil
}

// URLSessionConfiguration is Foundation `URLSessionConfiguration`, with the fields the session honours.
type URLSessionConfiguration struct {
	TimeoutIntervalForRequest     float64
	TimeoutIntervalForResource    float64
	HttpMaximumConnectionsPerHost int
}

// NSURLSessionConfigurationEphemeral is `URLSessionConfiguration.ephemeral` (and `.default`, which behaves
// the same here: neither keeps a cache or cookie store between processes). Each read is a fresh copy.
var NSURLSessionConfigurationEphemeral = URLSessionConfiguration{
	TimeoutIntervalForRequest:     60,
	TimeoutIntervalForResource:    604800,
	HttpMaximumConnectionsPerHost: 6,
}

// URLSession is Foundation `URLSession`.
type URLSession struct {
	client *http.Client
	config URLSessionConfiguration
}

// NSURLSessionInitConfiguration is Foundation `URLSession(configuration:)`.
func NSURLSessionInitConfiguration(c URLSessionConfiguration) URLSession {
	idle := time.Duration(c.TimeoutIntervalForRequest * float64(time.Second))
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: idle}).DialContext,
		TLSHandshakeTimeout:   idle,
		ResponseHeaderTimeout: idle,
		MaxConnsPerHost:       c.HttpMaximumConnectionsPerHost,
		ForceAttemptHTTP2:     true,
	}
	return URLSession{
		client: &http.Client{Transport: transport, Timeout: time.Duration(c.TimeoutIntervalForResource * float64(time.Second))},
		config: c,
	}
}

// NSURLSessionFinishTasksAndInvalidate is `URLSession.finishTasksAndInvalidate()` / `invalidateAndCancel()`: requests here
// complete before the call that started them returns, so it only releases idle connections.
func NSURLSessionFinishTasksAndInvalidate(s URLSession) {
	if s.client != nil {
		s.client.CloseIdleConnections()
	}
}

// URLResponse is Foundation `URLResponse`; an HTTP request's response is an HTTPURLResponse.
type URLResponse interface{ isURLResponse() }

// HTTPURLResponse is Foundation `HTTPURLResponse`.
type HTTPURLResponse struct {
	Url        *URL
	StatusCode int
	header     http.Header
}

func (HTTPURLResponse) isURLResponse() {}

// NSHTTPURLResponseStatusCode is `HTTPURLResponse.statusCode`.
func NSHTTPURLResponseStatusCode(r HTTPURLResponse) int { return r.StatusCode }

// HTTPURLResponseValueForHTTPHeaderField is `HTTPURLResponse.value(forHTTPHeaderField:)`, matching
// the name case-insensitively.
func HTTPURLResponseValueForHTTPHeaderField(r HTTPURLResponse, field string) *string {
	if v := r.header.Values(field); len(v) > 0 {
		return Some(strings.Join(v, ", "))
	}
	return nil
}

// HTTPURLResponseAllHeaderFields is `HTTPURLResponse.allHeaderFields`.
func HTTPURLResponseAllHeaderFields(r HTTPURLResponse) map[string]any {
	out := map[string]any{}
	for k, v := range r.header {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

// URLError is Foundation `URLError`, thrown when a request cannot complete.
type URLError struct {
	Code int
	URL  string
	Err  error
}

// URLError codes.
const (
	URLErrorUnknown                = -1
	URLErrorCancelled              = -999
	URLErrorBadURL                 = -1000
	URLErrorTimedOut               = -1001
	URLErrorUnsupportedURL         = -1002
	URLErrorCannotFindHost         = -1003
	URLErrorCannotConnectToHost    = -1004
	URLErrorNetworkConnectionLost  = -1005
	URLErrorSecureConnectionFailed = -1200
)

func (e URLError) Error() string {
	return fmt.Sprintf("The operation couldn't be completed. (NSURLErrorDomain error %d.) %v", e.Code, e.Err)
}

func (e URLError) Unwrap() error { return e.Err }

func urlErrorCode(err error) int {
	var dnsErr *net.DNSError
	var opErr *net.OpError
	var netErr net.Error
	switch {
	case errors.As(err, &dnsErr):
		return URLErrorCannotFindHost
	case errors.As(err, &netErr) && netErr.Timeout():
		return URLErrorTimedOut
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return URLErrorCannotConnectToHost
	case strings.Contains(err.Error(), "tls:") || strings.Contains(err.Error(), "x509:"):
		return URLErrorSecureConnectionFailed
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		return URLErrorNetworkConnectionLost
	}
	return URLErrorUnknown
}

// NSURLSessionDataFor is `URLSession.data(for:)`: it performs the request and returns the body and
// response. Non-2xx statuses are responses, not errors; a request that cannot complete throws URLError,
// and cancelling the current task throws CancellationError.
func NSURLSessionDataFor(s URLSession, r URLRequest) struct {
	F0 []byte
	F1 URLResponse
} {
	if r.Url == nil {
		panic(Throw(URLError{Code: URLErrorBadURL, Err: errors.New("missing URL")}))
	}
	parsed, err := url.Parse(r.Url.Raw)
	if err != nil {
		panic(Throw(URLError{Code: URLErrorBadURL, URL: r.Url.Raw, Err: err}))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		panic(Throw(URLError{Code: URLErrorUnsupportedURL, URL: r.Url.Raw, Err: errors.New("unsupported URL")}))
	}
	TaskCheckCancellation()
	method := "GET"
	if r.HttpMethod != nil {
		method = *r.HttpMethod
	}
	var body io.Reader
	if r.HttpBody != nil {
		body = bytes.NewReader(*r.HttpBody)
	}
	req, err := http.NewRequest(method, r.Url.Raw, body)
	if err != nil {
		panic(Throw(URLError{Code: URLErrorBadURL, URL: r.Url.Raw, Err: err}))
	}
	for k, v := range r.headers {
		req.Header[k] = append([]string(nil), v...)
	}
	if r.HttpBody != nil {
		req.ContentLength = int64(len(*r.HttpBody))
	}
	if strings.EqualFold(req.Header.Get("Connection"), "close") {
		req.Close = true
	}
	client := s.client
	if client == nil {
		client = NSURLSessionInitConfiguration(NSURLSessionConfigurationEphemeral).client
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(Throw(URLError{Code: urlErrorCode(err), URL: r.Url.Raw, Err: err}))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(Throw(URLError{Code: urlErrorCode(err), URL: r.Url.Raw, Err: err}))
	}
	final := URL{resp.Request.URL.String()}
	return struct {
		F0 []byte
		F1 URLResponse
	}{data, HTTPURLResponse{Url: &final, StatusCode: resp.StatusCode, header: resp.Header}}
}

// URLComponents is Foundation `URLComponents`, parsed into the parts the runtime reads.
type URLComponents struct {
	host *string
	path string
}

func isSchemeChar(r rune, first bool) bool {
	letter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	return letter || !first && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.')
}

// percentDecodeLenient decodes each valid %XX escape and leaves malformed ones as written.
func percentDecodeLenient(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			v, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			b.WriteByte(byte(v))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

// URLComponentsOptInitString is Foundation `URLComponents(string:)`. Like swift-foundation it accepts
// characters a URL must percent-encode in the path, query and user info, and returns nil for an invalid
// scheme, a host containing such characters, or a non-numeric port.
func URLComponentsOptInitString(s string) *URLComponents {
	rest := s
	if i := strings.IndexAny(s, ":/?#"); i > 0 && s[i] == ':' {
		for j, r := range s[:i] {
			if !isSchemeChar(r, j == 0) {
				return nil
			}
		}
		rest = s[i+1:]
	}
	c := &URLComponents{}
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			end = len(rest)
		}
		authority := rest[:end]
		rest = rest[end:]
		if at := strings.LastIndexByte(authority, '@'); at >= 0 {
			authority = authority[at+1:]
		}
		host, port := authority, ""
		if strings.HasPrefix(authority, "[") {
			close := strings.IndexByte(authority, ']')
			if close < 0 {
				return nil
			}
			host, port = authority[:close+1], strings.TrimPrefix(authority[close+1:], ":")
		} else if colon := strings.IndexByte(authority, ':'); colon >= 0 {
			host, port = authority[:colon], authority[colon+1:]
		}
		for _, r := range port {
			if r < '0' || r > '9' {
				return nil
			}
		}
		for _, r := range host {
			if r <= ' ' || r >= 0x7f || strings.ContainsRune(`"<>\^`+"`{|}/?#@", r) {
				return nil
			}
		}
		host = percentDecodeLenient(host)
		c.host = &host
	}
	if end := strings.IndexAny(rest, "?#"); end >= 0 {
		rest = rest[:end]
	}
	c.path = percentDecodeLenient(rest)
	return c
}

// URLComponentsHost is `URLComponents.host`: percent-decoded, nil when the URL has no authority.
func URLComponentsHost(c URLComponents) *string { return c.host }

// URLComponentsPath is `URLComponents.path`: percent-decoded.
func URLComponentsPath(c URLComponents) string { return c.path }
