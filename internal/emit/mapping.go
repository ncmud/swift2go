package emit

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/swiftmod"
	"github.com/ncmud/swift2go/internal/types"
)

// Mapping sends references to a Swift module that is not transpiled in the run to a hand-written Go package.
// Generated code names the module's declarations as it would name them in swiftrt; a name the package
// exports is taken from the package instead, under the import name Alias.
type Mapping struct {
	Module     string
	ImportPath string
	// Package is the package name the Go files declare; generated code imports the package under it.
	Package string
	Exports map[string]bool
	// Generic holds the number of type parameters of each exported generic type.
	Generic map[string]int
	// Requirements lists the requirements of the module's protocols, keyed by protocol name without the
	// module. A local type conforming to one registers its implementations of them with the package.
	Requirements map[string][]swiftmod.Requirement
}

// Alias is the name generated code imports the package under: its package name, or the module name in lower
// case, which is how the emitter qualifies a foreign module's types.
func (mp Mapping) Alias() string {
	if mp.Package != "" {
		return mp.Package
	}
	return strings.ToLower(mp.Module)
}

// PackageExports lists the exported package-level identifiers declared in the non-test Go files of dir, and
// the number of type parameters of each exported generic type.
func PackageExports(dir string) (pkg string, exports map[string]bool, generic map[string]int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, nil, err
	}
	fset := token.NewFileSet()
	exports = map[string]bool{}
	generic = map[string]int{}
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return "", nil, nil, err
		}
		pkg = f.Name.Name
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *goast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					exports[d.Name.Name] = true
				}
			case *goast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *goast.TypeSpec:
						if s.Name.IsExported() {
							exports[s.Name.Name] = true
							if s.TypeParams != nil {
								generic[s.Name.Name] = s.TypeParams.NumFields()
							}
						}
					case *goast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								exports[n.Name] = true
							}
						}
					}
				}
			}
		}
	}
	if len(exports) == 0 {
		return "", nil, nil, fmt.Errorf("%s: no exported Go declarations", dir)
	}
	return pkg, exports, generic, nil
}

// mappedName reports whether a mapped package exports the runtime-style name.
func (m *Module) mappedName(name string) bool {
	for _, mp := range m.Mappings {
		if mp.Exports[name] {
			return true
		}
	}
	return false
}

var runtimeRef = regexp.MustCompile(`\bswiftrt\.([A-Za-z_][A-Za-z0-9_]*)`)

// applyMappings points runtime references that a mapped package exports at that package, and returns the
// imports, as Go import specs, that src now needs.
func (m *Module) applyMappings(src string) (string, []string) {
	if len(m.Mappings) == 0 {
		return src, nil
	}
	aliasOf := map[string]string{} // import path → the alias of the first module mapped to it
	for _, mp := range m.Mappings {
		if _, ok := aliasOf[mp.ImportPath]; !ok {
			aliasOf[mp.ImportPath] = mp.Alias()
		}
	}
	src = runtimeRef.ReplaceAllStringFunc(src, func(ref string) string {
		name := ref[len("swiftrt."):]
		for _, mp := range m.Mappings {
			if mp.Exports[name] {
				return aliasOf[mp.ImportPath] + "." + name
			}
		}
		return ref
	})
	for _, mp := range m.Mappings {
		// The emitter qualifies a foreign module's types with the module name in lower case.
		if alias, qual := aliasOf[mp.ImportPath], strings.ToLower(mp.Module); alias != qual {
			src = qualifiedRef(qual).ReplaceAllString(src, alias+".$1")
		}
	}
	var specs []string
	for importPath, alias := range aliasOf {
		if qualifiedRef(alias).MatchString(src) {
			specs = append(specs, importSpec(alias, importPath))
		}
	}
	sort.Strings(specs)
	return src, specs
}

func qualifiedRef(alias string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\.([A-Z][A-Za-z0-9_]*)`)
}

// foreignValueName is the runtime-style name of a value declared outside the transpiled modules, such as a
// C macro or a global of an imported Swift module, when a mapped package exports it; otherwise "".
func (m *Module) foreignValueName(usr, name string, sig declSig) string {
	var cand string
	switch {
	case strings.HasPrefix(usr, "c:objc("):
		return ""
	case strings.HasPrefix(usr, "c:"):
		cand = "CV" + name
	case sig.owner != "" && !strings.Contains(sig.owner, "."):
		cand = capitalize(sig.base)
	case sig.owner != "":
		cand = lastComponent(sig.owner) + capitalize(sig.base)
	}
	if cand != "" && m.mappedName(cand) {
		return cand
	}
	return ""
}

// importSpec renders an import of importPath, naming it pkg when the path's last element differs.
func importSpec(pkg, importPath string) string {
	if path.Base(importPath) == pkg {
		return `"` + importPath + `"`
	}
	return pkg + ` "` + importPath + `"`
}

// mappedModule returns the mapping for a Swift module name, or nil.
func (m *Module) mappedModule(module string) *Mapping {
	for i := range m.Mappings {
		if m.Mappings[i].Module == module {
			return &m.Mappings[i]
		}
	}
	return nil
}

// mappedTypeArgs renders the type arguments of a type from a mapped module, "[A, B]", when the mapped package
// declares the Go type generic with that many parameters; otherwise "".
func (m *Module) mappedTypeArgs(t *types.Type, generics map[string]string) string {
	if t == nil || t.Kind != types.Named || len(t.Args) == 0 {
		return ""
	}
	mod, rest, ok := strings.Cut(t.Name, ".")
	if !ok {
		return ""
	}
	mp := m.mappedModule(mod)
	if mp == nil || mp.Generic[strings.ReplaceAll(rest, ".", "")] != len(t.Args) {
		return ""
	}
	args := make([]string, len(t.Args))
	for i, a := range t.Args {
		args[i] = m.GoType(a, generics)
	}
	return "[" + strings.Join(args, ", ") + "]"
}

// mappedResultTypeArg is "[R]", R the Go type of the call's result, for a call to a mapped function whose result
// depends on a generic parameter Go cannot infer from the arguments: one the function declares itself or, for
// a free or static function (a call with no receiver), one of the generic type it belongs to. Mapped functions
// take the result type as their first type parameter.
func (e *fileEmitter) mappedResultTypeArg(name, dem string, n ast.Node, static bool) string {
	if !e.m.mappedName(name) {
		return ""
	}
	arrow := strings.LastIndex(dem, " -> ")
	if arrow < 0 {
		return ""
	}
	result := dem[arrow+4:]
	depends := static && genericRef.MatchString(result)
	for _, p := range ownGenericParams(dem) {
		if regexp.MustCompile(`(^|[^\w.])` + p + `($|[^\w.])`).MatchString(result) {
			depends = true
		}
	}
	if gt := e.exprGoType(n); depends && gt != "" && !isVoidGo(gt) {
		return "[" + gt + "]"
	}
	return ""
}

// ownGenericParams names the generic parameters a demangled function declares itself, as its signature
// refers to them. The demangler lists them without depth but refers to them with the depth of the generic
// context they belong to ("checkArgument<A>(Swift.Int32) -> A1" inside a generic extension declares A1), and
// the deepest depth the signature uses is the function's own.
func ownGenericParams(dem string) []string {
	s := dem
	if strings.HasPrefix(s, "(extension in ") {
		if i := strings.Index(s, "):"); i >= 0 {
			s = s[i+2:]
		}
	}
	paren := strings.IndexByte(s, '(')
	if paren < 0 || paren == 0 || s[paren-1] != '>' {
		return nil
	}
	head := s[:paren]
	depth, open := 0, -1
	for i := len(head) - 1; i >= 0 && open < 0; i-- {
		switch head[i] {
		case '>':
			depth++
		case '<':
			depth--
			if depth == 0 {
				open = i
			}
		}
	}
	if open <= 0 {
		return nil
	}
	suffix := ""
	for _, m := range genericRef.FindAllStringSubmatch(s[paren:], -1) {
		if d := m[3]; len(d) > len(suffix) || len(d) == len(suffix) && d > suffix {
			suffix = d
		}
	}
	var names []string
	list, _, _ := strings.Cut(head[open+1:len(head)-1], " where ")
	for _, p := range strings.Split(list, ",") {
		if f := strings.Fields(p); len(f) > 0 {
			names = append(names, strings.TrimSuffix(f[0], ":")+suffix)
		}
	}
	return names
}

var genericRef = regexp.MustCompile(`(^|[^\w.])([A-Z])(\d*)\b`)

var callHead = regexp.MustCompile(`^(\w+)\.([A-Za-z_][A-Za-z0-9_]*)[\[(]`)

// mappedCall reports whether a rendered expression is a single call to a function of a mapped package.
func (m *Module) mappedCall(src string) bool {
	sm := callHead.FindStringSubmatch(src)
	if sm == nil {
		return false
	}
	switch {
	case sm[1] == "swiftrt" && m.mappedName(sm[2]):
	case sm[1] != "swiftrt" && slices.ContainsFunc(m.Mappings, func(mp Mapping) bool { return mp.Alias() == sm[1] }):
	default:
		return false
	}
	open := strings.IndexByte(src, '(')
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i == len(src)-1
			}
		}
	}
	return false
}

// mappedCase names the constant for a payload-free case of an enum from a mapped module, as the emitter names
// foreign enum cases in expressions; "" when the pattern has a payload or the package does not export it.
func (e *fileEmitter) mappedCase(p ast.Node, st *types.Type) string {
	if p.Child("sub_pattern") != nil {
		return ""
	}
	for st != nil && st.Kind == types.Optional && len(st.Args) == 1 {
		st = st.Args[0]
	}
	if st == nil || st.Kind != types.Named {
		return ""
	}
	if mp := e.m.mappedTypeModule(st.Name); mp != nil {
		return mp.Alias() + "." + mappedOwner(st.Name) + capitalize(p.Child("element").Child("base_name").Str("name"))
	}
	name := lastComponent(st.Name) + capitalize(p.Child("element").Child("base_name").Str("name"))
	if !e.m.mappedName(name) {
		return ""
	}
	return "swiftrt." + name
}
