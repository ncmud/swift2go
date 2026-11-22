package swiftmod

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Requirement is one requirement of a protocol, named as Swift spells it with argument labels
// ("didInsert(_:)", "databaseTableName").
type Requirement struct {
	Name string
	// Static marks a static (type-level) requirement.
	Static bool
	// Kind is "method", "property", "init", "subscript" or "associatedtype".
	Kind string
}

// ProtocolRequirements reads the requirements of each public protocol of a built Swift module from its
// symbol graph, keyed by the protocol's name within the module ("TableRecord", "Database.Observer").
// flags are the swiftc flags that locate the module and its dependencies; only -I, -F, -Xcc, -sdk, -target
// and -resource-dir pass through. Without -target the module is read as built for the host.
func ProtocolRequirements(module string, flags []string) (map[string][]Requirement, error) {
	dir, err := os.MkdirTemp("", "swift2go-symbols-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	args := []string{"-module-name", module, "-output-dir", dir, "-minimum-access-level", "public"}
	for i := 0; i < len(flags); i++ {
		switch flags[i] {
		case "-I", "-F", "-Xcc", "-sdk", "-target", "-resource-dir":
			if i+1 < len(flags) {
				args = append(args, flags[i], flags[i+1])
				i++
			}
		}
	}
	if !slices.Contains(args, "-target") {
		target, err := hostTarget()
		if err != nil {
			return nil, err
		}
		args = append(args, "-target", target)
	}
	if !slices.Contains(args, "-sdk") {
		if sdk, err := exec.Command("xcrun", "--show-sdk-path").Output(); err == nil {
			args = append(args, "-sdk", strings.TrimSpace(string(sdk)))
		}
	}
	tool, extra := symbolGraphTool()
	cmd := exec.Command(tool, append(extra, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("swift-symbolgraph-extract %s: %w\n%s", module, err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, module+".symbols.json"))
	if err != nil {
		return nil, err
	}
	return parseRequirements(data)
}

func parseRequirements(data []byte) (map[string][]Requirement, error) {
	var graph struct {
		Symbols []struct {
			Identifier struct {
				Precise string `json:"precise"`
			} `json:"identifier"`
			Kind struct {
				Identifier string `json:"identifier"`
			} `json:"kind"`
			PathComponents []string `json:"pathComponents"`
		} `json:"symbols"`
		Relationships []struct {
			Kind   string `json:"kind"`
			Source string `json:"source"`
			Target string `json:"target"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}
	type symbol struct {
		kind string
		path []string
	}
	syms := map[string]symbol{}
	for _, s := range graph.Symbols {
		syms[s.Identifier.Precise] = symbol{s.Kind.Identifier, s.PathComponents}
	}
	out := map[string][]Requirement{}
	for _, r := range graph.Relationships {
		if r.Kind != "requirementOf" && r.Kind != "optionalRequirementOf" {
			continue
		}
		src, ok := syms[r.Source]
		proto, pok := syms[r.Target]
		if !ok || !pok || proto.kind != "swift.protocol" || len(src.path) == 0 {
			continue
		}
		kind := strings.TrimPrefix(src.kind, "swift.")
		static := strings.HasPrefix(kind, "type.")
		kind = strings.TrimPrefix(kind, "type.")
		name := strings.Join(proto.path, ".")
		out[name] = append(out[name], Requirement{Name: src.path[len(src.path)-1], Static: static, Kind: kind})
	}
	return out, nil
}

// hostTarget is the target triple swiftc builds for by default, which modules built here carry.
func hostTarget() (string, error) {
	out, err := exec.Command("swiftc", "-print-target-info").Output()
	if err != nil {
		return "", fmt.Errorf("swiftc -print-target-info: %w", err)
	}
	var info struct {
		Target struct {
			Triple string `json:"triple"`
		} `json:"target"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", err
	}
	return info.Target.Triple, nil
}

func symbolGraphTool() (string, []string) {
	if _, err := exec.LookPath("xcrun"); err == nil {
		return "xcrun", []string{"swift-symbolgraph-extract"}
	}
	return "swift-symbolgraph-extract", nil
}
