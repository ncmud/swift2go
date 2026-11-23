package emit

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ncmud/swift2go/internal/ast"
)

// emitCacheFormat changes whenever what the emit cache stores, or what an entry's reuse depends on,
// changes.
const emitCacheFormat = "swift2go-emit-cache-2"

// The kinds of state a file's emission reads that other files decide.
const (
	obsEffect    = "fx"      // whether a function may write a shared place (effects.go)
	obsInit      = "init"    // a stored property's initial value
	obsHoist     = "hoist"   // the module's count of hoisted local declarations
	obsImport    = "import"  // the import path the module recorded for a package
	obsFrozen    = "frozen"  // whether a struct's stored properties are all `let` (frozenStruct)
	claimAdapter = "adapter" // an adapter type emitted once per module (foreign.go)
	claimRetro   = "retro"   // a method added once to a dependency's type (retroactive conformances)
)

// emitCache is what an incremental [Module.WriteGo] keeps between runs: the environment every Go file
// was written in, and what each Swift file's Go file read from the rest of the program.
type emitCache struct {
	Env   string
	Files map[string]*fileRecord // by Go file name
}

// fileRecord describes the Go file written for one Swift file: the AST it was written from, what its
// emission read that other files decide, and what it changed that later files read.
type fileRecord struct {
	Source string
	Hash   string // the AST's [ast.File.Hash]
	Out    string // hash of the Go file written
	// Obs maps an observation key (kind, module, name) to the value the emission read.
	Obs    map[string]string
	Hoists map[string]int // module → hoisted declarations numbered
	Claims []string       // claim keys (kind, module, name) taken
	Retro  []retroChunk
	Unsup  []unsupNote
	// Imports lists the packages the emission recorded as imported, as (module, pkg, path).
	Imports [][3]string

	own map[string]bool // claim and import keys this emission set itself
}

type retroChunk struct{ Module, Pkg, Text string }

type unsupNote struct{ Module, Kind, Where string }

func obsKey(kind, mod, name string) string { return kind + "\x00" + mod + "\x00" + name }

func boolObs(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// observe records what the emission of the current file read, unless it read the same thing before.
func (m *Module) observe(kind, mod, name, value string) {
	if r := m.rec; r != nil {
		k := obsKey(kind, mod, name)
		if _, ok := r.Obs[k]; !ok {
			r.Obs[k] = value
		}
	}
}

// taken reports whether name is in set, a module-wide record of what earlier code emitted once.
func (m *Module) taken(kind string, set map[string]bool, name string) bool {
	if r := m.rec; r != nil {
		k := obsKey(kind, m.Name, name)
		if !r.own[k] {
			m.observe(kind, m.Name, name, boolObs(set[name]))
		}
	}
	return set[name]
}

// take adds name to set.
func (m *Module) take(kind string, set map[string]bool, name string) {
	set[name] = true
	if r := m.rec; r != nil {
		k := obsKey(kind, m.Name, name)
		if !r.own[k] {
			r.own[k] = true
			r.Claims = append(r.Claims, k)
		}
	}
}

// claim adds name to set and reports whether it was missing.
func (m *Module) claim(kind string, set map[string]bool, name string) bool {
	if m.taken(kind, set, name) {
		return false
	}
	m.take(kind, set, name)
	return true
}

// importPkg records that the module's Go code refers to dependency d's package.
func (m *Module) importPkg(d *Module) {
	m.importPkgs[d.Pkg] = d.ImportPath
	if r := m.rec; r != nil {
		k := obsKey(obsImport, m.Name, d.Pkg)
		if !r.own[k] {
			r.own[k] = true
			r.Imports = append(r.Imports, [3]string{m.Name, d.Pkg, d.ImportPath})
		}
	}
}

// importedPackages is the import path of each recorded package that the Go source src refers to.
func (m *Module) importedPackages(src string) map[string]string {
	out := map[string]string{}
	for pkg, path := range m.importPkgs {
		if strings.Contains(src, pkg+".") {
			out[pkg] = path
		}
	}
	if r := m.rec; r != nil {
		// Another file may record a package that src mentions, so which ones were recorded counts.
		pkgs := map[string]bool{}
		for _, d := range m.Deps {
			pkgs[d.Pkg] = true
		}
		for pkg := range m.importPkgs {
			pkgs[pkg] = true
		}
		for pkg := range pkgs {
			if strings.Contains(src, pkg+".") && !r.own[obsKey(obsImport, m.Name, pkg)] {
				m.observe(obsImport, m.Name, pkg, m.importPkgs[pkg])
			}
		}
	}
	return out
}

// initialValue is the expression that initializes the stored property owner|name, or nil.
func (m *Module) initialValue(key string) ast.Node {
	owner, init := m, m.varInit[key]
	for _, d := range m.Deps {
		if init != nil {
			break
		}
		// A dependency's struct, whose implicit memberwise initializer takes the property's initial value.
		owner, init = d, d.varInit[key]
	}
	if init == nil {
		owner = m
	}
	if m.rec != nil {
		m.observe(obsInit, owner.Name, key, hashNode(init))
	}
	return init
}

func (m *Module) noteHoist() {
	if r := m.rec; r != nil {
		if r.Hoists[m.Name] == 0 {
			m.observe(obsHoist, m.Name, "", strconv.Itoa(m.hoists))
		}
		r.Hoists[m.Name]++
	}
}

func (m *Module) noteRetro(pkg, text string) {
	if r := m.rec; r != nil {
		r.Retro = append(r.Retro, retroChunk{m.Name, pkg, text})
	}
}

func (m *Module) noteUnsupported(kind, where string) {
	if r := m.rec; r != nil {
		r.Unsup = append(r.Unsup, unsupNote{m.Name, kind, where})
	}
}

// chain is the module and its dependencies, by name.
func (m *Module) chain() map[string]*Module {
	mods := map[string]*Module{m.Name: m}
	for _, d := range m.Deps {
		if _, ok := mods[d.Name]; !ok {
			mods[d.Name] = d
		}
	}
	return mods
}

// recordInto directs what the emission of the current file reads and changes into r; nil stops it.
func (m *Module) recordInto(r *fileRecord) {
	m.rec = r
	for _, d := range m.Deps {
		d.rec = r
	}
}

// holds reports whether every observation of r reads the same now.
func (m *Module) holds(r *fileRecord, mods map[string]*Module) bool {
	for k, want := range r.Obs {
		kind, rest, _ := strings.Cut(k, "\x00")
		modName, name, _ := strings.Cut(rest, "\x00")
		mod := mods[modName]
		var got string
		switch kind {
		case obsEffect:
			got = boolObs(m.mayWriteFact(name))
		case obsInit:
			if mod == nil {
				return false
			}
			got = hashNode(mod.varInit[name])
		case obsHoist:
			if mod == nil {
				return false
			}
			got = strconv.Itoa(mod.hoists)
		case obsImport:
			if mod == nil {
				return false
			}
			got = mod.importPkgs[name]
		case obsFrozen:
			if mod == nil {
				return false
			}
			got = boolObs(mod.frozenStruct(name))
		case claimAdapter:
			if mod == nil {
				return false
			}
			got = boolObs(mod.adapters[name])
		case claimRetro:
			if mod == nil {
				return false
			}
			got = boolObs(mod.retroDone[name])
		default:
			return false
		}
		if got != want {
			return false
		}
	}
	return true
}

// replay applies to the modules what the emission r describes changed, as if the file were emitted.
func (m *Module) replay(r *fileRecord, mods map[string]*Module) {
	for name, n := range r.Hoists {
		mods[name].hoists += n
	}
	for _, k := range r.Claims {
		kind, rest, _ := strings.Cut(k, "\x00")
		modName, name, _ := strings.Cut(rest, "\x00")
		mod := mods[modName]
		switch kind {
		case claimAdapter:
			if mod.adapters == nil {
				mod.adapters = map[string]bool{}
			}
			mod.adapters[name] = true
		case claimRetro:
			mod.retroDone[name] = true
		}
	}
	for _, imp := range r.Imports {
		mods[imp[0]].importPkgs[imp[1]] = imp[2]
	}
	for _, c := range r.Retro {
		mod := mods[c.Module]
		b := mod.retroFiles[c.Pkg]
		if b == nil {
			b = &strings.Builder{}
			mod.retroFiles[c.Pkg] = b
		}
		b.WriteString(c.Text)
	}
	for _, u := range r.Unsup {
		mods[u.Module].countUnsupported(u.Kind, u.Where)
	}
}

// reusable reports whether the Go file out, written by an earlier run for f as r describes, is what
// emitting f now would write.
func (m *Module) reusable(r *fileRecord, f ast.File, out string, mods map[string]*Module) bool {
	if r == nil || f.Hash == "" || r.Hash != f.Hash || r.Source != f.Filename {
		return false
	}
	for modName := range r.Hoists {
		if mods[modName] == nil {
			return false
		}
	}
	for _, imp := range r.Imports {
		if mods[imp[0]] == nil {
			return false
		}
	}
	for _, c := range r.Retro {
		if mods[c.Module] == nil || mods[c.Module].retroFiles == nil {
			return false
		}
	}
	for _, u := range r.Unsup {
		if mods[u.Module] == nil {
			return false
		}
	}
	for _, k := range r.Claims {
		kind, rest, _ := strings.Cut(k, "\x00")
		modName, _, _ := strings.Cut(rest, "\x00")
		if mods[modName] == nil || kind == claimRetro && mods[modName].retroDone == nil {
			return false
		}
	}
	if !m.holds(r, mods) {
		return false
	}
	data, err := os.ReadFile(out) //nolint:gosec // out is a file this module writes
	return err == nil && hashBytes(data) == r.Out
}

func newRecord(f ast.File) *fileRecord {
	return &fileRecord{Source: f.Filename, Hash: f.Hash, Obs: map[string]string{}, Hoists: map[string]int{}, own: map[string]bool{}}
}

// emitEnv identifies everything besides the files' own ASTs and the observations of each file that
// decides the Go of the module: the transpiler, the options, and the declarations of the module and
// its dependencies. It is "" when the module cannot keep Go files between runs.
func (m *Module) emitEnv(overlay map[string]bool) string {
	if m.EmitCache == "" || m.Interface == "" || m.EmitKey == "" {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", emitCacheFormat, m.EmitKey)
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%v\x00%s\x00%v\x00", m.Name, m.Pkg, m.ImportPath, m.RTPath, m.Test, m.Target, m.corelibsFoundation)
	fmt.Fprintf(h, "overlay %q\x00", slices.Sorted(maps.Keys(overlay)))
	mappings, err := json.Marshal(m.Mappings)
	if err != nil {
		return ""
	}
	h.Write(mappings)
	for _, f := range m.Files {
		fmt.Fprintf(h, "file %s\x00", f.Filename)
	}
	fmt.Fprintf(h, "self %s\x00%s\x00", m.Interface, m.declDigest())
	for _, d := range m.Deps {
		if d.Interface == "" {
			return ""
		}
		fmt.Fprintf(h, "dep %s\x00%s\x00%s\x00%v\x00%s\x00%s\x00", d.Name, d.Pkg, d.ImportPath, d.Mapped, d.Interface, d.declDigest())
	}
	return hex.EncodeToString(h.Sum(nil))
}

// declDigest identifies what indexing derived from the module's files beyond their declarations'
// fingerprints: names given to declarations, including types local to function bodies, and the flags
// read from initial values.
func (m *Module) declDigest() string {
	lines := make([]string, 0, len(m.decls))
	for key, d := range m.decls {
		if strings.HasPrefix(key, "nousr:") {
			// The key holds a source offset, which edits elsewhere in the file move.
			key = key[:strings.LastIndex(key, "@")]
		}
		info := *d
		info.Node = nil
		lines = append(lines, fmt.Sprintf("%s\x00%+v", key, info))
	}
	slices.Sort(lines)
	h := sha256.New()
	for _, l := range lines {
		fmt.Fprintf(h, "%s\n", l)
	}
	writeTable(h, "kinds", m.typeKinds)
	writeTable(h, "aliases", m.typeAliases)
	writeTable(h, "objc", m.objcNames)
	writeTable(h, "lazy", m.lazyInits)
	writeTable(h, "overridden", m.overridden)
	writeTable(h, "conformers", m.conformers)
	writeTable(h, "parents", m.protoParents)
	writeTable(h, "typeparams", m.typeParamNames)
	reqs := map[string]int{}
	for p, rs := range m.protoReqs {
		reqs[p] = len(rs)
	}
	writeTable(h, "reqs", reqs)
	return hex.EncodeToString(h.Sum(nil))
}

func writeTable[V any](h hash.Hash, name string, t map[string]V) {
	fmt.Fprintf(h, "%s\n", name)
	for _, k := range slices.Sorted(maps.Keys(t)) {
		fmt.Fprintf(h, "%q=%v\n", k, t[k])
	}
}

// hashNode identifies an AST subtree apart from source offsets, which edits before it move, and the
// dump's numbering of pointers, which edits anywhere in the file move. Offsets of magic literals
// (#line), which the Go spells out, count. A nil node hashes as "".
func hashNode(n ast.Node) string {
	if n == nil {
		return ""
	}
	h := sha256.New()
	writeNode(h, map[string]any(n), map[string]int{})
	return hex.EncodeToString(h.Sum(nil))
}

func writeNode(w io.Writer, v any, ptrs map[string]int) {
	switch n := v.(type) {
	case map[string]any:
		magic := n["_kind"] == "magic_identifier_literal_expr"
		_, _ = io.WriteString(w, "{")
		for _, k := range slices.Sorted(maps.Keys(n)) {
			if k == "range" && !magic {
				continue
			}
			fmt.Fprintf(w, "%q:", k)
			writeNode(w, n[k], ptrs)
			_, _ = io.WriteString(w, ",")
		}
		_, _ = io.WriteString(w, "}")
	case []any:
		_, _ = io.WriteString(w, "[")
		for _, c := range n {
			writeNode(w, c, ptrs)
			_, _ = io.WriteString(w, ",")
		}
		_, _ = io.WriteString(w, "]")
	case string:
		if strings.HasPrefix(n, "replaced-pointer-") {
			id, ok := ptrs[n]
			if !ok {
				id = len(ptrs)
				ptrs[n] = id
			}
			fmt.Fprintf(w, "ptr%d", id)
			return
		}
		fmt.Fprintf(w, "%q", n)
	default:
		data, _ := json.Marshal(n)
		_, _ = w.Write(data)
	}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func readEmitCache(path, env string) *emitCache {
	if path == "" || env == "" {
		return nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is the module's cache file
	if err != nil {
		return nil
	}
	var c emitCache
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&c); err != nil || c.Env != env {
		return nil
	}
	return &c
}

func writeEmitCache(path string, c *emitCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(c); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeIfChanged writes data to path unless the file already holds it, so its modification time
// says when its content last changed.
func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) { //nolint:gosec // path is a file this module writes
		return nil
	}
	return os.WriteFile(path, data, 0o644) //nolint:gosec // path is a file this module writes
}
