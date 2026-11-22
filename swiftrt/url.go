package swiftrt

import (
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// urlParts is a URL string split into the five RFC 3986 components, each as written. A nil part
// is absent; an empty one is present and empty.
type urlParts struct {
	scheme    *string
	authority *string
	path      string
	query     *string
	fragment  *string
}

func splitURL(s string) urlParts {
	var p urlParts
	rest := s
	if i := strings.IndexAny(s, ":/?#"); i > 0 && s[i] == ':' {
		p.scheme = new(s[:i])
		rest = s[i+1:]
	}
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		p.fragment = new(rest[i+1:])
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		p.query = new(rest[i+1:])
		rest = rest[:i]
	}
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		end := strings.IndexByte(rest, '/')
		if end < 0 {
			end = len(rest)
		}
		p.authority = new(rest[:end])
		rest = rest[end:]
	}
	p.path = rest
	return p
}

func (p urlParts) String() string {
	var b strings.Builder
	if p.scheme != nil {
		b.WriteString(*p.scheme + ":")
	}
	if p.authority != nil {
		b.WriteString("//" + *p.authority)
	}
	b.WriteString(p.path)
	if p.query != nil {
		b.WriteString("?" + *p.query)
	}
	if p.fragment != nil {
		b.WriteString("#" + *p.fragment)
	}
	return b.String()
}

// splitAuthority drops the user info and separates the host, with the brackets of an IP literal,
// from the port.
func splitAuthority(authority string) (host, port string) {
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		authority = authority[at+1:]
	}
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 {
			return authority, ""
		}
		return authority[:end+1], strings.TrimPrefix(authority[end+1:], ":")
	}
	if colon := strings.IndexByte(authority, ':'); colon >= 0 {
		return authority[:colon], authority[colon+1:]
	}
	return authority, ""
}

func isURLUnreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0
}

// isURLPathChar reports whether c may appear unencoded in a URL path: RFC 3986 `pchar` and `/`.
func isURLPathChar(c byte) bool {
	return isURLUnreserved(c) || strings.IndexByte("!$&'()*+,;=:@/", c) >= 0
}

// isURLQueryChar reports whether c may appear unencoded in a URL query or fragment.
func isURLQueryChar(c byte) bool { return isURLPathChar(c) || c == '?' }

// isURLQueryItemChar reports whether c may appear unencoded in a query item's name or value: the
// query characters without the `&` and `=` that delimit items.
func isURLQueryItemChar(c byte) bool { return isURLQueryChar(c) && c != '&' && c != '=' }

const upperHex = "0123456789ABCDEF"

// percentEncode encodes every byte allowed rejects. With keepEscapes a valid `%XX` stays as
// written; without, its `%` is encoded like any other disallowed byte.
func percentEncode(s string, allowed func(byte) bool, keepEscapes bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case allowed(c):
			b.WriteByte(c)
		case keepEscapes && c == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]):
			b.WriteString(s[i : i+3])
			i += 2
		default:
			b.WriteByte('%')
			b.WriteByte(upperHex[c>>4])
			b.WriteByte(upperHex[c&15])
		}
	}
	return b.String()
}

// URLAbsoluteString is Foundation `URL.absoluteString`.
func URLAbsoluteString(u URL) string { return u.Raw }

// URLScheme is Foundation `URL.scheme`: the scheme as written, or nil when the URL has none.
func URLScheme(u URL) *string { return splitURL(u.Raw).scheme }

// URLHost is Foundation `URL.host`: the percent-decoded host, without the brackets of an IPv6
// literal, or nil when the URL has no authority or an empty host.
func URLHost(u URL) *string {
	p := splitURL(u.Raw)
	if p.authority == nil {
		return nil
	}
	host, _ := splitAuthority(*p.authority)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if host == "" {
		return nil
	}
	return new(percentDecodeLenient(host))
}

// lastPathComponent is the part of a path after its last slash, ignoring trailing slashes; a path
// of only slashes gives "/".
func lastPathComponent(p string) string {
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	return trimmed[strings.LastIndexByte(trimmed, '/')+1:]
}

// deletingLastPathComponent is a path without its last component and the slashes around it: the
// root for a component directly under it, and empty for a relative path of one component.
func deletingLastPathComponent(p string) string {
	trimmed := strings.TrimRight(p, "/")
	if trimmed == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	i := strings.LastIndexByte(trimmed, '/')
	if i < 0 {
		return ""
	}
	parent := strings.TrimRight(trimmed[:i], "/")
	if parent == "" {
		return "/"
	}
	return parent
}

// pathExtension is the text after the last dot of a path's last component. A component that
// starts or ends with its only dot has none, and neither does one whose extension holds a space,
// a slash, or a bidirectional control.
func pathExtension(p string) string {
	c := lastPathComponent(p)
	dot := strings.LastIndexByte(c, '.')
	if dot <= 0 || dot == len(c)-1 {
		return ""
	}
	ext := c[dot+1:]
	for _, r := range ext {
		switch {
		case r == ' ', r == '/', r == 0x061c, r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
			return ""
		}
	}
	return ext
}

// NSStringLastPathComponent is Foundation `NSString.lastPathComponent`.
func NSStringLastPathComponent(s string) string { return lastPathComponent(s) }

// NSStringDeletingLastPathComponent is Foundation `NSString.deletingLastPathComponent`.
func NSStringDeletingLastPathComponent(s string) string { return deletingLastPathComponent(s) }

// URLLastPathComponent is Foundation `URL.lastPathComponent`: the percent-decoded last component of
// the path, "/" for a root path and empty for an empty one.
func URLLastPathComponent(u URL) string {
	return percentDecodeLenient(lastPathComponent(splitURL(u.Raw).path))
}

// URLDeletingLastPathComponent is Foundation `URL.deletingLastPathComponent()`: the URL of the
// directory holding the last component, with a trailing slash. A URL with an empty or root path, or
// one whose path ends in `..`, keeps its path.
func URLDeletingLastPathComponent(u URL) URL {
	p := splitURL(u.Raw)
	if p.path == "" {
		return u
	}
	if lastPathComponent(p.path) != ".." {
		p.path = deletingLastPathComponent(p.path)
	}
	if p.path != "" && !strings.HasSuffix(p.path, "/") {
		p.path += "/"
	}
	return URL{p.String()}
}

// URLDeletingPathExtension is Foundation `URL.deletingPathExtension()`: the URL without the
// extension of its last path component, keeping a trailing slash.
func URLDeletingPathExtension(u URL) URL {
	p := splitURL(u.Raw)
	ext := pathExtension(percentDecodeLenient(p.path))
	if ext == "" {
		return u
	}
	trimmed := strings.TrimRight(p.path, "/")
	stem := trimmed[:strings.LastIndexByte(trimmed, '.')]
	if trimmed != p.path {
		stem += "/"
	}
	p.path = stem
	return URL{p.String()}
}

// URLAppendingPath is Foundation `URL.appending(path:)`: it appends the path, which may hold
// several components, percent-encoding it and joining it to the URL's path with exactly one slash.
func URLAppendingPath(u URL, appended string) URL {
	p := splitURL(u.Raw)
	enc := percentEncode(appended, isURLPathChar, false)
	switch {
	case p.path == "":
		if p.authority != nil && !strings.HasPrefix(enc, "/") {
			enc = "/" + enc
		}
		p.path = enc
	case strings.HasSuffix(p.path, "/") && strings.HasPrefix(enc, "/"):
		p.path += enc[1:]
	case !strings.HasSuffix(p.path, "/") && !strings.HasPrefix(enc, "/"):
		p.path += "/" + enc
	default:
		p.path += enc
	}
	return URL{p.String()}
}

// fileURLRewritingPath is a file URL with its path passed through resolve; a directory URL's
// trailing slash stays.
func fileURLRewritingPath(u URL, resolve func(string) string) URL {
	parsed, err := url.Parse(u.Raw)
	if err != nil || parsed.Scheme != "file" {
		return u
	}
	resolved := resolve(parsed.Path)
	if strings.HasSuffix(parsed.Path, "/") && !strings.HasSuffix(resolved, "/") {
		resolved += "/"
	}
	return URL{(&url.URL{Scheme: "file", Path: resolved}).String()}
}

// URLStandardizedFileURL is Foundation `URL.standardizedFileURL`: a file URL with the `.`, `..` and
// empty components of its path removed. It does not resolve symbolic links, and returns any other
// URL unchanged.
func URLStandardizedFileURL(u URL) URL { return fileURLRewritingPath(u, path.Clean) }

// URLResolvingSymlinksInPath is Foundation `URL.resolvingSymlinksInPath()`: a file URL with its
// path standardized and its symbolic links resolved, or only standardized when part of the path
// does not exist. On Darwin a leading `/private` is dropped from `/private/var`, `/private/tmp`
// and `/private/etc`, as Foundation does there. Any other URL is returned unchanged.
func URLResolvingSymlinksInPath(u URL) URL {
	return fileURLRewritingPath(u, func(p string) string {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return path.Clean(p)
		}
		if runtime.GOOS == "darwin" {
			for _, dir := range []string{"/private/var", "/private/tmp", "/private/etc"} {
				if resolved == dir || strings.HasPrefix(resolved, dir+"/") {
					return strings.TrimPrefix(resolved, "/private")
				}
			}
		}
		return resolved
	})
}

// StringInitContentsOfEncoding is Foundation `String(contentsOf:encoding:)`: the text of the file
// at a file URL. It throws when the file cannot be read or is not text in the encoding.
func StringInitContentsOfEncoding(u URL, e StringEncoding) string {
	return StringInitContentsOfFileEncoding(fileURLPath(u), e)
}

// URLQueryItem is Foundation `URLQueryItem`.
type URLQueryItem struct {
	Name  string
	Value *string
}

// URLQueryItemInitNameValue is Foundation `URLQueryItem(name:value:)`.
func URLQueryItemInitNameValue(name string, value *string) URLQueryItem {
	return URLQueryItem{Name: name, Value: value}
}

// parseQueryItems is the `queryItems` of a query string: one item per `&`-separated piece, split
// at its first `=` and percent-decoded, with a nil value for a piece that has no `=`.
func parseQueryItems(query string) []URLQueryItem {
	pieces := strings.Split(query, "&")
	items := make([]URLQueryItem, len(pieces))
	for i, piece := range pieces {
		name, value, found := strings.Cut(piece, "=")
		items[i].Name = percentDecodeLenient(name)
		if found {
			items[i].Value = new(percentDecodeLenient(value))
		}
	}
	return items
}

func sameQueryItems(a, b []URLQueryItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || (a[i].Value == nil) != (b[i].Value == nil) || a[i].Value != nil && *a[i].Value != *b[i].Value {
			return false
		}
	}
	return true
}

// query is the percent-encoded query: the one the components were parsed from while QueryItems
// still holds its items, and otherwise the items encoded as `name=value` pairs joined by `&`.
func (c URLComponents) query() *string {
	if c.QueryItems == nil {
		return nil
	}
	if c.rawQuery != nil && sameQueryItems(*c.QueryItems, c.parsed) {
		return new(percentEncode(*c.rawQuery, isURLQueryChar, true))
	}
	pieces := make([]string, len(*c.QueryItems))
	for i, item := range *c.QueryItems {
		pieces[i] = percentEncode(item.Name, isURLQueryItemChar, false)
		if item.Value != nil {
			pieces[i] += "=" + percentEncode(*item.Value, isURLQueryItemChar, false)
		}
	}
	return new(strings.Join(pieces, "&"))
}

// URLComponentsOptInitUrlResolvingAgainstBaseURL is Foundation
// `URLComponents(url:resolvingAgainstBaseURL:)`. A URL here is always absolute, so the flag changes
// nothing.
func URLComponentsOptInitUrlResolvingAgainstBaseURL(u URL, _ bool) *URLComponents {
	return URLComponentsOptInitString(u.Raw)
}

// URLComponentsUrl is Foundation `URLComponents.url`: the URL built from the components, with
// characters a part cannot hold percent-encoded. It is nil when the components are empty, or when
// the path cannot follow the authority: a relative path after one, or a `//` path without one.
func URLComponentsUrl(c URLComponents) *URL {
	p := urlParts{scheme: c.scheme, authority: c.authority, query: c.query()}
	p.path = percentEncode(c.rawPath, isURLPathChar, true)
	if c.fragment != nil {
		p.fragment = new(percentEncode(*c.fragment, isURLQueryChar, true))
	}
	if p.authority != nil && p.path != "" && !strings.HasPrefix(p.path, "/") {
		return nil
	}
	if p.authority == nil && strings.HasPrefix(p.path, "//") {
		return nil
	}
	s := p.String()
	if s == "" {
		return nil
	}
	return &URL{s}
}
