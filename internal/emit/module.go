// Package emit turns a Swift module's typed AST into Go source.
package emit

import (
	"bytes"
	"cmp"
	"fmt"
	"go/format"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

// DeclInfo describes one declaration defined in the module being transpiled.
type DeclInfo struct {
	// QName is a type declaration's qualified name, as its type is spelled ("Mod.Outer.Inner").
	QName string
	// Settable marks a computed property with a setter, emitted as SetX.
	Settable bool
	// Observed marks a stored property with willSet/didSet, assigned through SetX.
	Observed bool
	// NotWitness marks a class member that shares a protocol requirement's name but does not satisfy
	// it, because a superclass conforms through the protocol's default.
	NotWitness bool
	// Overrides is the USR of the superclass member this member overrides.
	Overrides string
	// Weak marks a `weak` stored property, kept in a swiftrt.Weak field and read and written through
	// a getter and SetX, as a computed property is.
	Weak bool
	// OpenOverridable marks a member that a subclass in another module can override: an open method
	// or property, or a designated initializer of an open class visible outside its module.
	OpenOverridable bool
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
	file          string // the Swift file that declares it
}

// keyedOverload names the Go functions of a module's own keyed-container overloads for one generic type:
// `KeyedEncodingContainer.encode(_:forKey:)` taking a value of it and `KeyedDecodingContainer.decode(_:forKey:)`
// taking its metatype. Each takes the key type, then the type's argument, as type parameters.
type keyedOverload struct{ encode, decode string }

// keyedOverloads maps each generic type, by qualified Swift name, to the module's keyed-container overloads for it.
func (m *Module) keyedOverloads() map[string]keyedOverload {
	m.keyedOnce.Do(func() {
		m.keyed = map[string]keyedOverload{}
		for _, d := range m.decls {
			if d.Kind != "func_decl" || d.Static || d.GoName == "" {
				continue
			}
			ps := d.Node.Child("params").List("params")
			if len(ps) != 2 {
				continue
			}
			arg := m.typeOf(ps[0].Str("interface_type"))
			switch {
			case d.Owner == "Swift.KeyedEncodingContainer" && d.Node.Name() == "encode" && arg != nil && arg.Kind == types.Named && len(arg.Args) == 1:
				o := m.keyed[arg.Name]
				o.encode = d.GoName
				m.keyed[arg.Name] = o
			case d.Owner == "Swift.KeyedDecodingContainer" && d.Node.Name() == "decode" && arg != nil && arg.Kind == types.Meta && len(arg.Args) == 1 && arg.Args[0].Kind == types.Named && len(arg.Args[0].Args) == 1:
				o := m.keyed[arg.Args[0].Name]
				o.decode = d.GoName
				m.keyed[arg.Args[0].Name] = o
			}
		}
	})
	return m.keyed
}

// Module is one Swift module being transpiled.
type Module struct {
	keyedOnce sync.Once
	keyed     map[string]keyedOverload

	Name            string
	Pkg             string
	RTPath          string // import path of swiftrt
	Files           []ast.File
	Overlay         string   // directory of hand-written Go files that replace Swift files of the same name
	Replaced        []string // Swift files WriteGo left to the overlay
	Test            bool     // transpiling a test target into the package under test
	Target          string
	dm              *types.Demangler
	typesByQName    map[string]*DeclInfo
	decls           map[string]*DeclInfo
	sorted          []*DeclInfo       // decls ordered by key, built on first use after a change to decls
	typeKinds       map[string]string // qualified Swift type name → decl kind
	protoReqs       map[string][]ast.Node
	protoParents    map[string][]string   // a local protocol's directly inherited protocols
	conformers      map[string][]string   // protocol qualified name → conforming local types
	optConforms     map[string]ast.Node   // local protocol → the extension that conforms Optional to it where Wrapped does
	foreignConforms map[string][]ast.Node // protocol → the extensions that conform a foreign type to it
	varInit         map[string]ast.Node   // stored var usr → initializer expr
	Unsupported     map[string]int
	UnsupportedAt   map[string][]string
	Deps            []*Module
	outDir          string                      // the directory WriteGo writes this module to
	retroFiles      map[string]*strings.Builder // dependency package → methods this module adds to its types
	retroDone       map[string]bool             // pkg.Type.Method already added
	ImportPath      string
	Mapped          bool                       // read for its declarations only: a hand-written Go package replaces it
	overlaid        map[string]bool            // USRs of the functions declared in Swift files an overlay replaces
	Mappings        []Mapping                  // hand-written Go packages standing in for modules not transpiled in this run
	mappedKinds     map[string]map[string]byte // nominal kinds of each mapped module's types, read on first use
	adapters        map[string]bool            // adapter types emitted for retroactive conformances to local protocols
	importPkgs      map[string]string
	typeCache       map[string]*types.Type
	payloadEnums    map[string]bool
	enumComparable  map[string]bool
	typeParamNames  map[string][]string // generic struct/class → its Go type parameter names
	shares          map[string]bool     // memo of sharesStorage for named types
	frozen          map[string]bool     // structs whose stored properties are all `let` (frozenStruct)
	classProtos     map[string]bool     // protocols only classes can conform to (`: AnyObject`)
	scopes          *valueScopes        // local variables of every file, resolved
	fx              *effects            // which functions can write shared places
	inPlace         map[string]bool     // USRs of properties some code writes into (replacedWhole)
	lazyInits       map[string]bool     // owner|name of stored statics and globals with non-literal initializers
	lineStarts      map[string][]int
	sources         map[string][]byte
	objcNames       map[string]string // Swift names of Objective-C members, by USR
	hoists          int
	keyable         map[string]bool // whether a local struct can be a Go map key
	mutatingTypes   map[string]bool
	overridden      map[string]bool   // USRs of class members some subclass overrides
	superOf         map[string]string // a class's superclass, qualified by package when a dependency declares it
	openClasses     map[string]bool   // local open classes, which another module can subclass
	methodNames     map[string]bool   // "owner name" of every function declared on a type
	typeAliases     map[string]string // qualified typealias name → mangled aliased type
	members         map[string][]*DeclInfo
	protocolsByType map[string][]string

	// corelibsFoundation is set by [CorelibsFoundation].
	corelibsFoundation bool

	// Interface identifies the module's declarations, as the AST cache fingerprints them. An
	// incremental run sets it on the module and its dependencies.
	Interface string
	// EmitCache is the file in which [Module.WriteGo] records what each Go file it writes depends on.
	// When set, WriteGo keeps a Go file whose Swift file and dependencies are unchanged since that
	// record, instead of emitting it again.
	EmitCache string
	// EmitKey identifies the transpiler that writes the Go; a cache made by another is not used.
	EmitKey string
	// CheckEmitCache makes WriteGo emit the files it would keep too, and list in Stale those whose
	// Go would differ.
	CheckEmitCache bool
	// Reused counts the Go files the last WriteGo kept from an earlier run, or found it could keep
	// under CheckEmitCache.
	Reused int
	// Stale lists the Go files WriteGo would have kept with content that emitting them changes.
	Stale []string

	rec      *fileRecord // what the emission of the current file reads and changes
	indexing string      // the file New is indexing
}

// protocolsOf lists the protocols the qualified type q declares conformance to, in this module or a
// dependency.
func (m *Module) protocolsOf(q string) []string {
	if m.protocolsByType == nil {
		m.protocolsByType = map[string][]string{}
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, p := range slices.Sorted(maps.Keys(mod.conformers)) {
				for _, t := range mod.conformers[p] {
					if !slices.Contains(m.protocolsByType[t], p) {
						m.protocolsByType[t] = append(m.protocolsByType[t], p)
					}
				}
			}
		}
	}
	return m.protocolsByType[q]
}

// assocErasure reports which parameters, by index, and whether the result of fn, a method of q, take
// the place of an associated type in the protocol requirement fn implements. Those are any in Go, so
// fn's Go signature must use any there too to satisfy the protocol's interface.
func (m *Module) assocErasure(fn ast.Node, q string) (params map[int]bool, result bool) {
	for _, proto := range m.protocolsOf(q) {
		p, r := m.requirementErasure(proto, fn)
		for i := range p {
			if params == nil {
				params = map[int]bool{}
			}
			params[i] = true
		}
		result = result || r
	}
	return params, result
}

// requirementErasure is assocErasure for an implementation of a requirement of proto, or of a protocol
// proto refines.
func (m *Module) requirementErasure(proto string, fn ast.Node) (params map[int]bool, result bool) {
	key := funcSignatureKey(fn)
	seen := map[string]bool{}
	var visit func(p string)
	visit = func(p string) {
		if seen[p] {
			return
		}
		seen[p] = true
		for _, pm := range append([]*Module{m}, m.Deps...) {
			for _, r := range pm.protoReqs[p] {
				if r.Kind() != "func_decl" || funcSignatureKey(r) != key || r.Bool("static") != fn.Bool("static") {
					continue
				}
				isAssoc := func(mangled string) bool {
					t := pm.typeOf(mangled)
					return t.Kind == types.Named && m.isAssociatedType(t.Name)
				}
				result = result || isAssoc(r.Str("result"))
				for i, rp := range r.Child("params").List("params") {
					if !rp.Bool("inout") && isAssoc(rp.Str("interface_type")) {
						if params == nil {
							params = map[int]bool{}
						}
						params[i] = true
					}
				}
			}
			for _, parent := range pm.protoParents[p] {
				visit(parent)
			}
		}
	}
	visit(proto)
	return params, result
}

// membersOf lists the declarations this module makes in type q and its extensions.
func (m *Module) membersOf(q string) []*DeclInfo {
	if m.members == nil {
		m.members = map[string][]*DeclInfo{}
		for _, d := range m.sortedDecls() {
			if d.Owner != "" && d.Node != nil {
				m.members[d.Owner] = append(m.members[d.Owner], d)
			}
		}
	}
	return m.members[q]
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

// HasSubclass reports whether some local class inherits from goName, or goName is an open class,
// which a class of another module can inherit from.
func (m *Module) HasSubclass(goName string) bool {
	m.classTree()
	if m.openClasses[goName] {
		return true
	}
	for _, sup := range m.superOf {
		if sup == goName {
			return true
		}
	}
	return false
}

// inHierarchy reports whether a local class has a superclass or a subclass.
func (m *Module) inHierarchy(goName string) bool {
	return m.Superclass(goName) != "" || m.HasSubclass(goName)
}

// ancestors lists goName's superclasses, nearest first, following the chain into the dependencies
// that declare them. A dependency's class is qualified by its package ("networking.Descriptor").
func (m *Module) ancestors(goName string) []string {
	var out []string
	mod, name, prefix := m, goName, ""
	for {
		s := mod.Superclass(name)
		if s == "" {
			return out
		}
		pkg, rest, qualified := strings.Cut(s, ".")
		if !qualified {
			out = append(out, prefix+s)
			name = s
			continue
		}
		out = append(out, s)
		dep := mod.depByPkg(pkg)
		if dep == nil {
			return out
		}
		mod, name, prefix = dep, rest, pkg+"."
	}
}

// depByPkg returns the transpiled dependency whose Go package is pkg.
func (m *Module) depByPkg(pkg string) *Module {
	for _, d := range m.Deps {
		if d.Pkg == pkg {
			return d
		}
	}
	return nil
}

// embeddedField is the field name Go gives an embedded type: its name without package or type
// arguments.
func embeddedField(goType string) string {
	goType = strings.TrimPrefix(goType, "*")
	if i := strings.Index(goType, "["); i >= 0 {
		goType = goType[:i]
	}
	return goType[strings.LastIndex(goType, ".")+1:]
}

// IsAncestor reports whether anc is goName's superclass, directly or transitively. Both names may be
// qualified by the package of a dependency that declares the classes ("world.Player").
func (m *Module) IsAncestor(anc, goName string) bool {
	if pkg, name, ok := strings.Cut(goName, "."); ok {
		// The dependency names its own types bare and every other module's qualified.
		ancName, found := strings.CutPrefix(anc, pkg+".")
		if !found && !strings.Contains(anc, ".") {
			ancName = m.Pkg + "." + anc
		}
		for _, d := range m.Deps {
			if d.Pkg == pkg {
				return d.IsAncestor(ancName, name)
			}
		}
		return false
	}
	return slices.Contains(m.ancestors(goName), anc)
}

// markOpenOverridable marks the members of the open class n that another module can override.
func (m *Module) markOpenOverridable(n ast.Node) {
	for _, mem := range n.List("members") {
		info := m.decls[mem.Str("usr")]
		if info == nil || info.Static || mem.Bool("final") {
			continue
		}
		switch mem.Kind() {
		case "func_decl", "var_decl", "subscript_decl":
			info.OpenOverridable = mem.Str("access") == "open"
		case "constructor_decl":
			access := mem.Str("access")
			info.OpenOverridable = mem.Bool("designated") && (access == "open" || access == "public")
		}
	}
}

// upcastMethod names the generated method that upcasts to class goName, which may be package-qualified.
func upcastMethod(goName string) string {
	return "As" + goName[strings.LastIndex(goName, ".")+1:]
}

func (m *Module) classTree() {
	if m.superOf != nil {
		return
	}
	m.superOf = map[string]string{}
	m.openClasses = map[string]bool{}
	for _, d := range m.decls {
		if d.Kind != "class_decl" || d.Node == nil {
			continue
		}
		if d.Node.Str("access") == "open" {
			m.openClasses[d.GoName] = true
		}
		st := d.Node.Child("inherits").Str("superclass_type")
		if st == "" {
			continue
		}
		t := m.typeOf(st)
		g := strings.TrimPrefix(m.GoType(t, nil), "*")
		if g == "" || g == "any" {
			continue
		}
		if pkg, _, ok := strings.Cut(g, "."); ok {
			// A superclass another transpiled module declares.
			if dep := m.depByPkg(pkg); dep == nil || dep.typeKinds[t.Name] != "class_decl" {
				continue
			}
		}
		m.superOf[d.GoName] = g
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

// An Option configures a module that [New] prepares.
type Option func(*Module)

// Dependencies sets the transpiled modules the module imports, so that indexing sees the classes it
// inherits from them.
func Dependencies(deps []*Module) Option {
	return func(m *Module) { m.Deps = deps }
}

// CorelibsFoundation marks a module compiled against Foundation as Linux and the other platforms
// without Objective-C declare it. Its Foundation names are respelled as Darwin's, so it calls the
// runtime by the names a module compiled for macOS does.
func CorelibsFoundation() Option {
	return func(m *Module) { m.corelibsFoundation = true }
}

// New prepares a module: demangles every symbol and indexes declarations.
func New(name string, files []ast.File, rtPath string, opts ...Option) (*Module, error) {
	m := &Module{
		Name: name, Pkg: strings.ToLower(name), RTPath: rtPath, Files: files,
		decls: map[string]*DeclInfo{}, typeKinds: map[string]string{},
		protoReqs: map[string][]ast.Node{}, protoParents: map[string][]string{}, conformers: map[string][]string{}, optConforms: map[string]ast.Node{}, foreignConforms: map[string][]ast.Node{},
		varInit: map[string]ast.Node{}, typeParamNames: map[string][]string{}, lazyInits: map[string]bool{}, overridden: map[string]bool{}, importPkgs: map[string]string{}, objcNames: map[string]string{}, typeCache: map[string]*types.Type{}, typeAliases: map[string]string{}, Unsupported: map[string]int{}, UnsupportedAt: map[string][]string{},
	}
	for _, o := range opts {
		o(m)
	}
	type fileSyms struct {
		syms []string
		objc [][2]string // decl_usr, base_name
	}
	found := make([]fileSyms, len(files))
	parallel(len(files), func(i int) {
		seen := map[string]bool{}
		fs := &found[i]
		for _, it := range files[i].Items {
			ast.Walk(it, func(n ast.Node) {
				for _, k := range symbolKeys {
					if s := n.Str(k); (strings.HasPrefix(s, "$s") || strings.HasPrefix(s, "s:")) && !seen[s] {
						seen[s] = true
						fs.syms = append(fs.syms, s)
					}
				}
				if usr := n.Str("decl_usr"); strings.HasPrefix(usr, "c:objc(") && n.Str("base_name") != "" {
					fs.objc = append(fs.objc, [2]string{usr, n.Str("base_name")})
				}
			})
		}
	})
	var syms []string
	for _, fs := range found {
		syms = append(syms, fs.syms...)
		for _, o := range fs.objc {
			m.objcNames[o[0]] = o[1]
		}
	}
	dm, err := types.NewDemanglerDistinct(syms, typesNamedApart(files))
	if err != nil {
		return nil, err
	}
	m.dm = dm
	for _, f := range files {
		m.indexing = f.Filename
		m.index(f.Items, "", "")
	}
	m.indexing = ""
	m.markLazy()
	m.markGenericFree()
	m.assignNames()
	return m, nil
}

// symbolKeys are the fields that hold the mangled types and USRs a module's demangler answers for.
var symbolKeys = []string{"type", "usr", "decl_usr", "interface_type", "result", "extended_type", "type_usr", "first_type", "second_type", "protocol", "generic_param", "replacement_type"}

// parallel calls f for 0..n-1, on as many goroutines at a time as there are CPUs.
func parallel(n int, f func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i := range n {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			f(i)
		})
	}
	wg.Wait()
}

// SetTarget marks the module as a test target whose code joins the package of under. Package-level names the
// two modules share, such as file-private globals both declare, are renamed in the test module.
// SetTestPackage makes m a test target with a Go package of its own, which imports the target it tests like
// any other dependency. A test target needs one when another of its dependencies imports the tested target.
func (m *Module) SetTestPackage() {
	m.Test = true
}

func (m *Module) SetTarget(under *Module) {
	m.Test = true
	m.Pkg = under.Pkg
	taken := map[string]bool{}
	for _, info := range under.decls {
		if info.GoName != "" && under.packageLevel(info) {
			taken[info.GoName] = true
		}
	}
	var usrs []string
	for usr, info := range m.decls {
		switch info.Kind {
		case "struct_decl", "class_decl", "enum_decl", "protocol":
			continue
		}
		if info.GoName != "" && m.packageLevel(info) {
			usrs = append(usrs, usr)
		}
	}
	sort.Strings(usrs)
	for _, usr := range usrs {
		info := m.decls[usr]
		if !taken[info.GoName] {
			continue
		}
		base := info.GoName + "Test"
		name := base
		for n := 2; taken[name]; n++ {
			name = base + strconv.Itoa(n)
		}
		info.GoName = name
		taken[name] = true
	}
}

func (m *Module) demangle(s string) string {
	if strings.HasPrefix(s, "c:objc(") {
		if d := m.objcDemangle(s); d != "" {
			return d
		}
	}
	if d, ok := m.dm.Lookup(s); ok {
		return m.foundationName(d)
	}
	// Signatures of a dependency's declarations carry symbols only its demangler has seen.
	for _, dep := range m.Deps {
		if d, ok := dep.dm.Lookup(s); ok {
			return m.foundationName(d)
		}
	}
	return s
}

func (m *Module) foundationName(d string) string {
	if m.corelibsFoundation {
		return darwinFoundationName(d)
	}
	return d
}

// objcMember matches an Objective-C member USR: c:objc(cs)Class(kind)selector.
var objcMember = regexp.MustCompile(`^c:objc\((?:cs|pl)\)(\w+)\((im|cm|py|cpy)\)`)

// objcDemangle renders an Objective-C member USR as "__C.Class.swiftName", with "static " for class
// members, using the Swift name the call sites refer to it by.
func (m *Module) objcDemangle(usr string) string {
	match := objcMember.FindStringSubmatch(usr)
	name := m.objcNames[usr]
	if match == nil || name == "" {
		return ""
	}
	out := "__C." + match[1] + "." + name
	if match[2] == "cm" || match[2] == "cpy" {
		out = "static " + out
	}
	return out
}

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
	// Reference ownership doesn't change the Go type: a weak reference is the same pointer.
	for _, own := range []string{"weak ", "unowned(unsafe) ", "unowned "} {
		dem = strings.TrimPrefix(dem, own)
	}
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
			if owner == "" {
				if t := m.typeOf(n.Str("interface_type")); t != nil && t.Kind == types.Meta && len(t.Args) == 1 && strings.HasPrefix(t.Args[0].Name, m.Name+"."+n.Name()+"_") {
					// A file-private or local type named apart from another of the same name.
					q = t.Args[0].Name
				}
			}
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
			m.decls[declKey(n)] = &DeclInfo{Node: n, Kind: n.Kind(), Owner: owner, OwnerKind: ownerKind, file: m.indexing, QName: q}
			m.sorted = nil
			for _, c := range n.Child("inherits").List("conformances") {
				if p := c.Str("protocol"); p != "" {
					pn := m.demangle(p)
					m.conformers[pn] = append(m.conformers[pn], q)
				}
			}
			if n.Kind() == "protocol" {
				m.protoReqs[q] = n.List("members")
				for _, p := range n.Child("inherits").Strings("protocols") {
					m.protoParents[q] = append(m.protoParents[q], m.demangle(p))
				}
				for _, r := range n.Child("requirement_signature").List("requirements") {
					if r.Str("kind") == "has_layout" && r.Str("layout") == "AnyObject" {
						if m.classProtos == nil {
							m.classProtos = map[string]bool{}
						}
						m.classProtos[q] = true
					}
				}
			}
			m.index(n.List("members"), q, n.Kind())
			if n.Kind() == "class_decl" && n.Str("access") == "open" {
				m.markOpenOverridable(n)
			}
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
					if q == "Swift.Optional" && wrappedConformsTo(n, p) {
						m.optConforms[pn] = n
					}
					if kind == "foreign" && q != "Swift.Optional" {
						m.foreignConforms[pn] = append(m.foreignConforms[pn], n)
					}
				}
			}
			m.index(n.List("members"), q, kind)
		case "typealias":
			m.typeAliases[m.qualify(owner, n.Name())] = n.Str("type")
		case "top_level_code_decl":
			m.indexLocalTypes(n.Child("body"))
		case "func_decl", "constructor_decl", "var_decl", "enum_element_decl", "subscript_decl":
			switch n.Kind() {
			case "func_decl", "constructor_decl":
				m.indexLocalTypes(n.Child("body"))
			case "var_decl", "subscript_decl":
				// Types declared inside accessors.
				for _, a := range n.List("accessors") {
					m.indexLocalTypes(a.Child("body"))
				}
			}
			info := &DeclInfo{Node: n, Kind: n.Kind(), Owner: owner, OwnerKind: ownerKind, Static: n.Bool("static"), file: m.indexing}
			if ownerKind == "protocol_ext" {
				info.ProtoExt = true
			}
			if n.Kind() == "var_decl" {
				impl := n.Str("readImpl")
				info.Computed = impl != "" && impl != "stored"
				info.Settable = info.Computed && n.Str("writeImpl") == "setter"
				info.Observed = n.Str("writeImpl") == "stored_with_observers"
				if owner != "" && !info.Static && !info.Computed && !info.Observed && strings.HasPrefix(m.demangle(n.Str("interface_type")), "weak ") {
					info.Weak, info.Computed, info.Settable = true, true, true
				}
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
				m.sorted = nil
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

// typesNamedApart names the file-private and function-local types that share their name with another type
// at the top level of the module or inside a function, which Go, with one package scope, cannot tell apart.
func typesNamedApart(files []ast.File) map[string]bool {
	count, apart := map[string]int{}, map[string]bool{}
	var locals func(n ast.Node)
	locals = func(n ast.Node) {
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
					count[k.Name()]++
					apart[k.Name()] = true
				default:
					locals(k)
				}
			}
		}
	}
	var members func(items []ast.Node, top bool)
	members = func(items []ast.Node, top bool) {
		for _, n := range items {
			switch n.Kind() {
			case "struct_decl", "class_decl", "enum_decl", "protocol":
				if top {
					count[n.Name()]++
					if a := n.Str("access"); a == "private" || a == "fileprivate" {
						apart[n.Name()] = true
					}
				}
				members(n.List("members"), false)
			case "extension_decl":
				members(n.List("members"), false)
			case "func_decl", "constructor_decl", "var_decl", "subscript_decl", "top_level_code_decl":
				locals(n)
			}
		}
	}
	for _, f := range files {
		members(f.Items, true)
	}
	names := map[string]bool{}
	for name, n := range count {
		if n > 1 && apart[name] {
			names[name] = true
		}
	}
	return names
}

// indexLocalTypes indexes types declared inside a function body or top-level code; they are hoisted to
// package level.
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

// typeDecl is the declaration of the module's type with qualified name q, or nil.
func (m *Module) typeDecl(q string) *DeclInfo {
	if q == "" {
		return nil
	}
	if m.typesByQName == nil {
		m.typesByQName = map[string]*DeclInfo{}
		for _, d := range m.sortedDecls() {
			switch d.Kind {
			case "struct_decl", "class_decl", "enum_decl":
				m.typesByQName[m.typeQName(d)] = d
			}
		}
	}
	return m.typesByQName[q]
}

// typeQName is the qualified name of the type that info declares.
func (m *Module) typeQName(info *DeclInfo) string {
	if info.QName != "" {
		return info.QName
	}
	return m.qualify(info.Owner, info.Node.Name())
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
	if base, ok := strings.CutSuffix(s, "_"); ok && shadowedPkgs[base] {
		s = base
	}
	if s[0] == '_' {
		// Go exports no name that starts with an underscore.
		return "X" + s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// shadowedPkgs holds the names of the Go packages the module being written can import. A Swift
// local or parameter with one of these names would hide the package from the rest of its
// function, so goSafe renames it.
var shadowedPkgs map[string]bool

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
			info.GoName = m.LocalTypeName(m.typeQName(info))
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
			if rest, ok := strings.CutPrefix(name, "$__lazy_storage_$_"); ok {
				info.GoName = m.memberName(info, owner, capitalize(goSafe(rest))+"LazyStorage")
				break
			}
			if strings.HasPrefix(name, "$") {
				info.GoName = m.memberName(info, owner, capitalize(name[1:])+"Projected")
				break
			}
			info.GoName = m.memberName(info, owner, capitalize(goSafe(name)))
		case "enum_element_decl":
			info.GoName = owner + capitalize(name)
		case "subscript_decl":
			info.GoName = m.memberName(info, owner, "Subscript"+labelSuffix(paramLabels(info.Node)))
		}
	}
	m.settleClassMemberNames()
	m.dedupePackageNames()
}

// settleClassMemberNames gives every member of an override chain the Go name of the chain's root, so
// a call through any class in the chain reaches the root's dispatching method. A class member that
// overrides nothing but has the name of a requirement of a protocol a superclass conforms to is
// renamed: the superclass's conformance uses the protocol's default, which Swift keeps for
// subclasses, so the member must not satisfy the Go interface in its place.
func (m *Module) settleClassMemberNames() {
	classQ := map[string]string{}
	for _, d := range m.decls {
		if d.Kind == "class_decl" && d.Node != nil {
			classQ[d.GoName] = m.typeQName(d)
		}
	}
	usrs := slices.Sorted(maps.Keys(m.decls))
	member := func(info *DeclInfo) bool {
		return (info.Kind == "func_decl" || info.Kind == "var_decl" || info.Kind == "subscript_decl") && !info.Static && m.typeKinds[info.Owner] == "class_decl"
	}
	for _, usr := range usrs {
		info := m.decls[usr]
		if !member(info) || info.Overrides != "" {
			continue
		}
		owner := m.LocalTypeName(info.Owner)
		for sup := m.Superclass(owner); sup != ""; sup = m.Superclass(sup) {
			if m.defaultWitnessNamed(classQ[sup], info.Node.Kind(), info.Node.Name()) {
				info.GoName += "_" + owner
				info.NotWitness = true
				break
			}
		}
	}
	for _, usr := range usrs {
		info := m.decls[usr]
		if !member(info) || info.Overrides == "" {
			continue
		}
		root := info
		for root.Overrides != "" && m.declOf(root.Overrides) != nil {
			root = m.declOf(root.Overrides)
		}
		info.GoName, info.NotWitness = root.GoName, root.NotWitness
	}
}

// defaultWitnessNamed reports whether the class q conforms to a protocol with a requirement of the
// given kind and name.
func (m *Module) defaultWitnessNamed(q, kind, name string) bool {
	for proto, types := range m.conformers {
		if !slices.Contains(types, q) {
			continue
		}
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, r := range mod.protoReqs[proto] {
				if r.Kind() == kind && r.Name() == name && !r.Bool("static") {
					return true
				}
			}
		}
	}
	return false
}

// dedupePackageNames renames package-level Go names that collide, keeping type names unchanged.
func (m *Module) dedupePackageNames() {
	byName := map[string][]*DeclInfo{}
	usrs := slices.Sorted(maps.Keys(m.decls))
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
	// A method such as `setTarget(_:)` collides with the generated setter of `target`.
	setters := map[string]bool{}
	for _, usr := range usrs {
		info := m.decls[usr]
		if info.Kind == "var_decl" && info.Owner != "" && !info.Static && (info.Settable || info.Observed) {
			setters[info.Owner+".Set"+info.GoName] = true
		}
	}
	// A property such as `setId` collides with the setter a sibling property `id` can get.
	vars := map[string]bool{}
	for _, usr := range usrs {
		if info := m.decls[usr]; info.Kind == "var_decl" && info.Owner != "" && !info.Static {
			vars[info.Owner+"."+info.GoName] = true
		}
	}
	for _, usr := range usrs {
		info := m.decls[usr]
		if rest, ok := strings.CutPrefix(info.GoName, "Set"); ok && info.Kind == "var_decl" && info.Owner != "" && !info.Static && vars[info.Owner+"."+rest] {
			info.GoName += "Value"
		}
	}
	// A method such as `coloredTag(_:)` also collides with the getter of `var coloredTag`.
	for _, usr := range usrs {
		info := m.decls[usr]
		if info.Kind == "var_decl" && info.Owner != "" && !info.Static {
			setters[info.Owner+"."+info.GoName] = true
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
	case "func_decl", "var_decl", "subscript_decl":
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
	if strings.ContainsFunc(n, isRawIdentifierRune) {
		n = rawIdentifier(n)
	}
	if len(n) > 1 && n[0] == '$' && n[1] >= '0' && n[1] <= '9' {
		return "p" + n[1:]
	}
	n = strings.ReplaceAll(n, "$", "_")
	if goKeywords[n] || shadowedPkgs[n] {
		return n + "_"
	}
	return n
}

// isRawIdentifierRune reports a character that only a raw identifier (“ `get and take` “) can hold.
func isRawIdentifierRune(r rune) bool {
	return r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// rawIdentifier spells a raw identifier's words as one camel-case Go identifier, with an underscore before a
// leading digit.
func rawIdentifier(n string) string {
	var b strings.Builder
	for i, w := range strings.FieldsFunc(n, isRawIdentifierRune) {
		if i > 0 {
			w = capitalize(w)
		}
		b.WriteString(w)
	}
	out := b.String()
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "_" + out
	}
	return out
}

func (m *Module) countUnsupported(kind, where string) {
	m.Unsupported[kind]++
	if len(m.UnsupportedAt[kind]) < 5 {
		m.UnsupportedAt[kind] = append(m.UnsupportedAt[kind], where)
	}
}

// Unsup records an unsupported construct and returns a Go expression that compiles.
func (m *Module) Unsup(kind, where, goType string) string {
	m.countUnsupported(kind, where)
	m.noteUnsupported(kind, where)
	if goType == "" || goType == "any" {
		return fmt.Sprintf("swiftrt.Unsupported[any](%q)", kind)
	}
	return fmt.Sprintf("swiftrt.Unsupported[%s](%q)", goType, kind)
}

// WriteGo writes one Go file per Swift file into dir and returns their paths. A Swift file with a
// hand-written replacement in the Overlay directory is not translated: the replacement is copied in
// its place, along with any other Go file in the directory.
func (m *Module) WriteGo(dir string) ([]string, error) {
	m.settleWitnessNames()
	m.separateFieldsFromMethods()
	m.outDir, m.retroFiles, m.retroDone = dir, map[string]*strings.Builder{}, map[string]bool{}
	shadowedPkgs = map[string]bool{"swiftrt": true}
	for _, d := range m.Deps {
		shadowedPkgs[d.Pkg] = true
	}
	for _, mp := range m.Mappings {
		shadowedPkgs[mp.Alias()] = true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	overlay, err := m.overlayFiles()
	if err != nil {
		return nil, err
	}
	env := m.emitEnv(overlay)
	cache := readEmitCache(m.EmitCache, env)
	next := &emitCache{Env: env, Files: map[string]*fileRecord{}}
	mods := m.chain()
	if env != "" {
		// Effects read by the first file that needs them would be recorded as that file's own reads.
		for _, mod := range mods {
			mod.effectsOf()
		}
	}
	var paths []string
	for _, f := range m.Files {
		base := strings.TrimSuffix(filepath.Base(f.Filename), ".swift")
		suffix := ".go"
		if m.Test {
			suffix = "_test.go"
		}
		name := toSnake(base) + suffix
		if overlay[name] {
			m.Replaced = append(m.Replaced, filepath.Base(f.Filename))
			continue
		}
		out := filepath.Join(dir, name)
		kept := false
		if cache != nil {
			r := cache.Files[name]
			kept = m.reusable(r, f, out, mods)
			if kept && !m.CheckEmitCache {
				m.replay(r, mods)
				next.Files[name] = r
				m.Reused++
				paths = append(paths, out)
				continue
			}
		}
		var rec *fileRecord
		if env != "" {
			rec = newRecord(f)
			m.recordInto(rec)
		}
		fe := &fileEmitter{m: m, file: f, lvSubst: map[string]string{}, localFuncs: map[string]ast.Node{}}
		src := fe.emitFile()
		m.recordInto(nil)
		src = strings.ReplaceAll(src, "for _, _ := range ", "for range ")
		formatted, err := format.Source([]byte(src))
		if err != nil {
			formatted = []byte(src)
			fmt.Fprintf(os.Stderr, "gofmt %s: %v\n", out, err)
		}
		if kept {
			if old, err := os.ReadFile(out); err == nil && !bytes.Equal(old, formatted) { //nolint:gosec // out is a file this module writes
				m.Stale = append(m.Stale, name)
			} else {
				m.Reused++
			}
		}
		if err := writeIfChanged(out, formatted); err != nil {
			return nil, err
		}
		if rec != nil {
			rec.Out = hashBytes(formatted)
			next.Files[name] = rec
		}
		paths = append(paths, out)
	}
	for _, pkg := range slices.Sorted(maps.Keys(m.retroFiles)) {
		// Methods that retroactive conformances add to a dependency's types live in its package.
		src := "// Code generated by swift2go for " + m.Name + "'s conformances of this package's types. DO NOT EDIT.\n\npackage " + pkg + "\n\n"
		body := m.retroFiles[pkg].String()
		if strings.Contains(body, "swiftrt.") {
			src += "import \"" + m.RTPath + "\"\n\n"
		}
		out := filepath.Join(filepath.Dir(dir), pkg, "retroactive_"+m.Pkg+".go")
		formatted, err := format.Source([]byte(src + body))
		if err != nil {
			formatted = []byte(src + body)
			fmt.Fprintf(os.Stderr, "gofmt %s: %v\n", out, err)
		}
		if err := writeIfChanged(out, formatted); err != nil {
			return nil, err
		}
		paths = append(paths, out)
	}
	if !m.Test {
		if err := m.removeStaleRetro(dir); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(overlay)) {
		data, err := os.ReadFile(filepath.Join(m.Overlay, name))
		if err != nil {
			return nil, err
		}
		out := filepath.Join(dir, name)
		if err := writeIfChanged(out, data); err != nil {
			return nil, err
		}
		paths = append(paths, out)
	}
	if err := m.removeStaleGo(dir, paths); err != nil {
		return nil, err
	}
	switch {
	case m.EmitCache == "":
	case env == "":
		_ = os.Remove(m.EmitCache)
	case cache == nil || len(next.Files) != len(cache.Files) || m.Reused < len(next.Files) || m.CheckEmitCache:
		if err := writeEmitCache(m.EmitCache, next); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// removeStaleGo deletes the files in dir that an earlier run generated for this target from a
// Swift file that no longer exists. It only touches files carrying swift2go's generated-code
// header, leaves the other target kind's files alone (a test target shares its directory with the
// target it tests), and keeps the retroactive-conformance files that dependent targets write here.
func (m *Module) removeStaleGo(dir string, written []string) error {
	keep := map[string]bool{}
	for _, p := range written {
		keep[filepath.Base(p)] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || keep[name] || !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "retroactive_") ||
			strings.HasSuffix(name, "_test.go") != m.Test {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path) //nolint:gosec // name is a directory entry of the output directory
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(data, []byte("// Code generated by swift2go")) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// removeStaleRetro deletes the retroactive-conformance files an earlier run of this target wrote in
// the packages of dependencies it no longer adds methods to. dir is the target's own output directory.
func (m *Module) removeStaleRetro(dir string) error {
	for _, d := range m.Deps {
		if d.Mapped || m.retroFiles[d.Pkg] != nil {
			continue
		}
		path := filepath.Join(filepath.Dir(dir), d.Pkg, "retroactive_"+m.Pkg+".go")
		data, err := os.ReadFile(path) //nolint:gosec // a file this module wrote in a dependency's output directory
		if err != nil || !bytes.HasPrefix(data, []byte("// Code generated by swift2go")) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// overlayFiles is the set of Go files in the Overlay directory that belong to this target: test
// files for a test target, the others otherwise.
func (m *Module) overlayFiles() (map[string]bool, error) {
	files := map[string]bool{}
	if m.Overlay == "" {
		return files, nil
	}
	entries, err := os.ReadDir(m.Overlay)
	if err != nil {
		return nil, fmt.Errorf("overlay for %s: %w", m.Name, err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") != m.Test {
			continue
		}
		files[name] = true
	}
	return files, nil
}

// overlaidFunc reports whether the function usr is declared in a Swift file that a hand-written
// overlay file replaces.
func (m *Module) overlaidFunc(usr string) bool {
	if m.overlaid == nil {
		m.overlaid = map[string]bool{}
		overlay, _ := m.overlayFiles()
		for _, f := range m.Files {
			suffix := ".go"
			if m.Test {
				suffix = "_test.go"
			}
			if !overlay[toSnake(strings.TrimSuffix(filepath.Base(f.Filename), ".swift"))+suffix] {
				continue
			}
			for _, item := range f.Items {
				ast.Walk(item, func(n ast.Node) {
					if n.Kind() == "func_decl" && n.Str("usr") != "" {
						m.overlaid[n.Str("usr")] = true
					}
				})
			}
		}
	}
	return m.overlaid[usr]
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
	slices.SortFunc(all, func(a, b kv) int { return cmp.Or(cmp.Compare(b.v, a.v), strings.Compare(a.k, b.k)) })
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d unsupported\n", m.Name, total)
	if len(m.Replaced) > 0 {
		fmt.Fprintf(&b, "  replaced by overlay: %s\n", strings.Join(m.Replaced, ", "))
	}
	for _, e := range all {
		fmt.Fprintf(&b, "  %5d %s   e.g. %s\n", e.v, e.k, strings.Join(m.UnsupportedAt[e.k], "; "))
	}
	return b.String()
}

var foreignRuntimeName = regexp.MustCompile(`swiftrt\.[A-Za-z0-9_]*[:@$][A-Za-z0-9_@$][A-Za-z0-9_:@$]*`)

// sanitizeRuntimeNames turns clang/ObjC USR fragments that leaked into swiftrt identifiers into Go identifiers.
func sanitizeRuntimeNames(src string) string {
	return foreignRuntimeName.ReplaceAllStringFunc(src, func(s string) string {
		// A colon that ends the match is the text after the name (a dictionary literal's separator).
		tail := ""
		if rest, ok := strings.CutSuffix(s, ":"); ok {
			s, tail = rest, ":"
		}
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
		return "swiftrt." + b.String() + tail
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
	i := max(sort.SearchInts(starts, off+1)-1, 0)
	return i + 1, off - starts[i] + 1
}

// FreeSelf reports whether a member is emitted as a package-level function taking self as its first parameter.
func (info *DeclInfo) FreeSelf() bool {
	return info.OwnerKind == "foreign" || info.ProtoExt || info.OnPayloadEnum || info.GenericFree
}

// markGenericFree flags methods that must become generic package-level functions.
func (m *Module) markGenericFree() {
	for _, d := range m.decls {
		if d.Kind == "subscript_decl" && !d.Static && d.Owner != "" && d.OwnerKind != "protocol" && m.typeKinds[d.Owner] != "protocol" {
			// A subscript with generic parameters of its own becomes a pair of generic functions.
			if len(m.typeParamNames[d.Owner]) == 0 && len(d.Node.Child("generic_signature").Strings("generic_params")) > 0 {
				d.GenericFree = true
			}
			continue
		}
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
			// A conformer of a protocol with a mutating or settable requirement gets pointer-receiver
			// setters and forwarders, so only a pointer to it satisfies the protocol.
			for proto, reqs := range mod.protoReqs {
				for _, r := range reqs {
					if !r.Bool("static") && (r.Kind() == "func_decl" && r.Child("implicit_self_decl").Bool("inout") || r.Kind() == "var_decl" && r.Str("writeImpl") == "setter") {
						for _, q := range mod.conformers[proto] {
							m.mutatingTypes[q] = true
						}
						break
					}
				}
			}
			for q, k := range mod.typeKinds {
				if k == "class_decl" || k == "enum_decl" {
					delete(m.mutatingTypes, q)
				}
			}
		}
		m.markMutatingDefaults()
	}
	return m.mutatingTypes[q]
}

// markMutatingDefaults marks the structs that receive a mutating protocol-extension default as a
// pointer-receiver forwarder, since only a pointer to them then satisfies the protocol.
func (m *Module) markMutatingDefaults() {
	mods := append([]*Module{m}, m.Deps...)
	for _, pm := range mods {
		for _, d := range pm.sortedDecls() {
			if !d.ProtoExt || !d.Mutating || d.Kind != "func_decl" || m.inheritedRequirementName(d.Owner, d.Node) == "" {
				continue
			}
			key := funcSignatureKey(d.Node)
			for _, cm := range mods {
				for _, q := range cm.conformers[d.Owner] {
					if cm.typeKinds[q] != "struct_decl" || m.mutatingTypes[q] || !m.extensionApplies(d.Node, q, d.Owner, pm) {
						continue
					}
					defined := false
					for _, om := range mods {
						for _, mem := range om.membersOf(q) {
							if mem.Kind == "func_decl" && !mem.ProtoExt && funcSignatureKey(mem.Node) == key {
								defined = true
							}
						}
					}
					if !defined {
						m.mutatingTypes[q] = true
					}
				}
			}
		}
	}
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
		if d.Kind == "var_decl" && !d.Computed && (d.Static || d.Owner == "") && !d.Node.Bool("top_level_global") && m.lazyInits[d.Owner+"|"+d.Node.Name()] {
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

// classOnly reports whether only classes can conform to the protocol q: it, or a protocol it refines,
// is constrained to AnyObject.
func (m *Module) classOnly(q string, seen map[string]bool) bool {
	if seen[q] {
		return false
	}
	seen[q] = true
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.classProtos[q] {
			return true
		}
		for _, parent := range mod.protoParents[q] {
			if m.classOnly(parent, seen) {
				return true
			}
		}
	}
	return false
}

// isTranspiledType reports a type declared in this module or a transpiled dependency.
func (m *Module) isTranspiledType(q string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if mod.typeKinds[q] != "" {
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
	"__C.NSJSONWritingOptions":                  true,
	"__C.NSJSONReadingOptions":                  true,
}

// sourceText returns a Swift source file's bytes, cached.
func (m *Module) sourceText(file string) []byte {
	if m.sources == nil {
		m.sources = map[string][]byte{}
	}
	if b, ok := m.sources[file]; ok {
		return b
	}
	b, _ := os.ReadFile(file)
	m.sources[file] = b
	return b
}

func (m *Module) nextHoist() int {
	m.noteHoist()
	m.hoists++
	return m.hoists
}

// goPackage is the name in the package clause of the module's files: main for a module with
// top-level code, which is a program's entry point, and the package's own name otherwise.
func (m *Module) goPackage() string {
	for _, f := range m.Files {
		for _, it := range f.Items {
			if it.Kind() == "top_level_code_decl" {
				return "main"
			}
		}
	}
	return m.Pkg
}

// protocolGoName reports whether g is the Go type of a protocol's interface, in this module or a
// transpiled dependency.
func (m *Module) protocolGoName(g string) bool {
	if g == "" {
		return false
	}
	for _, mod := range append([]*Module{m}, m.Deps...) {
		pfx := ""
		if mod.Pkg != m.Pkg {
			pfx = mod.Pkg + "."
		}
		rest, ok := strings.CutPrefix(g, pfx)
		if !ok || strings.Contains(rest, ".") {
			continue
		}
		for q, kind := range mod.typeKinds {
			if kind == "protocol" && mod.LocalTypeName(q) == rest {
				return true
			}
		}
	}
	return false
}

// settleWitnessNames gives a method that satisfies a protocol requirement the requirement's Go name.
// The two differ when the type overloads the method (`request(for: Account)` beside the witness
// `request(for: Session)`), since overloads are told apart by suffixes the requirement does not
// have; the overload that held the name takes a suffix for its parameter types instead.
func (m *Module) settleWitnessNames() {
	mods := append([]*Module{m}, m.Deps...)
	for _, pm := range mods {
		for proto, reqs := range pm.protoReqs {
			for _, q := range m.conformers[proto] {
				for _, r := range reqs {
					req := pm.decls[r.Str("usr")]
					if r.Kind() != "func_decl" || r.Bool("static") || req == nil {
						continue
					}
					var witness, holder *DeclInfo
					for _, mem := range m.membersOf(q) {
						if mem.Kind != "func_decl" || mem.Static || mem.ProtoExt || mem.NotWitness {
							continue
						}
						if mem.GoName == req.GoName {
							holder = mem
						}
						if funcSignatureKey(mem.Node) == funcSignatureKey(r) && m.sameParamTypes(mem.Node, r, pm) {
							witness = mem
						}
					}
					if witness == nil || witness.GoName == req.GoName || witness.FreeSelf() {
						continue
					}
					if holder != nil && holder != witness {
						holder.GoName += m.paramTypeSuffix(holder)
					}
					witness.GoName = req.GoName
				}
			}
		}
	}
}

// separateFieldsFromMethods renames a property that shares its Go name with a method of the same type, as a
// stored `kills` does beside the witness `kills(ofRun:)` of a protocol: Go has one namespace for a type's
// fields and methods. The property takes a trailing underscore.
func (m *Module) separateFieldsFromMethods() {
	for _, t := range m.sortedDecls() {
		switch t.Kind {
		case "struct_decl", "class_decl", "enum_decl":
		default:
			continue
		}
		members := m.membersOf(m.typeQName(t))
		methods := map[string]bool{}
		for _, mem := range members {
			if mem.Kind == "func_decl" && !mem.Static && !mem.FreeSelf() {
				methods[mem.GoName] = true
			}
		}
		for _, mem := range members {
			if mem.Kind == "var_decl" && !mem.Static && methods[mem.GoName] {
				mem.GoName += "_"
			}
		}
	}
}

// sameParamTypes reports whether fn's parameters have the types of the requirement req, declared in
// pm. A requirement's parameter typed by Self or an associated type matches any type.
func (m *Module) sameParamTypes(fn, req ast.Node, pm *Module) bool {
	fps, rps := fn.Child("params").List("params"), req.Child("params").List("params")
	if len(fps) != len(rps) {
		return false
	}
	for i := range fps {
		ft, rt := m.demangle(fps[i].Str("interface_type")), pm.demangle(rps[i].Str("interface_type"))
		if ft != rt && strings.Contains(rt, ".") && !strings.HasPrefix(rt, "A.") {
			return false
		}
	}
	return true
}

// retro appends to the file this module writes into a dependency's package.
func (m *Module) retro(pkg, format string, args ...any) {
	b := m.retroFiles[pkg]
	if b == nil {
		b = &strings.Builder{}
		m.retroFiles[pkg] = b
	}
	text := fmt.Sprintf(format, args...)
	b.WriteString(text)
	m.noteRetro(pkg, text)
}

var methodDecl = regexp.MustCompile(`(?m)^func \(\w* ?\*?(\w+)(?:\[[^\]]*\])?\) (\w+)\(`)

// emittedMethods lists the methods the Go files in dir declare on typeName, leaving out the files
// that earlier retroactive conformances wrote there.
func (m *Module) emittedMethods(dir, typeName string) map[string]bool {
	found := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), ".go") || strings.HasPrefix(ent.Name(), "retroactive_") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(dir, ent.Name()))
		for _, mt := range methodDecl.FindAllSubmatch(data, -1) {
			if string(mt[1]) == typeName {
				found[string(mt[2])] = true
			}
		}
	}
	return found
}

// namesLocalType reports whether the Go signature text names a type this module declares.
func (m *Module) namesLocalType(sig string) bool {
	for _, d := range m.decls {
		switch d.Kind {
		case "struct_decl", "class_decl", "enum_decl", "protocol":
			if regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(d.GoName) + `($|[^\w])`).MatchString(sig) {
				return true
			}
		}
	}
	return false
}
