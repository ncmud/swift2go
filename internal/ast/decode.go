package ast

import (
	"errors"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// errSyntax sends a document the decoder does not read to encoding/json, which reports the error.
var errSyntax = errors.New("ast: unsupported JSON")

// decoder reads one JSON value into the values encoding/json gives an `any`: map[string]any,
// []any, string, float64, bool and nil. Maps and slices are allocated at their final size, and
// equal strings within one document share their memory.
type decoder struct {
	data    []byte
	pos     int
	strs    map[string]string
	members []member
	elems   []any
}

type member struct {
	key string
	val any
}

// decodeValue decodes the JSON value at the start of data and returns it with the offset just
// past it.
func decodeValue(data []byte) (any, int, error) {
	d := &decoder{data: data, strs: make(map[string]string, 1024)}
	v, err := d.value()
	if err != nil {
		return nil, 0, err
	}
	return v, d.pos, nil
}

func (d *decoder) skipSpace() {
	for d.pos < len(d.data) {
		switch d.data[d.pos] {
		case ' ', '\t', '\n', '\r':
			d.pos++
		default:
			return
		}
	}
}

func (d *decoder) value() (any, error) {
	d.skipSpace()
	if d.pos >= len(d.data) {
		return nil, errSyntax
	}
	switch c := d.data[d.pos]; {
	case c == '{':
		return d.object()
	case c == '[':
		return d.array()
	case c == '"':
		return d.str()
	case c == 't':
		return true, d.literal("true")
	case c == 'f':
		return false, d.literal("false")
	case c == 'n':
		return nil, d.literal("null")
	case c == '-' || c >= '0' && c <= '9':
		return d.number()
	}
	return nil, errSyntax
}

func (d *decoder) literal(lit string) error {
	if len(d.data)-d.pos < len(lit) || string(d.data[d.pos:d.pos+len(lit)]) != lit {
		return errSyntax
	}
	d.pos += len(lit)
	return nil
}

func (d *decoder) object() (any, error) {
	d.pos++
	base := len(d.members)
	d.skipSpace()
	if d.pos < len(d.data) && d.data[d.pos] == '}' {
		d.pos++
		return map[string]any{}, nil
	}
	for {
		d.skipSpace()
		if d.pos >= len(d.data) || d.data[d.pos] != '"' {
			return nil, errSyntax
		}
		k, err := d.str()
		if err != nil {
			return nil, err
		}
		d.skipSpace()
		if d.pos >= len(d.data) || d.data[d.pos] != ':' {
			return nil, errSyntax
		}
		d.pos++
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		d.members = append(d.members, member{k, v})
		d.skipSpace()
		if d.pos >= len(d.data) {
			return nil, errSyntax
		}
		c := d.data[d.pos]
		d.pos++
		if c == '}' {
			break
		}
		if c != ',' {
			return nil, errSyntax
		}
	}
	ms := d.members[base:]
	m := make(map[string]any, len(ms))
	for _, kv := range ms {
		m[kv.key] = kv.val
	}
	clear(ms)
	d.members = d.members[:base]
	return m, nil
}

func (d *decoder) array() (any, error) {
	d.pos++
	base := len(d.elems)
	d.skipSpace()
	if d.pos < len(d.data) && d.data[d.pos] == ']' {
		d.pos++
		return []any{}, nil
	}
	for {
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		d.elems = append(d.elems, v)
		d.skipSpace()
		if d.pos >= len(d.data) {
			return nil, errSyntax
		}
		c := d.data[d.pos]
		d.pos++
		if c == ']' {
			break
		}
		if c != ',' {
			return nil, errSyntax
		}
	}
	es := d.elems[base:]
	a := make([]any, len(es))
	copy(a, es)
	clear(es)
	d.elems = d.elems[:base]
	return a, nil
}

// str decodes the string at d.pos as encoding/json does, replacing invalid UTF-8 and unpaired
// surrogates with U+FFFD.
func (d *decoder) str() (string, error) {
	start := d.pos + 1
	i := start
	ascii := true
	for i < len(d.data) {
		c := d.data[i]
		if c == '"' {
			raw := d.data[start:i]
			d.pos = i + 1
			if !ascii && !utf8.Valid(raw) {
				return d.intern(d.unquoteSlow(raw)), nil
			}
			return d.intern(raw), nil
		}
		if c == '\\' {
			return d.escaped(start)
		}
		if c < 0x20 {
			return "", errSyntax
		}
		if c >= utf8.RuneSelf {
			ascii = false
		}
		i++
	}
	return "", errSyntax
}

func (d *decoder) intern(b []byte) string {
	if s, ok := d.strs[string(b)]; ok {
		return s
	}
	s := string(b)
	d.strs[s] = s
	return s
}

// escaped decodes a string that holds an escape sequence, starting at its first content byte.
func (d *decoder) escaped(start int) (string, error) {
	i := start
	for i < len(d.data) {
		switch c := d.data[i]; {
		case c == '\\':
			i += 2
			continue
		case c == '"':
			out, ok := unquote(d.data[start:i])
			if !ok {
				return "", errSyntax
			}
			d.pos = i + 1
			return d.intern(out), nil
		case c < 0x20:
			return "", errSyntax
		}
		i++
	}
	return "", errSyntax
}

func (d *decoder) unquoteSlow(raw []byte) []byte {
	out, _ := unquote(raw)
	return out
}

// unquote decodes the content of a JSON string, following encoding/json's unquoteBytes.
func unquote(s []byte) ([]byte, bool) {
	b := make([]byte, 0, len(s)+2*utf8.UTFMax)
	for r := 0; r < len(s); {
		c := s[r]
		switch {
		case c == '\\':
			r++
			if r >= len(s) {
				return nil, false
			}
			switch s[r] {
			default:
				return nil, false
			case '"', '\\', '/':
				b = append(b, s[r])
				r++
			case 'b':
				b = append(b, '\b')
				r++
			case 'f':
				b = append(b, '\f')
				r++
			case 'n':
				b = append(b, '\n')
				r++
			case 'r':
				b = append(b, '\r')
				r++
			case 't':
				b = append(b, '\t')
				r++
			case 'u':
				r--
				rr := getu4(s[r:])
				if rr < 0 {
					return nil, false
				}
				r += 6
				if utf16.IsSurrogate(rr) {
					rr1 := getu4(s[r:])
					if dec := utf16.DecodeRune(rr, rr1); dec != utf8.RuneError {
						r += 6
						b = utf8.AppendRune(b, dec)
						break
					}
					rr = utf8.RuneError
				}
				b = utf8.AppendRune(b, rr)
			}
		case c == '"' || c < ' ':
			return nil, false
		case c < utf8.RuneSelf:
			b = append(b, c)
			r++
		default:
			rr, size := utf8.DecodeRune(s[r:])
			r += size
			b = utf8.AppendRune(b, rr)
		}
	}
	return b, true
}

// getu4 decodes \uXXXX from the beginning of s, returning -1 when it is not one.
func getu4(s []byte) rune {
	if len(s) < 6 || s[0] != '\\' || s[1] != 'u' {
		return -1
	}
	var r rune
	for _, c := range s[2:6] {
		switch {
		case c >= '0' && c <= '9':
			c -= '0'
		case c >= 'a' && c <= 'f':
			c = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			c = c - 'A' + 10
		default:
			return -1
		}
		r = r*16 + rune(c)
	}
	return r
}

func (d *decoder) number() (any, error) {
	start := d.pos
	i := d.pos
	if d.data[i] == '-' {
		i++
	}
	digits := func() int {
		n := 0
		for i < len(d.data) && d.data[i] >= '0' && d.data[i] <= '9' {
			i++
			n++
		}
		return n
	}
	if i < len(d.data) && d.data[i] == '0' {
		i++
	} else if digits() == 0 {
		return nil, errSyntax
	}
	if i < len(d.data) && d.data[i] == '.' {
		i++
		if digits() == 0 {
			return nil, errSyntax
		}
	}
	if i < len(d.data) && (d.data[i] == 'e' || d.data[i] == 'E') {
		i++
		if i < len(d.data) && (d.data[i] == '+' || d.data[i] == '-') {
			i++
		}
		if digits() == 0 {
			return nil, errSyntax
		}
	}
	f, err := strconv.ParseFloat(string(d.data[start:i]), 64)
	if err != nil {
		return nil, errSyntax
	}
	d.pos = i
	return f, nil
}
