// Package deps decides which files of a Swift module an edit can affect, for incremental transpiles.
//
// A module's interface is a [Table]: a digest for each [Key], which names a top-level declaration, a member
// of a type, or a type as a whole. A file depends on a key when it mentions the key's name, or for a change
// to a type as a whole or to a member Swift finds without the file naming it, such as an initializer, when
// it mentions the type: [Affects] decides. The names a file mentions are the identifiers and operators of
// its source and those its typed AST records, which include the types the compiler inferred.
package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
)

// Key names a part of a module's interface.
type Key struct {
	// File, when set, limits the key to one file: a private declaration, which only its file sees.
	File string
	// Owner is the simple name of the type a member belongs to, or of the type itself; "" at top level.
	Owner string
	// Name is a member's or top-level declaration's base name; "" for the type Owner as a whole.
	Name string
}

// All is the key of a declaration that can change how any file compiles, such as a typealias or an
// operator, whose uses the typed AST does not always record.
var All = Key{Owner: "*", Name: "*"}

// Table maps each key a module declares to a digest of the declarations under it.
type Table map[Key]string

// Hash identifies the table's content.
func (t Table) Hash() string {
	h := sha256.New()
	for _, k := range sortedKeys(t) {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\n", k.File, k.Owner, k.Name, t[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ExportedHash identifies what the table shows other files than those declaring it: its keys that
// are not limited to one file.
func (t Table) ExportedHash() string {
	exported := Table{}
	for k, d := range t {
		if k.File == "" {
			exported[k] = d
		}
	}
	return exported.Hash()
}

// Add merges a digest into the key's: a key some files declare more than once, such as an overloaded
// name, digests all of its declarations.
func (t Table) Add(k Key, digest string) {
	if old, ok := t[k]; ok {
		digest = combine(old, digest)
	}
	t[k] = digest
}

// Merge adds every key of other.
func (t Table) Merge(other Table) {
	for _, k := range sortedKeys(other) {
		t.Add(k, other[k])
	}
}

func combine(a, b string) string {
	if b < a {
		a, b = b, a
	}
	sum := sha256.Sum256([]byte(a + "+" + b))
	return hex.EncodeToString(sum[:])
}

func sortedKeys(t Table) []Key {
	keys := slices.Collect(maps.Keys(t))
	slices.SortFunc(keys, func(a, b Key) int {
		return strings.Compare(a.File+"\x00"+a.Owner+"\x00"+a.Name, b.File+"\x00"+b.Owner+"\x00"+b.Name)
	})
	return keys
}

// Changed lists the keys whose digests differ between two tables, including those only one has.
func Changed(old, cur Table) []Key {
	var out []Key
	for k, d := range cur {
		if old[k] != d {
			out = append(out, k)
		}
	}
	for k := range old {
		if _, ok := cur[k]; !ok {
			out = append(out, k)
		}
	}
	slices.SortFunc(out, func(a, b Key) int {
		return strings.Compare(a.File+"\x00"+a.Owner+"\x00"+a.Name, b.File+"\x00"+b.Owner+"\x00"+b.Name)
	})
	return out
}

// Names are the distinct names a file mentions, sorted.
type Names []string

// Has reports whether the file mentions name.
func (n Names) Has(name string) bool {
	_, ok := slices.BinarySearch(n, name)
	return ok
}

// implicit are the names of members that Swift calls without the source naming them, for a value or
// metatype of their type: initializers
// (`T(...)`), subscripts, the requirements behind literals, loops, patterns, property wrappers, result
// builders and synthesized conformances, and dynamic member lookup.
var implicit = map[string]bool{
	"init": true, "deinit": true, "subscript": true, "callAsFunction": true, "makeIterator": true, "next": true,
	"makeAsyncIterator": true, "hash": true, "hashValue": true, "encode": true, "rawValue": true, "allCases": true,
	"description": true, "debugDescription": true, "wrappedValue": true, "projectedValue": true, "~=": true,
	"==": true, "<": true, "buildBlock": true, "buildExpression": true, "buildOptional": true, "buildEither": true,
	"buildArray": true, "buildFinalResult": true, "buildPartialBlock": true, "buildLimitedAvailability": true,
}

// Affects reports whether a change to k can change how the file, which mentions names, compiles or
// translates.
func Affects(k Key, file string, names Names) bool {
	switch {
	case k == All:
		return true
	case k.File != "":
		return k.File == file
	case k.Name == "":
		return names.Has(k.Owner)
	case k.Owner != "" && implicit[k.Name]:
		// The typed AST names the type whose member Swift calls, as the type of an expression.
		return names.Has(k.Owner)
	}
	return names.Has(k.Name)
}

// AffectsAny reports whether a change to any of keys can affect the file.
func AffectsAny(keys []Key, file string, names Names) bool {
	return slices.ContainsFunc(keys, func(k Key) bool { return Affects(k, file, names) })
}

// symbolKeys are the fields of the typed AST that hold mangled types and USRs.
var symbolKeys = []string{"type", "usr", "decl_usr", "type_usr", "interface_type", "result", "extended_type", "protocol", "superclass_type", "first_type", "second_type", "replacement_type", "generic_param"}

// Symbols lists the mangled types and USRs in a file's typed AST, which [Scan] needs demangled.
func Symbols(items []ast.Node) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		ast.Walk(it, func(n ast.Node) {
			for _, k := range symbolKeys {
				if s := n.Str(k); (strings.HasPrefix(s, "$s") || strings.HasPrefix(s, "s:")) && !seen[s] {
					seen[s] = true
					out = append(out, s)
				}
			}
			for _, p := range n.Strings("protocols") {
				if strings.HasPrefix(p, "s:") && !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
		})
	}
	return out
}

// Scan reads one file: the interface it declares and the names it mentions. src is the file's source,
// whose identifiers count among the names; demangle turns a symbol from [Symbols] into Swift's spelling.
func Scan(file string, items []ast.Node, src []byte, demangle func(string) string) (Table, Names) {
	names := map[string]bool{}
	addTokens(names, string(src))
	for _, it := range items {
		ast.Walk(it, func(n ast.Node) {
			if b := n.Str("base_name"); b != "" {
				names[b] = true
			}
			if d := n.Str("decl"); d != "" {
				// "Mod.(file).Type.member(_:)@/path/File.swift:3:5"
				d, _, _ = strings.Cut(d, "@")
				addTokens(names, d)
			}
			for _, k := range symbolKeys {
				if s := n.Str(k); strings.HasPrefix(s, "$s") || strings.HasPrefix(s, "s:") {
					addTokens(names, demangle(s))
				}
			}
			for _, p := range n.Strings("protocols") {
				addTokens(names, demangle(p))
			}
		})
	}
	t := Table{}
	s := scanner{file: file, table: t, demangle: demangle}
	for _, it := range items {
		s.decl(it, "", "", false)
	}
	return t, slices.Sorted(maps.Keys(names))
}

type scanner struct {
	file     string
	table    Table
	demangle func(string) string
}

func private(n ast.Node) bool {
	a := n.Str("access")
	return a == "private" || a == "fileprivate"
}

// decl records declaration n, a member of owner (a type of the given kind) or top level when owner is "".
func (s *scanner) decl(n ast.Node, owner, ownerKind string, hidden bool) {
	kind := n.Kind()
	hidden = hidden || private(n)
	switch kind {
	case "struct_decl", "class_decl", "enum_decl", "protocol":
		name := n.Name()
		s.add(Key{Owner: name}, hidden, TypeShape(n))
		if owner != "" {
			s.add(Key{Owner: owner, Name: name}, hidden, TypeShape(n))
		}
		for _, m := range n.List("members") {
			s.decl(m, name, kind, hidden)
		}
	case "extension_decl":
		ext := SimpleName(s.demangle(n.Str("extended_type")))
		if inh := n.Child("inherits"); len(inh.List("conformances")) > 0 || len(inh.Strings("protocols")) > 0 {
			shape := Digest(map[string]any{"inherits": map[string]any(inh), "generic_signature": n["generic_signature"]})
			s.add(Key{Owner: ext}, hidden, shape)
			for _, c := range inh.List("conformances") {
				s.add(Key{Owner: SimpleName(s.demangle(c.Str("protocol")))}, hidden, "conformer "+ext+" "+shape)
			}
		}
		extKind := "extension"
		if strings.HasPrefix(s.demangle(n.Str("extended_type")), "any ") {
			extKind = "protocol"
		}
		for _, m := range n.List("members") {
			s.decl(m, ext, extKind, hidden)
		}
	case "typealias", "associated_type_decl", "infix_operator_decl", "prefix_operator_decl", "postfix_operator_decl", "precedence_group_decl", "macro_decl":
		if kind == "associated_type_decl" && owner != "" {
			s.add(Key{Owner: owner}, hidden, Digest(n))
			return
		}
		s.add(All, hidden, Digest(n))
	case "top_level_code_decl":
		// A script's top-level statements declare globals.
		for _, el := range n.Child("body").List("elements") {
			s.decl(el, owner, ownerKind, hidden)
		}
	case "func_decl", "constructor_decl", "destructor_decl", "var_decl", "subscript_decl", "enum_element_decl":
		name := n.Name()
		if kind == "subscript_decl" {
			name = "subscript"
		}
		d := Digest(n)
		// A struct's stored properties decide its memberwise initializer, and an enum's cases its
		// exhaustive switches, whoever can see them; a protocol's members decide its conformers.
		switch {
		case kind == "enum_element_decl",
			ownerKind == "struct_decl" && kind == "var_decl" && !n.Bool("static") && isStored(n):
			s.table.Add(Key{Owner: owner}, d)
		case ownerKind == "protocol" && !hidden:
			s.table.Add(Key{Owner: owner}, d)
		}
		s.add(Key{Owner: owner, Name: name}, hidden, d)
	}
}

// add records a digest under k, or under the file's own key when the declaration is private.
func (s *scanner) add(k Key, hidden bool, digest string) {
	if hidden {
		k.File = s.file
	}
	s.table.Add(k, digest)
}

func isStored(n ast.Node) bool {
	impl := n.Str("readImpl")
	return impl == "" || impl == "stored"
}

// TypeShape digests what a type's declaration says of the type as a whole: its kind, attributes, access,
// inheritance and generic signature, and the members that decide its layout: stored properties and cases.
func TypeShape(n ast.Node) string {
	shape := map[string]any{}
	for k, v := range ast.Fields(n) {
		if k != "members" {
			shape[k] = v
		}
	}
	var layout []any
	for _, m := range n.List("members") {
		switch m.Kind() {
		case "enum_element_decl", "enum_case_decl":
			layout = append(layout, map[string]any(m))
		case "var_decl":
			if !m.Bool("static") && isStored(m) {
				layout = append(layout, map[string]any(m))
			}
		}
	}
	shape["members"] = layout
	return Digest(shape)
}

// SimpleName is the last component of a demangled type's name: "World.Outer.Inner<Swift.Int>" is "Inner".
func SimpleName(t string) string {
	t = strings.TrimPrefix(t, "any ")
	if i := strings.IndexAny(t, "<("); i >= 0 {
		t = t[:i]
	}
	if i := strings.LastIndex(t, "."); i >= 0 {
		t = t[i+1:]
	}
	return t
}

// addTokens adds the identifiers and operators in s.
func addTokens(set map[string]bool, s string) {
	for i := 0; i < len(s); {
		c := s[i]
		j := i + 1
		switch {
		case isIdentStart(c):
			for j < len(s) && isIdent(s[j]) {
				j++
			}
		case strings.IndexByte(operatorChars, c) >= 0:
			for j < len(s) && strings.IndexByte(operatorChars, s[j]) >= 0 {
				j++
			}
		default:
			i = j
			continue
		}
		set[s[i:j]] = true
		i = j
	}
}

const operatorChars = "/=-+!*%<>&|^~?."

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdent(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }

// bodyKinds are the declarations whose bodies no other file type-checks against.
var bodyKinds = map[string]bool{
	"func_decl": true, "constructor_decl": true, "destructor_decl": true, "accessor_decl": true,
}

// Digest hashes what a declaration shows other files: its types, attributes and conformances, but not
// the bodies of functions, initializers and accessors, nor the expressions that initialize variables,
// whose types the AST records with the variable, nor source ranges and the dump's numbering of
// pointers, which shift when a body is edited. The body or initial value that decides the underlying
// type of an opaque type (`some P`) counts.
func Digest(v any) string {
	h := sha256.New()
	writeCanonical(h, StripBodies(v))
	return hex.EncodeToString(h.Sum(nil))
}

// StripBodies returns v without what [Digest] leaves out.
func StripBodies(v any) any {
	switch n := v.(type) {
	case ast.Node:
		return StripBodies(map[string]any(n))
	case map[string]any:
		kind, _ := n["_kind"].(string)
		dropBody := bodyKinds[kind] && !opaqueResult(n)
		pattern, _ := n["pattern"].(map[string]any)
		dropInit := kind == "pattern_entry" && !opaqueResult(pattern)
		out := make(map[string]any, len(n))
		for k, c := range n {
			switch {
			case k == "range" || k == "decl_context":
			case k == "body" && dropBody:
			case dropInit && (k == "original_init" || k == "processed_init"):
				out[k] = "present"
			case dropInit && k == "init_context":
			default:
				out[k] = StripBodies(c)
			}
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, c := range n {
			out[i] = StripBodies(c)
		}
		return out
	case string:
		if strings.HasPrefix(n, "replaced-pointer-") {
			return "pointer"
		}
	}
	return v
}

// opaqueResult reports whether a declaration's type mentions an opaque result type. Opaque types
// mangle as `Qo` (a reference) or `QO`, `QR` and `Qr` (the declaration's own).
func opaqueResult(n map[string]any) bool {
	for _, k := range []string{"interface_type", "result", "type"} {
		if s, ok := n[k].(string); ok {
			for _, m := range []string{"Qo", "QO", "QR", "Qr"} {
				if strings.Contains(s, m) {
					return true
				}
			}
		}
	}
	return false
}

// WriteCanonical writes v as JSON with its object keys sorted.
func writeCanonical(w io.Writer, v any) {
	switch n := v.(type) {
	case ast.Node:
		writeCanonical(w, map[string]any(n))
	case map[string]any:
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_, _ = io.WriteString(w, "{")
		for _, k := range keys {
			_, _ = fmt.Fprintf(w, "%q:", k)
			writeCanonical(w, n[k])
			_, _ = io.WriteString(w, ",")
		}
		_, _ = io.WriteString(w, "}")
	case []any:
		_, _ = io.WriteString(w, "[")
		for _, c := range n {
			writeCanonical(w, c)
			_, _ = io.WriteString(w, ",")
		}
		_, _ = io.WriteString(w, "]")
	default:
		data, _ := json.Marshal(n)
		_, _ = w.Write(data)
	}
}
