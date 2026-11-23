package swiftmod

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/ncmud/swift2go/internal/ast"
	"github.com/ncmud/swift2go/internal/deps"
	"github.com/ncmud/swift2go/internal/types"
)

// cacheFormat changes whenever what the cache stores, or how a file's interface or names are read, changes.
const cacheFormat = "swift2go-ast-cache-3"

// CachedDump is the typed AST of a target, one JSON document per source file in source order.
type CachedDump struct {
	// Docs holds each file's document, nil for a file the compiler could not dump.
	Docs [][]byte
	// Names holds the names each file mentions (see [deps.Scan]), nil for a file without a document.
	Names []deps.Names
	// Interface is what the target's files declare for one another and for the targets that import it.
	Interface *Interface
	// Dumped counts the files dumped in this call; the others came from the cache.
	Dumped int
}

// Interface is the [deps.Table] of a dumped target.
type Interface struct {
	Table deps.Table
	// Hash identifies the table's content; two dumps with the same Hash type-check every other file the
	// same way.
	Hash  string
	store deps.Store
}

// Earlier returns the table the target had when its Hash was hash, as an earlier dump kept it.
func (i *Interface) Earlier(hash string) (deps.Table, bool) {
	if i == nil {
		return nil, false
	}
	return i.store.Load(hash)
}

// DumpFunc dumps the sources at the indices in primaries, reading the copies in place of the
// files they belong to; see [Dumper.Dump].
type DumpFunc func(primaries []int, copies map[int]string) ([]FileDump, error)

type tableEntry struct {
	Key    deps.Key
	Digest string
}

func tableEntries(t deps.Table) []tableEntry {
	out := make([]tableEntry, 0, len(t))
	for _, k := range deps.Changed(nil, t) {
		out = append(out, tableEntry{k, t[k]})
	}
	return out
}

func entriesTable(es []tableEntry) deps.Table {
	t := deps.Table{}
	for _, e := range es {
		t[e.Key] = e.Digest
	}
	return t
}

type cacheEntry struct {
	Hash string
	Copy string
	// Failed marks a file the compiler could not dump; it has no document.
	Failed bool
	// Decls is the file's part of the target's interface; Names the names it mentions.
	Decls []tableEntry
	Names deps.Names
}

type manifest struct {
	Env string
	// Imports holds the interface hash of each transpiled dependency the files were dumped against.
	Imports map[string]string
	Files   map[string]cacheEntry
}

// DumpCached dumps sources through dump, reusing the documents an earlier call cached in dir.
// env identifies everything besides the sources and the imports that the documents depend on: the
// compiler and its flags, and the modules the target imports that are not transpiled in this run.
// imports holds the interfaces of those that are.
//
// While env and the list of sources stay the same, a file is dumped again when its content changed,
// or when it mentions a declaration that changed (see [deps.Affects]): one of the imports', or one of
// another file of the target that was dumped again. Otherwise its document comes from the cache.
func DumpCached(dir, env string, imports map[string]*Interface, sources []string, dump DumpFunc) (CachedDump, error) {
	env = envKey(env, sources, imports)
	hashes, err := hashFiles(sources)
	if err != nil {
		return CachedDump{}, err
	}
	old := readCacheManifest(dir)
	if old.Env == env {
		d, ok, err := dumpChanged(dir, sources, hashes, old, imports, dump)
		if err != nil || ok {
			return d, err
		}
	}
	return dumpAll(dir, env, sources, hashes, imports, dump)
}

// dumpChanged dumps the files whose content changed since old was written and the files that
// mention what changed, and completes the dump from the cache. It reports false when the whole
// target must be dumped again.
func dumpChanged(dir string, sources, hashes []string, old manifest, imports map[string]*Interface, dump DumpFunc) (CachedDump, bool, error) {
	var changedKeys []deps.Key
	for _, name := range slices.Sorted(maps.Keys(imports)) {
		iface := imports[name]
		was := old.Imports[name]
		if was == iface.Hash {
			continue
		}
		prev, ok := iface.Earlier(was)
		if !ok {
			return CachedDump{}, false, nil
		}
		changedKeys = append(changedKeys, deps.Changed(prev, iface.Table)...)
	}
	entries := make([]cacheEntry, len(sources))
	docs := make([][]byte, len(sources))
	dumped := make([]bool, len(sources))
	var todo []int
	for i, src := range sources {
		e, ok := old.Files[src]
		entries[i] = e
		if !ok || e.Hash != hashes[i] || !e.Failed && !exists(docPath(dir, src)) || e.Copy != "" && !exists(e.Copy) {
			todo = append(todo, i)
		}
	}
	affected := func(keys []deps.Key) []int {
		var out []int
		for i, src := range sources {
			if dumped[i] || slices.Contains(todo, i) || len(keys) == 0 {
				continue
			}
			if entries[i].Failed || deps.AffectsAny(keys, src, entries[i].Names) {
				out = append(out, i)
			}
		}
		return out
	}
	todo = append(todo, affected(changedKeys)...)
	n := 0
	for len(todo) > 0 {
		slices.Sort(todo)
		copies := map[int]string{}
		for i := range sources {
			if !slices.Contains(todo, i) && entries[i].Copy != "" {
				copies[i] = entries[i].Copy
			}
		}
		fresh, err := dump(todo, copies)
		if err != nil {
			return CachedDump{}, false, err
		}
		scanned, err := scanDumps(sources, hashes, todo, fresh)
		if err != nil {
			return CachedDump{}, false, err
		}
		var keys []deps.Key
		for k, i := range todo {
			keys = append(keys, deps.Changed(entriesTable(entries[i].Decls), entriesTable(scanned[k].Decls))...)
			if entries[i].Failed != scanned[k].Failed {
				keys = append(keys, deps.All)
			}
			entries[i] = scanned[k]
			docs[i] = fresh[k].Doc
			dumped[i] = true
		}
		n += len(todo)
		todo = nil
		todo = affected(keys)
	}
	missing := make([]bool, len(sources))
	parallel(len(sources), func(i int) {
		if dumped[i] || entries[i].Failed {
			return
		}
		doc, err := os.ReadFile(docPath(dir, sources[i]))
		docs[i], missing[i] = doc, err != nil
	})
	if slices.Contains(missing, true) {
		return CachedDump{}, false, nil
	}
	if CheckIncremental {
		if err := checkKept(sources, docs, dumped, entries, dump); err != nil {
			return CachedDump{}, false, err
		}
	}
	m := manifest{Env: old.Env, Imports: importHashes(imports), Files: map[string]cacheEntry{}}
	for i, src := range sources {
		if dumped[i] {
			if err := writeDoc(dir, src, docs[i]); err != nil {
				return CachedDump{}, false, err
			}
		}
		m.Files[src] = entries[i]
	}
	if n > 0 || !maps.Equal(old.Imports, m.Imports) {
		if err := writeManifest(dir, m); err != nil {
			return CachedDump{}, false, err
		}
	}
	d, err := result(dir, sources, docs, entries)
	d.Dumped = n
	return d, true, err
}

func dumpAll(dir, env string, sources, hashes []string, imports map[string]*Interface, dump DumpFunc) (CachedDump, error) {
	all := make([]int, len(sources))
	for i := range all {
		all[i] = i
	}
	fresh, err := dump(all, nil)
	if err != nil {
		return CachedDump{}, err
	}
	entries, err := scanDumps(sources, hashes, all, fresh)
	if err != nil {
		return CachedDump{}, err
	}
	if err := os.RemoveAll(filepath.Join(dir, "files")); err != nil {
		return CachedDump{}, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		return CachedDump{}, err
	}
	m := manifest{Env: env, Imports: importHashes(imports), Files: map[string]cacheEntry{}}
	docs := make([][]byte, len(sources))
	for i, src := range sources {
		if err := writeDoc(dir, src, fresh[i].Doc); err != nil {
			return CachedDump{}, err
		}
		m.Files[src] = entries[i]
		docs[i] = fresh[i].Doc
	}
	if err := writeManifest(dir, m); err != nil {
		return CachedDump{}, err
	}
	d, err := result(dir, sources, docs, entries)
	d.Dumped = len(sources)
	return d, err
}

// result assembles the dump and keeps its interface for the targets that import it.
func result(dir string, sources []string, docs [][]byte, entries []cacheEntry) (CachedDump, error) {
	d := CachedDump{Docs: docs, Names: make([]deps.Names, len(sources))}
	table := deps.Table{}
	for i, src := range sources {
		e := entries[i]
		d.Names[i] = e.Names
		if e.Failed {
			// A file the compiler could not dump counts by its content.
			table.Add(deps.Key{File: src}, "failed:"+e.Hash)
			continue
		}
		for _, te := range e.Decls {
			table.Add(te.Key, te.Digest)
		}
	}
	d.Interface = &Interface{Table: table, Hash: table.Hash(), store: deps.Store(filepath.Join(dir, "tables"))}
	return d, d.Interface.store.Save(table)
}

func importHashes(imports map[string]*Interface) map[string]string {
	out := map[string]string{}
	for name, iface := range imports {
		out[name] = iface.Hash
	}
	return out
}

// CheckIncremental makes [DumpCached] dump the files it keeps from the cache too, and fail when a
// fresh document differs from the cached one.
var CheckIncremental bool

// checkKept dumps the files dumpChanged kept and reports those whose documents differ.
func checkKept(sources []string, docs [][]byte, dumped []bool, entries []cacheEntry, dump DumpFunc) error {
	var kept []int
	copies := map[int]string{}
	for i := range sources {
		if !dumped[i] && !entries[i].Failed {
			kept = append(kept, i)
		} else if entries[i].Copy != "" {
			copies[i] = entries[i].Copy
		}
	}
	if len(kept) == 0 {
		return nil
	}
	fresh, err := dump(kept, copies)
	if err != nil {
		return err
	}
	var stale []string
	for k, i := range kept {
		if !bytes.Equal(fresh[k].Doc, docs[i]) {
			stale = append(stale, filepath.Base(sources[i]))
		}
	}
	if len(stale) > 0 {
		return fmt.Errorf("the incremental dump kept %d documents that dumping again changes: %s", len(stale), strings.Join(stale, ", "))
	}
	return nil
}

// newDemangler demangles the symbols of the files [scanDumps] reads.
var newDemangler = func(syms []string) (func(string) string, error) {
	d, err := types.NewDemangler(syms)
	if err != nil {
		return nil, err
	}
	return d.Get, nil
}

// scanDumps reads the interface and the names of each fresh dump of the sources at the indices in which.
func scanDumps(sources, hashes []string, which []int, fresh []FileDump) ([]cacheEntry, error) {
	items := make([][]ast.Node, len(which))
	syms := make([][]string, len(which))
	errs := make([]error, len(which))
	parallel(len(which), func(k int) {
		if fresh[k].Doc == nil {
			return
		}
		files, err := ast.Load(fresh[k].Doc)
		if err != nil {
			errs[k] = fmt.Errorf("%s: %w", sources[which[k]], err)
			return
		}
		for _, f := range files {
			items[k] = append(items[k], f.Items...)
		}
		syms[k] = deps.Symbols(items[k])
	})
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	demangle, err := newDemangler(slices.Concat(syms...))
	if err != nil {
		return nil, err
	}
	out := make([]cacheEntry, len(which))
	parallel(len(which), func(k int) {
		i := which[k]
		e := cacheEntry{Hash: hashes[i], Copy: fresh[k].Copy, Failed: fresh[k].Doc == nil}
		if !e.Failed {
			src := sources[i]
			if e.Copy != "" {
				src = e.Copy
			}
			text, err := os.ReadFile(src)
			if err != nil {
				errs[k] = err
				return
			}
			table, names := deps.Scan(sources[i], items[k], text, demangle)
			e.Decls, e.Names = tableEntries(table), names
		}
		out[k] = e
	})
	return out, errors.Join(errs...)
}

func envKey(env string, sources []string, imports map[string]*Interface) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", cacheFormat, env)
	for _, s := range sources {
		fmt.Fprintf(h, "%s\x00", s)
	}
	for _, name := range slices.Sorted(maps.Keys(imports)) {
		fmt.Fprintf(h, "import %s\x00", name)
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
	if !deps.ReadGob(filepath.Join(dir, "manifest.gob"), &m) {
		return manifest{}
	}
	return m
}

func writeManifest(dir string, m manifest) error {
	return deps.WriteGob(filepath.Join(dir, "manifest.gob"), m)
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

// DependencyKey identifies the modules t imports, directly or not, that are not dumped in this run, by
// the modification time of the built module in searchDirs, or of the sources for a C target. A module
// dumped in this run, which has an interface in ifaces, counts by name: [DumpCached] follows its
// declarations.
func DependencyKey(t Target, targets map[string]Target, ifaces map[string]*Interface, searchDirs []string) string {
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
		case ifaces[n] != nil:
			b.WriteString("dumped")
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
