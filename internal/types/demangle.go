// Package types demangles Swift symbols and maps Swift types to Go types.
package types

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// Demangler caches demangled names for mangled types ("$s…") and USRs ("s:…").
type Demangler struct {
	cache    map[string]string
	distinct map[string]bool
}

// NewDemangler demangles every symbol in syms in one swift-demangle run.
func NewDemangler(syms []string) (*Demangler, error) {
	return NewDemanglerDistinct(syms, nil)
}

// NewDemanglerDistinct is NewDemangler for a module in which the file-private or function-local types named
// in distinct share their name with another of its types. Each such type is named apart: "Foo #1 in
// Mod.Type.run()" becomes "Mod.Foo_run", and a file-private "(Foo in _4A3B…)" becomes "Foo_4A3B…" with
// the first eight digits of its discriminator.
func NewDemanglerDistinct(syms []string, distinct map[string]bool) (*Demangler, error) {
	d := &Demangler{cache: map[string]string{}, distinct: distinct}
	return d, d.Add(syms)
}

func mangledForm(s string) string {
	if strings.HasPrefix(s, "s:") {
		return "$s" + s[2:]
	}
	return s
}

// Add demangles symbols not yet cached.
func (d *Demangler) Add(syms []string) error {
	var todo []string
	seen := map[string]bool{}
	for _, s := range syms {
		if s == "" || seen[s] {
			continue
		}
		if _, ok := d.cache[s]; ok {
			continue
		}
		seen[s] = true
		todo = append(todo, s)
	}
	if len(todo) == 0 {
		return nil
	}
	// swift-demangle answers one line per symbol; chunks of the list run in parallel.
	n := max(1, min(runtime.NumCPU(), len(todo)/demangleChunk))
	results := make([]string, len(todo))
	errs := make([]error, n)
	var wg sync.WaitGroup
	for c := range n {
		lo, hi := c*len(todo)/n, (c+1)*len(todo)/n
		wg.Go(func() { errs[c] = demangleInto(todo[lo:hi], results[lo:hi], d.distinct) })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	for i, s := range todo {
		d.cache[s] = results[i]
	}
	return nil
}

// demangleChunk is the fewest symbols worth a swift-demangle process of their own.
var demangleChunk = 4096

// demangleInto demangles syms in one swift-demangle run and stores each one's normalized form at
// the same index of out.
func demangleInto(syms, out []string, distinct map[string]bool) error {
	var in bytes.Buffer
	for _, s := range syms {
		in.WriteString(mangledForm(s))
		in.WriteByte('\n')
	}
	cmd := exec.Command("xcrun", "swift-demangle", "--compact")
	cmd.Stdin = &in
	data, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("swift-demangle: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != len(syms) {
		return fmt.Errorf("swift-demangle: %d lines for %d symbols", len(lines), len(syms))
	}
	raws := make([]string, len(syms))
	for i := range syms {
		raws[i] = privateName.ReplaceAllString(nameApart(stripClosures(lines[i]), distinct), "$1")
	}
	if err := spellLocals(syms, raws, distinct); err != nil {
		return err
	}
	for i, s := range syms {
		out[i] = localWrappers(s, raws[i], normalizeLocal(raws[i]))
	}
	return nil
}

// spellLocals rewrites, in each demangled symbol that names function-local types more than once
// ("(inout TaskGroup<Foo #1 in f() -> Swift.Int?>) -> Foo #1 in f() -> Swift.Int?"), every local type to
// its normalized name, keeping the optionals that follow it. Such a symbol mangles the local type once
// and refers back to it, so [localWrappers] cannot read each occurrence's optionals from the mangling;
// instead each local type's mangling is demangled on its own, which gives the exact text of its context,
// and the "?" that follow that text in the symbol are the type's optionals.
func spellLocals(syms, raws []string, distinct map[string]bool) error {
	var bare []string
	owner := map[string][]int{}
	for i, s := range syms {
		if len(localStart.FindAllStringIndex(raws[i], 2)) < 2 || !strings.HasSuffix(s, "D") {
			continue
		}
		m := mangledForm(s)
		for _, mark := range localMangling.FindAllStringIndex(m, -1) {
			for j := 2; j < mark[0]; j++ {
				if m[j] >= '1' && m[j] <= '9' && (m[j-1] < '0' || m[j-1] > '9') {
					b := "$s" + m[j:mark[1]] + "D"
					if len(owner[b]) == 0 {
						bare = append(bare, b)
					}
					owner[b] = append(owner[b], i)
				}
			}
		}
	}
	if len(bare) == 0 {
		return nil
	}
	var in bytes.Buffer
	for _, b := range bare {
		in.WriteString(b)
		in.WriteByte('\n')
	}
	cmd := exec.Command("xcrun", "swift-demangle", "--compact")
	cmd.Stdin = &in
	data, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("swift-demangle: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != len(bare) {
		return fmt.Errorf("swift-demangle: %d lines for %d symbols", len(lines), len(bare))
	}
	// The longest spelling of a symbol's local type is its whole context; shorter ones are suffixes of it.
	spellings := map[int][]string{}
	for k, b := range bare {
		text := privateName.ReplaceAllString(nameApart(stripClosures(lines[k]), distinct), "$1")
		if loc := localStart.FindStringIndex(text); loc == nil || loc[0] != 0 {
			continue
		}
		for _, i := range owner[b] {
			if strings.Contains(raws[i], text) {
				spellings[i] = append(spellings[i], text)
			}
		}
	}
	for i, texts := range spellings {
		slices.SortFunc(texts, func(a, b string) int { return len(b) - len(a) })
		for _, text := range texts {
			raws[i] = strings.ReplaceAll(raws[i], text, normalizeLocal(text))
		}
	}
	return nil
}

var privateName = regexp.MustCompile(`\(([^()\s]+) in _[0-9A-F]+\)`)

var (
	privateApart = regexp.MustCompile(`(^|[^.\w])(\w+\.)\((\w+) in _([0-9A-F]+)\)`)
	localApart   = regexp.MustCompile(`(\w+) #(\d+) in (?:static )?([\w.]+)`)
)

// nameApart renames, in a demangled symbol, the file-private and function-local types whose names are in
// distinct: a file-private type at the top level of its module after its discriminator, a local type after the
// function that declares it and, past the first, its number there. A file-private type nested in another type
// keeps its name, which its owner's qualifies.
func nameApart(s string, distinct map[string]bool) string {
	if len(distinct) == 0 {
		return s
	}
	s = privateApart.ReplaceAllStringFunc(s, func(m string) string {
		sub := privateApart.FindStringSubmatch(m)
		if !distinct[sub[3]] {
			return m
		}
		return sub[1] + sub[2] + "(" + sub[3] + "_" + sub[4][:min(8, len(sub[4]))] + " in _" + sub[4] + ")"
	})
	return localApart.ReplaceAllStringFunc(s, func(m string) string {
		sub := localApart.FindStringSubmatch(m)
		if !distinct[sub[1]] {
			return m
		}
		path := sub[3]
		fn := path[strings.LastIndex(path, ".")+1:]
		name := sub[1] + "_" + fn
		if sub[2] != "1" {
			name += sub[2]
		}
		return strings.Replace(m, sub[1], name, 1)
	})
}

// Get returns the demangled form, or the input when unknown.
func (d *Demangler) Get(s string) string {
	if v, ok := d.cache[s]; ok {
		return v
	}
	return s
}

// Lookup returns the demangled form and whether s was demangled.
func (d *Demangler) Lookup(s string) (string, bool) {
	v, ok := d.cache[s]
	return v, ok
}

var closureStart = regexp.MustCompile(`(?:implicit )?closure #\d+ `)

// stripClosures removes the closures that enclose a function-local type from its context, "Foo #1 in closure #1
// () throws -> () in Mod.Type.fn() -> ()" becoming "Foo #1 in Mod.Type.fn() -> ()", since the type belongs to
// the function the closures are written in.
func stripClosures(s string) string {
	for {
		loc := closureStart.FindStringIndex(s)
		if loc == nil {
			return s
		}
		i := loc[1]
		if i < len(s) && s[i] == '(' {
			i = skipBalanced(s, i)
		}
		depth := 0
		for i < len(s) && (depth > 0 || !strings.HasPrefix(s[i:], " in ")) {
			switch s[i] {
			case '(', '[', '<':
				depth++
			case ')', ']', '>':
				depth--
			}
			i++
		}
		if i >= len(s) {
			return s
		}
		s = s[:loc[0]] + s[i+len(" in "):]
	}
}

var localStart = regexp.MustCompile(`(\w+) #\d+ in (?:static )?(\w+)(\.)?`)

// normalizeLocal rewrites a type declared inside a function, "Foo #1 in static Mod.Type.fn(a: [Int]) -> [Bar]",
// to "Mod.Foo", skipping the enclosing function's signature by balancing brackets. A type declared in a
// program's top-level code, "Foo #1 in Mod", becomes "Mod.Foo" as well.
func normalizeLocal(s string) string {
	for {
		loc := localStart.FindStringSubmatchIndex(s)
		if loc == nil {
			return s
		}
		name, mod := s[loc[2]:loc[3]], s[loc[4]:loc[5]]
		i := loc[1]
		if loc[6] < 0 {
			s = s[:loc[0]] + mod + "." + name + s[i:]
			continue
		}
		for i < len(s) && s[i] != '(' && s[i] != ' ' && s[i] != ',' && s[i] != ']' && s[i] != '>' {
			i++
		}
		if i < len(s) && s[i] == '(' {
			i = skipBalanced(s, i)
			for _, kw := range []string{" async", " throws"} {
				if strings.HasPrefix(s[i:], kw) {
					i += len(kw)
				}
			}
			if strings.HasPrefix(s[i:], " -> ") {
				i = skipType(s, i+len(" -> "))
			}
		} else if strings.HasPrefix(s[i:], " : ") {
			// Declared in a computed property's getter: "Foo #1 in Mod.Type.prop : Swift.String".
			i = skipType(s, i+len(" : "))
		}
		s = s[:loc[0]] + mod + "." + name + s[i:]
	}
}

// skipType returns the end of the type that starts at s[i:], which ends at a closing bracket,
// a comma, or a dictionary's key-value separator at its own depth.
func skipType(s string, i int) int {
	depth := 0
	for i < len(s) {
		c := s[i]
		if c == '(' || c == '[' || c == '<' {
			depth++
		} else if c == ')' || c == ']' || c == '>' {
			if depth == 0 {
				break
			}
			depth--
		} else if c == ',' && depth == 0 || c == ' ' && depth == 0 && strings.HasPrefix(s[i:], " : ") {
			break
		}
		i++
	}
	return i
}

func skipBalanced(s string, i int) int {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return i
}

// localWrappers restores the optionals and metatype around a type declared in a function, which
// the demangler prints ambiguously ("Foo #1 in f() -> Swift.Int?" for Foo?): it reads them from the
// mangled suffix that follows the local type ("…L_VSgD" is Foo?, "…L_VmD" is Foo.Type).
//
// A local type nested in another type, such as the result of a closure type ("() throws -> Foo #1 in f() -> ()?"
// for `() throws -> Foo?`), gets the optionals that directly follow it in the mangling.
// localMangling is the mangling of a local struct, class or enum: L, its discriminator past the first
// ("L_" is #1, "L0_" is #2), and its kind.
var localMangling = regexp.MustCompile(`L\d*_[VCO]`)

func localWrappers(mangled, raw, norm string) string {
	loc := localStart.FindStringSubmatchIndex(raw)
	if loc == nil || !strings.HasSuffix(mangled, "D") {
		return norm
	}
	marks := localMangling.FindAllStringIndex(mangled, -1)
	if len(marks) == 0 {
		return norm
	}
	// The wrappers' mangling follows the local type's: mangled[after:].
	after := marks[len(marks)-1][1]
	if loc[0] != 0 {
		if len(localStart.FindAllStringIndex(raw, -1)) != 1 || len(marks) != 1 {
			return norm
		}
		at := loc[0] + len(raw[loc[4]:loc[5]]) + 1 + len(raw[loc[2]:loc[3]])
		if !strings.HasPrefix(norm[loc[0]:], raw[loc[4]:loc[5]]+"."+raw[loc[2]:loc[3]]) {
			return norm
		}
		opt := ""
		for rest := mangled[after:]; strings.HasPrefix(rest, "Sg"); rest = rest[2:] {
			opt += "?"
		}
		return norm[:at] + opt + norm[at:]
	}
	end := 0
	for end < len(norm) && (norm[end] == '.' || norm[end] == '_' || isAlnum(norm[end])) {
		end++
	}
	var out strings.Builder
	out.WriteString(norm[:end])
	rest := strings.TrimSuffix(mangled[after:], "D")
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, "Sg"):
			out.WriteString("?")
			rest = rest[2:]
		case rest == "m":
			out.WriteString(".Type")
			rest = ""
		default:
			return norm
		}
	}
	return out.String()
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// Nominal kinds reported by [Demangler.NominalKinds].
const (
	KindClass    = 'C'
	KindStruct   = 'V'
	KindEnum     = 'O'
	KindProtocol = 'P'
)

// NominalKinds reads the kind of every nominal type of module named in the cached symbols, keyed by its
// qualified name ("GRDB.Database.ColumnType"): class, struct, enum or protocol.
func (d *Demangler) NominalKinds(module string) (map[string]byte, error) {
	prefix := fmt.Sprintf("%d%s", len(module), module)
	var syms []string
	for s := range d.cache {
		if strings.Contains(s, prefix) && !strings.HasPrefix(s, "c:") {
			syms = append(syms, mangledForm(s))
		}
	}
	kinds := map[string]byte{}
	if len(syms) == 0 {
		return kinds, nil
	}
	cmd := exec.Command("xcrun", "swift-demangle", "--tree-only")
	cmd.Stdin = strings.NewReader(strings.Join(syms, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("swift-demangle --tree-only: %w", err)
	}
	parseNominalKinds(string(out), module, kinds)
	return kinds, nil
}

var treeNode = regexp.MustCompile(`^( *)kind=(\w+)(?:, text="([^"]*)")?`)

var nominalKinds = map[string]byte{"Class": KindClass, "Structure": KindStruct, "Enum": KindEnum, "Protocol": KindProtocol}

// parseNominalKinds walks swift-demangle's node trees and records each Class, Structure, Enum and Protocol
// node of module. A nominal node's first child is its context (a Module or another nominal type) and its
// Identifier child its name.
func parseNominalKinds(tree, module string, kinds map[string]byte) {
	type frame struct {
		depth int
		kind  byte // 0 for other nodes
		name  string
		named bool
	}
	var stack []*frame
	pop := func() {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.kind == 0 || !f.named {
			return
		}
		if strings.HasPrefix(f.name, module+".") {
			kinds[f.name] = f.kind
		}
		if len(stack) > 0 {
			if parent := stack[len(stack)-1]; parent.kind != 0 && parent.name == "" {
				parent.name = f.name
			}
		}
	}
	for _, line := range strings.Split(tree, "\n") {
		m := treeNode.FindStringSubmatch(line)
		if m == nil {
			for len(stack) > 0 {
				pop()
			}
			continue
		}
		depth := len(m[1])
		for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
			pop()
		}
		if len(stack) > 0 {
			if parent := stack[len(stack)-1]; parent.kind != 0 {
				switch {
				case m[2] == "Module" && parent.name == "":
					parent.name = m[3]
				case m[2] == "Identifier" && parent.name != "" && !parent.named:
					parent.name += "." + m[3]
					parent.named = true
				}
			}
		}
		stack = append(stack, &frame{depth: depth, kind: nominalKinds[m[2]]})
	}
	for len(stack) > 0 {
		pop()
	}
}
