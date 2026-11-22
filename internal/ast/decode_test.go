package ast

import (
	"encoding/json"
	"reflect"
	"testing"
)

var decodeCases = []string{
	`{}`, `[]`, `null`, `true`, `false`, `0`, `-0`, `12.5e-3`, `1E+2`, `-7`,
	`{"_kind":"source_file","items":[{"a":1,"b":[true,false,null],"c":{"d":"e"}}]}`,
	`{"k":"v","k":"w"}`,
	` { "a" : [ 1 , 2 ] , "b" : { } } `,
	`"plain"`, `"esc \" \\ \/ \b \f \n \r \t"`, `"é中"`, `"😀"`, `"\ud83d"`, `"\ud83dx"`,
	`"\udc00\ud800"`, "\"caf\xc3\xa9\"", "\"bad \xff utf8\"", "\"trunc \xe4\xb8\"", `"\u0000"`,
	`{"s":"replaced-pointer-1","t":"replaced-pointer-1","u":["replaced-pointer-1"]}`,
	`1e400`, `01`, `"\x"`, `"\'"`, "\"ctl \x01\"", `[1,]`, `{"a":1,}`, `{"a" 1}`, `tru`, `-`, `1.`, `1e`, `"open`, `[1 2]`,
}

func checkDecode(t *testing.T, in string) {
	t.Helper()
	got, end, err := decodeValue([]byte(in))
	if err != nil {
		return // the caller falls back to encoding/json
	}
	var want any
	if err := json.Unmarshal([]byte(in[:end]), &want); err != nil {
		t.Fatalf("decodeValue accepted %q, which encoding/json rejects: %v", in, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeValue(%q) = %#v, encoding/json gives %#v", in, got, want)
	}
}

func TestDecodeValueMatchesEncodingJSON(t *testing.T) {
	for _, in := range decodeCases {
		checkDecode(t, in)
	}
	for _, in := range []string{`{"a":[1,{"b":"c"}]}`, `"é"`, `-12.5e3`, `[]`, `{}`} {
		if _, _, err := decodeValue([]byte(in)); err != nil {
			t.Errorf("decodeValue(%q): %v", in, err)
		}
	}
}

func FuzzDecodeValue(f *testing.F) {
	for _, in := range decodeCases {
		f.Add(in)
	}
	f.Fuzz(checkDecode)
}

func TestLoadDecodesConcatenatedDocuments(t *testing.T) {
	data := `noise {"_kind":"source_file","filename":"a.swift","items":[{"_kind":"import_decl"}]}
{"_kind":"source_file","filename":"b.swift","items":[]}`
	files, err := Load([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Filename != "a.swift" || files[1].Filename != "b.swift" || files[0].Items[0].Kind() != "import_decl" {
		t.Fatalf("Load = %+v", files)
	}
}
