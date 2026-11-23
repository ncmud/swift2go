package deps

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// keptTables is how many earlier tables a [Store] keeps.
const keptTables = 8

type tableEntry struct {
	Key    Key
	Digest string
}

// Store keeps the latest tables of one module in a directory, by hash, for the modules that depend on it
// to compare the table they last saw with the current one.
type Store string

// Save keeps t and drops all but the most recently saved tables.
func (s Store) Save(t Table) error {
	if err := os.MkdirAll(string(s), 0o755); err != nil {
		return err
	}
	file := filepath.Join(string(s), t.Hash()+".gob")
	if _, err := os.Stat(file); err == nil {
		now := time.Now()
		return os.Chtimes(file, now, now)
	}
	es := make([]tableEntry, 0, len(t))
	for _, k := range sortedKeys(t) {
		es = append(es, tableEntry{k, t[k]})
	}
	if err := WriteGob(file, es); err != nil {
		return err
	}
	entries, err := os.ReadDir(string(s))
	if err != nil || len(entries) <= keptTables {
		return err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		ia, _ := a.Info()
		ib, _ := b.Info()
		if ia == nil || ib == nil {
			return 0
		}
		return ib.ModTime().Compare(ia.ModTime())
	})
	for _, e := range entries[keptTables:] {
		_ = os.Remove(filepath.Join(string(s), e.Name()))
	}
	return nil
}

// loaded memoizes the tables [Store.Load] read: every target of a run compares against its
// dependencies' earlier tables, and a table's file never changes.
var loaded sync.Map // path → Table

// Load returns the table with the given hash, if the store still keeps it. Callers must not modify it.
func (s Store) Load(hash string) (Table, bool) {
	if s == "" || hash == "" {
		return nil, false
	}
	path := filepath.Join(string(s), hash+".gob")
	if t, ok := loaded.Load(path); ok {
		return t.(Table), true
	}
	var es []tableEntry
	if !ReadGob(path, &es) {
		return nil, false
	}
	t := make(Table, len(es))
	for _, e := range es {
		t[e.Key] = e.Digest
	}
	loaded.Store(path, t)
	return t, true
}

// ReadGob decodes the gob file at path into v and reports whether it could.
func ReadGob(path string, v any) bool {
	f, err := os.Open(path) //nolint:gosec // a file a cache wrote
	if err != nil {
		return false
	}
	defer f.Close()
	return gob.NewDecoder(bufio.NewReader(f)).Decode(v) == nil
}

// WriteGob writes v to path through a temporary file, so a reader never sees half of it.
func WriteGob(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(v); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
