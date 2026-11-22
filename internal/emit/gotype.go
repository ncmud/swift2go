package emit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ncmud/swift2go/internal/types"
)

var scalarTypes = map[string]string{
	"Swift.Int128":            "swiftrt.Int128",
	"Swift.Duration":          "swiftrt.Duration",
	"Swift.CancellationError": "swiftrt.CancellationError",
	"Swift.Int":               "int", "Swift.Int8": "int8", "Swift.Int16": "int16", "Swift.Int32": "int32", "Swift.Int64": "int64",
	"Swift.UInt": "uint", "Swift.UInt8": "uint8", "Swift.UInt16": "uint16", "Swift.UInt32": "uint32", "Swift.UInt64": "uint64",
	"Swift.Double": "float64", "Swift.Float": "float32", "Swift.Bool": "bool",
	"Swift.String": "string", "Swift.Substring": "swiftrt.Substring",
	"Swift.Character": "swiftrt.Character", "Swift.Unicode.Scalar": "rune", "Unicode.Scalar": "rune",
	"Swift.String.Index": "int", "Swift.String.UnicodeScalarView": "[]rune", "Swift.String.UTF8View": "[]byte",
	"Swift.Substring.UnicodeScalarView": "[]rune",
	"Swift.Error":                       "error", "Swift.Void": "", "Swift.Never": "",
	"Swift.StaticString": "string",
}

// GoType maps a parsed Swift type to Go; generics maps generic parameter names in scope.
func (m *Module) GoType(t *types.Type, generics map[string]string) string {
	if t == nil {
		return "any"
	}
	switch {
	case (t.Kind == types.Array || t.Kind == types.Optional) && len(t.Args) < 1, t.Kind == types.Dict && len(t.Args) < 2:
		return "any"
	}
	switch t.Kind {
	case types.Array:
		return "[]" + m.GoType(t.Args[0], generics)
	case types.Dict:
		if !m.goKeyable(t.Args[0], map[string]bool{}) {
			return "swiftrt.HMap[" + m.GoType(t.Args[0], generics) + ", " + m.GoType(t.Args[1], generics) + "]"
		}
		return "map[" + m.GoType(t.Args[0], generics) + "]" + m.GoType(t.Args[1], generics)
	case types.Optional:
		inner := m.GoType(t.Args[0], generics)
		if inner == "" {
			return "*struct{}"
		}
		if m.isReferenceGo(t.Args[0], inner) {
			return inner
		}
		return "*" + inner
	case types.Tuple:
		if len(t.Args) == 0 {
			return ""
		}
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			g := m.GoType(a, generics)
			if g == "" {
				g = "struct{}"
			}
			parts[i] = fmt.Sprintf("F%d %s", i, g)
		}
		return "struct{ " + strings.Join(parts, "; ") + " }"
	case types.Func:
		params := make([]string, len(t.Args))
		for i, a := range t.Args {
			p := m.GoType(a, generics)
			if i < len(t.Inout) && t.Inout[i] {
				p = "*" + p
			}
			params[i] = p
		}
		res := m.GoType(t.Result, generics)
		s := "func(" + strings.Join(params, ", ") + ")"
		if res != "" {
			s += " " + res
		}
		return s
	case types.Meta:
		if len(t.Args) == 1 && (t.Args[0].Kind == types.Existential || t.Args[0].Kind == types.Named && m.isLocalProtocol(t.Args[0].Name)) {
			if g := m.GoType(t.Args[0], generics); !strings.HasPrefix(g, "any") {
				return g
			}
		}
		if len(t.Args) == 1 && t.Args[0].Kind == types.Generic {
			if g, ok := generics[t.Args[0].Name]; ok && g != "any" {
				return "swiftrt.TypeT[" + g + "]"
			}
		}
		return "swiftrt.Type"
	case types.Existential:
		return m.namedGo(t.Name, nil, generics)
	case types.Generic:
		if g, ok := generics[t.Name]; ok {
			return g
		}
		return "any"
	case types.Named:
		return m.namedGo(t.Name, t.Args, generics)
	}
	return "any"
}

func (m *Module) namedGo(name string, args []*types.Type, generics map[string]string) string {
	if g, ok := generics[name]; ok {
		return g
	}
	if s, ok := scalarTypes[name]; ok {
		return s
	}
	arg := func(i int) string {
		if i < len(args) {
			return m.GoType(args[i], generics)
		}
		return "any"
	}
	switch name {
	case "Swift.Array", "Swift.ArraySlice", "Swift.ContiguousArray":
		return "[]" + arg(0)
	case "Swift.ReversedCollection", "Swift.Slice":
		// The runtime reverses or slices into a slice of the base collection's elements.
		if base := arg(0); strings.HasPrefix(base, "[]") {
			return base
		} else if base == "string" || base == "swiftrt.Substring" {
			return "[]swiftrt.Character"
		}
	case "Swift.Optional":
		return m.GoType(&types.Type{Kind: types.Optional, Args: args}, generics)
	case "Swift.Dictionary":
		if len(args) == 2 && !m.goKeyable(args[0], map[string]bool{}) {
			return "swiftrt.HMap[" + arg(0) + ", " + arg(1) + "]"
		}
		return "map[" + arg(0) + "]" + arg(1)
	case "Swift.Set":
		if len(args) == 1 && !m.goKeyable(args[0], map[string]bool{}) {
			return "swiftrt.HSet[" + arg(0) + "]"
		}
		return "map[" + arg(0) + "]struct{}"
	case "Swift.Range":
		return "swiftrt.Range[" + arg(0) + "]"
	case "Swift.ClosedRange":
		return "swiftrt.ClosedRange[" + arg(0) + "]"
	case "Swift.PartialRangeFrom":
		return "swiftrt.PartialRangeFrom[" + arg(0) + "]"
	case "Swift.PartialRangeUpTo":
		return "swiftrt.PartialRangeUpTo[" + arg(0) + "]"
	case "Swift.PartialRangeThrough":
		return "swiftrt.PartialRangeThrough[" + arg(0) + "]"
	case "Swift.CustomStringConvertible":
		return "any"
	case "Swift.DefaultIndices":
		return "[]int"
	case "Swift.KeyPath", "Swift.PartialKeyPath":
		return "func(" + arg(0) + ") " + arg(1)
	case "Swift.WritableKeyPath", "Swift.ReferenceWritableKeyPath":
		return "swiftrt.WritableKeyPath[" + arg(0) + ", " + arg(1) + "]"
	case "Swift.IndexingIterator":
		return "swiftrt.IndexingIterator[" + strings.TrimPrefix(arg(0), "[]") + "]"
	case "Testing.Comment":
		return "string"
	case "Testing.SourceLocation":
		return "any"
	case "_Concurrency.TaskLocal":
		return "*swiftrt.TaskLocal[" + arg(0) + "]"
	}
	typeArgs := func(mod *Module) string {
		ps := mod.typeParamNames[name]
		if len(ps) == 0 {
			return ""
		}
		out := make([]string, len(ps))
		for i, p := range ps {
			if i < len(args) {
				out[i] = m.GoType(args[i], generics)
				if out[i] == "" {
					out[i] = "struct{}"
				}
			} else if g, ok := generics[p]; ok {
				out[i] = g
			} else {
				out[i] = p
			}
		}
		return "[" + strings.Join(out, ", ") + "]"
	}
	if m.isAssociatedType(name) {
		// Go interfaces have no associated types: a protocol's requirements see them as any.
		return "any"
	}
	if strings.HasPrefix(name, m.Name+".") {
		g := m.LocalTypeName(name) + typeArgs(m)
		if m.typeKinds[name] == "class_decl" {
			return "*" + g
		}
		return g
	}
	for _, d := range m.Deps {
		if strings.HasPrefix(name, d.Name+".") {
			g := d.LocalTypeName(name) + typeArgs(d)
			if d.Pkg != m.Pkg {
				m.importPkgs[d.Pkg] = d.ImportPath
				g = d.Pkg + "." + g
			}
			if d.typeKinds[name] == "class_decl" {
				return "*" + g
			}
			return g
		}
	}
	if (name == "Swift.UnsafePointer" || name == "Swift.UnsafeMutablePointer") && len(args) == 1 && args[0].Kind == types.Named && args[0].Name == "Swift.Int8" {
		// C strings are the runtime's pointers to NUL-terminated bytes.
		return "*any"
	}
	if g, ok := foreignTypes[name]; ok {
		if strings.Contains(g, "%s") {
			a0, a1 := arg(0), arg(1)
			if a0 == "" {
				a0 = "struct{}"
			}
			if a1 == "" {
				a1 = "struct{}"
			}
			return strings.ReplaceAll(strings.ReplaceAll(g, "%s", a0), "%t", a1)
		}
		return g
	}
	if strings.HasPrefix(name, "Swift.") {
		return "any /* " + name + " */"
	}
	if rest, ok := strings.CutPrefix(name, "__C."); ok && rest != "" && rest[0] >= 'a' && rest[0] <= 'z' && !strings.Contains(rest, ".") {
		// C structs (timeval, timespec) are exported Go structs in the runtime.
		return "swiftrt.C" + capitalize(rest)
	}
	for _, pfx := range []string{"Foundation.", "__C.NS", "__C.", "Synchronization.", "Dispatch.", "_Concurrency.", "CryptoKit.", "Crypto.", "Logging.", "YYJSON.", "WSCore.", "WSClient.", "NIOSSL.", "NIOCore.", "NIOPosix."} {
		if after, ok := strings.CutPrefix(name, pfx); ok {
			return "swiftrt." + strings.ReplaceAll(after, ".", "")
		}
	}
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 && m.mappedKind(name) == types.KindClass {
		return "*" + strings.ToLower(parts[0]) + "." + strings.ReplaceAll(parts[1], ".", "") + m.mappedTypeArgs(&types.Type{Kind: types.Named, Name: name, Args: args}, generics)
	}
	if len(parts) == 2 {
		return strings.ToLower(parts[0]) + "." + strings.ReplaceAll(parts[1], ".", "") + m.mappedTypeArgs(&types.Type{Kind: types.Named, Name: name, Args: args}, generics)
	}
	return "any"
}

var genericParam = regexp.MustCompile(`^(?:[A-Z][0-9]*|τ_\d+_\d+)$`)

// isAssociatedType reports whether name is an associated type reached through a generic parameter,
// as the demangler prints it: "A.Module.Protocol.Assoc".
func (m *Module) isAssociatedType(name string) bool {
	first, rest, ok := strings.Cut(name, ".")
	if !ok || !genericParam.MatchString(first) {
		return false
	}
	i := strings.LastIndex(rest, ".")
	return i > 0 && m.isProtocol(rest[:i])
}

// isProtocol reports whether the qualified Swift name is a protocol of this module or a dependency.
func (m *Module) isProtocol(q string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.typeKinds[q] == "protocol" {
			return true
		}
	}
	return false
}

func (m *Module) isReferenceGo(t *types.Type, goType string) bool {
	if strings.HasPrefix(goType, "*") || strings.HasPrefix(goType, "func(") || goType == "error" || goType == "any" {
		return true
	}
	if t.Kind == types.Existential {
		return true
	}
	if t.Kind == types.Named && (m.isLocalProtocol(t.Name) || m.mappedKind(t.Name) == types.KindProtocol) {
		return true
	}
	return false
}

// Recv names the runtime family for a Swift type, used to build swiftrt function names.
func (m *Module) Recv(t *types.Type) string {
	if t == nil {
		return "Any"
	}
	switch t.Kind {
	case types.Array:
		return "Slice"
	case types.Dict:
		if len(t.Args) == 2 && !m.goKeyable(t.Args[0], map[string]bool{}) {
			return "HMap"
		}
		return "Map"
	case types.Optional:
		// A type parameter's Optional is *T, so it maps like a value's, not a reference's.
		if len(t.Args) == 1 && !isGenericParamType(t.Args[0]) && m.isReferenceGo(t.Args[0], m.GoType(t.Args[0], nil)) {
			return "Ref"
		}
		return "Opt"
	case types.Tuple:
		return "Tuple"
	case types.Func:
		return "Func"
	case types.Generic:
		return "Any"
	}
	switch t.Name {
	case "Swift.String", "Swift.StaticString":
		return "String"
	case "Swift.Substring":
		return "Substring"
	case "Swift.Array", "Swift.ArraySlice", "Swift.ContiguousArray", "Swift.String.UnicodeScalarView", "Swift.String.UTF8View", "Swift.Substring.UnicodeScalarView":
		return "Slice"
	case "Swift.Dictionary":
		if len(t.Args) == 2 && !m.goKeyable(t.Args[0], map[string]bool{}) {
			return "HMap"
		}
		return "Map"
	case "Swift.Set":
		if len(t.Args) == 1 && !m.goKeyable(t.Args[0], map[string]bool{}) {
			return "HSet"
		}
		return "Set"
	case "Swift.Character":
		return "Character"
	case "Swift.Unicode.Scalar", "Unicode.Scalar":
		return "Scalar"
	case "Swift.String.Index", "Swift.Int", "Swift.Int8", "Swift.Int16", "Swift.Int32", "Swift.Int64", "Swift.UInt", "Swift.UInt8", "Swift.UInt16", "Swift.UInt32", "Swift.UInt64":
		return "Int"
	case "Swift.Double", "Swift.Float":
		return "Float"
	case "Swift.Bool":
		return "Bool"
	case "Swift.Range":
		return "Range"
	case "Swift.ClosedRange":
		return "ClosedRange"
	case "Swift.PartialRangeFrom":
		return "PartialRangeFrom"
	case "Swift.PartialRangeUpTo":
		return "PartialRangeUpTo"
	case "Swift.PartialRangeThrough":
		return "PartialRangeThrough"
	case "Swift.Optional":
		return "Opt"
	case "Swift.DefaultIndices", "DequeModule.Deque":
		return "Slice"
	case "Swift.EnumeratedSequence":
		return "Enumerated"
	case "Swift.AsyncStream.Iterator":
		return "AsyncStreamIterator"
	}
	if m.IsOptionSet(t.Name) && !foreignOptionSets[t.Name] {
		return "OptionSet"
	}
	if rest, ok := strings.CutPrefix(t.Name, "__C."); ok && rest != "" && rest[0] >= 'a' && rest[0] <= 'z' && !strings.Contains(rest, ".") {
		return "C" + capitalize(rest)
	}
	if strings.HasSuffix(t.Name, ".AllCases") {
		// CaseIterable.AllCases is an array of the cases here.
		return "Slice"
	}
	parts := strings.Split(t.Name, ".")
	last := strings.Map(func(r rune) rune {
		if r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return -1
	}, parts[len(parts)-1])
	// A leading underscore (`_NSRange`) would make the runtime name unexported.
	last = strings.TrimLeft(last, "_")
	if last == "" {
		return "Any"
	}
	return last
}

func isIntegerName(n string) bool {
	switch n {
	case "Swift.Int", "Swift.Int8", "Swift.Int16", "Swift.Int32", "Swift.Int64", "Swift.UInt", "Swift.UInt8", "Swift.UInt16", "Swift.UInt32", "Swift.UInt64":
		return true
	}
	return false
}

// isGoComparable reports whether Go's == works on the Go form of t.
func (m *Module) isGoComparable(t *types.Type) bool {
	switch t.Kind {
	case types.Array, types.Dict, types.Func, types.Optional:
		return false
	case types.Named:
		switch t.Name {
		case "Swift.Array", "Swift.Dictionary", "Swift.Set", "Swift.Optional", "Swift.String.UnicodeScalarView", "Swift.Substring":
			return false
		}
		if strings.HasPrefix(t.Name, m.Name+".") && m.typeKinds[t.Name] == "struct_decl" {
			return false
		}
		if gt := m.GoType(t, nil); strings.HasPrefix(gt, "[]") || strings.HasPrefix(gt, "map[") {
			return false
		}
	}
	return true
}

// foreignTypes maps Foundation and other Apple-module types to their Go runtime equivalents.
var foreignTypes = map[string]string{
	"Swift.AnyObject":                         "any",
	"Swift.Result":                            "swiftrt.Result[%s]",
	"__C.NSNumber":                            "swiftrt.NSNumber",
	"Swift.UnsafeMutablePointer":              "*%s",
	"Swift.UnsafePointer":                     "*%s",
	"_StringProcessing.Regex":                 "swiftrt.SwiftRegex[%s]",
	"_StringProcessing.Regex.Match":           "swiftrt.RegexMatch[%s]",
	"OrderedCollections.OrderedDictionary":    "swiftrt.OrderedDictionary[%s, %t]",
	"OrderedCollections.OrderedSet":           "swiftrt.OrderedSet[%s]",
	"DequeModule.Deque":                       "[]%s",
	"HeapModule.Heap":                         "swiftrt.Heap[%s]",
	"Swift.KeyedDecodingContainer":            "swiftrt.KeyedDecodingContainer",
	"Swift.KeyedEncodingContainer":            "swiftrt.KeyedEncodingContainer",
	"Swift.SingleValueDecodingContainer":      "swiftrt.SingleValueDecodingContainer",
	"Swift.SingleValueEncodingContainer":      "swiftrt.SingleValueEncodingContainer",
	"Swift.UnkeyedEncodingContainer":          "swiftrt.UnkeyedEncodingContainer",
	"Swift.Encoder":                           "*swiftrt.Encoder",
	"Swift.Decoder":                           "*swiftrt.Decoder",
	"Swift.CodingKey":                         "any",
	"Swift.DecodingError":                     "swiftrt.CodingError",
	"Swift.EncodingError":                     "swiftrt.CodingError",
	"Foundation.Data":                         "[]byte",
	"Foundation.TimeInterval":                 "float64",
	"Foundation.NSRange":                      "swiftrt.Range[int]",
	"__C.NSComparisonResult":                  "int",
	"__C.NSDateFormatter":                     "*swiftrt.DateFormatter",
	"__C.NSISO8601DateFormatter":              "*swiftrt.ISO8601DateFormatter",
	"__C.NSLock":                              "*swiftrt.Lock",
	"__C.NSRegularExpression":                 "*swiftrt.Regex",
	"Foundation.JSONEncoder":                  "*swiftrt.JSONEncoder",
	"Foundation.JSONDecoder":                  "*swiftrt.JSONDecoder",
	"Synchronization.Mutex":                   "*swiftrt.Mutex[%s]",
	"Synchronization.Atomic":                  "*swiftrt.Atomic[%s]",
	"Foundation.Calendar.Component":           "swiftrt.CalendarComponent",
	"Foundation.Calendar.Identifier":          "swiftrt.CalendarIdentifier",
	"Foundation.JSONEncoder.OutputFormatting": "swiftrt.JSONEncoderOutputFormatting",
	"Swift.AsyncStream":                       "swiftrt.AsyncStream[%s]",
	"Swift.AsyncStream.Continuation":          "swiftrt.AsyncStreamContinuation[%s]",
	"Swift.AsyncStream.Iterator":              "swiftrt.AsyncStreamIterator[%s]",
}

// goKeyable reports whether a Swift Hashable type's Go form can be a Go map key with the same
// equality: arrays, sets and dictionaries cannot, nor can structs holding them.
func (m *Module) goKeyable(t *types.Type, seen map[string]bool) bool {
	switch t.Kind {
	case types.Array, types.Dict:
		return false
	case types.Tuple:
		for _, a := range t.Args {
			if !m.goKeyable(a, seen) {
				return false
			}
		}
		return true
	case types.Named:
		switch t.Name {
		case "Swift.Array", "Swift.Set", "Swift.Dictionary", "Swift.ArraySlice":
			return false
		}
		if m.typeKinds[t.Name] != "struct_decl" || seen[t.Name] {
			return true
		}
		if k, ok := m.keyable[t.Name]; ok {
			return k
		}
		seen[t.Name] = true
		k := true
		for _, d := range m.decls {
			if d.Kind == "var_decl" && d.Owner == t.Name && !d.Static && !d.Computed && d.Node != nil {
				if !m.goKeyable(m.typeOf(d.Node.Str("interface_type")), seen) {
					k = false
					break
				}
			}
		}
		if m.keyable == nil {
			m.keyable = map[string]bool{}
		}
		m.keyable[t.Name] = k
		return k
	}
	return true
}

func isGenericParamType(t *types.Type) bool {
	return t.Kind == types.Generic || t.Kind == types.Named && genericParamName.MatchString(t.Name)
}
