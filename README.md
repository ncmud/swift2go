# swift2go

[![CI](https://github.com/ncmud/swift2go/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/ncmud/swift2go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/ncmud/swift2go/actions/workflows/codeql.yml/badge.svg?branch=trunk)](https://github.com/ncmud/swift2go/actions/workflows/codeql.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ncmud/swift2go)](https://goreportcard.com/report/github.com/ncmud/swift2go)
[![Go Reference](https://pkg.go.dev/badge/github.com/ncmud/swift2go.svg)](https://pkg.go.dev/github.com/ncmud/swift2go)
![Go version](https://img.shields.io/github/go-mod/go-version/ncmud/swift2go?logo=go&logoColor=white)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Swift 6.4.2 AST](https://img.shields.io/badge/swift_AST-6.4.2-F05138?logo=swift&logoColor=white)

Transpiles ncmud's Swift targets to Go by reading the compiler's typed AST
(`swiftc -dump-ast -dump-ast-format json`, Swift 6.4.2).

- `cmd/swift2go` — CLI: `swift2go transpile <Target> [<TestTarget> ...]`, targets in dependency order.
- `internal/swiftmod` — `swift package describe`, `-emit-module`, AST dumps (with swift-testing macros).
- `internal/ast` — loader for the concatenated per-file JSON documents.
- `internal/types` — batch `swift-demangle` and a parser for demangled types.
- `internal/emit` — declaration, statement and expression emitters.
- `swiftrt` — runtime the generated code calls: checked arithmetic, grapheme-aware strings,
  optionals as pointers, stdlib collection methods, swift-testing `#expect`/`#require`.

Generated Go goes to `out/` (gitignored). Unsupported constructs compile to
`swiftrt.Unsupported[T]("kind")` and are counted in the transpile report.

```
just termcore        # transpile TermCore + TermCoreTests, vet, go test
just errors termcore # compile errors by message
```

## Mappings

| Swift | Go |
|---|---|
| struct / class | struct / `*struct` |
| enum without payload | `int` type + consts |
| enum with payload | sealed interface + case structs |
| protocol + extension defaults | interface + `ProtoMethod(self, …)` + forwarding methods on conformers |
| `T?` | `*T` (reference types stay bare) |
| `throws` / `try` / `do-catch` | panic with `swiftrt.Thrown`, `swiftrt.DoCatch` |
| `Int` `+ - *` | `swiftrt.Add/Sub/Mul` (trap on overflow) |
| `String.Index` | byte offset; `Character` = grapheme string; `Substring` = base + range |
| stdlib member `x.foo(bar:)` | `swiftrt.<Kind>FooBar(x, …)` |
| `@Test` func | `func TestSuiteMethod(t *testing.T)` |
