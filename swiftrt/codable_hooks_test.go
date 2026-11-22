package swiftrt

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// fakeRow is a keyed source in the shape a database row takes: ordered columns, looked up without regard
// to case, holding storage-class values rather than JSON-shaped ones.
type fakeRow struct {
	names  []string
	values []any
}

func (r fakeRow) Lookup(key string) (any, bool) {
	for i, n := range r.names {
		if strings.EqualFold(n, key) {
			if r.values[i] == nil {
				return nil, true
			}
			return fakeColumn{r.values[i]}, true
		}
	}
	return nil, false
}

func (r fakeRow) Keys() []string { return r.names }

// fakeColumn decodes one stored value: Bool from INTEGER, and nested values from JSON text.
type fakeColumn struct{ v any }

func (c fakeColumn) SwiftDecodeInto(dst reflect.Value, path []string) bool {
	switch dst.Kind() {
	case reflect.Bool:
		n, ok := c.v.(int64)
		if !ok {
			codingFail("not an integer at %s", strings.Join(path, "."))
		}
		dst.SetBool(n != 0)
		return true
	case reflect.Int, reflect.Int64:
		n, ok := c.v.(int64)
		if !ok {
			return false
		}
		dst.SetInt(n)
		return true
	case reflect.Slice, reflect.Map, reflect.Struct:
		text, ok := c.v.(string)
		if !ok {
			return false
		}
		var tree any
		if err := json.Unmarshal([]byte(text), &tree); err != nil {
			codingFail("bad JSON at %s", strings.Join(path, "."))
		}
		decodeInto(dst, tree, path)
		return true
	case reflect.String:
		s, ok := c.v.(string)
		if !ok {
			return false
		}
		dst.SetString(s)
		return true
	}
	return false
}

type hookedRecord struct {
	ID    int64    `swift:"id"`
	Read  bool     `swift:"read"`
	Tags  []string `swift:"tags"`
	Owner *string  `swift:"owner"`
	Note  *string  `swift:"note"`
}

func TestDecodeFromRawKeyedSource(t *testing.T) {
	row := fakeRow{names: []string{"ID", "read", "tags", "owner"}, values: []any{int64(7), int64(1), `["a","b"]`, nil}}
	got := DecodeValue[hookedRecord](row, nil)
	want := hookedRecord{ID: 7, Read: true, Tags: []string{"a", "b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

type handDecoded struct {
	Flag    bool
	Present bool
	Keys    int
}

func TestRegisteredDecoderReadsRawKeyedSource(t *testing.T) {
	RegisterDecoder(func(d *Decoder) handDecoded {
		c := DecoderContainerKeyedBy(d, nil)
		return handDecoded{
			Flag:    KeyedDecodingContainerDecodeForKey(c, TypeT[bool]{}, "flag"),
			Present: KeyedDecodingContainerContains(c, "other"),
			Keys:    len(KeyedDecodingContainerAllKeys(c, func(s string) *string { return &s })),
		}
	})
	got := DecodeValue[handDecoded](fakeRow{names: []string{"flag", "extra"}, values: []any{int64(0), int64(3)}}, nil)
	if want := (handDecoded{Flag: false, Present: false, Keys: 2}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRawKeyedMissingKeyThrows(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("decoding a missing non-optional key did not throw")
		}
	}()
	DecodeValue[hookedRecord](fakeRow{names: []string{"read"}, values: []any{int64(0)}}, nil)
}
