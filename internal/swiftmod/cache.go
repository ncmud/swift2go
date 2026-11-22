package swiftmod

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
)

// cacheFormat changes whenever what the cache stores, or how a fingerprint is computed, changes.
const cacheFormat = "swift2go-ast-cache-2"

// CachedDump is the typed AST of a target, one JSON document per source file in source order.
type CachedDump struct {
	// Docs holds each file's document, nil for a file the compiler could not dump.
	Docs [][]byte
	// Interface identifies the declarations the target's files make visible to one another and to
	// the targets that import it. Two dumps with the same Interface type-check every other file the
	// same way.
	Interface string
	// Dumped counts the files dumped in this call; the others came from the cache.
	Dumped int
}

// DumpFunc dumps the sources at the indices in primaries, reading the copies in place of the
// files they belong to; see [Dumper.Dump].
type DumpFunc func(primaries []int, copies map[int]string) ([]FileDump, error)

type cacheEntry struct {
	Hash      string `json:"hash"`
	Interface string `json:"interface"`
	Copy      string `json:"copy,omitempty"`
	// Failed marks a file the compiler could not dump; it has no document.
	Failed bool `json:"failed,omitempty"`
}

type manifest struct {
	Env   string                `json:"env"`
	Files map[string]cacheEntry `json:"files"`
}

// DumpCached dumps sources through dump, reusing the documents an earlier call cached in dir.
// env identifies everything besides the sources that the documents depend on: the compiler and
// its flags, and the interfaces of the modules the target imports.
//
// When env and the list of sources are unchanged, only the files whose content changed are
// dumped. If each of them keeps the [Fingerprint] it had, the other files' documents still hold
// and come from the cache; otherwise every file is dumped again, since any of them may
// type-check differently against the changed declarations.
func DumpCached(dir, env string, sources []string, dump DumpFunc) (CachedDump, error) {
	env = envKey(env, sources)
	hashes, err := hashFiles(sources)
	if err != nil {
		return CachedDump{}, err
	}
	old := readCacheManifest(dir)
	if old.Env == env {
		d, ok, err := dumpChanged(dir, sources, hashes, old, dump)
		if err != nil || ok {
			return d, err
		}
	}
	return dumpAll(dir, env, sources, hashes, dump)
}

// dumpChanged dumps the files whose content changed since old was written and, when none of
// their fingerprints changed, completes the dump from the cache. It reports false when the
// whole target must be dumped again.
func dumpChanged(dir string, sources, hashes []string, old manifest, dump DumpFunc) (CachedDump, bool, error) {
	var changed []int
	copies := map[int]string{}
	for i, src := range sources {
		e, ok := old.Files[src]
		if !ok || e.Hash != hashes[i] || !e.Failed && !exists(docPath(dir, src)) || e.Copy != "" && !exists(e.Copy) {
			changed = append(changed, i)
			continue
		}
		if e.Copy != "" {
			copies[i] = e.Copy
		}
	}
	m := manifest{Env: old.Env, Files: map[string]cacheEntry{}}
	d := CachedDump{Docs: make([][]byte, len(sources)), Dumped: len(changed)}
	if len(changed) > 0 {
		fresh, err := dump(changed, copies)
		if err != nil {
			return CachedDump{}, false, err
		}
		for k, i := range changed {
			e := entry(hashes[i], fresh[k])
			if prev, ok := old.Files[sources[i]]; !ok || prev.Interface != e.Interface || prev.Failed != e.Failed {
				return CachedDump{}, false, nil
			}
			d.Docs[i] = fresh[k].Doc
			m.Files[sources[i]] = e
		}
	}
	missing := make([]bool, len(sources))
	parallel(len(sources), func(i int) {
		if _, ok := m.Files[sources[i]]; ok || old.Files[sources[i]].Failed {
			return
		}
		doc, err := os.ReadFile(docPath(dir, sources[i]))
		d.Docs[i], missing[i] = doc, err != nil
	})
	if slices.Contains(missing, true) {
		return CachedDump{}, false, nil
	}
	for i, src := range sources {
		if _, ok := m.Files[src]; ok {
			if err := writeDoc(dir, src, d.Docs[i]); err != nil {
				return CachedDump{}, false, err
			}
			continue
		}
		m.Files[src] = old.Files[src]
	}
	if len(changed) > 0 {
		if err := writeManifest(dir, m); err != nil {
			return CachedDump{}, false, err
		}
	}
	d.Interface = interfaceKey(sources, m)
	return d, true, nil
}

func dumpAll(dir, env string, sources, hashes []string, dump DumpFunc) (CachedDump, error) {
	all := make([]int, len(sources))
	for i := range all {
		all[i] = i
	}
	fresh, err := dump(all, nil)
	if err != nil {
		return CachedDump{}, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return CachedDump{}, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		return CachedDump{}, err
	}
	m := manifest{Env: env, Files: map[string]cacheEntry{}}
	entries := make([]cacheEntry, len(sources))
	parallel(len(sources), func(i int) { entries[i] = entry(hashes[i], fresh[i]) })
	d := CachedDump{Docs: make([][]byte, len(sources)), Dumped: len(sources)}
	for i, src := range sources {
		if err := writeDoc(dir, src, fresh[i].Doc); err != nil {
			return CachedDump{}, err
		}
		m.Files[src] = entries[i]
		d.Docs[i] = fresh[i].Doc
	}
	if err := writeManifest(dir, m); err != nil {
		return CachedDump{}, err
	}
	d.Interface = interfaceKey(sources, m)
	return d, nil
}

func entry(hash string, f FileDump) cacheEntry {
	return cacheEntry{Hash: hash, Interface: Fingerprint(f.Doc), Copy: f.Copy, Failed: f.Doc == nil}
}

// interfaceKey combines the fingerprints of a target's files. A file the compiler could not dump
// counts by its content.
func interfaceKey(sources []string, m manifest) string {
	h := sha256.New()
	for _, src := range sources {
		e := m.Files[src]
		if e.Failed {
			fmt.Fprintf(h, "%s\x00failed:%s\x00", src, e.Hash)
			continue
		}
		fmt.Fprintf(h, "%s\x00%s\x00", src, e.Interface)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func envKey(env string, sources []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", cacheFormat, env)
	for _, s := range sources {
		fmt.Fprintf(h, "%s\x00", s)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func docPath(dir, src string) string {
	sum := sha256.Sum256([]byte(src))
	return filepath.Join(dir, "files", strings.TrimSuffix(filepath.Base(src), ".swift")+"-"+hex.EncodeToString(sum[:6])+".json")
}

// writeDoc caches a file's document, or removes the cached one when the file has none.
func writeDoc(dir, src string, doc []byte) error {
	if doc == nil {
		if err := os.Remove(docPath(dir, src)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(docPath(dir, src), doc, 0o644)
}

func readCacheManifest(dir string) manifest {
	var m manifest
	if data, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

func writeManifest(dir string, m manifest) error {
	data, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "manifest.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "manifest.json"))
}

func hashFiles(paths []string) ([]string, error) {
	hashes := make([]string, len(paths))
	errs := make([]error, len(paths))
	parallel(len(paths), func(i int) {
		f, err := os.Open(paths[i])
		if err != nil {
			errs[i] = err
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			errs[i] = err
			return
		}
		hashes[i] = hex.EncodeToString(h.Sum(nil))
	})
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

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

// bodyKinds are the declarations whose bodies no other file type-checks against.
var bodyKinds = map[string]bool{
	"func_decl": true, "constructor_decl": true, "destructor_decl": true, "accessor_decl": true,
}

// Fingerprint hashes what a file's JSON AST shows other files: every declaration with its types,
// attributes and conformances, but not the bodies of functions, initializers and accessors, nor
// the expressions that initialize variables, whose types the AST records with the variable, nor
// source ranges and the dump's numbering of pointers, which shift when a body is edited. The body
// or initial value that decides the underlying type of an opaque type (`some P`) counts. A nil
// document has the empty fingerprint.
func Fingerprint(doc []byte) string {
	if doc == nil {
		return ""
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		sum := sha256.Sum256(doc)
		return "raw:" + hex.EncodeToString(sum[:])
	}
	h := sha256.New()
	writeCanonical(h, stripBodies(v))
	return hex.EncodeToString(h.Sum(nil))
}

func stripBodies(v any) any {
	switch n := v.(type) {
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
				out[k] = stripBodies(c)
			}
		}
		return out
	case []any:
		out := make([]any, len(n))
		for i, c := range n {
			out[i] = stripBodies(c)
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

func writeCanonical(w io.Writer, v any) {
	switch n := v.(type) {
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

// DependencyKey identifies the interfaces of the modules t imports, directly or not. A module
// dumped in this run is identified by its [CachedDump.Interface] in ifaces; any other by the
// modification time of its built module in searchDirs, or of its sources for a C target.
func DependencyKey(t Target, targets map[string]Target, ifaces map[string]string, searchDirs []string) string {
	seen := map[string]bool{}
	var names []string
	var visit func(deps []string)
	visit = func(deps []string) {
		for _, n := range deps {
			if seen[n] {
				continue
			}
			seen[n] = true
			names = append(names, n)
			if d, ok := targets[n]; ok {
				visit(d.AllDeps())
			}
		}
	}
	visit(t.AllDeps())
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s=", n)
		dep, ok := targets[n]
		switch {
		case ifaces[n] != "":
			b.WriteString(ifaces[n])
		case ok && dep.ModuleType != "SwiftTarget":
			b.WriteString(modTime(filepath.Join(dep.Dir, dep.Path)).String())
		default:
			modName := n
			if ok && dep.C99Name != "" {
				modName = dep.C99Name
			}
			for _, dir := range searchDirs {
				if mod := filepath.Join(dir, modName+".swiftmodule"); exists(mod) {
					b.WriteString(modTime(mod).String())
					break
				}
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Key identifies the frontend command, apart from the files it reads, and the compiler that runs it.
func (d *Dumper) Key() string {
	return fmt.Sprintf("%s\x00%s\x00%s", strings.Join(d.job, "\x00"), frontendVersion(d.job[0]), modTime(d.job[0]))
}
