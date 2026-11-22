package swiftmod

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// A macroExpansion is one attached-macro expansion buffer that the frontend printed for
// -dump-macro-expansions.
type macroExpansion struct {
	Role    byte   // the mangled role letter: 'e' extension, 'm' member, 'p' peer, ...
	Macro   string // the macro's name
	Decl    string // the name of the declaration the macro is attached to
	Context string // the name that encloses the declaration: a type, or the module
	Text    string
}

var (
	expansionBuffer = regexp.MustCompile(`(?m)^@__swiftmacro_(\S+)\.swift\n-{30}\n((?s:.*?))\n-{30}$`)
	expansionRole   = regexp.MustCompile(`fM([a-z])\d*_$`)
	expansionName   = regexp.MustCompile(`^\w+ macro @(\S+) expansion #\d+ of (.+)$`)
)

// parseExpansions reads the buffers -dump-macro-expansions printed to stderr. demangle maps a
// buffer's mangled name to swift-demangle's description of it.
func parseExpansions(stderr string, demangle func(mangled []string) ([]string, error)) ([]macroExpansion, error) {
	found := expansionBuffer.FindAllStringSubmatch(stderr, -1)
	var mangled []string
	var exps []macroExpansion
	for _, m := range found {
		role := expansionRole.FindStringSubmatch(m[1])
		if role == nil {
			continue
		}
		mangled = append(mangled, "$s"+m[1])
		exps = append(exps, macroExpansion{Role: role[1][0], Text: m[2]})
	}
	if len(exps) == 0 {
		return nil, nil
	}
	described, err := demangle(mangled)
	if err != nil {
		return nil, err
	}
	for i, d := range described {
		m := expansionName.FindStringSubmatch(d)
		if m == nil {
			// Not attached to a declaration: an expansion nested inside another expansion.
			exps[i].Role = 0
			continue
		}
		exps[i].Macro = m[1][strings.LastIndex(m[1], ".")+1:]
		decl, context := m[2], ""
		if i := strings.LastIndex(decl, " in "); i >= 0 {
			decl, context = decl[:i], decl[i+len(" in "):]
		}
		// A private declaration is "(Name in _FileDiscriminator)".
		decl = strings.TrimPrefix(decl, "(")
		if i := strings.IndexByte(decl, ' '); i >= 0 {
			decl = decl[:i]
		}
		exps[i].Decl = decl
		exps[i].Context = context[strings.LastIndex(context, ".")+1:]
	}
	return exps, nil
}

func swiftDemangle(mangled []string) ([]string, error) {
	cmd := exec.Command("xcrun", "swift-demangle")
	cmd.Stdin = strings.NewReader(strings.Join(mangled, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("swift-demangle: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(mangled) {
		return nil, fmt.Errorf("swift-demangle returned %d names for %d", len(lines), len(mangled))
	}
	return lines, nil
}

// An attachedMacro is a macro attribute on a declaration in a dumped file.
type attachedMacro struct {
	Macro, Decl, Context string
	Start, End           int // the attribute's range: the offsets of its first and last tokens
}

// attachedMacros finds the macro attributes in one file's JSON AST.
func attachedMacros(astJSON []byte) ([]attachedMacro, error) {
	var root any
	if err := json.Unmarshal(astJSON, &root); err != nil {
		return nil, err
	}
	var found []attachedMacro
	var walk func(n any, context string)
	walk = func(n any, context string) {
		switch n := n.(type) {
		case []any:
			for _, c := range n {
				walk(c, context)
			}
		case map[string]any:
			name := declName(n)
			attrs, _ := n["attrs"].([]any)
			for _, a := range attrs {
				attr, _ := a.(map[string]any)
				macro, _ := attr["macro"].(map[string]any)
				rng, _ := attr["range"].(map[string]any)
				if attr["_kind"] != "custom_attr" || macro == nil || rng == nil || name == "" {
					continue
				}
				macroName, _ := macro["base_name"].(string)
				start, _ := rng["start"].(float64)
				end, _ := rng["end"].(float64)
				found = append(found, attachedMacro{Macro: macroName, Decl: name, Context: context, Start: int(start), End: int(end)})
			}
			inner := context
			if kind, _ := n["_kind"].(string); name != "" && (kind == "struct_decl" || kind == "class_decl" || kind == "enum_decl" || kind == "protocol") {
				inner = name
			}
			for _, c := range n {
				walk(c, inner)
			}
		}
	}
	walk(root, "")
	return found, nil
}

func declName(n map[string]any) string {
	name, _ := n["name"].(map[string]any)
	base, _ := name["base_name"].(map[string]any)
	s, _ := base["name"].(string)
	return s
}

// spliceExtensions rewrites a source file so that the extensions its attached macros generate
// are ordinary source: each attribute whose macro produced an extension is blanked out, keeping
// every other offset, and the extensions follow the file's last line. It returns nil when no
// macro in the file produced an extension. The JSON AST leaves such extensions out, because they
// live in the compiler's expansion buffers and not in the file. Expansions that match no
// attribute in the file belong to other files and are skipped.
func spliceExtensions(src []byte, attrs []attachedMacro, exps []macroExpansion) ([]byte, error) {
	var out []byte
	for _, x := range exps {
		if x.Role != 'e' {
			continue
		}
		var match []attachedMacro
		for _, a := range attrs {
			if a.Macro == x.Macro && a.Decl == x.Decl {
				match = append(match, a)
			}
		}
		if len(match) > 1 {
			// A top-level declaration's context is the module, which the AST does not name.
			nested := slices.ContainsFunc(match, func(a attachedMacro) bool { return a.Context == x.Context })
			match = slices.DeleteFunc(match, func(a attachedMacro) bool {
				if nested {
					return a.Context != x.Context
				}
				return a.Context != ""
			})
		}
		if len(match) == 0 {
			// The frontend also prints the expansions it needed from other files.
			continue
		}
		if len(match) > 1 {
			return nil, fmt.Errorf("found %d declarations named %s with @%s for its extension", len(match), x.Decl, x.Macro)
		}
		for _, other := range exps {
			if other.Role != 'e' && other.Macro == x.Macro && other.Decl == x.Decl {
				return nil, fmt.Errorf("@%s on %s has a %q role besides its extension role, which is not supported", x.Macro, x.Decl, other.Role)
			}
		}
		if out == nil {
			out = bytes.Clone(src)
		}
		a := match[0]
		if a.Start < 0 || a.End >= len(src) || a.Start > a.End {
			return nil, fmt.Errorf("@%s on %s: attribute range %d-%d is outside the file", x.Macro, x.Decl, a.Start, a.End)
		}
		end := a.End + 1
		for end < len(src) && isIdentByte(src[end-1]) && isIdentByte(src[end]) {
			end++
		}
		for i := a.Start; i < end; i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
		out = append(out, "\n"+x.Text+"\n"...)
	}
	return out, nil
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// expandedSource writes the copy of src that spliceExtensions produces under dir and returns its
// path, or "" when src needs no copy. The copy keeps the path from the source's Sources or Tests
// directory down, so reports name it as they name the original.
func expandedSource(src, dir string, astJSON []byte, stderr string) (string, error) {
	if !strings.Contains(stderr, "@__swiftmacro_") {
		return "", nil
	}
	exps, err := parseExpansions(stderr, swiftDemangle)
	if err != nil || len(exps) == 0 {
		return "", err
	}
	attrs, err := attachedMacros(astJSON)
	if err != nil {
		return "", err
	}
	text, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	spliced, err := spliceExtensions(text, attrs, exps)
	if err != nil || spliced == nil {
		return "", err
	}
	rel := filepath.Base(src)
	for _, root := range []string{"/Sources/", "/Tests/"} {
		if i := strings.LastIndex(src, root); i >= 0 {
			rel = src[i+1:]
			break
		}
	}
	dst := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	return dst, os.WriteFile(dst, spliced, 0o644) //nolint:gosec // dst is under the output directory, named after a source of the target
}
