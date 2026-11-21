// Package emit turns a Swift module's typed AST into Go source.
package emit

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// DeclInfo describes one declaration defined in the module being transpiled.
type DeclInfo struct {
	// Settable marks a computed property with a setter, emitted as SetX.
	Settable bool
	// Observed marks a stored property with willSet/didSet, assigned through SetX.
	Observed bool
	// Overrides is the USR of the superclass member this member overrides.
	Overrides string
	// GenericFree marks a method whose generic parameter needs a protocol's static requirements; Go methods
	// cannot declare type parameters, so it is emitted as a package-level function taking self.
	GenericFree bool
	// Lazy marks a static or global stored property with a non-literal initializer, emitted as a swiftrt.Lazy cell.
	Lazy bool
	// OnPayloadEnum marks a member of an enum emitted as a sealed interface, which cannot have methods.
	OnPayloadEnum bool
	Node          ast.Node
	Kind          string // func_decl, var_decl, constructor_decl, enum_element_decl, struct_decl, ...
	Owner         string // qualified Swift name of the owning type ("" for globals)
	OwnerKind     string // struct_decl, class_decl, enum_decl, protocol, extension of foreign type
	ProtoExt      bool   // declared in an extension of a protocol
	Static        bool
	GoName        string
	Computed      bool
	Mutating      bool
}

// Module is one Swift module being transpiled.
type Module struct {
	Name           string
	Pkg            string
	RTPath         string // import path of swiftrt
	Files          []ast.File
	Test           bool // transpiling a test target into the package under test
	Target         string
	dm             *types.Demangler
	decls          map[string]*DeclInfo
	typeKinds      map[string]string // qualified Swift type name → decl kind
	protoReqs      map[string][]ast.Node
	conformers     map[string][]string // protocol qualified name → conforming local types
	varInit        map[string]ast.Node // stored var usr → initializer expr
	Unsupported    map[string]int
	UnsupportedAt  map[string][]string
	Deps           []*Module
	ImportPath     string
	importPkgs     map[string]string
	typeCache      map[string]*types.Type
	payloadEnums   map[string]bool
	typeParamNames map[string][]string // generic struct/class → its Go type parameter names
	lazyInits      map[string]bool     // owner|name of stored statics and globals with non-literal initializers
	lineStarts     map[string][]int
	mutatingTypes  map[string]bool
	overridden     map[string]bool // USRs of class members some subclass overrides
	superOf        map[string]string
}

// IsOptionSet reports whether the qualified Swift type conforms to OptionSet, in this module or a dependency.
func (m *Module) IsOptionSet(q string) bool {
	if foreignOptionSets[q] {
		return true
	}
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.typeKinds[q+"#OptionSet"] != "" {
			return true
		}
	}
	return false
}

// Superclass returns the Go name of a local class's superclass, or "".
func (m *Module) Superclass(goName string) string {
	m.classTree()
	return m.superOf[goName]
}

// HasSubclass reports whether some local class inherits from goName.
func (m *Module) HasSubclass(goName string) bool {
	m.classTree()
	for _, sup := range m.superOf {
		if sup == goName {
			return true
		}
	}
	return false
}

// IsAncestor reports whether anc is goName's superclass, directly or transitively.
func (m *Module) IsAncestor(anc, goName string) bool {
	for s := m.Superclass(goName); s != ""; s = m.Superclass(s) {
		if s == anc {
			return true
		}
	}
	return false
}

func (m *Module) classTree() {
	if m.superOf != nil {
		return
	}
	m.superOf = map[string]string{}
	for _, d := range m.decls {
		if d.Kind != "class_decl" || d.Node == nil {
			continue
		}
		st := d.Node.Child("inherits").Str("superclass_type")
		if st == "" {
			continue
		}
		g := strings.TrimPrefix(m.GoType(m.typeOf(st), nil), "*")
		if g != "" && !strings.Contains(g, ".") && g != "any" {
			m.superOf[d.GoName] = g
		}
	}
}

// IsPayloadEnum reports whether the enum named owner (a qualified Swift name) is emitted as a sealed interface.
func (m *Module) IsPayloadEnum(owner string) bool {
	if m.payloadEnums == nil {
		m.payloadEnums = map[string]bool{}
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, d := range mod.decls {
				if d.Kind != "enum_element_decl" || d.Node == nil {
					continue
				}
				if ps := d.Node.Child("params"); ps != nil && len(ps.List("params")) > 0 {
					m.payloadEnums[d.Owner] = true
				}
			}
		}
	}
	return m.payloadEnums[owner]
}

// New prepares a module: demangles every symbol and indexes declarations.
func New(name string, files []ast.File, rtPath string) (*Module, error) {
	m := &Module{
		Name: name, Pkg: strings.ToLower(name), RTPath: rtPath, Files: files,
		decls: map[string]*DeclInfo{}, typeKinds: map[string]string{},
		protoReqs: map[string][]ast.Node{}, conformers: map[string][]string{},
		varInit: map[string]ast.Node{}, typeParamNames: map[string][]string{}, lazyInits: map[string]bool{}, overridden: map[string]bool{}, importPkgs: map[string]string{}, typeCache: map[string]*types.Type{}, Unsupported: map[string]int{}, UnsupportedAt: map[string][]string{},
	}
	var syms []string
	for _, f := range files {
		for _, it := range f.Items {
			ast.Walk(it, func(n ast.Node) {
				for _, k := range []string{"type", "usr", "decl_usr", "interface_type", "result", "extended_type", "type_usr", "first_type", "second_type", "protocol"} {
					if s := n.Str(k); strings.HasPrefix(s, "$s") || strings.HasPrefix(s, "s:") {
						syms = append(syms, s)
					}
				}
			})
		}
	}
	dm, err := types.NewDemangler(syms)
	if err != nil {
		return nil, err
	}
	m.dm = dm
	for _, f := range files {
		m.index(f.Items, "", "")
	}
	m.markLazy()
	m.markGenericFree()
	m.assignNames()
	return m, nil
}

// SetTarget marks the module as a test target whose code joins package pkg.
func (m *Module) SetTarget(pkg string) { m.Test = true; m.Pkg = pkg }

func (m *Module) demangle(s string) string { return m.dm.Get(s) }

func (m *Module) typeOf(mangled string) *types.Type {
	if mangled == "" {
		return &types.Type{Kind: types.Unknown}
	}
	if t, ok := m.typeCache[mangled]; ok {
		return t
	}
	dem := m.demangle(mangled)
	for _, marker := range []string{" & Swift.Sendable", "Swift.Sendable & ", "@isolated(any) ", "@MainActor ", "nonisolated(nonsending) ", "@concurrent "} {
		dem = strings.ReplaceAll(dem, marker, "")
	}
	dem = extensionInPrefix.ReplaceAllString(dem, "")
	t := types.Parse(dem)
	if t.Kind == types.Unknown {
		m.Unsup("type-parse", m.demangle(mangled), "")
	}
	m.typeCache[mangled] = t
	return t
}

func (m *Module) index(items []ast.Node, owner, ownerKind string) {
	for _, n := range items {
		if isMacroName(n.Name()) {
			continue
		}
		switch n.Kind() {
		case "struct_decl", "class_decl", "enum_decl", "protocol":
			q := m.qualify(owner, n.Name())
			m.typeKinds[q] = n.Kind()
			if n.Kind() == "struct_decl" || n.Kind() == "class_decl" || n.Kind() == "enum_decl" && hasPayloadCase(n) {
				for _, gp := range n.Child("generic_signature").Strings("generic_params") {
					if name := m.demangle(gp); name != "" {
						m.typeParamNames[q] = append(m.typeParamNames[q], name)
					}
				}
			}
			for _, c := range n.Child("inherits").List("conformances") {
				if c.Str("protocol") == "s:s9OptionSetP" {
					m.typeKinds[q+"#OptionSet"] = "yes"
				}
			}
			m.decls[declKey(n)] = &DeclInfo{Node: n, Kind: n.Kind(), Owner: owner, OwnerKind: ownerKind}
			for _, c := range n.Child("inherits").List("conformances") {
				if p := c.Str("protocol"); p != "" {
					pn := m.demangle(p)
					m.conformers[pn] = append(m.conformers[pn], q)
				}
			}
			for _, p := range n.Strings("inherits") {
				_ = p
			}
			if n.Kind() == "protocol" {
				m.protoReqs[q] = n.List("members")
			}
			m.index(n.List("members"), q, n.Kind())
		case "extension_decl":
			t := m.typeOf(n.Str("extended_type"))
			q := t.Name
			if t.Kind == types.Existential {
				q = t.Name
			}
			kind := m.typeKinds[q]
			if kind == "" {
				kind = "foreign"
				if strings.HasPrefix(q, m.Name+".") {
					kind = "local?"
				}
			}
			if t.Kind == types.Existential || kind == "protocol" {
				kind = "protocol_ext"
			}
			for _, c := range n.Child("inherits").List("conformances") {
				if p := c.Str("protocol"); p != "" {
					pn := m.demangle(p)
					m.conformers[pn] = append(m.conformers[pn], q)
				}
			}
			m.index(n.List("members"), q, kind)
		case "func_decl", "constructor_decl", "var_decl", "enum_element_decl", "subscript_decl":
			if n.Kind() == "func_decl" || n.Kind() == "constructor_decl" {
				m.indexLocalTypes(n.Child("body"))
			}
			info := &DeclInfo{Node: n, Kind: n.Kind(), Owner: owner, OwnerKind: ownerKind, Static: n.Bool("static")}
			if ownerKind == "protocol_ext" {
				info.ProtoExt = true
			}
			if n.Kind() == "var_decl" {
				impl := n.Str("readImpl")
				info.Computed = impl != "" && impl != "stored"
				info.Settable = info.Computed && n.Str("writeImpl") == "setter"
				info.Observed = n.Str("writeImpl") == "stored_with_observers"
			}
			for _, a := range n.List("attrs") {
				if a.Kind() == "mutating_attr" {
					info.Mutating = true
				}
			}
			if n.Child("implicit_self_decl").Bool("inout") && n.Kind() == "func_decl" {
				info.Mutating = true
			}
			if ov := n.Strings("override"); len(ov) > 0 {
				info.Overrides = ov[0]
				m.overridden[ov[0]] = true
			}
			if n.Str("usr") != "" {
				m.decls[n.Str("usr")] = info
			}
		case "enum_case_decl":
			m.index(n.List("elements"), owner, ownerKind)
		case "pattern_binding_decl":
			for _, e := range n.List("pattern_entries") {
				init := e.Child("processed_init")
				if init == nil {
					init = e.Child("original_init")
				}
				if init == nil {
					continue
				}
				if name := patternName(e.Child("pattern")); name != "" {
					m.pendingInit(owner, name, init)
					if !isLiteralInit(init) {
						m.lazyInits[owner+"|"+name] = true
					}
				}
			}
		}
	}
}

// indexLocalTypes indexes types declared inside a function body; they are hoisted to package level.
func (m *Module) indexLocalTypes(body ast.Node) {
	if body == nil {
		return
	}
	var visit func(n ast.Node)
	visit = func(n ast.Node) {
		for _, v := range n {
			var kids []ast.Node
			switch c := v.(type) {
			case map[string]any:
				kids = []ast.Node{ast.Node(c)}
			case []any:
				for _, cc := range c {
					if mm, ok := cc.(map[string]any); ok {
						kids = append(kids, ast.Node(mm))
					}
				}
			}
			for _, k := range kids {
				switch k.Kind() {
				case "struct_decl", "class_decl", "enum_decl":
					m.index([]ast.Node{k}, "", "")
				default:
					visit(k)
				}
			}
		}
	}
	visit(body)
}

// pendingInit records an initializer by owner+name; resolved to a usr after indexing.
func (m *Module) pendingInit(owner, name string, init ast.Node) {
	m.varInit[owner+"|"+name] = init
}

func (m *Module) initFor(info *DeclInfo) ast.Node {
	return m.varInit[info.Owner+"|"+info.Node.Name()]
}

// isMacroName reports a mangled name the compiler gives macro-expanded declarations.
func isMacroName(n string) bool {
	return len(n) > 2 && strings.HasPrefix(n, "$s") && n[2] >= '0' && n[2] <= '9'
}

func patternName(p ast.Node) string {
	for p != nil {
		switch p.Kind() {
		case "pattern_named":
			return p.Name()
		case "pattern_typed", "pattern_binding", "pattern":
			p = p.Child("sub_pattern")
			if p == nil {
				return ""
			}
		default:
			return ""
		}
	}
	return ""
}

func (m *Module) qualify(owner, name string) string {
	if owner == "" {
		return m.Name + "." + name
	}
	return owner + "." + name
}

// LocalTypeName maps a qualified Swift type in this module to its Go identifier.
func (m *Module) LocalTypeName(q string) string {
	rest := strings.TrimPrefix(q, m.Name+".")
	return strings.ReplaceAll(rest, ".", "")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	s = strings.TrimPrefix(s, "`")
	s = strings.TrimSuffix(s, "`")
	return strings.ToUpper(s[:1]) + s[1:]
}

func (m *Module) ownerGoName(info *DeclInfo) string {
	if info.Owner == "" {
		return ""
	}
	if strings.HasPrefix(info.Owner, m.Name+".") {
		return m.LocalTypeName(info.Owner)
	}
	parts := strings.Split(info.Owner, ".")
	return strings.Join(parts[1:], "")
}

func paramLabels(n ast.Node) []string {
	var out []string
	for _, p := range n.Child("params").List("params") {
		out = append(out, p.Str("apiName"))
	}
	return out
}

func labelSuffix(labels []string) string {
	var b strings.Builder
	for _, l := range labels {
		if l != "" {
			b.WriteString(capitalize(l))
		}
	}
	return b.String()
}

func (m *Module) assignNames() {
	type key struct {
		owner, base string
		static      bool
	}
	groups := map[key][]*DeclInfo{}
	for _, info := range m.decls {
		switch info.Kind {
		case "func_decl", "constructor_decl", "var_decl":
			if info.ProtoExt {
				continue
			}
			k := key{info.Owner, info.Node.Name(), info.Static}
			groups[k] = append(groups[k], info)
		}
	}
	payloadOwners := map[string]bool{}
	for _, d := range m.decls {
		if d.Kind == "enum_element_decl" && d.Node != nil {
			if ps := d.Node.Child("params"); ps != nil && len(ps.List("params")) > 0 {
				payloadOwners[d.Owner] = true
			}
		}
	}
	for _, info := range m.decls {
		if payloadOwners[info.Owner] && (info.Kind == "func_decl" || info.Kind == "var_decl") {
			info.OnPayloadEnum = true
		}
	}
	for _, info := range m.decls {
		name := info.Node.Name()
		owner := m.ownerGoName(info)
		switch info.Kind {
		case "struct_decl", "class_decl", "enum_decl", "protocol":
			info.GoName = m.LocalTypeName(m.qualify(info.Owner, name))
		case "constructor_decl":
			base := "New" + owner
			if len(groups[key{info.Owner, name, info.Static}]) > 1 {
				base += labelSuffix(paramLabels(info.Node))
			}
			info.GoName = base
		case "func_decl":
			base := capitalize(goSafe(name))
			if strings.HasPrefix(name, "__derived") || isOperatorName(name) {
				base = "Op" + operatorWord(name)
			}
			if len(groups[key{info.Owner, name, info.Static}]) > 1 {
				base += labelSuffix(paramLabels(info.Node))
			}
			info.GoName = m.memberName(info, owner, base)
		case "var_decl":
			if strings.HasPrefix(name, "$") {
				info.GoName = m.memberName(info, owner, capitalize(name[1:])+"Projected")
				break
			}
			info.GoName = m.memberName(info, owner, capitalize(goSafe(name)))
		case "enum_element_decl":
			info.GoName = owner + capitalize(name)
		case "subscript_decl":
			info.GoName = "Subscript" + labelSuffix(paramLabels(info.Node))
		}
	}
	m.dedupePackageNames()
}

// dedupePackageNames renames package-level Go names that collide, keeping type names unchanged.
func (m *Module) dedupePackageNames() {
	byName := map[string][]*DeclInfo{}
	var usrs []string
	for usr := range m.decls {
		usrs = append(usrs, usr)
	}
	sort.Strings(usrs)
	for _, usr := range usrs {
		info := m.decls[usr]
		if info.GoName == "" || !m.packageLevel(info) {
			continue
		}
		byName[info.GoName] = append(byName[info.GoName], info)
	}
	methods := map[string][]*DeclInfo{}
	for _, usr := range usrs {
		info := m.decls[usr]
		if info.GoName == "" || m.packageLevel(info) || (info.Kind != "func_decl" && info.Kind != "subscript_decl") || info.Owner == "" {
			continue
		}
		k := info.Owner + "." + info.GoName
		methods[k] = append(methods[k], info)
	}
	for _, infos := range methods {
		for i, info := range infos {
			if i == 0 {
				continue
			}
			info.GoName += m.paramTypeSuffix(info)
		}
	}
	// A method such as `setFightingId(_:)` collides with the generated setter of `fightingId`.
	setters := map[string]bool{}
	for _, usr := range usrs {
		info := m.decls[usr]
		if info.Kind == "var_decl" && info.Owner != "" && !info.Static && (info.Settable || info.Observed) {
			setters[info.Owner+".Set"+info.GoName] = true
		}
	}
	for k, infos := range methods {
		if setters[k] {
			for _, info := range infos {
				info.GoName += "Fn"
			}
		}
	}
	suffix := map[string]string{"enum_element_decl": "Case", "var_decl": "Value", "func_decl": "Fn", "constructor_decl": "Init"}
	for _, infos := range byName {
		if len(infos) < 2 {
			continue
		}
		n := 0
		for _, info := range infos {
			switch info.Kind {
			case "struct_decl", "class_decl", "enum_decl", "protocol":
				continue
			}
			if n == 0 {
				info.GoName += suffix[info.Kind]
			} else {
				info.GoName += suffix[info.Kind] + strconv.Itoa(n+1)
			}
			n++
		}
	}
}

func (m *Module) packageLevel(info *DeclInfo) bool {
	switch info.Kind {
	case "struct_decl", "class_decl", "enum_decl", "protocol", "constructor_decl", "enum_element_decl":
		return true
	case "func_decl", "var_decl":
		return info.Owner == "" || info.Static || info.FreeSelf()
	}
	return false
}

// memberName decides between a Go method/field name and a package-level name.
func (m *Module) memberName(info *DeclInfo, owner, base string) string {
	if info.Owner == "" {
		return base
	}
	if info.Static || info.FreeSelf() {
		return owner + base
	}
	return base
}

func isOperatorName(n string) bool {
	if n == "" {
		return false
	}
	c := n[0]
	return !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z')
}

func operatorWord(n string) string {
	words := map[string]string{"==": "Eq", "<": "Lt", "<=": "Le", ">": "Gt", ">=": "Ge", "+": "Add", "-": "Sub", "*": "Mul", "/": "Div", "!=": "Ne"}
	if w, ok := words[n]; ok {
		return w
	}
	var b strings.Builder
	for _, c := range n {
		fmt.Fprintf(&b, "%X", c)
	}
	return b.String()
}

var goKeywords = map[string]bool{"break": true, "case": true, "chan": true, "const": true, "continue": true, "default": true, "defer": true, "else": true, "fallthrough": true, "for": true, "func": true, "go": true, "goto": true, "if": true, "import": true, "interface": true, "map": true, "package": true, "range": true, "return": true, "select": true, "struct": true, "switch": true, "type": true, "var": true, "string": true, "int": true, "len": true, "cap": true, "new": true, "make": true, "copy": true, "append": true, "error": true, "rune": true, "byte": true, "bool": true, "nil": true, "true": true, "false": true, "max": true, "min": true, "print": true, "println": true, "close": true, "delete": true, "panic": true, "recover": true, "clear": true, "iota": true}

func goSafe(n string) string {
	n = strings.Trim(n, "`")
	if len(n) > 1 && n[0] == '$' && n[1] >= '0' && n[1] <= '9' {
		return "p" + n[1:]
	}
	n = strings.ReplaceAll(n, "$", "_")
	if goKeywords[n] {
		return n + "_"
	}
	return n
}

// Unsup records an unsupported construct and returns a Go expression that compiles.
func (m *Module) Unsup(kind, where, goType string) string {
	m.Unsupported[kind]++
	if len(m.UnsupportedAt[kind]) < 5 {
		m.UnsupportedAt[kind] = append(m.UnsupportedAt[kind], where)
	}
	if goType == "" || goType == "any" {
		return fmt.Sprintf("swiftrt.Unsupported[any](%q)", kind)
	}
	return fmt.Sprintf("swiftrt.Unsupported[%s](%q)", goType, kind)
}

// WriteGo writes one Go file per Swift file into dir and returns their paths.
func (m *Module) WriteGo(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, f := range m.Files {
		fe := &fileEmitter{m: m, file: f}
		src := fe.emitFile()
		base := strings.TrimSuffix(filepath.Base(f.Filename), ".swift")
		suffix := ".go"
		if m.Test {
			suffix = "_test.go"
		}
		out := filepath.Join(dir, toSnake(base)+suffix)
		src = sanitizeRuntimeNames(src)
		src = strings.ReplaceAll(src, "for _, _ := range ", "for range ")
		formatted, err := format.Source([]byte(src))
		if err != nil {
			formatted = []byte(src)
			fmt.Fprintf(os.Stderr, "gofmt %s: %v\n", out, err)
		}
		if err := os.WriteFile(out, formatted, 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, out)
	}
	return paths, nil
}

func toSnake(s string) string {
	var b strings.Builder
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(c + 32)
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Report summarises unsupported constructs, most frequent first.
func (m *Module) Report() string {
	type kv struct {
		k string
		v int
	}
	var all []kv
	total := 0
	for k, v := range m.Unsupported {
		all = append(all, kv{k, v})
		total += v
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v || all[i].v == all[j].v && all[i].k < all[j].k })
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d unsupported\n", m.Name, total)
	for _, e := range all {
		fmt.Fprintf(&b, "  %5d %s   e.g. %s\n", e.v, e.k, strings.Join(m.UnsupportedAt[e.k], "; "))
	}
	return b.String()
}

var foreignRuntimeName = regexp.MustCompile(`swiftrt\.[A-Za-z0-9_]*[:@$][A-Za-z0-9_@$][A-Za-z0-9_:@$]*`)

// sanitizeRuntimeNames turns clang/ObjC USR fragments that leaked into swiftrt identifiers into Go identifiers.
func sanitizeRuntimeNames(src string) string {
	return foreignRuntimeName.ReplaceAllStringFunc(src, func(s string) string {
		var b strings.Builder
		up := false
		for _, r := range s[len("swiftrt."):] {
			if r == ':' || r == '@' || r == '$' {
				up = true
				continue
			}
			if up {
				b.WriteString(strings.ToUpper(string(r)))
				up = false
				continue
			}
			b.WriteRune(r)
		}
		return "swiftrt." + b.String()
	})
}

// lineCol converts a byte offset in a source file to a 1-based line and column.
func (m *Module) lineCol(file string, off int) (int, int) {
	if m.lineStarts == nil {
		m.lineStarts = map[string][]int{}
	}
	starts, ok := m.lineStarts[file]
	if !ok {
		data, _ := os.ReadFile(file)
		starts = []int{0}
		for i, b := range data {
			if b == '\n' {
				starts = append(starts, i+1)
			}
		}
		m.lineStarts[file] = starts
	}
	if off < 0 {
		return 0, 0
	}
	i := sort.SearchInts(starts, off+1) - 1
	if i < 0 {
		i = 0
	}
	return i + 1, off - starts[i] + 1
}

// FreeSelf reports whether a member is emitted as a package-level function taking self as its first parameter.
func (info *DeclInfo) FreeSelf() bool {
	return info.OwnerKind == "foreign" || info.ProtoExt || info.OnPayloadEnum || info.GenericFree
}

// markGenericFree flags methods that must become generic package-level functions.
func (m *Module) markGenericFree() {
	for _, d := range m.decls {
		if d.Kind != "func_decl" || d.Static || d.Owner == "" || d.OwnerKind == "protocol" || d.Node.Child("implicit_self_decl") == nil {
			continue
		}
		for _, r := range d.Node.Child("generic_signature").List("generic_reqs") {
			if r.Str("kind") == "conforms_to" && m.HasStaticRequirements(m.demangle(r.Str("protocol"))) {
				d.GenericFree = true
			}
		}
		// A method with generic parameters of its own needs Go type parameters, which only functions can declare.
		if own := len(d.Node.Child("generic_signature").Strings("generic_params")) - len(m.typeParamNames[d.Owner]); own > 0 && m.typeKinds[d.Owner] != "protocol" && !m.overridden[d.Node.Str("usr")] && d.Overrides == "" {
			d.GenericFree = true
		}
	}
}

// HasMutatingMethods reports whether a local struct has pointer-receiver methods, so only a pointer satisfies its protocols.
func (m *Module) HasMutatingMethods(q string) bool {
	if m.mutatingTypes == nil {
		m.mutatingTypes = map[string]bool{}
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, d := range mod.decls {
				if d.Mutating && d.Kind == "func_decl" && !d.FreeSelf() && !d.Static {
					m.mutatingTypes[d.Owner] = true
				}
				if (d.Settable || d.Observed) && !d.FreeSelf() && !d.Static {
					m.mutatingTypes[d.Owner] = true
				}
			}
			for q, k := range mod.typeKinds {
				if k == "class_decl" || k == "enum_decl" {
					delete(m.mutatingTypes, q)
				}
			}
		}
	}
	return m.mutatingTypes[q]
}

// paramTypeSuffix names an overload by its parameter types, for overloads whose labels match.
func (m *Module) paramTypeSuffix(info *DeclInfo) string {
	var b strings.Builder
	for _, p := range info.Node.Child("params").List("params") {
		t := m.typeOf(p.Str("interface_type"))
		name := "Any"
		if t != nil {
			name = t.Name
			if i := strings.LastIndex(name, "."); i >= 0 {
				name = name[i+1:]
			}
			if name == "" {
				name = m.Recv(t)
			}
		}
		b.WriteString(capitalize(goSafe(name)))
	}
	if b.Len() == 0 {
		return "2"
	}
	return "By" + b.String()
}

// markLazy flags static and global stored properties whose initializers must run lazily, as Swift's do.
func (m *Module) markLazy() {
	for _, d := range m.decls {
		if d.Kind == "var_decl" && !d.Computed && (d.Static || d.Owner == "") && m.lazyInits[d.Owner+"|"+d.Node.Name()] {
			d.Lazy = true
		}
	}
}

// isLiteralInit reports an initializer Go can evaluate eagerly without creating initialization cycles.
func isLiteralInit(n ast.Node) bool {
	for n.Kind() == "paren_expr" {
		n = n.Child("sub_expr")
	}
	switch n.Kind() {
	case "integer_literal_expr", "float_literal_expr", "boolean_literal_expr", "nil_literal_expr":
		return true
	case "string_literal_expr":
		return true
	case "inject_into_optional":
		return isLiteralInit(n.Child("sub_expr"))
	}
	return false
}

// HasStaticRequirements reports whether a local protocol declares static members, which Go can only reach through a type parameter.
func (m *Module) HasStaticRequirements(proto string) bool {
	for _, r := range m.protoReqs[proto] {
		if (r.Kind() == "func_decl" || r.Kind() == "var_decl") && r.Bool("static") {
			return true
		}
	}
	return false
}

// isLocalProtocol reports a protocol declared in this module or a transpiled dependency.
func (m *Module) isLocalProtocol(q string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.typeKinds[q] == "protocol" {
			return true
		}
	}
	return false
}

// TypeParams returns a local generic type's Go type parameter names, looking through dependencies.
func (m *Module) TypeParams(q string) []string {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if ps := mod.typeParamNames[q]; len(ps) > 0 {
			return ps
		}
	}
	return nil
}

// typeParamDecl renders "[A any, B any]" for a generic type, or "".
func (m *Module) typeParamDecl(q string) string {
	ps := m.TypeParams(q)
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p + " any"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// typeParamUse renders "[A, B]" for a generic type, or "".
func (m *Module) typeParamUse(q string) string {
	ps := m.TypeParams(q)
	if len(ps) == 0 {
		return ""
	}
	return "[" + strings.Join(ps, ", ") + "]"
}

// mergeTypeParams joins two "[...]" type parameter lists.
func mergeTypeParams(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a[:len(a)-1] + ", " + b[1:]
}

// extensionInPrefix strips the demangler's "(extension in Foundation):" qualifier from type names.
var extensionInPrefix = regexp.MustCompile(`\(extension in [^)]*\):`)

// declKey indexes a declaration by its USR, or by kind, name and source position for the few the
// compiler dumps without one (types local to some static functions).
func declKey(n ast.Node) string {
	if usr := n.Str("usr"); usr != "" {
		return usr
	}
	start := ""
	if r := n.Child("range"); r != nil {
		start = fmt.Sprint(r["start"])
	}
	return "nousr:" + n.Kind() + ":" + n.Name() + "@" + start
}

func hasPayloadCase(n ast.Node) bool {
	payload := false
	ast.Walk(n, func(x ast.Node) {
		if x.Kind() == "enum_element_decl" {
			if ps := x.Child("params"); ps != nil && len(ps.List("params")) > 0 {
				payload = true
			}
		}
	})
	return payload
}

// foreignOptionSets are the imported OptionSet types the runtime models as `struct{ RawValue R }`.
var foreignOptionSets = map[string]bool{
	"Foundation.JSONEncoder.OutputFormatting":   true,
	"Foundation.ISO8601DateFormatter.Options":   true,
	"YYJSON.YYJSONSerialization.ReadingOptions": true,
	"YYJSON.YYJSONSerialization.WritingOptions": true,
	"YYJSON.YYJSONWriteOptions":                 true,
	"YYJSON.GameJSONEncoder.OutputFormatting":   true,
}
