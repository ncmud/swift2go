package emit

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/types"
)

type fileEmitter struct {
	m        *Module
	file     ast.File
	b        strings.Builder
	generics map[string]string
	tmp      int
	// chainStmt is set while an optional chain is emitted as a statement, whose `()?` value is unused.
	chainStmt bool
	// function context
	inInit        bool
	preamble      string                  // statements body() writes first, then clears
	topLevel      map[uintptr]bool        // statements directly in the current function body
	paramNames    map[string]bool         // parameters, which share the function body's Go scope
	doRet         *doReturn               // set while emitting a do body
	jumpDepth     int                     // loops and switches open around the statement being emitted
	loopDepth     int                     // loops open around the statement being emitted
	slots         []slot                  // dictionary elements taken inout by the call being emitted
	extraDecls    strings.Builder         // package-level declarations hoisted out of function bodies
	localGenerics map[string]localGeneric // generic local functions, by Swift name
	renames       map[string]string       // local names that refer to another Go variable here
	localGetters  map[string]bool         // local computed variables in scope, emitted as getter functions
	emittingFree  bool                    // the function being scoped is emitted as a free function
	metaValues    map[string]string       // generic parameter name -> the Go value that carries its metatype
	pendingOpens  int                     // scopes a statement opened that the enclosing block must close
	lvSubst       map[string]string       // lvalue overrides by node identity while a parent chain is emitted
	pinned        map[uintptr]bool        // operands to evaluate before a later operand's call, by node identity
	exprSubst     map[uintptr]string      // expressions already evaluated into a temporary, by node identity
	writes        map[uintptr]bool        // memoized mayWrite results, by node identity
	post          []string                // writebacks to emit after the current statement
	callee        *DeclInfo               // the local function whose arguments argStrings is rendering
	unitCallee    bool                    // the foreign callee takes a generic-result body closure
	// unitValueFirst marks a unitCallee whose first argument is a value, not a body: TaskLocal.withValue's.
	unitValueFirst bool
	typedErasure   map[uintptr]bool // erasures converted to the interface explicitly, for Go's type inference
	initRet        string           // what `return` yields inside an init: "self", "&self", or "true" in a class initializer body
	initFail       string           // what `return nil` yields inside a failable init
	initOwner      string           // the class whose initializer body is being emitted
	resultType     string
	// voidValue is set while emitting the branches of an if or switch expression of type Void, whose
	// branch values, such as a @discardableResult call's, are discarded.
	voidValue   bool
	optPtrs     map[string]bool // Go expressions that are exactly a swiftrt.Opt followed by .Ptr()
	throws      bool
	opaque      map[string]string // opaque_value_expr identity → Go expression
	where       string
	inoutParams map[string]bool
	typeParams  []string
	typeBounds  map[string]string // type parameter → Go constraint, when not `any`
	hoisted     []ast.Node
	erased      map[int]bool        // parameters paramList types as any, by index
	scopes      *valueScopes        // the file's local variables, resolved to their declarations
	bindOrigin  origin              // where the subject of the pattern being bound lives
	opened      map[string]ast.Node // opaque_value_expr identity → the existential it opens
	snapshots   map[uintptr]string  // receivers already emitted as a copy, by node
	storeInto   *types.Type         // the collection a mutating library call's arguments are stored into
	argSink     *sink               // where the call being emitted puts its arguments, when not lent
	partSink    sink                // where the parts of the aggregate partsOf is building go
	partsOf     uintptr             // the literal or memberwise call owned is emitting, by node
	keeps       int                 // owned is emitting a call whose result is stored or returned
	lent        map[uintptr]bool    // closures a library call holds only inside its result
	sliceLos    map[string]string   // local ArraySlice constant → the Go variable holding its start index
	sliceDry    bool                // sliceBase only reports whether a start index can be told
	localFuncs  map[string]ast.Node // functions declared inside the file's function bodies, by USR
	keptParams  map[string]bool     // type parameters callers name though no parameter or result does
	witnessOf   map[string]string   // type parameter → the witnessed protocol it conforms to, its Go bound being any
	registered  map[string]bool     // carriers of generic foreign types this file registers, by Go type and carrier
}

// blankStringLiterals is Go source with the contents of its string and rune literals and its
// comments blanked out, so that searching it finds only code.
func blankStringLiterals(src string) string {
	b := []byte(src)
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == '"' || c == '\'':
			for i++; i < len(b) && b[i] != c && b[i] != '\n'; i++ {
				if b[i] == '\\' && i+1 < len(b) {
					b[i] = ' '
					i++
				}
				b[i] = ' '
			}
		case c == '`':
			for i++; i < len(b) && b[i] != '`'; i++ {
				b[i] = ' '
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for ; i < len(b) && b[i] != '\n'; i++ {
				b[i] = ' '
			}
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			for ; i+1 < len(b) && (b[i] != '*' || b[i+1] != '/'); i++ {
				if b[i] != '\n' {
					b[i] = ' '
				}
			}
		}
	}
	return string(b)
}

func (e *fileEmitter) w(format string, args ...any) { fmt.Fprintf(&e.b, format, args...) }

func (e *fileEmitter) newTmp(prefix string) string {
	e.tmp++
	return fmt.Sprintf("%s%d", prefix, e.tmp)
}

func (e *fileEmitter) emitFile() string {
	e.opaque = map[string]string{}
	e.scopes = e.m.valueScopes()
	var body strings.Builder
	saved := e.b
	e.b = strings.Builder{}
	emitHoisted := func() {
		for len(e.hoisted) > 0 {
			h := e.hoisted[0]
			e.hoisted = e.hoisted[1:]
			e.guarded(h.Kind(), func() { e.topDecl(h, "") })
		}
	}
	for _, it := range e.file.Items {
		e.guarded(it.Kind(), func() { e.topDecl(it, "") })
		emitHoisted()
		ast.Walk(map[string]any(it), func(n ast.Node) {
			if n.Kind() == "declref_expr" {
				e.registerWitnesses(n.Child("decl"))
			}
		})
	}
	e.guarded("top_level_code_decl", e.topLevelCode)
	emitHoisted()
	body.WriteString(e.b.String())
	body.WriteString(e.extraDecls.String())
	e.b = saved
	src, mapped := e.m.applyMappings(sanitizeRuntimeNames(body.String()))
	var hdr strings.Builder
	fmt.Fprintf(&hdr, "// Code generated by swift2go from %s. DO NOT EDIT.\n\npackage %s\n\n", shortPath(e.file.Filename), e.m.goPackage())
	// A string literal can read like a package reference ("the whole world."): look for the
	// packages a file uses in its code alone.
	code := blankStringLiterals(src)
	var imports []string
	for _, spec := range mapped {
		alias, _, aliased := strings.Cut(spec, " ")
		if !aliased {
			alias = path.Base(strings.Trim(spec, `"`))
		}
		if qualifiedRef(alias).MatchString(code) {
			imports = append(imports, spec)
		}
	}
	if strings.Contains(code, "swiftrt.") {
		imports = append(imports, `"`+e.m.RTPath+`"`)
	}
	for pkg, path := range e.m.importedPackages(code) {
		imports = append(imports, importSpec(pkg, path))
	}
	if strings.Contains(code, "testing.T") {
		imports = append(imports, `"testing"`)
	}
	if strings.Contains(code, "fmt.Sprint(") {
		imports = append(imports, `"fmt"`)
	}
	if strings.Contains(code, "(math.M") {
		imports = append(imports, `"math"`)
	}
	if strings.Contains(code, " cmp.Ordered") {
		imports = append(imports, `"cmp"`)
	}
	if len(imports) > 0 {
		slices.Sort(imports)
		hdr.WriteString("import (\n")
		for _, i := range imports {
			hdr.WriteString("\t" + i + "\n")
		}
		hdr.WriteString(")\n\n")
	}
	return hdr.String() + src
}

func shortPath(p string) string {
	if i := strings.Index(p, "/Sources/"); i >= 0 {
		return p[i+1:]
	}
	if i := strings.Index(p, "/Tests/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (e *fileEmitter) info(n ast.Node) *DeclInfo { return e.m.decls[declKey(n)] }

func (e *fileEmitter) topDecl(n ast.Node, owner string) {
	if n.Implicit() || isMacroName(n.Name()) {
		return
	}
	switch n.Kind() {
	case "struct_decl", "class_decl":
		e.typeDecl(n)
	case "enum_decl":
		e.enumDecl(n)
	case "protocol":
		e.protocolDecl(n)
	case "extension_decl":
		e.extensionDecl(n)
	case "func_decl":
		e.funcDecl(n, nil)
	case "pattern_binding_decl":
		e.globalBinding(n)
	case "var_decl":
		if info := e.info(n); info != nil && !info.Computed && n.Bool("top_level_global") {
			e.w("var %s %s\n\n", info.GoName, e.m.GoType(e.m.typeOf(n.Str("interface_type")), nil))
		}
		if info := e.info(n); info != nil && info.Computed {
			e.computedVar(n, info, nil)
		}
		if info := e.info(n); info != nil && (info.Settable || info.Observed) {
			e.setterVar(n, info, nil)
		}
	case "import_decl", "top_level_code_decl", "typealias", "enum_case_decl":
	case "macro_decl", "infix_operator_decl", "prefix_operator_decl", "postfix_operator_decl", "precedence_group_decl":
		// Uses of a macro are expanded where they appear, and an operator's implementations are
		// functions; the declarations themselves have nothing to emit.
	default:
		e.m.Unsup("decl:"+n.Kind(), shortPath(e.file.Filename), "")
		e.w("// swift2go: unsupported declaration %s\n\n", n.Kind())
	}
}

// guarded runs emit and, if it panics, discards its partial output and counts the declaration as unsupported.
func (e *fileEmitter) guarded(what string, emit func()) {
	mark := e.b.Len()
	saved := *e
	defer func() {
		if r := recover(); r != nil {
			out := e.b.String()[:mark]
			hoisted := e.hoisted
			*e = saved
			e.hoisted = hoisted
			e.b = strings.Builder{}
			e.b.WriteString(out)
			e.m.Unsup("panic:"+what, shortPath(e.file.Filename)+": "+fmt.Sprint(r), "")
			e.w("// swift2go: transpiler panic in %s\n\n", what)
		}
	}()
	emit()
}

func (e *fileEmitter) members(n ast.Node, self *DeclInfo) {
	for _, mem := range n.List("members") {
		e.guarded(mem.Kind(), func() { e.member(n, mem, self) })
	}
}

func (e *fileEmitter) member(n, mem ast.Node, self *DeclInfo) {
	{
		if mem.Implicit() && mem.Kind() != "constructor_decl" && !(mem.Kind() == "enum_decl" && mem.Name() == "CodingKeys") || isMacroName(mem.Name()) {
			return
		}
		if mem.Kind() == "pattern_binding_decl" && isMacroName(patternName(mem.List("pattern_entries")[0].Child("pattern"))) {
			return
		}
		switch mem.Kind() {
		case "func_decl":
			e.funcDecl(mem, self)
			e.equalityHook(mem, self)
		case "constructor_decl":
			e.ctorDecl(mem, self, n)
		case "var_decl":
			info := e.info(mem)
			if info != nil && info.Weak {
				e.weakVar(mem, info, self)
				break
			}
			if info != nil && info.Computed {
				e.computedVar(mem, info, self)
			}
			if info != nil && (info.Settable || info.Observed) {
				e.setterVar(mem, info, self)
			}
		case "pattern_binding_decl":
			for _, pe := range mem.List("pattern_entries") {
				name := patternName(pe.Child("pattern"))
				vi := e.findVar(n, name)
				if vi != nil && vi.Static && !vi.Computed {
					init := pe.Child("processed_init")
					t := e.m.typeOf(pe.Child("pattern").Type())
					gt := e.m.GoType(t, e.generics)
					e.storedGlobal(vi, vi.GoName, gt, init)
				}
			}
		case "struct_decl", "class_decl":
			e.typeDecl(mem)
		case "enum_decl":
			e.enumDecl(mem)
		case "protocol":
			e.protocolDecl(mem)
		case "enum_case_decl", "typealias", "enum_element_decl":
		case "subscript_decl":
			e.subscriptDecl(mem, self)
		case "destructor_decl":
			e.deinitDecl(mem, self)
		default:
			e.m.Unsup("member:"+mem.Kind(), shortPath(e.file.Filename), "")
		}
	}
}

// equalityHook gives a type that declares its own `==` a SwiftEqual method, so the runtime's
// equality (`!=`, contains, firstIndex(of:), array ==) calls it as Swift does, and one that declares its
// own `<` a SwiftLess method, which swiftrt.Less calls for a type parameter that is only Comparable.
func (e *fileEmitter) equalityHook(fn ast.Node, self *DeclInfo) {
	info := e.info(fn)
	if self == nil || self.Node == nil || info == nil || (fn.Name() != "==" && fn.Name() != "<") || !info.Static {
		return
	}
	recv := ""
	switch self.Kind {
	case "struct_decl", "enum_decl":
		recv = self.GoName
	case "class_decl":
		recv = "*" + self.GoName
	default:
		return
	}
	if q := e.m.typeQName(self); len(e.m.TypeParams(q)) > 0 || e.m.IsPayloadEnum(q) {
		return
	}
	ps := fn.Child("params").List("params")
	if len(ps) != 2 {
		return
	}
	for _, p := range ps {
		if e.m.GoType(e.m.typeOf(p.Str("interface_type")), nil) != recv {
			return
		}
	}
	if fn.Name() == "<" {
		e.w("// SwiftLess is %s's own <.\nfunc (self %s) SwiftLess(other any) bool {\n\to, ok := other.(%s)\n\treturn ok && %s(self, o)\n}\n\n", self.GoName, recv, recv, info.GoName)
		return
	}
	e.w("// SwiftEqual is %s's own ==.\nfunc (self %s) SwiftEqual(other any) bool {\n\to, ok := other.(%s)\n\treturn ok && %s(self, o)\n}\n\n", self.GoName, recv, recv, info.GoName)
}

func (e *fileEmitter) findVar(typeNode ast.Node, name string) *DeclInfo {
	for _, mem := range typeNode.List("members") {
		if mem.Kind() == "var_decl" && mem.Name() == name {
			return e.info(mem)
		}
	}
	return nil
}

// unconstrainedFunc reports whether the Go source declares the function name with type parameters that are
// all `any`, so a method of the generic type can call it with the type's own parameters.
func unconstrainedFunc(src, name string) bool {
	i := strings.LastIndex(src, "func "+name+"[")
	if i < 0 {
		return false
	}
	params, _, ok := strings.Cut(src[i+len("func "+name+"["):], "]")
	if !ok {
		return false
	}
	for _, p := range strings.Split(params, ",") {
		if !strings.HasSuffix(strings.TrimSpace(p), " any") {
			return false
		}
	}
	return true
}

// wrappedProperties names the properties of type n that a property wrapper attribute wraps.
func wrappedProperties(n ast.Node) map[string]bool {
	out := map[string]bool{}
	for _, mem := range n.List("members") {
		if mem.Kind() != "var_decl" {
			continue
		}
		for _, a := range mem.List("attrs") {
			if a.Kind() == "custom_attr" {
				out[mem.Name()] = true
			}
		}
	}
	return out
}

func (e *fileEmitter) typeDecl(n ast.Node) {
	info := e.info(n)
	if info == nil {
		return
	}
	q := e.m.typeQName(info)
	saveG := e.generics
	e.generics = e.typeGenerics(q)
	defer func() { e.generics = saveG }()
	e.w("type %s%s struct {\n", info.GoName, e.m.typeParamDecl(q))
	fieldsStart := e.b.Len()
	if sup := e.m.Superclass(info.GoName); sup != "" {
		e.w("\t%s\n", sup)
	}
	if n.Kind() == "class_decl" && e.m.HasSubclass(info.GoName) && e.m.Superclass(info.GoName) == "" {
		e.w("\t// SwiftSub is the most-derived object when this value is embedded in a subclass.\n\tSwiftSub any\n")
	}
	keys := codingKeys(n)
	var fieldKeys, keyedEncodes []string
	wrapped := wrappedProperties(n)
	for _, mem := range n.List("members") {
		if mem.Kind() != "var_decl" {
			continue
		}
		vi := e.info(mem)
		if vi != nil && vi.Weak {
			e.w("\t%s swiftrt.Weak[%s]\n", weakField(vi), e.m.GoType(e.m.typeOf(mem.Str("interface_type")), e.generics))
			continue
		}
		if vi == nil || vi.Static || vi.Computed {
			continue
		}
		if conformsTo(n, "s:s9OptionSetP") {
			e.w("\t%s %s\n", vi.GoName, e.m.GoType(e.m.typeOf(mem.Str("interface_type")), e.generics))
			continue
		}
		tag := "swift:\"" + mem.Name() + "\""
		// A property wrapper's storage, `_name`, codes under the wrapped property's name.
		name := mem.Name()
		if w, ok := strings.CutPrefix(name, "_"); ok && mem.Bool("implicit") && wrapped[w] {
			name = w
		}
		if keys != nil {
			if key, ok := keys[name]; !ok {
				tag += " key:\"-\""
			} else {
				fieldKeys = append(fieldKeys, key)
				if key != mem.Name() {
					tag += " key:\"" + key + "\""
				}
			}
		} else if name != mem.Name() {
			tag += " key:\"" + name + "\""
		}
		if t := e.m.typeOf(mem.Str("interface_type")); t != nil && t.Kind == types.Named && len(t.Args) == 1 && e.m.keyedOverloads()[t.Name].encode != "" && !strings.Contains(tag, `key:"-"`) {
			keyedEncodes = append(keyedEncodes, fmt.Sprintf("\tcase %q:\n\t\t%s[string, %s](c, self.%s, key)\n\t\treturn true\n",
				vi.GoName, e.m.keyedOverloads()[t.Name].encode, e.m.GoType(t.Args[0], e.generics), vi.GoName))
		}
		if name != mem.Name() {
			if t := e.m.typeOf(mem.Str("interface_type")); t != nil && t.Kind == types.Named && e.m.keyedOverloads()[t.Name].decode != "" {
				// Its wrapper's own keyed decode overload, which Go cannot call for every wrapped type, reads a
				// missing key; the runtime keeps the property's initial value then.
				tag += " coding:\"ifPresent\""
			}
		}
		e.w("\t%s %s `%s`\n", vi.GoName, e.m.GoType(e.m.typeOf(mem.Str("interface_type")), e.generics), tag)
	}
	if n.Kind() == "class_decl" && e.b.Len() == fieldsStart {
		// Go may give every zero-size allocation the same address; each instance of a class needs its own
		// identity for ===, ObjectIdentifier and weak references.
		e.w("\t_ byte\n")
	}
	e.w("}\n\n")
	recvT := info.GoName + e.m.typeParamUse(q)
	if len(keyedEncodes) > 0 {
		// A synthesized encode(to:) calls the module's own overload of KeyedEncodingContainer.encode(_:forKey:)
		// for a property of the type the overload takes, which Swift resolves statically.
		e.w("func (self %s) SwiftEncodeKeyed(c *swiftrt.KeyedEncodingContainer, field, key string) bool {\n\tswitch field {\n%s\t}\n\treturn false\n}\n\n", recvT, strings.Join(keyedEncodes, ""))
	}
	if order := codingKeyOrder(n); len(order) > 1 && !slices.Equal(order, fieldKeys) {
		// A synthesized encode(to:) writes the keys in the order the CodingKeys cases are declared.
		e.w("func (%s) SwiftCodingKeyOrder() []string { return %#v }\n\n", recvT, order)
	}
	if n.Kind() == "class_decl" {
		e.w("// SwiftClass marks %s as a reference type.\nfunc (*%s) SwiftClass() {}\n\n", info.GoName, recvT)
		e.w("func (*%s) SwiftTypeName() string { return %q }\n\n", recvT, q)
		e.descriptionMarkers(q, "*"+recvT)
	} else if !conformsTo(n, "s:s9OptionSetP") {
		e.w("func (%s) SwiftTypeName() string { return %q }\n\n", recvT, q)
		e.descriptionMarkers(q, recvT)
	} else if e.conformsToCodable(n) {
		e.w("// SwiftRawCodable codes %s as its raw value, as Swift codes a RawRepresentable type.\nfunc (%s) SwiftRawCodable() {}\n\n", info.GoName, recvT)
	}
	if n.Kind() == "class_decl" && e.m.HasSubclass(info.GoName) && e.m.Superclass(info.GoName) == "" {
		e.w("// SwiftDynamic returns the most-derived object, for downcasts and identity.\nfunc (self *%s) SwiftDynamic() any {\n\tif self != nil && self.SwiftSub != nil {\n\t\treturn self.SwiftSub\n\t}\n\treturn self\n}\n\n", recvT)
	}
	for _, sup := range e.m.ancestors(info.GoName) {
		if pkg, _, ok := strings.Cut(sup, "."); ok {
			if dep := e.m.depByPkg(pkg); dep != nil {
				e.m.importPkg(dep)
			}
		}
		e.w("// %s upcasts to the superclass.\nfunc (self *%s) %s() *%s {\n\tif self == nil {\n\t\treturn nil\n\t}\n\treturn &self.%s\n}\n\n", upcastMethod(sup), recvT, upcastMethod(sup), sup, embeddedField(sup))
	}
	if sups := e.m.ancestors(info.GoName); len(sups) > 0 {
		var cases strings.Builder
		for i, sup := range sups {
			fmt.Fprintf(&cases, "\tcase %d:\n\t\treturn &self.%s\n", i, embeddedField(sup))
		}
		e.w("// SwiftBase returns superclass part i, nearest first, or nil past the last, for casts to a superclass.\nfunc (self *%s) SwiftBase(i int) any {\n\tswitch i {\n%s\t}\n\treturn nil\n}\n\n", recvT, cases.String())
	}
	e.members(n, info)
	e.codingKeyOmissionDefaults(n, recvT, keys)
	e.protocolForwarders(n, info)
}

// codingKeyOmissionDefaults gives a type whose CodingKeys leave out stored properties a
// SwiftDecodeDefaults method that sets those properties to their initial values, which the
// runtime's synthesized decoding keeps, as Swift's synthesized init(from:) does.
func (e *fileEmitter) codingKeyOmissionDefaults(n ast.Node, recvT string, keys map[string]string) {
	if keys == nil {
		return
	}
	var inits []string
	for _, mem := range n.List("members") {
		if mem.Kind() != "pattern_binding_decl" {
			continue
		}
		for _, pe := range mem.List("pattern_entries") {
			name := patternName(pe.Child("pattern"))
			if _, coded := keys[name]; coded {
				continue
			}
			vi := e.findVar(n, name)
			if vi != nil && !vi.Static && !vi.Computed && pe.Child("processed_init") != nil {
				inits = append(inits, fmt.Sprintf("self.%s = %s\n", vi.GoName, e.owned(pe.Child("processed_init"), sinkStore)))
			}
		}
	}
	if len(inits) == 0 {
		return
	}
	e.w("func (self *%s) SwiftDecodeDefaults() {\n%s}\n\n", recvT, strings.Join(inits, ""))
}

// protocolForwarders gives a conforming type the protocol-extension default methods it does not define.
func (e *fileEmitter) protocolForwarders(n ast.Node, self *DeclInfo) {
	q := e.m.typeQName(self)
	e.protocolForwardersFor(n, q)
	e.conformanceRegistrations(n, q)
	e.optionalWitnessRegistrations(n, q)
}

func (e *fileEmitter) protocolForwardersFor(n ast.Node, q string) {
	if e.m.IsPayloadEnum(q) {
		return
	}
	defined := map[string]bool{}
	for _, mem := range n.List("members") {
		if mem.Kind() == "func_decl" {
			defined[funcSignatureKey(mem)] = true
		}
	}
	// The type's own implementation may sit in an extension, in this file or another.
	for _, om := range append([]*Module{e.m}, e.m.Deps...) {
		for _, mem := range om.membersOf(q) {
			if mem.Kind == "func_decl" && !mem.ProtoExt && mem.Node != nil {
				defined[funcSignatureKey(mem.Node)] = true
			}
		}
	}
	var conformed []string
	for _, c := range n.Child("inherits").List("conformances") {
		conformed = append(conformed, e.m.demangle(c.Str("protocol")))
	}
	// overridden reports whether a protocol that refines proto, and that the type also conforms to,
	// supplies its own default for the same requirement: Swift picks the more specific one.
	overridden := func(proto string, impl ast.Node) bool {
		for _, other := range conformed {
			if other == proto || !e.m.refines(other, proto) {
				continue
			}
			for _, pm := range append([]*Module{e.m}, e.m.Deps...) {
				for _, d := range pm.decls {
					if d.ProtoExt && d.Owner == other && d.Kind == "func_decl" && funcSignatureKey(d.Node) == funcSignatureKey(impl) && e.m.extensionApplies(d.Node, q, other, pm) {
						return true
					}
				}
			}
		}
		return false
	}
	for _, proto := range conformed {
		for _, pm := range append([]*Module{e.m}, e.m.Deps...) {
			if !strings.HasPrefix(proto, pm.Name+".") {
				continue
			}
			pfx := ""
			if pm.Pkg != e.m.Pkg {
				pfx = pm.Pkg + "."
			}
			e.propertyGetters(n, nil, q, proto, conformed, pm, pfx)
			e.staticForwarders(q, proto, conformed, pm, pfx)
			e.initForwarders(q, proto, pm, pfx)
			for _, info := range pm.sortedDecls() {
				if !info.ProtoExt || info.Owner != proto || info.Kind != "func_decl" || defined[funcSignatureKey(info.Node)] {
					continue
				}
				if !e.m.extensionApplies(info.Node, q, proto, pm) {
					continue
				}
				req := e.m.inheritedRequirementName(proto, info.Node)
				if req == "" || overridden(proto, info.Node) {
					continue
				}
				erased, erasedResult := e.m.requirementErasure(proto, info.Node)
				e.erased = erased
				params, args := e.paramList(info.Node)
				e.erased = nil
				res := e.resultGo(info.Node)
				ptr := ""
				if erasedResult {
					res = "any"
					if e.m.inlineOptResult(info, pm) {
						// The requirement's associated type holds the Optional in its pointer form.
						ptr = ".Ptr()"
					}
				}
				if len(erased) > 0 {
					// The default takes the conformer's own type where the requirement takes an associated type.
					parts := strings.Split(args, ", ")
					for i, p := range info.Node.Child("params").List("params") {
						if erased[i] && i < len(parts) {
							parts[i] = "swiftrt.MustAs[" + e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics) + "](" + parts[i] + ")"
						}
					}
					args = strings.Join(parts, ", ")
				}
				recv := e.m.GoType(&types.Type{Kind: types.Named, Name: q}, nil)
				if info.Mutating && !strings.HasPrefix(recv, "*") && n.Kind() == "struct_decl" {
					recv = "*" + recv
				}
				ret := "return "
				if res == "" {
					ret = ""
				}
				e.w("func (self %s) %s(%s) %s { %s%s%s%s(self%s)%s }\n\n", recv, req, params, res, ret, pfx, info.GoName, e.conformerAssocTypeArgs(info, q, pm), prefixComma(args), ptr)
			}
		}
	}
}

// extensionApplies reports whether the where clause of the protocol-extension member fn holds for the
// conformer q, so a default from `extension P where Assoc == X` reaches only the conformers that bind
// Assoc to X. A requirement it cannot decide counts as holding.
func (m *Module) extensionApplies(fn ast.Node, q, proto string, pm *Module) bool {
	sig := fn.Child("generic_signature")
	params := sig.Strings("generic_params")
	if len(params) == 0 {
		return true
	}
	self := m.demangle(params[0])
	for _, r := range sig.List("generic_reqs") {
		first := m.demangle(r.Str("first_type"))
		var subject *types.Type
		switch {
		case first == self:
			subject = &types.Type{Kind: types.Named, Name: q}
		case strings.HasPrefix(first, self+"."+proto+"."):
			assoc := strings.TrimPrefix(first, self+"."+proto+".")
			subject = m.assocWitness(q, proto, assoc, pm)
		}
		if subject == nil {
			continue
		}
		switch r.Str("kind") {
		case "same_type":
			if want := m.typeOf(r.Str("second_type")); want.Kind != types.Unknown && want.String() != subject.String() {
				return false
			}
		case "conforms_to":
			if p := m.demangle(r.Str("protocol")); subject.Kind == types.Named && p != proto && !m.conformsTo(subject.Name, p) {
				return false
			}
		}
	}
	return true
}

// conformsTo reports whether the qualified Swift type q declares a conformance to protocol p, in this
// module or a dependency.
// descriptionMarkers marks a type whose `description` or `debugDescription` is a member of its own
// rather than its CustomStringConvertible or CustomDebugStringConvertible witness: Swift's
// String(describing:) ignores such a member, and the runtime's Str reads the marker.
func (e *fileEmitter) descriptionMarkers(q, recvT string) {
	has := func(name string) bool {
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			for _, d := range mod.sortedDecls() {
				if d.Owner == q && !d.Static && d.Node != nil && d.Node.Name() == name && (d.Kind == "var_decl" || d.Kind == "func_decl" && len(d.Node.Child("params").List("params")) == 0) {
					return true
				}
			}
		}
		return false
	}
	if has("description") && !e.m.conformsTo(q, "Swift.CustomStringConvertible") && !e.m.conformsTo(q, "Swift.LosslessStringConvertible") {
		e.w("// SwiftOwnDescription marks description as a member, not a CustomStringConvertible witness.\nfunc (%s) SwiftOwnDescription() {}\n\n", recvT)
	}
	if has("debugDescription") && !e.m.conformsTo(q, "Swift.CustomDebugStringConvertible") {
		e.w("// SwiftOwnDebugDescription marks debugDescription as a member, not a CustomDebugStringConvertible witness.\nfunc (%s) SwiftOwnDebugDescription() {}\n\n", recvT)
	}
}

func (m *Module) conformsTo(q, p string) bool {
	for _, mod := range append([]*Module{m}, m.Deps...) {
		if slices.Contains(mod.conformers[p], q) {
			return true
		}
	}
	return false
}

// assocWitness is the type conformer q binds to associated type assoc of protocol proto: a member type
// or typealias of that name, or else the type q's implementation of a requirement uses where the
// requirement names the associated type. It is nil when neither says.
func (m *Module) assocWitness(q, proto, assoc string, pm *Module) *types.Type {
	mods := append([]*Module{m}, m.Deps...)
	for _, mod := range mods {
		if mangled, ok := mod.typeAliases[q+"."+assoc]; ok {
			return mod.typeOf(mangled)
		}
		if _, ok := mod.typeKinds[q+"."+assoc]; ok {
			return &types.Type{Kind: types.Named, Name: q + "." + assoc}
		}
	}
	isAssoc := func(mangled string) bool {
		return strings.HasSuffix(pm.demangle(mangled), "."+proto+"."+assoc)
	}
	for _, r := range pm.protoReqs[proto] {
		if r.Kind() != "func_decl" {
			continue
		}
		key := funcSignatureKey(r)
		for _, mod := range mods {
			for _, d := range mod.membersOf(q) {
				if d.Kind != "func_decl" || d.ProtoExt || funcSignatureKey(d.Node) != key {
					continue
				}
				if isAssoc(r.Str("result")) {
					return mod.typeOf(d.Node.Str("result"))
				}
				rps, dps := r.Child("params").List("params"), d.Node.Child("params").List("params")
				for i, rp := range rps {
					if i < len(dps) && isAssoc(rp.Str("interface_type")) {
						return mod.typeOf(dps[i].Str("interface_type"))
					}
				}
			}
		}
	}
	return nil
}

// funcSignatureKey is a function's base name and argument labels, which tell apart overloads such as
// `f(_:write:)` and `f(_:log:)`.
func funcSignatureKey(n ast.Node) string {
	return n.Name() + "(" + strings.Join(paramLabels(n), ":") + ")"
}

func prefixComma(s string) string {
	if s == "" {
		return ""
	}
	return ", " + s
}

// sortedDecls returns the module's declarations ordered by key. Callers must not modify the slice.
func (m *Module) sortedDecls() []*DeclInfo {
	if m.sorted != nil {
		return m.sorted
	}
	keys := slices.Sorted(maps.Keys(m.decls))
	m.sorted = make([]*DeclInfo, len(keys))
	for i, k := range keys {
		m.sorted[i] = m.decls[k]
	}
	return m.sorted
}

// inheritedRequirementName is the Go name of the requirement that a default in an extension of proto
// implements, declared by proto or by a protocol it inherits from.
func (m *Module) inheritedRequirementName(proto string, impl ast.Node) string {
	seen := map[string]bool{}
	var find func(p string) string
	find = func(p string) string {
		if seen[p] {
			return ""
		}
		seen[p] = true
		for _, mod := range append([]*Module{m}, m.Deps...) {
			if name := mod.requirementName(p, impl); name != "" {
				return name
			}
			for _, parent := range mod.protoParents[p] {
				if name := find(parent); name != "" {
					return name
				}
			}
		}
		return ""
	}
	return find(proto)
}

// refines reports whether the local protocol sub inherits from proto, directly or indirectly.
func (m *Module) refines(sub, proto string) bool {
	seen := map[string]bool{}
	var walk func(p string) bool
	walk = func(p string) bool {
		if seen[p] {
			return false
		}
		seen[p] = true
		for _, mod := range append([]*Module{m}, m.Deps...) {
			for _, parent := range mod.protoParents[p] {
				if parent == proto || walk(parent) {
					return true
				}
			}
		}
		return false
	}
	return walk(sub)
}

func (m *Module) requirementName(proto string, impl ast.Node) string {
	for _, r := range m.protoReqs[proto] {
		if r.Kind() == "func_decl" && funcSignatureKey(r) == funcSignatureKey(impl) {
			if info := m.decls[r.Str("usr")]; info != nil {
				return info.GoName
			}
		}
	}
	return ""
}

func (e *fileEmitter) enumDecl(n ast.Node) {
	info := e.info(n)
	if info == nil {
		return
	}
	var cases []ast.Node
	payload := false
	hasCaseDecls := false
	for _, mem := range n.List("members") {
		if mem.Kind() == "enum_case_decl" {
			hasCaseDecls = true
		}
	}
	for _, mem := range n.List("members") {
		if mem.Kind() == "enum_element_decl" && !hasCaseDecls {
			cases = append(cases, mem)
			continue
		}
		if mem.Kind() == "enum_case_decl" {
			for _, el := range mem.List("elements") {
				cases = append(cases, el)
				if el.Child("params") != nil && len(el.Child("params").List("params")) > 0 {
					payload = true
				}
			}
		}
	}
	if payload {
		saveG := e.generics
		e.generics = e.typeGenerics(e.m.typeQName(info))
		defer func() { e.generics = saveG }()
		e.payloadEnum(n, info, cases)
	} else if len(cases) == 0 && len(n.Child("inherits").List("conformances")) > 0 {
		// An enum without cases that conforms to a protocol is used as a type, through its statics:
		// its zero value carries them.
		e.w("type %s struct{}\n\n", info.GoName)
		e.w("func (%s) SwiftTypeName() string { return %q }\n\n", info.GoName, e.m.qualify(info.Owner, n.Name()))
	} else if len(cases) > 0 {
		e.w("type %s int\n\nconst (\n", info.GoName)
		for i, c := range cases {
			ci := e.info(c)
			if i == 0 {
				e.w("\t%s %s = iota\n", ci.GoName, info.GoName)
			} else {
				e.w("\t%s\n", ci.GoName)
			}
		}
		e.w(")\n\n")
		var names []string
		for _, c := range cases {
			names = append(names, strconv.Quote(c.Name()))
		}
		e.w("func (self %s) SwiftCase() (string, []string) {\n\treturn [...]string{%s}[self], nil\n}\n\n", info.GoName, strings.Join(names, ", "))
		e.w("func (%s) SwiftTypeName() string { return %q }\n\n", info.GoName, e.m.qualify(info.Owner, n.Name()))
		e.descriptionMarkers(e.m.typeQName(info), info.GoName)
		e.rawValue(n, info, cases)
		if !hasRawType(n) && e.synthesizesCodable(n) {
			e.w("func init() { swiftrt.RegisterCaseEnum[%s](%s) }\n\n", info.GoName, strings.Join(names, ", "))
		}
		if conformsTo(n, "s:s9CodingKeyP") {
			e.w("func (self %s) StringValue() string {\n\tswitch self {\n", info.GoName)
			for _, c := range cases {
				key := c.Name()
				if v := c.Child("raw_value_expr"); v != nil && v.Kind() == "string_literal_expr" {
					key = v.Str("value")
				}
				e.w("\tcase %s:\n\t\treturn %q\n", e.info(c).GoName, key)
			}
			e.w("\t}\n\treturn \"\"\n}\n\n")
		}
		if conformsTo(n, "s:s12CaseIterableP") && !hasExplicitMember(n, "allCases") {
			var names []string
			for _, c := range cases {
				names = append(names, e.info(c).GoName)
			}
			e.w("func %sAllCases() []%s { return []%s{%s} }\n\n", info.GoName, info.GoName, info.GoName, strings.Join(names, ", "))
			e.w("func init() { swiftrt.RegisterAllCases(%sAllCases) }\n\n", info.GoName)
		}
	}
	e.members(n, info)
	if len(cases) > 0 || len(n.Child("inherits").List("conformances")) > 0 {
		e.protocolForwarders(n, info)
	}
}

func (e *fileEmitter) rawValue(n ast.Node, info *DeclInfo, cases []ast.Node) {
	var ctor *DeclInfo
	rawGo := ""
	for _, mem := range n.List("members") {
		if mem.Kind() == "constructor_decl" && mem.Implicit() {
			ps := mem.Child("params").List("params")
			if len(ps) == 1 && ps[0].Name() == "rawValue" {
				ctor = e.info(mem)
				rawGo = e.m.GoType(e.m.typeOf(ps[0].Str("interface_type")), nil)
			}
		}
	}
	var raw []string
	next := 0
	for _, c := range cases {
		if v := c.Child("raw_value_expr"); v != nil {
			raw = append(raw, e.expr(v))
			if i, err := strconv.Atoi(e.expr(v)); err == nil {
				next = i + 1
			}
			continue
		}
		switch rawGo {
		case "string":
			raw = append(raw, strconv.Quote(c.Name()))
		case "":
		default:
			raw = append(raw, strconv.Itoa(next))
			next++
		}
	}
	if len(raw) != len(cases) || len(raw) == 0 {
		return
	}
	if rawGo == "" {
		rawGo = e.exprGoType(cases[0].Child("raw_value_expr"))
	}
	e.w("func (self %s) RawValue() %s {\n\tswitch self {\n", info.GoName, rawGo)
	for i, c := range cases {
		e.w("\tcase %s:\n\t\treturn %s\n", e.info(c).GoName, raw[i])
	}
	e.w("\t}\n\tpanic(\"unreachable\")\n}\n\n")
	if ctor != nil {
		e.w("func %s(rawValue %s) *%s {\n\tswitch rawValue {\n", ctor.GoName, rawGo, info.GoName)
		for i, c := range cases {
			e.w("\tcase %s:\n\t\tv := %s\n\t\treturn &v\n", raw[i], e.info(c).GoName)
		}
		e.w("\t}\n\treturn nil\n}\n\n")
		e.w("func init() { swiftrt.RegisterRawInit(%s) }\n\n", ctor.GoName)
	}
}

func (e *fileEmitter) payloadEnum(n ast.Node, info *DeclInfo, cases []ast.Node) {
	q := e.m.typeQName(info)
	decl, use := e.m.typeParamDecl(q), e.m.typeParamUse(q)
	g := e.typeGenerics(q)
	e.w("type %s%s interface{ is%s() }\n\n", info.GoName, decl, info.GoName)
	describe := ""
	for _, d := range e.m.decls {
		if d.Owner == q && d.Kind == "var_decl" && !d.Static && d.OnPayloadEnum && d.Node.Name() == "description" {
			describe = d.GoName
		}
	}
	for _, c := range cases {
		ci := e.info(c)
		e.w("type %s%s struct {\n", ci.GoName, decl)
		for i, p := range c.Child("params").List("params") {
			e.w("\tF%d %s\n", i, e.m.GoType(e.m.typeOf(p.Str("interface_type")), g))
		}
		e.w("}\n\nfunc (%s%s) is%s() {}\n\n", ci.GoName, use, info.GoName)
		var labels []string
		for _, p := range c.Child("params").List("params") {
			labels = append(labels, strconv.Quote(p.Name()))
		}
		ls := "nil"
		if slices.ContainsFunc(labels, func(l string) bool { return l != `""` }) {
			ls = "[]string{" + strings.Join(labels, ", ") + "}"
		}
		e.w("func (%s%s) SwiftCase() (string, []string) { return %q, %s }\n\n", ci.GoName, use, c.Name(), ls)
		e.w("func (%s%s) SwiftTypeName() string { return %q }\n\n", ci.GoName, use, q)
		if describe != "" {
			e.w("func (self %s%s) Description() string { return %s%s(self) }\n\n", ci.GoName, use, describe, use)
		}
	}
	if decl == "" && e.synthesizesCodable(n) {
		var cs []string
		for _, c := range cases {
			ci := e.info(c)
			parts := []string{strconv.Quote(c.Name())}
			for i, p := range c.Child("params").List("params") {
				label := p.Name()
				if label == "" {
					label = "_" + strconv.Itoa(i)
				}
				parts = append(parts, strconv.Quote(label))
			}
			cs = append(cs, "swiftrt.CaseOf["+ci.GoName+"]("+strings.Join(parts, ", ")+")")
		}
		e.w("func init() {\nswiftrt.RegisterPayloadEnum[%s](\n%s,\n)\n}\n\n", info.GoName, strings.Join(cs, ",\n"))
	}
}

// hasRawType reports whether enum n has a raw type, which its Codable conformance encodes instead
// of the case name.
func hasRawType(n ast.Node) bool {
	for _, mem := range n.List("members") {
		if mem.Kind() == "constructor_decl" && mem.Implicit() {
			if ps := mem.Child("params").List("params"); len(ps) == 1 && ps[0].Name() == "rawValue" {
				return true
			}
		}
	}
	return false
}

// conformsToCodable reports whether n declares a conformance to Encodable or Decodable.
func (e *fileEmitter) conformsToCodable(n ast.Node) bool {
	for _, c := range n.Child("inherits").List("conformances") {
		switch e.m.demangle(c.Str("protocol")) {
		case "Swift.Encodable", "Swift.Decodable":
			return true
		}
	}
	return false
}

// synthesizesCodable reports whether Swift synthesizes the enum's Codable conformance: it conforms
// to Encodable or Decodable and writes neither encode(to:) nor init(from:).
func (e *fileEmitter) synthesizesCodable(n ast.Node) bool {
	codable := false
	for _, c := range n.Child("inherits").List("conformances") {
		switch e.m.demangle(c.Str("protocol")) {
		case "Swift.Encodable", "Swift.Decodable":
			codable = true
		}
	}
	if !codable {
		return false
	}
	for _, mem := range n.List("members") {
		if mem.Implicit() {
			continue
		}
		if mem.Kind() == "func_decl" && mem.Name() == "encode" || mem.Kind() == "constructor_decl" && len(mem.Child("params").List("params")) == 1 && mem.Child("params").List("params")[0].Name() == "decoder" {
			return false
		}
	}
	return true
}

func (e *fileEmitter) protocolDecl(n ast.Node) {
	info := e.info(n)
	e.w("type %s interface {\n", info.GoName)
	for _, p := range n.Child("inherits").Strings("protocols") {
		pn := e.m.demangle(p)
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			if mod.typeKinds[pn] == "protocol" {
				g := mod.LocalTypeName(pn)
				if mod.Pkg != e.m.Pkg {
					e.m.importPkg(mod)
					g = mod.Pkg + "." + g
				}
				e.w("\t%s\n", g)
			}
		}
	}
	for _, mem := range n.List("members") {
		switch mem.Kind() {
		case "func_decl":
			mi := e.info(mem)
			params, _ := e.paramList(mem)
			name := mi.GoName
			if mi.Static {
				name = staticReqName(mi)
			}
			e.w("\t%s(%s) %s\n", name, params, e.resultGo(mem))
		case "constructor_decl":
			// An initializer requirement is reached through a value of the conforming type, as the
			// static requirements are, and returns the new value as the protocol.
			params, _ := e.paramList(mem)
			e.w("\t%s(%s) %s\n", staticInitName(mem), params, info.GoName)
		case "var_decl":
			mi := e.info(mem)
			if mi.Static {
				e.w("\t%s() %s\n", staticReqName(mi), e.m.GoType(e.m.typeOf(mem.Str("interface_type")), nil))
				continue
			}
			e.w("\tGet%s() %s\n", mi.GoName, e.m.GoType(e.m.typeOf(mem.Str("interface_type")), nil))
			if mem.Str("writeImpl") == "setter" {
				e.w("\tSet%s(%s)\n", mi.GoName, e.m.GoType(e.m.typeOf(mem.Str("interface_type")), nil))
			}
		}
	}
	e.w("}\n\n")
}

func (e *fileEmitter) extensionDecl(n ast.Node) {
	t := e.m.typeOf(n.Str("extended_type"))
	q := t.Name
	var self *DeclInfo
	for _, info := range e.m.decls {
		if (info.Kind == "struct_decl" || info.Kind == "class_decl" || info.Kind == "enum_decl") && e.m.typeQName(info) == q {
			self = info
		}
	}
	if self == nil {
		self = &DeclInfo{Kind: "foreign", GoName: e.m.GoType(t, nil), Owner: q}
		if t.Kind == types.Existential {
			self.Kind = "protocol_ext"
		}
	}
	saveG := e.generics
	if self.Kind != "foreign" && self.Kind != "protocol_ext" {
		e.generics = e.typeGenerics(q)
	}
	e.members(n, self)
	e.generics = saveG
	if self.Kind != "foreign" && self.Kind != "protocol_ext" && len(n.Child("inherits").List("conformances")) > 0 {
		e.protocolForwardersFor(n, q)
	}
	if self.Kind == "foreign" {
		e.retroactiveConformances(n, q)
		if q == "Swift.Optional" {
			e.optionalWitnessTypes(n)
		} else {
			e.foreignWitnessTypes(n, q)
		}
	}
	if self.Kind != "protocol_ext" {
		e.conformanceRegistrations(n, q)
	}
	if self.Kind != "foreign" && self.Kind != "protocol_ext" {
		e.optionalWitnessRegistrations(n, q)
	}
}

// nestedInParamTypes reports whether the generic parameter occurs in one of fn's parameter types
// other than as the whole type.
func nestedInParamTypes(m *Module, fn ast.Node, param string) bool {
	re := regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(param) + `($|[^\w])`)
	for _, p := range fn.Child("params").List("params") {
		if dem := m.demangle(p.Str("interface_type")); dem != param && re.MatchString(dem) {
			return true
		}
	}
	return false
}

// retroactiveConformances makes a type of a transpiled dependency satisfy the local protocols that
// an extension here conforms it to (`extension Row: Scripted {}`). Go lets only the declaring
// package add methods to a type, so the methods are written into the dependency's package, in a
// file named after this module. A requirement the type already meets with a stored or computed
// property gets a getter there; one that this extension implements gets a method that calls a
// hook variable, which this module's init sets to its implementation, since the dependency cannot
// import this package.
func (e *fileEmitter) retroactiveConformances(n ast.Node, q string) {
	var dep *Module
	for _, d := range e.m.Deps {
		if k := d.typeKinds[q]; k == "struct_decl" || k == "class_decl" {
			dep = d
		}
	}
	if dep == nil || dep.ImportPath == "" || e.m.outDir == "" {
		return
	}
	typeName := dep.LocalTypeName(q)
	recv := typeName
	if dep.typeKinds[q] == "class_decl" {
		recv = "*" + typeName
	}
	existing := dep.emittedMethods(filepath.Join(filepath.Dir(e.m.outDir), dep.Pkg), typeName)
	for _, c := range n.Child("inherits").List("conformances") {
		proto := e.m.demangle(c.Str("protocol"))
		if e.m.typeKinds[proto] != "protocol" {
			continue
		}
		for _, r := range e.m.protoReqs[proto] {
			req := e.info(r)
			if req == nil || r.Bool("static") || r.Kind() != "var_decl" && r.Kind() != "func_decl" {
				continue
			}
			name, params, args, res := req.GoName, "", "", ""
			if r.Kind() == "var_decl" {
				name = "Get" + req.GoName
				res = e.m.GoType(e.m.typeOf(r.Str("interface_type")), nil)
			} else {
				params, args = e.paramList(r)
				res = e.resultGo(r)
			}
			if existing[name] || e.m.taken(claimRetro, e.m.retroDone, dep.Pkg+"."+typeName+"."+name) {
				continue
			}
			// Types are written from inside the dependency's package.
			local := func(t string) string { return strings.ReplaceAll(t, dep.Pkg+".", "") }
			sig := local(params + " " + res)
			if regexp.MustCompile(`\b[a-z]\w*\.`).MatchString(strings.ReplaceAll(sig, "swiftrt.", "")) || e.m.namesLocalType(sig) {
				e.m.Unsup("retroactive-conformance-type:"+name, e.where, "")
				continue
			}
			var own, mine *DeclInfo
			for _, mem := range dep.membersOf(q) {
				if mem.Kind == r.Kind() && !mem.Static && mem.Node.Name() == r.Name() && (r.Kind() == "var_decl" || funcSignatureKey(mem.Node) == funcSignatureKey(r)) {
					own = mem
				}
			}
			for _, mem := range e.m.membersOf(q) {
				if mem.Kind == r.Kind() && !mem.Static && mem.Node.Name() == r.Name() && (r.Kind() == "var_decl" || funcSignatureKey(mem.Node) == funcSignatureKey(r)) {
					mine = mem
				}
			}
			ret := "return "
			if res == "" {
				ret = ""
			}
			var body string
			switch {
			case mine != nil:
				hook := typeName + "Retro" + name
				e.m.retro(dep.Pkg, "// %s is set by the package that conforms %s to a protocol of its own.\nvar %s func(%s) %s\n\n", hook, typeName, hook, local(strings.TrimSuffix("self "+recv+", "+params, ", ")), local(res))
				body = ret + hook + "(self" + prefixComma(args) + ")"
				e.w("func init() { %s.%s = %s }\n\n", dep.Pkg, hook, mine.GoName)
				e.m.importPkg(dep)
			case own != nil && own.Kind == "var_decl" && !own.FreeSelf():
				body = "return self." + own.GoName
				if own.Computed {
					body += "()"
				}
			case own != nil && own.Kind == "func_decl" && !own.FreeSelf():
				body = ret + "self." + own.GoName + "(" + args + ")"
			default:
				e.m.Unsup("retroactive-conformance:"+name, e.where, "")
				continue
			}
			e.m.take(claimRetro, e.m.retroDone, dep.Pkg+"."+typeName+"."+name)
			e.m.retro(dep.Pkg, "// %s lets %s satisfy %s.%s.\nfunc (self %s) %s(%s) %s { %s }\n\n", name, typeName, e.m.Pkg, e.m.LocalTypeName(proto), recv, name, local(params), local(res), body)
		}
	}
}

func (e *fileEmitter) globalBinding(n ast.Node) {
	for _, pe := range n.List("pattern_entries") {
		name := patternName(pe.Child("pattern"))
		if isMacroName(name) {
			continue
		}
		t := e.m.typeOf(pe.Child("pattern").Type())
		init := pe.Child("processed_init")
		goName := capitalize(goSafe(name))
		var info *DeclInfo
		for _, it := range e.file.Items {
			if it.Kind() == "var_decl" && it.Name() == name {
				if vi := e.info(it); vi != nil {
					goName = vi.GoName
					info = vi
				}
			}
		}
		if init != nil {
			e.storedGlobal(info, goName, e.m.GoType(t, nil), init)
		}
	}
}

// genericScope maps a function's generic parameters to Go types (protocol constraints become interfaces).
func (e *fileEmitter) genericScope(fn ast.Node) map[string]string {
	g := map[string]string{}
	maps.Copy(g, e.generics)
	e.typeParams = nil
	e.typeBounds = map[string]string{}
	e.keptParams = map[string]bool{}
	e.witnessOf = map[string]string{}
	own := map[string]bool{}
	for _, gp := range fn.Child("generic_signature").Strings("generic_params") {
		name := e.m.demangle(gp)
		if _, outer := g[name]; outer || name == "" {
			continue
		}
		tp := "T" + strconv.Itoa(len(e.typeParams))
		e.typeParams = append(e.typeParams, tp)
		g[name] = tp
		own[name] = true
	}
	dropParam := func(param string) {
		if own[param] {
			tp := g[param]
			e.typeParams = slices.DeleteFunc(e.typeParams, func(p string) bool { return p == tp })
		}
	}
	assoc := e.scopeAssocParams(fn, g)
	seqParams := 0
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") != "conforms_to" {
			continue
		}
		param := e.m.demangle(r.Str("first_type"))
		if assoc[param] {
			continue
		}
		proto := e.m.demangle(r.Str("protocol"))
		switch proto {
		case "Swift.Collection", "Swift.Sequence", "Swift.BidirectionalCollection", "Swift.RandomAccessCollection":
			if v, ok := g[param]; !ok || v == "any" || own[param] && !strings.HasPrefix(v, "[]") {
				dropParam(param)
				// Count element parameters separately: dropping params shrinks typeParams.
				tp := "E" + strconv.Itoa(seqParams)
				seqParams++
				e.typeParams = append(e.typeParams, tp)
				g[param] = "[]" + tp
				g[param+".Element"] = tp
				g[param+".Swift.Sequence.Element"] = tp
				g[param+".Swift.Collection.Element"] = tp
			}
			continue
		}
		switch proto {
		case "Swift.StringProtocol":
			// A String or Substring parameter is a Go string; callers convert Substrings.
			if own[param] {
				dropParam(param)
				g[param] = "string"
			}
			continue
		case "Swift.BinaryInteger", "Swift.FixedWidthInteger", "Swift.SignedInteger", "Swift.UnsignedInteger":
			if own[param] {
				// Integer protocols become the runtime's integer constraint, so operators work.
				e.typeBounds[g[param]] = "swiftrt.Integer"
			}
			continue
		case "Swift.Comparable":
			// `<` and `==` on the parameter call swiftrt.Less and swiftrt.Equal, so a struct with its own `<` is
			// a valid type argument.
			continue
		case "Swift.Equatable":
			// == on the parameter compares with swiftrt.Equal, so arrays, dictionaries and structs holding
			// them, which Go cannot compare, are valid type arguments.
			continue
		case "Swift.Hashable":
			// Map keys need Go's comparable; a stronger bound (Integer) already implies it.
			if own[param] {
				if _, set := e.typeBounds[g[param]]; !set {
					e.typeBounds[g[param]] = "comparable"
				}
			}
			continue
		}
		if own[param] && e.m.isLocalProtocol(proto) && !erasesMetaGeneric(e.m, fn, param) && e.m.witnessed(proto) {
			// Foreign types conform to the protocol through carriers, so the parameter is unbounded and
			// requirements reach its values through swiftrt.Witness.
			e.witnessOf[g[param]] = e.m.GoType(&types.Type{Kind: types.Named, Name: proto}, nil)
			continue
		}
		if own[param] && e.m.isLocalProtocol(proto) && erasesMetaGeneric(e.m, fn, param) {
			// The type arrives as a value (`_ type: C.Type`), which carries the protocol's statics.
			dropParam(param)
			g[param] = e.m.GoType(&types.Type{Kind: types.Named, Name: proto}, nil)
			continue
		}
		if strings.HasPrefix(proto, e.m.Name+".") {
			if own[param] && e.m.HasStaticRequirements(proto) {
				e.typeBounds[g[param]] = e.m.LocalTypeName(proto)
				continue
			}
			if own[param] && fn.Kind() == "func_decl" && !fn.Bool("local") && nestedInParamTypes(e.m, fn, param) {
				// The parameter occurs inside a function or collection type (`(S) -> Void`, `[S]`).
				// Go's function and slice types are invariant, so it stays a type parameter, bounded
				// by the protocol's interface, and callers' concrete closures and arrays fit.
				e.typeBounds[g[param]] = e.m.LocalTypeName(proto)
				continue
			}
			dropParam(param)
			g[param] = e.m.LocalTypeName(proto)
		} else if _, ok := g[param]; !ok {
			g[param] = "any"
			if e.m.isLocalProtocol(proto) {
				// A constrained associated type is at least its protocol's interface.
				g[param] = e.m.GoType(&types.Type{Kind: types.Named, Name: proto}, nil)
			}
		}
	}
	for _, r := range fn.Child("generic_signature").List("generic_reqs") {
		if r.Str("kind") != "same_type" {
			continue
		}
		first := e.m.demangle(r.Str("first_type"))
		concrete := e.m.GoType(e.m.typeOf(r.Str("second_type")), g)
		tp, ok := g[first]
		if !ok {
			// A foreign extension's parameter pinned to a concrete type (`extension Set where Element == Race`).
			g[first] = concrete
			continue
		}
		for k, v := range g {
			if v == tp {
				g[k] = concrete
			} else if v == "[]"+tp {
				g[k] = "[]" + concrete
			}
		}
		e.typeParams = slices.DeleteFunc(e.typeParams, func(p string) bool { return p == tp })
	}
	if (fn.Child("implicit_self_decl") != nil || fn.Kind() == "subscript_decl") && !fn.Bool("static") && len(e.typeParams) > 0 && !e.freeSelfFunc(fn) && !e.foreignOwner(fn) && !e.emittingFree {
		for k, v := range g {
			for _, tp := range e.typeParams {
				bound := "any"
				if b, ok := e.typeBounds[tp]; ok {
					bound = b
				}
				if v == tp {
					g[k] = bound
				} else if v == "[]"+tp {
					g[k] = "[]" + bound
				}
			}
		}
		e.typeParams = nil
	}
	return g
}

func (e *fileEmitter) typeParamList() string {
	if len(e.typeParams) == 0 {
		return ""
	}
	parts := make([]string, len(e.typeParams))
	for i, tp := range e.typeParams {
		bound := "any"
		if b, ok := e.typeBounds[tp]; ok {
			bound = b
		}
		parts[i] = tp + " " + bound
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (e *fileEmitter) paramList(fn ast.Node) (decl string, names string) {
	var ps, ns []string
	for _, p := range fn.Child("params").List("params") {
		name := goSafe(p.Name())
		if name == "_" || name == "" {
			name = e.newTmp("unused")
		}
		gt := e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics)
		// A parameter of type `C.Type`, with C erased to its protocol, is the value that stands for C.
		if pt := e.m.typeOf(p.Str("interface_type")); e.metaValues != nil && pt != nil && pt.Kind == types.Meta && len(pt.Args) == 1 && pt.Args[0].Kind == types.Generic && e.m.protocolGoName(e.generics[pt.Args[0].Name]) {
			e.metaValues[pt.Args[0].Name] = name
		}
		if e.erased[len(ps)] {
			gt = "any"
			if !strings.HasPrefix(name, "unused") {
				name += "Any"
			}
		}
		if p.Bool("inout") {
			gt = "*" + gt
			if e.inoutParams == nil {
				e.inoutParams = map[string]bool{}
			}
			e.inoutParams[p.Name()] = true
		}
		ps = append(ps, name+" "+gt)
		ns = append(ns, name)
	}
	return strings.Join(ps, ", "), strings.Join(ns, ", ")
}

// unerase declares, under each erased parameter's own name, its value at the type fn declares.
func (e *fileEmitter) unerase(fn ast.Node, erased map[int]bool) string {
	var b strings.Builder
	for i, p := range fn.Child("params").List("params") {
		if !erased[i] || p.Name() == "_" || p.Name() == "" {
			continue
		}
		name := goSafe(p.Name())
		fmt.Fprintf(&b, "%s := %sAny.(%s)\n_ = %s\n", name, name, e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics), name)
	}
	return b.String()
}

// resultGo is the Go result type of the function fn as it is declared: a value type's Optional is
// swiftrt.Opt when the function returns it inline (see Module.inlineOptResult).
func (e *fileEmitter) resultGo(fn ast.Node) string {
	if info, mod := e.lookupModule(fn.Str("usr")); fn.Kind() == "func_decl" && e.m.inlineOptResult(info, mod) {
		if opt := e.m.inlineOptGo(e.m.typeOf(fn.Str("result")), e.generics); opt != "" {
			return opt
		}
	}
	return e.resultGoPtr(fn)
}

// resultGoPtr is the Go type of fn's result as a value of its Swift type, with an Optional as a
// pointer: the type of a call to fn after Ptr, and of a function value of fn's type.
func (e *fileEmitter) resultGoPtr(fn ast.Node) string {
	return e.m.GoType(e.m.typeOf(fn.Str("result")), e.generics)
}

// inlineOptResult reports whether a call to the declaration info, from the module mod, returns a
// swiftrt.Opt that the caller turns into the Optional's pointer form with Ptr. A function of a mapped
// module or of a file an overlay replaces keeps the pointer form its hand-written Go returns, as
// does one whose result stands for an associated type. An override follows the method it overrides.
func (m *Module) inlineOptResult(info *DeclInfo, mod *Module) bool {
	if info == nil || mod == nil || mod.Mapped || info.Kind != "func_decl" || info.Node == nil || info.Node.Bool("local") {
		return false
	}
	for seen := 0; info.Overrides != "" && seen < 64; seen++ {
		base, baseMod := lookupDecl(append([]*Module{mod}, mod.Deps...), info.Overrides)
		if base == nil {
			break
		}
		info, mod = base, baseMod
	}
	if mod.Mapped || mod.overlaidFunc(info.Node.Str("usr")) {
		return false
	}
	if info.Owner != "" && !info.ProtoExt {
		if _, erased := mod.assocErasure(info.Node, info.Owner); erased {
			return false
		}
	}
	return mod.inlineOptGo(mod.typeOf(info.Node.Str("result")), nil) != ""
}

// lookupDecl finds the declaration usr in the first of mods that declares it.
func lookupDecl(mods []*Module, usr string) (*DeclInfo, *Module) {
	for _, mod := range mods {
		if info := mod.decls[usr]; info != nil {
			return info, mod
		}
	}
	return nil, nil
}

func (e *fileEmitter) funcDecl(n ast.Node, self *DeclInfo) {
	saveIO := e.inoutParams
	e.inoutParams = nil
	defer func() { e.inoutParams = saveIO }()
	info := e.info(n)
	if info == nil {
		return
	}
	saveG := e.generics
	e.generics = e.genericScope(n)
	defer func() { e.generics = saveG }()
	var erased map[int]bool
	var erasedResult bool
	if self != nil && info.Owner != "" && !info.ProtoExt {
		erased, erasedResult = e.m.assocErasure(n, info.Owner)
	}
	e.erased = erased
	saveMeta := e.metaValues
	e.metaValues = map[string]string{}
	defer func() { e.metaValues = saveMeta }()
	params, names := e.paramList(n)
	e.erased = nil
	res := e.resultGo(n)
	if erasedResult {
		res = "any"
	}
	e.where = shortPath(e.file.Filename) + ":" + info.Owner + "." + n.Name()
	switch {
	case self == nil || info.Static && !info.ProtoExt:
		if self == nil {
			e.erasePhantomTypeParams(params + " " + res)
		} else if owner := e.selfTypeDecl(self); owner != "" {
			defer e.dropUnusedOwnerParams(e.b.Len(), info.GoName, owner)()
		}
		e.w("func %s%s(%s) %s ", info.GoName, mergeTypeParams(e.selfTypeDecl(self), e.typeParamList()), params, res)
	case self.Kind == "foreign" || info.FreeSelf():
		selfT := self.GoName
		if info.ProtoExt {
			selfT = e.m.LocalTypeName(info.Owner)
			// When Self is a type parameter (the protocol has static requirements), type self by it so
			// callers infer it from the receiver.
			if st := e.m.GoType(e.m.typeOf(n.Child("implicit_self_decl").Str("interface_type")), e.generics); slices.Contains(e.typeParams, st) && !info.Mutating {
				selfT = st
			}
			// An extension constrained by `where Self: Q` takes values that conform to both protocols.
			selfName := ""
			if st := e.m.typeOf(n.Child("implicit_self_decl").Str("interface_type")); st != nil {
				if st.Kind == types.Meta && len(st.Args) == 1 {
					st = st.Args[0]
				}
				if st.Kind == types.Generic {
					selfName = st.Name
				}
			}
			var also []string
			for _, r := range n.Child("generic_signature").List("generic_reqs") {
				proto := e.m.demangle(r.Str("protocol"))
				if selfName == "" || r.Str("kind") != "conforms_to" || e.m.demangle(r.Str("first_type")) != selfName || proto == info.Owner || !e.m.isLocalProtocol(proto) || e.m.refines(info.Owner, proto) {
					continue
				}
				also = append(also, e.m.GoType(&types.Type{Kind: types.Named, Name: proto}, nil))
			}
			if len(also) > 0 && selfT == e.m.LocalTypeName(info.Owner) {
				slices.Sort(also)
				selfT = "interface {\n" + selfT + "\n" + strings.Join(also, "\n") + "\n}"
			}
		}
		if (self.Kind == "foreign" || info.ProtoExt) && !e.m.isLocalProtocol(info.Owner) {
			if st := e.m.GoType(e.m.typeOf(n.Child("implicit_self_decl").Str("interface_type")), e.generics); st != "" && !strings.HasPrefix(st, "any") {
				selfT = st
			}
		}
		if self.Kind == "class_decl" && !strings.HasPrefix(selfT, "*") {
			selfT = "*" + selfT + e.selfTypeArgs(self)
		}
		if info.Mutating && !info.ProtoExt && !strings.HasPrefix(selfT, "*") && !e.m.isLocalProtocol(info.Owner) {
			selfT = "*" + selfT
			if e.inoutParams == nil {
				e.inoutParams = map[string]bool{}
			}
			e.inoutParams["self"] = true
		}
		e.erasePhantomTypeParams(selfT + " " + params + " " + res)
		tps := e.typeParamList()
		if info.GenericFree && self.Kind != "foreign" && self.Kind != "class_decl" && !info.ProtoExt && !strings.Contains(selfT, "[") {
			// A method of a generic struct made free for its constraints keeps the struct's parameters.
			if args := e.selfTypeArgs(self); args != "" {
				if tps == "" {
					tps = strings.TrimSuffix(strings.TrimPrefix(args, "["), "]")
					tps = "[" + strings.ReplaceAll(tps, ", ", " any, ") + " any]"
				}
				selfT = strings.Replace(selfT, e.m.LocalTypeName(info.Owner), e.m.LocalTypeName(info.Owner)+args, 1)
			}
		}
		e.w("func %s%s(self %s%s) %s ", info.GoName, tps, selfT, prefixComma(params), res)
	default:
		recv := self.GoName + e.selfTypeArgs(self)
		if info.Mutating || self.Kind == "class_decl" {
			recv = "*" + recv
		}
		if info.Mutating && self.Kind != "class_decl" {
			// self is a pointer here; Swift's self is the value.
			if e.inoutParams == nil {
				e.inoutParams = map[string]bool{}
			}
			e.inoutParams["self"] = true
		}
		e.w("func (self %s) %s(%s) %s ", recv, e.dispatched(info, self, recv, params, names, res), params, res)
	}
	e.preamble += e.unerase(n, erased)
	e.body(n, res)
	e.w("\n\n")
	if ps := n.Child("params").List("params"); self != nil && info.FreeSelf() && n.Name() == "encode" && len(ps) == 1 && e.m.demangle(ps[0].Str("interface_type")) == "Swift.Encoder" && e.m.IsPayloadEnum(e.selfQualified(self)) {
		// An enum emitted as a Go interface has no Encode method for the runtime to find.
		e.w("func init() {\nswiftrt.RegisterEncoder(func(v %s, e *swiftrt.Encoder) { %s(v, e) })\n}\n\n", self.GoName, info.GoName)
	} else if self != nil && self.Kind == "struct_decl" && info.FreeSelf() && !info.Static && !info.Mutating && n.Name() == "encode" && len(ps) == 1 &&
		e.m.demangle(ps[0].Str("interface_type")) == "Swift.Encoder" && e.selfTypeArgs(self) != "" && unconstrainedFunc(e.b.String(), info.GoName) {
		// The encode(to:) of a generic struct, emitted as a function: give the runtime the method it looks for.
		recvT := self.GoName + e.selfTypeArgs(self)
		e.w("func (self %s) Encode(encoder *swiftrt.Encoder) { %s%s(self, encoder) }\n\n", recvT, info.GoName, e.selfTypeArgs(self))
	}
	if e.m.Test && isTestFunc(n) {
		e.goTest(n, info, self)
	}
}

func isTestFunc(n ast.Node) bool {
	for _, a := range n.List("attrs") {
		if a.Kind() == "custom_attr" {
			return true
		}
	}
	return false
}

// goTest emits a `go test` entry point that runs one Swift @Test function.
func (e *fileEmitter) goTest(n ast.Node, info *DeclInfo, self *DeclInfo) {
	name := "Test" + info.GoName
	if self != nil {
		name = "Test" + self.GoName + info.GoName
	}
	params := n.Child("params").List("params")
	skips, scopes := e.testTraits(n, self)
	call := func(args string) {
		if len(scopes) > 0 {
			e.w("\tswiftrt.WithTraits(%q, func() {\n", name)
			defer e.w("\t}, %s)\n", strings.Join(scopes, ", "))
		}
		switch {
		case self == nil || info.Static:
			e.w("\t%s(%s)\n", info.GoName, args)
		default:
			if ctor := e.m.zeroArgInit(self); ctor != "" {
				e.w("\tsuite := %s()\n", ctor)
			} else {
				e.w("\tvar suite %s\n", self.GoName)
			}
			e.w("\tsuite.%s(%s)\n", info.GoName, args)
		}
	}
	if len(params) == 0 {
		e.w("func %s(t *testing.T) {\n%s\tdefer swiftrt.BindT(t)()\n", name, skips)
		call("")
		e.w("}\n\n")
		return
	}
	// @Test(arguments: xs) runs once per element; @Test(arguments: xs, ys) runs over every pair.
	var argExprs []ast.Node
	for _, a := range n.List("attrs") {
		if a.Kind() == "custom_attr" {
			for _, arg := range a.Child("args").List("args") {
				if arg.Str("label") == "arguments" || len(argExprs) > 0 && arg.Str("label") == "" {
					argExprs = append(argExprs, arg.Child("expr"))
				}
			}
		}
	}
	if len(argExprs) == 0 || len(argExprs) > 1 && len(argExprs) != len(params) {
		e.m.Unsup("parameterized-test", e.where, "")
		e.w("func %s(t *testing.T) { t.Skip(\"swift2go: parameterized @Test\") }\n\n", name)
		return
	}
	e.w("func %s(t *testing.T) {\n%s", name, skips)
	var names []string
	for i, ax := range argExprs {
		var et string
		if len(argExprs) == 1 {
			et = e.m.GoType(e.m.typeOf(params[0].Str("interface_type")), nil)
			if len(params) > 1 {
				// One collection of tuples, such as `zip(xs, ys)`, for several parameters: each element is the
				// tuple of the parameters' types.
				tuple := &types.Type{Kind: types.Tuple}
				for _, p := range params {
					tuple.Args = append(tuple.Args, e.m.typeOf(p.Str("interface_type")))
					tuple.Labels = append(tuple.Labels, "")
				}
				et = e.m.GoType(tuple, nil)
			}
		} else {
			et = e.m.GoType(e.m.typeOf(params[i].Str("interface_type")), nil)
		}
		v := "arg" + itoa(i)
		names = append(names, v)
		e.w("for i%d, %s := range swiftrt.Iterate[%s](%s) {\n", i, v, et, e.expr(ax))
	}
	label := "fmt.Sprint(i0"
	for i := 1; i < len(argExprs); i++ {
		label += ", \"/\", i" + itoa(i)
	}
	e.w("t.Run(%s), func(t *testing.T) {\ndefer swiftrt.BindT(t)()\n", label)
	args := strings.Join(names, ", ")
	if len(argExprs) == 1 && len(params) > 1 {
		var fs []string
		for i := range params {
			fs = append(fs, "arg0.F"+itoa(i))
		}
		args = strings.Join(fs, ", ")
	}
	call(args)
	e.w("})\n")
	for range argExprs {
		e.w("}\n")
	}
	e.w("}\n\n")
}

// testTraits renders the traits of a @Test function and of its @Suite: the Go statements that skip the test
// when a ConditionTrait (`.enabled(if:)`, `.disabled`) rules it out, and the TestScoping traits that wrap it,
// the suite's outermost. The other traits of the Testing module, such as `.serialized` and `.timeLimit`, change
// nothing in Go.
func (e *fileEmitter) testTraits(fn ast.Node, self *DeclInfo) (skips string, scopes []string) {
	// The traits of the enclosing suites come first, outermost first: a suite's traits reach every test
	// nested in it.
	var suites []*DeclInfo
	for d := self; d != nil && d.Node != nil; d = e.m.typeDecl(d.Owner) {
		suites = append([]*DeclInfo{d}, suites...)
	}
	var attrs []ast.Node
	for _, d := range suites {
		attrs = append(attrs, d.Node.List("attrs")...)
	}
	attrs = append(attrs, fn.List("attrs")...)
	var b strings.Builder
	for _, a := range attrs {
		if a.Kind() != "custom_attr" {
			continue
		}
		for _, arg := range a.Child("args").List("args") {
			if arg.Str("label") == "arguments" {
				break
			}
			x := arg.Child("expr")
			for x != nil && (x.Kind() == "erasure_expr" || x.Kind() == "inject_into_optional") {
				x = x.Child("sub_expr")
			}
			if x == nil {
				continue
			}
			t := e.m.typeOf(x.Type())
			if t == nil || t.Kind != types.Named || t.Name == "Swift.String" {
				continue
			}
			if t.Name == "Testing.ConditionTrait" {
				b.WriteString(e.conditionSkip(x))
				continue
			}
			if strings.HasPrefix(t.Name, "Testing.") {
				continue
			}
			scopes = append(scopes, e.expr(x))
		}
	}
	return b.String(), scopes
}

// conditionSkip renders the t.Skip for a ConditionTrait: `.disabled(...)`, or `.enabled(if:)` whose condition
// is false.
func (e *fileEmitter) conditionSkip(call ast.Node) string {
	fn := stripConv(call.Child("fn"))
	if fn.Kind() == "dot_syntax_call_expr" {
		fn = stripConv(fn.Child("fn"))
	}
	switch fn.Child("decl").Str("base_name") {
	case "disabled":
		return "\tt.Skip(\"disabled by .disabled\")\n"
	case "enabled":
		for _, arg := range call.Child("args").List("args") {
			if arg.Str("label") != "if" {
				continue
			}
			cond := arg.Child("expr")
			if cond.Kind() == "autoclosure_expr" {
				cond = cond.Child("single_expr_body")
			}
			return "\tif !(" + e.expr(cond) + ") {\n\t\tt.Skip(\"disabled by .enabled(if:)\")\n\t}\n"
		}
	}
	return "\t_ = " + e.m.Unsup("condition-trait", e.where, "") + "\n"
}

func (m *Module) zeroArgInit(self *DeclInfo) string {
	for _, info := range m.decls {
		if info.Kind == "constructor_decl" && info.Owner == m.typeQName(self) && len(info.Node.Child("params").List("params")) == 0 {
			return info.GoName
		}
	}
	return ""
}

func (e *fileEmitter) body(fn ast.Node, res string) {
	saveDo := e.doRet
	e.doRet = nil
	defer func() { e.doRet = saveDo }()
	saveR, saveT := e.resultType, e.throws
	e.resultType, e.throws = res, fn.Str("thrown_type") != ""
	defer func() { e.resultType, e.throws = saveR, saveT }()
	b := fn.Child("body")
	if b == nil {
		e.w("{ panic(\"swift2go: no body\") }")
		return
	}
	e.w("{\n%s", e.preamble)
	e.preamble = ""
	e.paramNames = map[string]bool{}
	for _, p := range fn.Child("params").List("params") {
		e.paramNames[goSafe(p.Name())] = true
	}
	saveTop := e.topLevel
	e.topLevel = map[uintptr]bool{}
	for _, st := range b.List("elements") {
		e.topLevel[nodeKey(st)] = true
	}
	defer func() { e.topLevel = saveTop }()
	e.stmts(b.List("elements"))
	if res != "" && !endsWithReturn(b) {
		e.w("panic(\"unreachable\")\n")
	}
	e.w("}")
}

func endsWithReturn(b ast.Node) bool {
	els := b.List("elements")
	if len(els) == 0 {
		return false
	}
	last := els[len(els)-1]
	return last.Kind() == "return_stmt" || last.Kind() == "throw_stmt"
}

func (e *fileEmitter) computedVar(n ast.Node, info *DeclInfo, self *DeclInfo) {
	var getter ast.Node
	for _, a := range n.List("accessors") {
		if a.Bool("get") || a.Str("accessor_kind") == "get" {
			getter = a
		}
	}
	if getter == nil || getter.Child("body") == nil {
		e.m.Unsup("computed-var-without-getter", e.where, "")
		return
	}
	res := e.m.GoType(e.m.typeOf(n.Str("interface_type")), e.generics)
	switch {
	case self != nil && self.Kind == "foreign" && info.Static && !info.ProtoExt:
		// A static on a generic foreign type (`extension Optional where Wrapped: P`) takes the
		// type's parameters as its own, so `Wrapped.req` reaches P's statics through them.
		saveG := e.generics
		e.generics = e.genericScope(getter)
		defer func() { e.generics = saveG }()
		res = e.m.GoType(e.m.typeOf(n.Str("interface_type")), e.generics)
		e.w("func %s%s() %s ", info.GoName, e.typeParamList(), res)
	case self == nil || info.Static && !info.ProtoExt:
		if owner := e.selfTypeDecl(self); owner != "" {
			defer e.dropUnusedOwnerParams(e.b.Len(), info.GoName, owner)()
		}
		e.w("func %s%s() %s ", info.GoName, e.selfTypeDecl(self), res)
	case self.Kind == "foreign" || info.FreeSelf():
		selfT := self.GoName
		if info.ProtoExt {
			selfT = e.m.LocalTypeName(info.Owner)
			if e.m.isLocalProtocol(info.Owner) {
				// Self inside a protocol extension's getter is the protocol's interface.
				if st := e.m.typeOf(getter.Child("implicit_self_decl").Str("interface_type")); st != nil && st.Kind == types.Generic {
					saveG := e.generics
					e.generics = maps.Clone(e.generics)
					if e.generics == nil {
						e.generics = map[string]string{}
					}
					e.generics[st.Name] = selfT
					defer func() { e.generics = saveG }()
				}
			}
		}
		tps := ""
		if (self.Kind == "foreign" || info.ProtoExt) && !e.m.isLocalProtocol(info.Owner) {
			saveG := e.generics
			e.emittingFree = true
			e.generics = e.genericScope(getter)
			e.emittingFree = false
			tps = e.typeParamList()
			defer func() { e.generics = saveG }()
			res = e.m.GoType(e.m.typeOf(n.Str("interface_type")), e.generics)
			if st := e.m.GoType(e.m.typeOf(getter.Child("implicit_self_decl").Str("interface_type")), e.generics); st != "" && !strings.HasPrefix(st, "any") {
				selfT = st
			}
		}
		e.w("func %s%s(self %s) %s ", info.GoName, tps, selfT, res)
	default:
		recv := self.GoName + e.selfTypeArgs(self)
		if self.Kind == "class_decl" {
			recv = "*" + recv
		}
		e.w("func (self %s) %s() %s ", recv, e.dispatched(info, self, recv, "", "", res), res)
	}
	e.body(getter, res)
	e.w("\n\n")
}

// weakField is the Go field holding the weak stored property info.
func weakField(info *DeclInfo) string { return "weak_" + info.GoName }

// weakVar emits the getter and SetX of a weak stored property.
func (e *fileEmitter) weakVar(n ast.Node, info, self *DeclInfo) {
	vt := e.m.GoType(e.m.typeOf(n.Str("interface_type")), e.generics)
	recv := self.GoName + e.selfTypeArgs(self)
	getRecv := recv
	if self.Kind == "class_decl" {
		getRecv = "*" + recv
	}
	e.w("func (self %s) %s() %s {\nreturn self.%s.Get()\n}\n\n", getRecv, info.GoName, vt, weakField(info))
	e.w("func (self *%s) Set%s(newValue %s) {\nself.%s.Set(newValue)\n}\n\n", recv, info.GoName, vt, weakField(info))
}

// setterVar emits SetX for a computed property's setter or a stored property's observers.
func (e *fileEmitter) setterVar(n ast.Node, info *DeclInfo, self *DeclInfo) {
	var set, will, did ast.Node
	for _, a := range n.List("accessors") {
		switch {
		case a.Bool("set"):
			set = a
		case a.Bool("willSet"):
			will = a
		case a.Bool("didSet"):
			did = a
		}
	}
	if set != nil && set.Bool("implicit") && (will != nil || did != nil) {
		// The synthesized setter of an observed property calls its observers; emit them directly.
		set = nil
	}
	if info.Settable && set == nil || info.Observed && will == nil && did == nil {
		return
	}
	vt := e.m.GoType(e.m.typeOf(n.Str("interface_type")), e.generics)
	name := "Set" + info.GoName
	switch {
	case self == nil || info.Static:
		e.w("func %s(newValue %s) {\n", name, vt)
	case self.Kind == "foreign" || info.FreeSelf():
		selfT := self.GoName
		if info.ProtoExt {
			selfT = e.m.LocalTypeName(info.Owner)
		}
		e.w("func %s(self *%s, newValue %s) {\n", name, selfT, vt)
	default:
		e.w("func (self *%s%s) %s(newValue %s) {\n", self.GoName, e.selfTypeArgs(self), name, vt)
	}
	saveR := e.resultType
	e.resultType = ""
	defer func() { e.resultType = saveR }()
	accessorParam := func(a ast.Node, fallback string) string {
		if ps := a.Child("params").List("params"); len(ps) > 0 {
			return goSafe(ps[0].Name())
		}
		return fallback
	}
	if set != nil {
		if p := accessorParam(set, "newValue"); p != "newValue" {
			e.w("%s := newValue\n_ = %s\n", p, p)
		}
		if b := set.Child("body"); b != nil {
			e.paramNames = map[string]bool{"newValue": true, accessorParam(set, "newValue"): true}
			e.stmts(b.List("elements"))
		}
		e.w("}\n\n")
		return
	}
	field := "self." + info.GoName
	if self == nil || info.Static {
		field = info.GoName
		if info.Lazy {
			field = lazyCell(info, "")
		}
	}
	if will != nil {
		e.w("func() {\n")
		if p := accessorParam(will, "newValue"); p != "newValue" {
			e.w("%s := newValue\n_ = %s\n", p, p)
		}
		if b := will.Child("body"); b != nil {
			e.paramNames = map[string]bool{"newValue": true, accessorParam(will, "newValue"): true}
			e.stmts(b.List("elements"))
		}
		e.w("}()\n")
	}
	if did != nil {
		e.w("oldValue := %s\n_ = oldValue\n", field)
	}
	e.w("%s = newValue\n", field)
	if did != nil {
		e.w("func() {\n")
		if p := accessorParam(did, "oldValue"); p != "oldValue" {
			e.w("%s := oldValue\n_ = %s\n", p, p)
		}
		if b := did.Child("body"); b != nil {
			e.paramNames = map[string]bool{"newValue": true, "oldValue": true, accessorParam(did, "oldValue"): true}
			e.stmts(b.List("elements"))
		}
		e.w("}()\n")
	}
	e.w("}\n\n")
}

// classCtor emits an initializer of a class that has a superclass or subclasses as two functions:
// the allocator New<X>, which creates the object, makes it its own most-derived object and runs
// the initializer on it, and the initializer body, a method returning whether it succeeded. A
// subclass's body runs its superclass's body on its embedded base, so the object keeps one identity
// through initialization and calls from a superclass initializer dispatch to the subclass.
func (e *fileEmitter) classCtor(n ast.Node, info, self *DeclInfo, typeNode ast.Node) {
	params, names := e.paramList(n)
	t := self.GoName + e.selfTypeArgs(self)
	recv := "*" + t
	failable := n.Bool("failable")
	ret := "*" + t
	if failable {
		e.w("func %s%s(%s) %s {\nself := &%s{}\nself.SwiftSub = self\nif !self.%s(%s) {\nreturn nil\n}\nreturn self\n}\n\n", info.GoName, e.selfTypeDecl(self), params, ret, t, e.initBodyName(info), names)
	} else {
		e.w("func %s%s(%s) %s {\nself := &%s{}\nself.SwiftSub = self\nself.%s(%s)\nreturn self\n}\n\n", info.GoName, e.selfTypeDecl(self), params, ret, t, e.initBodyName(info), names)
	}
	e.w("func (self %s) %s(%s) bool {\n", recv, e.dispatched(info, self, recv, params, names, "bool"), params)
	saveRet, saveFail, saveInit, saveOwner := e.initRet, e.initFail, e.inInit, e.initOwner
	e.initRet, e.initFail, e.inInit, e.initOwner = "true", "false", true, info.Owner
	defer func() { e.initRet, e.initFail, e.inInit, e.initOwner = saveRet, saveFail, saveInit, saveOwner }()
	if !n.Bool("convenience") {
		e.storedPropertyInits(typeNode)
	}
	b := n.Child("body")
	if b != nil {
		e.paramNames = paramNameSet(n)
		e.stmts(b.List("elements"))
	}
	if b == nil || !endsWithReturn(b) {
		e.w("return true\n")
	}
	e.w("}\n\n")
	if ps := n.Child("params").List("params"); len(ps) == 1 && !failable && e.m.demangle(ps[0].Str("interface_type")) == "Swift.Decoder" && !n.Implicit() && !strings.Contains(ret, "[") {
		e.w("func init() { swiftrt.RegisterDecoder(func(d *swiftrt.Decoder) %s { return %s(d) }) }\n\n", ret, info.GoName)
	}
}

// deinitDecl emits a class's deinit as the method SwiftDeinit, which runs the body and then the
// superclass's deinit. Go has no deterministic destruction, so nothing calls it; a deinit that only
// deallocates pointer memory, which the garbage collector frees, needs no call. Any other deinit is
// reported, since code that relies on it running has to call SwiftDeinit itself.
func (e *fileEmitter) deinitDecl(n ast.Node, self *DeclInfo) {
	if self == nil || self.Kind != "class_decl" {
		e.m.Unsup("member:"+n.Kind(), shortPath(e.file.Filename), "")
		return
	}
	e.where = shortPath(e.file.Filename) + ":" + self.Owner + ".deinit"
	if !onlyFreesMemory(n.Child("body")) {
		e.m.Unsup("deinit-runs-only-when-called", e.where, "")
	}
	e.w("// SwiftDeinit runs the class's deinit. Nothing calls it when the object is collected.\n")
	e.w("func (self *%s%s) SwiftDeinit() {\n", self.GoName, e.selfTypeArgs(self))
	if b := n.Child("body"); b != nil {
		e.paramNames = map[string]bool{}
		e.stmts(b.List("elements"))
	}
	if sup := e.m.Superclass(self.GoName); sup != "" {
		e.w("if d, ok := any(&self.%s).(interface{ SwiftDeinit() }); ok {\nd.SwiftDeinit()\n}\n", embeddedField(sup))
	}
	e.w("}\n\n")
}

// onlyFreesMemory reports whether a deinit body does nothing but deallocate pointers.
func onlyFreesMemory(body ast.Node) bool {
	if body == nil {
		return true
	}
	for _, st := range body.List("elements") {
		call := stripConv(st)
		if call == nil || call.Kind() != "call_expr" {
			return false
		}
		fn := stripConv(call.Child("fn"))
		if fn == nil || fn.Kind() != "dot_syntax_call_expr" {
			return false
		}
		if d := stripConv(fn.Child("fn")).Child("decl"); d.Str("base_name") != "deallocate" || !strings.HasPrefix(d.Str("decl_usr"), "s:Sp") && !strings.HasPrefix(d.Str("decl_usr"), "s:Sv") && !strings.HasPrefix(d.Str("decl_usr"), "s:Sr") && !strings.HasPrefix(d.Str("decl_usr"), "s:SW") {
			return false
		}
	}
	return true
}

// isUnimplementedInitStub reports whether n is an initializer the compiler adds to a subclass that
// does not inherit its superclass's designated initializers. Swift code cannot call it.
func isUnimplementedInitStub(n ast.Node) bool {
	if !n.Implicit() {
		return false
	}
	stub := false
	ast.Walk(n.Child("body"), func(c ast.Node) {
		if c.Kind() == "declref_expr" && c.Child("decl").Str("base_name") == "_unimplementedInitializer" {
			stub = true
		}
	})
	return stub
}

// initializing reports whether an assignment to the observed property info inside the current
// initializer stores directly: an initializer skips the observers of its own class's properties, but
// a subclass initializer runs the observers of the properties it inherits.
func (e *fileEmitter) initializing(info *DeclInfo) bool {
	return e.inInit && (e.initOwner == "" || info.Owner == e.initOwner)
}

// initBodyName is the method holding a class initializer's body.
func (e *fileEmitter) initBodyName(info *DeclInfo) string {
	if root := e.overrideRoot(info, &DeclInfo{Kind: "class_decl"}); root != nil {
		return overrideHook(root)
	}
	return classMethodName(info)
}

// initDispatch is the call `self.init(...)` makes in a class initializer: the most-derived object's
// override of the initializer when it is in an override chain.
func (e *fileEmitter) initDispatch(info *DeclInfo, args string) string {
	root := e.overrideRoot(info, &DeclInfo{Kind: "class_decl"})
	if root == nil {
		return "self." + classMethodName(info) + "(" + args + ")"
	}
	var ts []string
	for _, p := range info.Node.Child("params").List("params") {
		gt := e.m.GoType(e.m.typeOf(p.Str("interface_type")), e.generics)
		if p.Bool("inout") {
			gt = "*" + gt
		}
		ts = append(ts, gt)
	}
	hook := overrideHook(root)
	return "self.SwiftSub.(interface{ " + hook + "(" + strings.Join(ts, ", ") + ") bool })." + hook + "(" + args + ")"
}

// storedPropertyInits assigns the initial values a type's stored properties declare.
func (e *fileEmitter) storedPropertyInits(typeNode ast.Node) {
	for _, mem := range typeNode.List("members") {
		if mem.Kind() != "pattern_binding_decl" {
			continue
		}
		for _, pe := range mem.List("pattern_entries") {
			vi := e.findVar(typeNode, patternName(pe.Child("pattern")))
			if vi != nil && vi.Weak && pe.Child("processed_init") != nil {
				e.w("self.%s.Set(%s)\n", weakField(vi), e.expr(pe.Child("processed_init")))
			} else if vi != nil && !vi.Static && !vi.Computed && pe.Child("processed_init") != nil {
				e.w("self.%s = %s\n", vi.GoName, e.owned(pe.Child("processed_init"), sinkStore))
			}
		}
	}
}

func (e *fileEmitter) ctorDecl(n ast.Node, self *DeclInfo, typeNode ast.Node) {
	saveIO := e.inoutParams
	e.inoutParams = nil
	defer func() { e.inoutParams = saveIO }()
	info := e.info(n)
	if info == nil || self == nil {
		return
	}
	if self.Kind == "foreign" {
		e.foreignCtor(n, info, self)
		return
	}
	if self.Kind == "enum_decl" && n.Implicit() {
		return
	}
	if self.Kind == "class_decl" && e.m.inHierarchy(self.GoName) {
		if !isUnimplementedInitStub(n) {
			e.classCtor(n, info, self, typeNode)
		}
		return
	}
	ownTypeParams := ""
	constrained := false
	for _, r := range n.Child("generic_signature").List("generic_reqs") {
		if proto := e.m.demangle(r.Str("protocol")); r.Str("kind") == "conforms_to" && !strings.Contains(e.m.demangle(r.Str("first_type")), ".") && proto != "Swift.Copyable" && proto != "Swift.Escapable" && proto != "Swift.Sendable" {
			if _, typeParam := e.generics[e.m.demangle(r.Str("first_type"))]; !typeParam {
				constrained = true
			}
		}
	}
	if len(n.Child("generic_signature").Strings("generic_params")) > len(e.m.TypeParams(info.Owner)) && !constrained {
		// An initializer with type parameters of its own (`init<C>(menu: Menu<C>, context: C)`) is a
		// Go function, which can keep them.
		saveG, saveTP, saveTB := e.generics, e.typeParams, e.typeBounds
		e.emittingFree = true
		e.generics = e.genericScope(n)
		e.emittingFree = false
		ownTypeParams = e.typeParamList()
		defer func() { e.generics, e.typeParams, e.typeBounds = saveG, saveTP, saveTB }()
	}
	params, _ := e.paramList(n)
	t := self.GoName
	ret := t
	failable := n.Bool("failable")
	if self.Kind == "class_decl" || failable {
		ret = "*" + t
	}
	saveRet := e.initRet
	e.initRet = "self"
	if failable && self.Kind != "class_decl" {
		e.initRet = "&self"
	}
	defer func() { e.initRet = saveRet }()
	if targs := e.selfTypeArgs(self); targs != "" {
		ret = strings.Replace(ret, t, t+targs, 1)
		t += targs
	}
	e.w("func %s%s(%s) %s {\n", info.GoName, mergeTypeParams(e.selfTypeDecl(self), ownTypeParams), params, ret)
	if ps := n.Child("params").List("params"); n.Implicit() && !failable && len(ps) == 1 && e.m.demangle(ps[0].Str("interface_type")) == "Swift.Decoder" {
		// The synthesized init(from:), called by name: the runtime decodes the type's coding keys.
		d := goSafe(ps[0].Name())
		e.w("return swiftrt.DecodeValue[%s](%s.Value, %s.CodingPath)\n}\n\n", ret, d, d)
		return
	}
	if self.Kind == "class_decl" {
		e.w("self := &%s{}\n", t)
	} else {
		e.w("var self %s\n", t)
	}
	e.storedPropertyInits(typeNode)
	saveInit := e.inInit
	e.inInit = true
	if b := n.Child("body"); b != nil && !n.Implicit() {
		e.paramNames = paramNameSet(n)
		e.stmts(b.List("elements"))
	} else {
		for _, p := range n.Child("params").List("params") {
			if vi := e.findVar(typeNode, p.Name()); vi != nil && vi.Weak {
				e.w("self.%s.Set(%s)\n", weakField(vi), goSafe(p.Name()))
			} else if vi != nil {
				// The memberwise initializer keeps its arguments; its callers hand over values of their own.
				e.w("self.%s = %s\n", vi.GoName, goSafe(p.Name()))
			}
		}
	}
	e.inInit = saveInit
	if b := n.Child("body"); b == nil || n.Implicit() || !endsWithReturn(b) {
		e.w("return %s\n", e.initRet)
	}
	e.w("}\n\n")
	if ps := n.Child("params").List("params"); len(ps) == 1 && !failable && e.m.demangle(ps[0].Str("interface_type")) == "Swift.Decoder" && !n.Implicit() {
		if !strings.Contains(ret, "[") {
			e.w("func init() { swiftrt.RegisterDecoder(func(d *swiftrt.Decoder) %s { return %s(d) }) }\n\n", ret, info.GoName)
		} else if self.Kind != "class_decl" {
			// A generic type has no one Go type to register: the runtime finds its init(from:) as a method.
			e.w("func (self *%s) SwiftInitFrom(d *swiftrt.Decoder) { *self = %s%s(d) }\n\n", t, info.GoName, e.selfTypeArgs(self))
		}
	}
}

func (e *fileEmitter) exprGoType(n ast.Node) string {
	return e.m.GoType(e.m.typeOf(n.Type()), e.generics)
}

// overrideRoot returns the member at the top of info's override chain, or nil when info is not a
// class member that overrides or is overridden.
func (e *fileEmitter) overrideRoot(info, self *DeclInfo) *DeclInfo {
	if self == nil || self.Kind != "class_decl" || info.Static || !e.m.overridden[e.usrOf(info)] && info.Overrides == "" && !info.OpenOverridable {
		return nil
	}
	root := info
	for root.Overrides != "" && e.m.declOf(root.Overrides) != nil {
		root = e.m.declOf(root.Overrides)
	}
	return root
}

// classMethodName is the Go method a class member's body is emitted as: an initializer's body is
// Init_<suffix> of its allocator New<suffix>.
func classMethodName(info *DeclInfo) string {
	if info.Kind == "constructor_decl" {
		return "Init_" + strings.TrimPrefix(info.GoName, "New")
	}
	return info.GoName
}

// overrideHook is the method name every implementation in root's override chain is emitted under.
func overrideHook(root *DeclInfo) string { return "Override_" + classMethodName(root) }

// dispatched returns the name to emit a class member's body under. A member in an override chain
// has its body emitted as the chain's Override_ method, which super calls reach directly; the
// chain's root also gets the dispatching method, which calls the most-derived object's Override_.
func (e *fileEmitter) dispatched(info, self *DeclInfo, recv, params, names, res string) string {
	root := e.overrideRoot(info, self)
	if root == nil {
		return classMethodName(info)
	}
	hook := overrideHook(root)
	if root == info && info.Kind != "constructor_decl" {
		ret := "return "
		if res == "" {
			ret = ""
		}
		e.w("func (self %s) %s(%s) %s {\nif o, ok := self.SwiftSub.(interface{ %s(%s) %s }); ok {\n%so.%s(%s)\n", recv, classMethodName(info), params, res, hook, params, res, ret, hook, names)
		if res == "" {
			e.w("return\n")
		}
		e.w("}\n%sself.%s(%s)\n}\n\n", ret, hook, names)
	}
	return hook
}

func (e *fileEmitter) usrOf(info *DeclInfo) string {
	if info.Node == nil {
		return ""
	}
	return info.Node.Str("usr")
}

// codingKeys maps each case of a type's own CodingKeys enum to the key it encodes as: its string raw
// value, or else its name. It returns nil when the type declares no CodingKeys.
// codingKeyOrder lists the keys of a type's own CodingKeys enum in the order its cases are
// declared, the order a synthesized encode(to:) writes them; nil without such an enum.
func codingKeyOrder(n ast.Node) []string {
	for _, mem := range n.List("members") {
		if mem.Kind() != "enum_decl" || mem.Name() != "CodingKeys" {
			continue
		}
		var order []string
		add := func(c ast.Node) {
			key := c.Name()
			if v := c.Child("raw_value_expr"); v != nil && v.Kind() == "string_literal_expr" {
				key = v.Str("value")
			}
			order = append(order, key)
		}
		for _, m := range mem.List("members") {
			switch m.Kind() {
			case "enum_case_decl":
				for _, el := range m.List("elements") {
					add(el)
				}
			case "enum_element_decl":
				if mem.Child("members") == nil || !slices.ContainsFunc(mem.List("members"), func(x ast.Node) bool { return x.Kind() == "enum_case_decl" }) {
					add(m)
				}
			}
		}
		return order
	}
	return nil
}

func codingKeys(n ast.Node) map[string]string {
	for _, mem := range n.List("members") {
		if mem.Kind() != "enum_decl" || mem.Name() != "CodingKeys" {
			continue
		}
		keys := map[string]string{}
		add := func(c ast.Node) {
			key := c.Name()
			if v := c.Child("raw_value_expr"); v != nil && v.Kind() == "string_literal_expr" {
				key = v.Str("value")
			}
			keys[c.Name()] = key
		}
		for _, m := range mem.List("members") {
			switch m.Kind() {
			case "enum_case_decl":
				for _, el := range m.List("elements") {
					add(el)
				}
			case "enum_element_decl":
				add(m)
			}
		}
		return keys
	}
	return nil
}

func conformsTo(n ast.Node, protocolUSR string) bool {
	for _, c := range n.Child("inherits").List("conformances") {
		if c.Str("protocol") == protocolUSR {
			return true
		}
	}
	return false
}

func hasExplicitMember(n ast.Node, name string) bool {
	for _, mem := range n.List("members") {
		if !mem.Implicit() && mem.Name() == name {
			return true
		}
	}
	return false
}

// foreignCtor emits an init declared in an extension of a type from another module.
func (e *fileEmitter) foreignCtor(n ast.Node, info, self *DeclInfo) {
	saveIO := e.inoutParams
	e.inoutParams = nil
	defer func() { e.inoutParams = saveIO }()
	saveG := e.generics
	e.generics = e.genericScope(n)
	defer func() { e.generics = saveG }()
	params, _ := e.paramList(n)
	t := self.GoName
	// A constrained extension (`extension Set where Element == Race`) pins the self type.
	if st := e.m.GoType(e.m.typeOf(n.Child("implicit_self_decl").Str("interface_type")), e.generics); st != "" && len(e.typeParams) == 0 && !strings.Contains(st, "any") {
		t = st
	}
	failable := n.Bool("failable")
	ret := t
	if failable && !strings.HasPrefix(t, "*") {
		ret = "*" + t
	}
	e.w("func %s(%s) %s {\n", info.GoName, params, ret)
	if strings.HasPrefix(t, "*") {
		e.w("self := &%s{}\n", t[1:])
	} else {
		e.w("var self %s\n", t)
	}
	saveInit, saveRet := e.inInit, e.initRet
	e.inInit = true
	e.initRet = "self"
	if failable && !strings.HasPrefix(t, "*") {
		e.initRet = "&self"
	}
	if b := n.Child("body"); b != nil {
		e.paramNames = paramNameSet(n)
		e.stmts(b.List("elements"))
		if !endsWithReturn(b) {
			e.w("return %s\n", e.initRet)
		}
	}
	e.inInit, e.initRet = saveInit, saveRet
	e.w("}\n\n")
}

// propertyGetters emits GetX methods so a conformer satisfies a protocol's property requirements.
func (e *fileEmitter) propertyGetters(n ast.Node, self *DeclInfo, q, proto string, conformed []string, pm *Module, pfx string) {
	if e.m.IsPayloadEnum(q) {
		return
	}
	recv := e.m.GoType(&types.Type{Kind: types.Named, Name: q}, nil)
	selfArg := "self"
	if e.m.HasMutatingMethods(q) {
		selfArg = "swiftrt.Ref(self)"
	}
	for _, r := range pm.protoReqs[proto] {
		if r.Kind() != "var_decl" {
			continue
		}
		req := pm.decls[r.Str("usr")]
		if req == nil {
			continue
		}
		name := r.Name()
		res := e.m.GoType(e.m.typeOf(r.Str("interface_type")), nil)
		body := ""
		stored := ""
		isDefault, from := false, ""
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			mpfx := ""
			if mod.Pkg != e.m.Pkg {
				mpfx = mod.Pkg + "."
			}
			for _, d := range mod.sortedDecls() {
				if d.Kind != "var_decl" || d.Node == nil || d.Node.Name() != name || d.NotWitness {
					continue
				}
				switch {
				case d.Owner == q && !d.Static && !d.FreeSelf():
					isDefault = false
					if d.Computed {
						body = "self." + d.GoName + "()"
					} else {
						body = "self." + d.GoName
						if !d.Observed {
							stored = "self." + d.GoName
						}
					}
				case d.Owner == q && d.FreeSelf() && (body == "" || isDefault):
					body, isDefault = d.GoName+"(self)", false
				case d.ProtoExt && d.Owner == proto && body == "":
					body, isDefault, from = pfx+d.GoName+"("+selfArg+")", true, proto
				case d.ProtoExt && e.moreRefinedDefault(d, body, isDefault, from, q, proto, conformed, mod):
					// A protocol that refines this one, and that the type conforms to, supplies the default.
					body, isDefault, from = mpfx+d.GoName+"("+selfArg+")", true, d.Owner
				case body == "" && !d.Static && !d.FreeSelf() && mod == e.m && e.m.IsAncestor(e.m.LocalTypeName(d.Owner), e.m.LocalTypeName(q)):
					// Inherited from a superclass: the embedded base promotes it.
					if d.Computed {
						body = "self." + d.GoName + "()"
					} else {
						body = "self." + d.GoName
					}
				}
			}
		}
		if body == "" {
			continue
		}
		e.w("func (self %s) Get%s() %s { return %s }\n\n", recv, req.GoName, res, body)
		// A settable requirement met by a stored property of a class gets a setter; computed and
		// observed properties already have one.
		if r.Str("writeImpl") == "setter" && stored != "" {
			setRecv := recv
			if !strings.HasPrefix(setRecv, "*") {
				// A struct sets through a pointer, so *T satisfies the protocol.
				setRecv = "*" + recv
			}
			e.w("func (self %s) Set%s(newValue %s) { %s = newValue }\n\n", setRecv, req.GoName, res, stored)
		}
	}
}

// subscriptDecl emits a user subscript as Subscript (getter) and SetSubscript (setter) methods.
func (e *fileEmitter) subscriptDecl(n ast.Node, self *DeclInfo) {
	saveIO := e.inoutParams
	e.inoutParams = nil
	defer func() { e.inoutParams = saveIO }()
	info := e.info(n)
	if info != nil && self != nil && self.Kind == "foreign" && !info.Static {
		// A read-only subscript an extension adds to another module's type (`extension Dictionary
		// where Key == K { subscript(_:) }`) is a function of the value.
		var getter ast.Node
		setter := false
		for _, a := range n.List("accessors") {
			if a.Bool("get") {
				getter = a
			}
			setter = setter || a.Bool("set")
		}
		if getter != nil && !setter {
			saveG := e.generics
			e.emittingFree = true
			e.generics = e.genericScope(getter)
			e.emittingFree = false
			defer func() { e.generics = saveG }()
			params, _ := e.paramList(getter)
			res := e.m.GoType(e.m.typeOf(getter.Str("result")), e.generics)
			if el := n.Str("element"); el != "" {
				res = e.m.GoType(e.m.typeOf(el), e.generics)
			}
			selfT := e.m.GoType(e.m.typeOf(getter.Child("implicit_self_decl").Str("interface_type")), e.generics)
			e.where = shortPath(e.file.Filename) + ":" + info.Owner + ".subscript"
			e.w("func %s%s(self %s%s) %s ", info.GoName, e.typeParamList(), selfT, prefixComma(params), res)
			e.body(getter, res)
			e.w("\n\n")
			return
		}
	}
	if info == nil || self == nil || self.Kind == "foreign" || info.FreeSelf() && !info.GenericFree || info.Static {
		e.m.Unsup("decl:subscript", shortPath(e.file.Filename), "")
		return
	}
	saveG := e.generics
	e.generics = e.genericScope(n)
	defer func() { e.generics = saveG }()
	var getter ast.Node
	for _, a := range n.List("accessors") {
		if a.Bool("get") {
			getter = a
		}
	}
	if getter == nil {
		e.m.Unsup("decl:subscript-without-getter", shortPath(e.file.Filename), "")
		return
	}
	params, _ := e.paramList(getter)
	res := e.m.GoType(e.m.typeOf(getter.Str("result")), e.generics)
	if el := n.Str("element"); el != "" {
		res = e.m.GoType(e.m.typeOf(el), e.generics)
	}
	recv := self.GoName + e.selfTypeArgs(self)
	if self.Kind == "class_decl" {
		recv = "*" + recv
	}
	e.where = shortPath(e.file.Filename) + ":" + info.Owner + ".subscript"
	for _, a := range n.List("accessors") {
		switch {
		case a.Bool("get"):
			if info.GenericFree {
				e.w("func %s%s(self %s%s) %s ", info.GoName, e.typeParamList(), recv, prefixComma(params), res)
			} else {
				e.w("func (self %s) %s(%s) %s ", recv, info.GoName, params, res)
			}
			e.body(a, res)
			e.w("\n\n")
		case a.Bool("set"):
			setRecv := recv
			if !strings.HasPrefix(setRecv, "*") {
				setRecv = "*" + setRecv
				if e.inoutParams == nil {
					e.inoutParams = map[string]bool{}
				}
				e.inoutParams["self"] = true
			}
			if info.GenericFree {
				e.w("func %s%s(self %s, %s) {\n", subscriptSetterName(info), e.typeParamList(), setRecv, prefixCommaTail(params, "newValue "+res))
			} else {
				e.w("func (self %s) Set%s(%s) {\n", setRecv, info.GoName, prefixCommaTail(params, "newValue "+res))
			}
			if ps := a.Child("params").List("params"); len(ps) > 0 && goSafe(ps[0].Name()) != "newValue" && !strings.Contains(params, goSafe(ps[0].Name())+" ") {
				e.w("%s := newValue\n_ = %s\n", goSafe(ps[0].Name()), goSafe(ps[0].Name()))
			}
			saveR := e.resultType
			e.resultType = ""
			if b := a.Child("body"); b != nil {
				e.stmts(b.List("elements"))
			}
			e.resultType = saveR
			delete(e.inoutParams, "self")
			e.w("}\n\n")
		}
	}
}

// subscriptSetterName names the setter function of a subscript emitted as free functions.
func subscriptSetterName(info *DeclInfo) string {
	i := strings.LastIndex(info.GoName, "Subscript")
	return info.GoName[:i] + "Set" + info.GoName[i:]
}

func prefixCommaTail(params, last string) string {
	if params == "" {
		return last
	}
	return params + ", " + last
}

// foreignOwner reports whether fn extends a type from another module, so it is emitted as a free
// function that can keep type parameters.
func (e *fileEmitter) foreignOwner(fn ast.Node) bool {
	info := e.m.decls[fn.Str("usr")]
	return info != nil && info.Owner != "" && e.m.typeKinds[info.Owner] == "" && !e.m.IsPayloadEnum(info.Owner)
}

func (e *fileEmitter) freeSelfFunc(fn ast.Node) bool {
	info := e.m.decls[fn.Str("usr")]
	return info != nil && info.FreeSelf()
}

// staticReqName is the interface method that carries a protocol's static requirement.
func staticReqName(info *DeclInfo) string {
	return "Static" + capitalize(goSafe(info.Node.Name()))
}

// staticInitName is the interface method that carries a protocol's initializer requirement.
func staticInitName(req ast.Node) string {
	return "StaticInit" + labelSuffix(paramLabels(req))
}

// initForwarders gives a conformer a method for each initializer its protocol requires, so code
// that holds the type only as a protocol metatype can build a value.
func (e *fileEmitter) initForwarders(q, proto string, pm *Module, pfx string) {
	recv := e.m.GoType(&types.Type{Kind: types.Named, Name: q}, nil)
	for _, r := range pm.protoReqs[proto] {
		if r.Kind() != "constructor_decl" {
			continue
		}
		labels := strings.Join(paramLabels(r), ":")
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			mpfx := ""
			if mod.Pkg != e.m.Pkg {
				mpfx = mod.Pkg + "."
			}
			for _, d := range mod.sortedDecls() {
				if d.Kind != "constructor_decl" || d.Owner != q || d.Node == nil || strings.Join(paramLabels(d.Node), ":") != labels {
					continue
				}
				params, args := e.paramList(r)
				made := mpfx + d.GoName + "(" + args + ")"
				if e.m.HasMutatingMethods(q) && !strings.HasPrefix(recv, "*") {
					made = "swiftrt.Ref(" + made + ")"
				}
				e.w("func (self %s) %s(%s) %s { return %s }\n\n", recv, staticInitName(r), params, pfx+pm.LocalTypeName(proto), made)
				return
			}
		}
	}
}

// moreRefinedDefault reports whether d, a default from a protocol extension, should replace the
// default chosen so far for one of proto's requirements on q: d's protocol must refine proto, q must
// conform to it and the extension must apply to q, and the default so far must be missing or come from
// a protocol d's refines. Swift picks the most refined default the same way.
func (e *fileEmitter) moreRefinedDefault(d *DeclInfo, chosen string, isDefault bool, from, q, proto string, conformed []string, mod *Module) bool {
	if chosen != "" && (!isDefault || d.Owner == from || !e.m.refines(d.Owner, from)) {
		return false
	}
	// conformed lists only this declaration's conformances; another extension, such as one a macro
	// adds, may declare the rest.
	conforms := e.m.conformsTo(q, d.Owner) || slices.ContainsFunc(conformed, func(c string) bool { return c == d.Owner || e.m.refines(c, d.Owner) })
	return conforms && e.m.refines(d.Owner, proto) && e.m.extensionApplies(d.Node, q, d.Owner, mod)
}

// staticForwarders gives a conformer methods for its protocol's static requirements, so generic code
// can reach them through a type parameter's zero value.
func (e *fileEmitter) staticForwarders(q, proto string, conformed []string, pm *Module, pfx string) {
	recv := e.m.GoType(&types.Type{Kind: types.Named, Name: q}, nil)
	selfArg := "self"
	if e.m.HasMutatingMethods(q) {
		selfArg = "swiftrt.Ref(self)"
	}
	for _, r := range pm.protoReqs[proto] {
		if !(r.Kind() == "func_decl" || r.Kind() == "var_decl") || !r.Bool("static") {
			continue
		}
		req := pm.decls[r.Str("usr")]
		if req == nil {
			continue
		}
		var params, args, res string
		if r.Kind() == "func_decl" {
			params, args = e.paramList(r)
			res = e.resultGo(r)
		} else {
			res = e.m.GoType(e.m.typeOf(r.Str("interface_type")), nil)
		}
		target, isDefault, from := "", false, ""
		for _, mod := range append([]*Module{e.m}, e.m.Deps...) {
			mpfx := ""
			if mod.Pkg != e.m.Pkg {
				mpfx = mod.Pkg + "."
			}
			for _, d := range mod.sortedDecls() {
				if d.Kind != r.Kind() || !d.Static || d.Node == nil || d.Node.Name() != r.Name() {
					continue
				}
				switch {
				case d.Owner == q && !d.ProtoExt && target == "" || d.Owner == q && !d.ProtoExt && isDefault:
					target, isDefault = mpfx+d.GoName, false
					if d.Kind == "var_decl" && (d.Computed || d.Lazy) {
						target += "()"
					} else if d.Kind == "func_decl" {
						target += "(" + args + ")"
					}
				case d.ProtoExt && d.Owner == proto && target == "":
					target, isDefault, from = mpfx+d.GoName+e.conformerAssocTypeArgs(d, q, mod)+"("+selfArg+prefixComma(args)+")", true, proto
				case d.ProtoExt && e.moreRefinedDefault(d, target, isDefault, from, q, proto, conformed, mod) &&
					(d.Kind != "func_decl" || funcSignatureKey(d.Node) == funcSignatureKey(r)):
					// A protocol that refines this one, and that the type conforms to, supplies the default.
					target, isDefault, from = mpfx+d.GoName+e.conformerAssocTypeArgs(d, q, mod)+"("+selfArg+prefixComma(args)+")", true, d.Owner
				}
			}
		}
		if target == "" {
			continue
		}
		ret := "return "
		if res == "" {
			ret = ""
		}
		if res == "[]any" {
			target = "swiftrt.ToAnySlice(" + target + ")"
		}
		ft := e.m.typeOf(r.Str("interface_type"))
		for ft != nil && ft.Kind == types.Func && ft.Result != nil && ft.Result.Kind == types.Func {
			ft = ft.Result
		}
		if e.m.HasMutatingMethods(q) && res == "any" && ft != nil && ft.Kind == types.Func && ft.Result != nil && ft.Result.Kind == types.Generic {
			// A requirement returning Self: generic code holds this type through a pointer, as its mutating
			// methods need, so hand it one.
			target = "swiftrt.Ref(" + target + ")"
		}
		e.w("func (self %s) %s(%s) %s { %s%s }\n\n", recv, staticReqName(req), params, res, ret, target)
	}
}

func paramNameSet(fn ast.Node) map[string]bool {
	set := map[string]bool{}
	for _, p := range fn.Child("params").List("params") {
		set[goSafe(p.Name())] = true
	}
	return set
}

// typeGenerics maps a generic type's parameters to themselves while its members are emitted.
func (e *fileEmitter) typeGenerics(q string) map[string]string {
	g := map[string]string{}
	maps.Copy(g, e.generics)
	for _, p := range e.m.TypeParams(q) {
		g[p] = p
	}
	return g
}

func (e *fileEmitter) selfQualified(self *DeclInfo) string {
	if self == nil || self.Node == nil {
		if self != nil {
			return self.Owner
		}
		return ""
	}
	return e.m.typeQName(self)
}

// selfTypeArgs is "[A, B]" for a method receiver on a generic type.
func (e *fileEmitter) selfTypeArgs(self *DeclInfo) string {
	if self == nil || self.Kind == "foreign" {
		return ""
	}
	return e.m.typeParamUse(e.selfQualified(self))
}

// selfTypeDecl is "[A any, B any]" for a static member or init of a generic type, leaving out
// parameters a same-type requirement (`where Element == Item`) has pinned.
// dropUnusedOwnerParams returns a function, deferred until a static member of a generic type has been
// written from mark on, that removes from its declaration the type's parameters (owner, "[A any, B any]")
// that neither its signature nor its body names. Callers name a static member without type arguments,
// which Go cannot infer for a parameter that no argument mentions.
func (e *fileEmitter) dropUnusedOwnerParams(mark int, name, owner string) func() {
	return func() {
		text := e.b.String()
		head := "func " + name + "["
		if !strings.HasPrefix(text[mark:], head) {
			return
		}
		open := mark + len(head) - 1
		end := strings.Index(text[open:], "]")
		if end < 0 {
			return
		}
		end += open
		rest := text[end+1:]
		var keep []string
		for _, tp := range strings.Split(text[open+1:end], ", ") {
			p, _, _ := strings.Cut(tp, " ")
			if !strings.Contains(owner, "["+p+" ") && !strings.Contains(owner, ", "+p+" ") || regexp.MustCompile(`\b`+regexp.QuoteMeta(p)+`\b`).MatchString(rest) {
				keep = append(keep, tp)
			}
		}
		decl := ""
		if len(keep) > 0 {
			decl = "[" + strings.Join(keep, ", ") + "]"
		}
		if decl == text[open:end+1] {
			return
		}
		e.b.Reset()
		e.b.WriteString(text[:open])
		e.b.WriteString(decl)
		e.b.WriteString(rest)
	}
}

func (e *fileEmitter) selfTypeDecl(self *DeclInfo) string {
	if self == nil || self.Kind == "foreign" {
		return ""
	}
	var parts []string
	for _, p := range e.m.TypeParams(e.selfQualified(self)) {
		if v, ok := e.generics[p]; ok && v != p {
			continue
		}
		parts = append(parts, p+" any")
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// topLevelCode emits the statements a main.swift holds outside any declaration as func main, in
// source order. The variables those statements declare are package variables, which other
// functions and files can read, and main assigns each one where Swift initializes it.
func (e *fileEmitter) topLevelCode() {
	var elements []ast.Node
	for _, it := range e.file.Items {
		if it.Kind() == "top_level_code_decl" {
			elements = append(elements, it.Child("body").List("elements")...)
		}
	}
	if len(elements) == 0 {
		return
	}
	globals := map[string]*DeclInfo{}
	for _, it := range e.file.Items {
		if it.Kind() == "var_decl" && it.Bool("top_level_global") {
			globals[it.Name()] = e.info(it)
		}
	}
	e.where = shortPath(e.file.Filename) + ":main"
	e.resultType, e.throws = "", false
	e.topLevel = map[uintptr]bool{}
	for _, st := range elements {
		e.topLevel[nodeKey(st)] = true
	}
	e.w("func main() {\n")
	var run []ast.Node
	flush := func() {
		e.stmts(run)
		run = nil
	}
	for _, st := range elements {
		if st.Kind() != "pattern_binding_decl" {
			run = append(run, st)
			continue
		}
		flush()
		for _, pe := range st.List("pattern_entries") {
			p := pe.Child("pattern")
			for p.Kind() == "pattern_typed" {
				p = p.Child("sub_pattern")
			}
			info := globals[patternName(p)]
			init := pe.Child("processed_init")
			switch {
			case p.Kind() != "pattern_named" || info == nil:
				e.m.Unsup("top-level-pattern:"+p.Kind(), e.where, "")
			case init != nil:
				e.w("%s = %s\n", info.GoName, e.owned(init, sinkStore))
			}
		}
	}
	flush()
	e.w("}\n\n")
}

// erasePhantomTypeParams drops the function's type parameters that occur nowhere in sig, its Go
// parameter and result types. Such a parameter occurs in Swift only inside a type Go erases (a
// foreign generic's nested type, or Self behind a protocol's interface), so no call could infer
// it; inside the body it names any.
func (e *fileEmitter) erasePhantomTypeParams(sig string) {
	e.typeParams = slices.DeleteFunc(e.typeParams, func(tp string) bool {
		if e.keptParams[tp] || regexp.MustCompile(`\b`+tp+`\b`).MatchString(sig) {
			return false
		}
		e.preamble += "type " + tp + " = any\n"
		return true
	})
}
