package emit

import "testing"

func TestSourcePathNamesGeneratedSourcesByTheirRoots(t *testing.T) {
	m := &Module{}
	SourceRoots(map[string]string{
		"/work/a/out/derived":                                            "derived",
		"/work/a/out/derived/Commands/macros":                            "",
		"/work/a/.build/plugins/outputs/ncmud/EditorAPISpec/destination": "plugins/EditorAPISpec",
	})(m)
	for p, want := range map[string]string{
		"/work/a/out/derived/WorldDatabase/resource_bundle_accessor.swift":                                             "derived/WorldDatabase/resource_bundle_accessor.swift",
		"/work/a/.build/plugins/outputs/ncmud/EditorAPISpec/destination/OpenAPIGenerator/GeneratedSources/Types.swift": "plugins/EditorAPISpec/OpenAPIGenerator/GeneratedSources/Types.swift",
		"/work/a/out/derived/Commands/macros/Sources/Commands/Admin/BossCommand.swift":                                 "Sources/Commands/Admin/BossCommand.swift",
		"/work/a/Sources/World/GameCharacter.swift":                                                                    "Sources/World/GameCharacter.swift",
		"/work/a/Tests/WorldTests/CombatTests.swift":                                                                   "Tests/WorldTests/CombatTests.swift",
		"/work/a/out/derivedish/Other.swift":                                                                           "/work/a/out/derivedish/Other.swift",
	} {
		if got := m.sourcePath(p); got != want {
			t.Errorf("sourcePath(%q) = %q, want %q", p, got, want)
		}
	}
}
