// Package ast loads the JSON AST that `swiftc -dump-ast -dump-ast-format json` writes.
package ast

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Node is one AST node: a JSON object whose "_kind" names the node type.
type Node map[string]any

// Kind returns the node's "_kind".
func (n Node) Kind() string { return n.Str("_kind") }

// Str returns a string field, or "" when absent.
func (n Node) Str(k string) string {
	if n == nil {
		return ""
	}
	s, _ := n[k].(string)
	return s
}

// Bool returns a boolean field; the dump writes some booleans as strings.
func (n Node) Bool(k string) bool {
	if n == nil {
		return false
	}
	switch v := n[k].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// Child returns an object field.
func (n Node) Child(k string) Node {
	if n == nil {
		return nil
	}
	m, _ := n[k].(map[string]any)
	return Node(m)
}

// List returns an array-of-objects field.
func (n Node) List(k string) []Node {
	if n == nil {
		return nil
	}
	arr, _ := n[k].([]any)
	out := make([]Node, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			out = append(out, Node(m))
		}
	}
	return out
}

// Strings returns an array-of-strings field.
func (n Node) Strings(k string) []string {
	arr, _ := n[k].([]any)
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

// Implicit reports whether the compiler synthesised the node.
func (n Node) Implicit() bool { return n.Bool("implicit") }

// Name returns decl_name.base_name.name (or "init"/special names).
func (n Node) Name() string {
	bn := n.Child("name").Child("base_name")
	if s := bn.Str("name"); s != "" {
		return s
	}
	return bn.Str("special")
}

// Type returns the node's mangled "type".
func (n Node) Type() string { return n.Str("type") }

// Walk calls f for n and every descendant object, depth first.
func Walk(n any, f func(Node)) {
	switch v := n.(type) {
	case map[string]any:
		f(Node(v))
		for _, c := range v {
			Walk(c, f)
		}
	case Node:
		Walk(map[string]any(v), f)
	case []any:
		for _, c := range v {
			Walk(c, f)
		}
	}
}

// File is one source file's AST.
type File struct {
	Filename string
	Items    []Node
	// Hash identifies the document the file was decoded from; "" when the loader did not hash it.
	Hash string
}

// Load decodes the concatenated per-file JSON documents of one dump.
func Load(data []byte) ([]File, error) {
	var files []File
	marker := []byte(`{"_kind":"source_file"`)
	for {
		i := bytes.Index(data, marker)
		if i < 0 {
			break
		}
		data = data[i:]
		dec := json.NewDecoder(bytes.NewReader(data))
		var m map[string]any
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("decode AST: %w", err)
		}
		data = data[dec.InputOffset():]
		n := Node(m)
		files = append(files, File{Filename: n.Str("filename"), Items: n.List("items")})
	}
	return files, nil
}
