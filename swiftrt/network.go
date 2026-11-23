package swiftrt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
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
	return URLRequest{Url: &u, HttpMethod: new("GET"), TimeoutInterval: 60}
}

// URLRequestUrl is `URLRequest.url`, read where the emitter calls the property's getter.
func URLRequestUrl(r URLRequest) *URL { return r.Url }

// URLRequestHttpMethod is `URLRequest.httpMethod`.
func URLRequestHttpMethod(r URLRequest) *string { return r.HttpMethod }

// URLRequestHttpBody is `URLRequest.httpBody`.
func URLRequestHttpBody(r URLRequest) *[]byte { return r.HttpBody }

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
		return new(strings.Join(v, ","))
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

// NSHTTPURLResponseOptInitUrlStatusCodeHttpVersionHeaderFields is `HTTPURLResponse(url:statusCode:httpVersion:headerFields:)`.
// The HTTP version is not kept.
func NSHTTPURLResponseOptInitUrlStatusCodeHttpVersionHeaderFields(url URL, statusCode int, _ *string, headerFields *map[string]string) *HTTPURLResponse {
	r := &HTTPURLResponse{Url: &url, StatusCode: statusCode, header: http.Header{}}
	if headerFields != nil {
		for k, v := range *headerFields {
			r.header.Set(k, v)
		}
	}
	return r
}

// NSURLResponseUrl is `URLResponse.url`.
func NSURLResponseUrl(r URLResponse) *URL {
	if h, ok := r.(HTTPURLResponse); ok {
		return h.Url
	}
	return nil
}

// NSHTTPURLResponseStatusCode is `HTTPURLResponse.statusCode`.
func NSHTTPURLResponseStatusCode(r HTTPURLResponse) int { return r.StatusCode }

// HTTPURLResponseValueForHTTPHeaderField is `HTTPURLResponse.value(forHTTPHeaderField:)`, matching
// the name case-insensitively.
func HTTPURLResponseValueForHTTPHeaderField(r HTTPURLResponse, field string) *string {
	if v := r.header.Values(field); len(v) > 0 {
		return new(strings.Join(v, ", "))
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

// urlErrorDescriptions are the localized descriptions Apple's URLSession gives the errors it reports.
var urlErrorDescriptions = map[int]string{
	URLErrorCancelled:              "cancelled",
	URLErrorBadURL:                 "bad URL",
	URLErrorTimedOut:               "The request timed out.",
	URLErrorUnsupportedURL:         "unsupported URL",
	URLErrorCannotFindHost:         "A server with the specified hostname could not be found.",
	URLErrorCannotConnectToHost:    "Could not connect to the server.",
	URLErrorNetworkConnectionLost:  "The network connection was lost.",
	URLErrorSecureConnectionFailed: "An SSL error has occurred and a secure connection to the server cannot be made.",
}

// localizedDescription is the error's `localizedDescription`, as URLSession on Apple platforms
// words it.
func (e URLError) localizedDescription() string {
	if d, ok := urlErrorDescriptions[e.Code]; ok {
		return d
	}
	return "The operation couldn’t be completed. (NSURLErrorDomain error " + strconv.Itoa(e.Code) + ".)"
}

func urlErrorCode(err error) int {
	_, isDNS := errors.AsType[*net.DNSError](err)
	netErr, isNet := errors.AsType[net.Error](err)
	opErr, isOp := errors.AsType[*net.OpError](err)
	switch {
	case isDNS:
		return URLErrorCannotFindHost
	case isNet && netErr.Timeout():
		return URLErrorTimedOut
	case isOp && opErr.Op == "dial":
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
	defer ActorSuspend()()
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

// URLComponents is Foundation `URLComponents`. QueryItems is `queryItems`; the other parts are read
// through the URLComponents functions.
type URLComponents struct {
	QueryItems *[]URLQueryItem

	scheme    *string
	authority *string
	host      *string
	path      string
	rawPath   string
	rawQuery  *string
	parsed    []URLQueryItem
	fragment  *string
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
	parts := splitURL(s)
	c := &URLComponents{scheme: parts.scheme, authority: parts.authority, rawQuery: parts.query, fragment: parts.fragment}
	if parts.scheme != nil {
		for j, r := range *parts.scheme {
			if !isSchemeChar(r, j == 0) {
				return nil
			}
		}
	}
	if parts.authority != nil {
		host, port := splitAuthority(*parts.authority)
		if strings.HasPrefix(host, "[") && !strings.HasSuffix(host, "]") {
			return nil
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
	c.rawPath = parts.path
	c.path = percentDecodeLenient(parts.path)
	if parts.query != nil {
		c.parsed = parseQueryItems(*parts.query)
		c.QueryItems = new(slices.Clone(c.parsed))
	}
	return c
}

// URLComponentsHost is `URLComponents.host`: percent-decoded, nil when the URL has no authority.
func URLComponentsHost(c URLComponents) *string { return c.host }

// URLComponentsPath is `URLComponents.path`: percent-decoded.
func URLComponentsPath(c URLComponents) string { return c.path }

// NSURLSessionShared is Foundation `URLSession.shared`.
var NSURLSessionShared = NSURLSessionInitConfiguration(NSURLSessionConfigurationEphemeral)

// Data is `URLSession.data(for:)` as a method, so a URLSession satisfies a protocol whose
// requirement is that method (`extension URLSession: Transport {}`).
func (s URLSession) Data(request URLRequest) struct {
	F0 []byte
	F1 URLResponse
} {
	return NSURLSessionDataFor(s, request)
}

// NSHTTPURLResponseValueForHTTPHeaderField is `HTTPURLResponse.value(forHTTPHeaderField:)` under the name the
// emitter gives the Objective-C class's method.
func NSHTTPURLResponseValueForHTTPHeaderField(r HTTPURLResponse, field string) *string {
	return HTTPURLResponseValueForHTTPHeaderField(r, field)
}

// HTTPURLResponseOptInitUrlStatusCodeHttpVersionHeaderFields is `HTTPURLResponse(url:statusCode:httpVersion:headerFields:)`
// under the name the emitter gives the swift-corelibs-foundation class's initializer.
func HTTPURLResponseOptInitUrlStatusCodeHttpVersionHeaderFields(url URL, statusCode int, version *string, headerFields *map[string]string) *HTTPURLResponse {
	return NSHTTPURLResponseOptInitUrlStatusCodeHttpVersionHeaderFields(url, statusCode, version, headerFields)
}

// URLResponseUrl is `URLResponse.url` under the swift-corelibs-foundation name.
func URLResponseUrl(r URLResponse) *URL { return NSURLResponseUrl(r) }
