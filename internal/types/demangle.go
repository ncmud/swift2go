// Package types demangles Swift symbols and maps Swift types to Go types.
package types

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Demangler caches demangled names for mangled types ("$s…") and USRs ("s:…").
type Demangler struct {
	cache map[string]string
}

// NewDemangler demangles every symbol in syms in one swift-demangle run.
func NewDemangler(syms []string) (*Demangler, error) {
	d := &Demangler{cache: map[string]string{}}
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
	var in bytes.Buffer
	for _, s := range todo {
		in.WriteString(mangledForm(s))
		in.WriteByte('\n')
	}
	cmd := exec.Command("xcrun", "swift-demangle", "--compact")
	cmd.Stdin = &in
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("swift-demangle: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(todo) {
		return fmt.Errorf("swift-demangle: %d lines for %d symbols", len(lines), len(todo))
	}
	for i, s := range todo {
		d.cache[s] = normalizeLocal(privateName.ReplaceAllString(lines[i], "$1"))
	}
	return nil
}

var privateName = regexp.MustCompile(`\(([^()\s]+) in _[0-9A-F]+\)`)

// localName matches a type declared inside a function: "Foo #1 in Mod.Type.fn() -> ()".
var localName = regexp.MustCompile(`(\w+) #\d+ in (\w+)\.[\w.]*\([^()]*\)(?: (?:async )?(?:throws )?-> (?:\(\)|[\w.?\[\]]+))?`)

// Get returns the demangled form, or the input when unknown.
func (d *Demangler) Get(s string) string {
	if v, ok := d.cache[s]; ok {
		return v
	}
	return s
}

var localStart = regexp.MustCompile(`(\w+) #\d+ in (?:static )?(\w+)\.`)

// normalizeLocal rewrites a type declared inside a function, "Foo #1 in static Mod.Type.fn(a: [Int]) -> [Bar]",
// to "Mod.Foo", skipping the enclosing function's signature by balancing brackets.
func normalizeLocal(s string) string {
	for {
		loc := localStart.FindStringSubmatchIndex(s)
		if loc == nil {
			return s
		}
		name, mod := s[loc[2]:loc[3]], s[loc[4]:loc[5]]
		i := loc[1]
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
				i += len(" -> ")
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
					} else if c == ',' && depth == 0 {
						break
					}
					i++
				}
			}
		}
		s = s[:loc[0]] + mod + "." + name + s[i:]
	}
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
