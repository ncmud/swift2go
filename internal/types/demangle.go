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
		raw := privateName.ReplaceAllString(lines[i], "$1")
		d.cache[s] = localWrappers(s, raw, normalizeLocal(raw))
	}
	return nil
}

var privateName = regexp.MustCompile(`\(([^()\s]+) in _[0-9A-F]+\)`)

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
func localWrappers(mangled, raw, norm string) string {
	if loc := localStart.FindStringIndex(raw); loc == nil || loc[0] != 0 || !strings.HasSuffix(mangled, "D") {
		return norm
	}
	i := strings.LastIndex(mangled, "L_")
	if i < 0 || i+3 > len(mangled) || !strings.ContainsRune("VCO", rune(mangled[i+2])) {
		return norm
	}
	end := 0
	for end < len(norm) && (norm[end] == '.' || norm[end] == '_' || isAlnum(norm[end])) {
		end++
	}
	out := norm[:end]
	rest := strings.TrimSuffix(mangled[i+3:], "D")
	for rest != "" {
		switch {
		case strings.HasPrefix(rest, "Sg"):
			out += "?"
			rest = rest[2:]
		case rest == "m":
			out += ".Type"
			rest = ""
		default:
			return norm
		}
	}
	return out
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
