# swift2go

[![CI](https://github.com/ncmud/swift2go/actions/workflows/ci.yml/badge.svg?branch=trunk)](https://github.com/ncmud/swift2go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/ncmud/swift2go/actions/workflows/codeql.yml/badge.svg?branch=trunk)](https://github.com/ncmud/swift2go/actions/workflows/codeql.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ncmud/swift2go.svg)](https://pkg.go.dev/github.com/ncmud/swift2go)
![Go version](https://img.shields.io/github/go-mod/go-version/ncmud/swift2go?logo=go&logoColor=white)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Swift 6.4.2 AST](https://img.shields.io/badge/swift_AST-6.4.2-F05138?logo=swift&logoColor=white)

swift2go translates the targets of a Swift package into Go packages. It reads the Swift compiler's
type-checked AST (`swiftc -dump-ast -dump-ast-format json`), so every expression arrives with its
resolved type, overload and conformances. The Go it writes calls a small runtime, `swiftrt`, for the
parts of Swift's standard library and Foundation that Go lacks. Swift test targets written with
swift-testing come out as `go test` tests, so a translated package can be checked against its own
test suite.

The project is driven by one large server codebase, so coverage follows what that code uses. It
contains nothing specific to that codebase, and it should work on other packages written in a
similar style.

## Requirements

- macOS with the Swift 6.4.2 toolchain (`swiftc`, `swift-demangle`)
- Go 1.27.4 or newer
- A SwiftPM package that builds

## Usage

Run swift2go from the root of the Go module that should hold the output. Generated packages are
written under `out/` and import one another by paths inside that module.

```sh
go run github.com/ncmud/swift2go/cmd/swift2go -pkg path/to/SwiftPackage transpile Core App AppTests
```

- List targets in dependency order. A target's dependencies must be transpiled first, or in the same run.
- A test target joins the package of the target it tests, as `_test.go` files.
- Each target prints a report of constructs it could not translate. Those compile to
  `swiftrt.Unsupported[T]("kind")`, which panics if it runs.
- A target that fails to compile or translate is reported, and every target in the run that depends
  on it is skipped. The run goes on with the others and exits non-zero.

### Dependencies and generated sources

- Targets can come from the root package or from any package in its dependency graph, including
  remote checkouts under `.build/checkouts`. `-pkg` may also point at a package checked out under
  another package's `.build/checkouts`.
- Dependencies that are not in the run are imported from the modules SwiftPM built, so the package
  must have been built with `swift build`. The build directory defaults to the package's `.build`, or
  to the enclosing package's for a checkout; `-build` overrides it. A dependency module older than
  any of its sources stops the target with an error: transpile that dependency in the same run, or
  rebuild it.
- C targets in the dependency graph are importable without a build. Their own module maps are used,
  and for those without one swift2go writes the module map SwiftPM would generate under `out/modulemaps`.
- A target with resources gets the `Bundle.module` accessor SwiftPM generates, written to
  `out/derived/<Target>`. It looks for the `<package>_<Target>.bundle` resource bundle next to the
  executable.
- Sources that build-tool plugins generate (for example swift-openapi-generator) are read from
  `<build>/plugins/outputs`, where `swift build` writes them. swift2go does not run plugins. Run
  `swift build --target <Target>` first; if the outputs are missing, or older than a non-Swift file in
  the target's directory (such as the plugin's input), the target fails and names the command to run.

### Replacing a module with a Go package

`-map Module=importpath` (repeatable) uses a hand-written Go package for a Swift module instead of
transpiled code. It suits modules that wrap C libraries or lean on pointer-level APIs, where a direct Go
implementation is simpler than a translation. The package must be importable from the Go module that holds
the output.

```sh
go run github.com/ncmud/swift2go/cmd/swift2go -pkg path/to/SwiftPackage \
    -map Engine=example.com/goengine/engine -map CEngine=example.com/goengine/engine \
    transpile Engine App
```

- **A target transpiled in the same run** (`Engine` above) is still read, so dependents see its
  declarations, but no Go is written for it. Dependents import the mapped path and call it by the names
  swift2go would have generated: transpile the target once without `-map` and implement the exported API
  of the output.
- **A Swift module that is not transpiled** (a third-party product such as GRDB) is called by names built
  from its declarations, so one hand-written Go function covers every call site of a Swift declaration.
  Its package is imported under its package name.
  - A member is named after the type or protocol that **declares** it, with nested owners flattened, then
    the base name and the argument-label suffix the runtime uses: `record.insert(db)` from an extension of
    `PersistableRecord` is `grdb.PersistableRecordInsert(record, db)`, `Database.ColumnType.text` is
    `grdb.DatabaseColumnTypeText`. A free function has no owner; an initializer is `<Type>Init<labels>`
    (unlabeled arguments add their runtime kind, as for runtime initializers); a member a module declares
    in an extension of another module's type is that module's (`grdb.SequenceContains`).
  - The receiver is the first argument (`&v` for a mutating method on a struct). An instance property is
    read with `<Owner><Prop>(recv)` and written with `<Owner>Set<Prop>(&v, x)`. A static property is a
    package variable, or `<Protocol><Prop>[T]()` when a protocol declares it. A payload case is a
    constructor function and a payload-free case a value that `case` patterns compare with.
  - An operator is a call, `<Owner>Op<Word>` or `Op<Word>` at module level (`==` is `OpEqual`, `&&`
    `OpAnd`, `!` `OpNot`, …), with the label suffix's kind for concrete standard-library operands:
    `Column("read") == false` is `grdb.SQLSpecificExpressibleOpEqualBool(…, false)`.
  - Type arguments are explicit when Go cannot infer them: the conforming type for a static member of a
    protocol (`MailRecord.filter(e)` is `grdb.TableRecordFilter[MailRecord](e)`), then all of a member's
    own type parameters when one occurs in its result (`row["id"]` is `grdb.RowSubscriptString[int64]`).
    `Void` is `struct{}`. A function returning its own type parameter's Optional (`T?`) returns `*T`; a
    call site with a class for `T` flattens it to the class pointer, as for runtime generics.
  - An array or dictionary literal of one of its types calls `<Type>InitArrayLiteral(elems...)` or
    `<Type>InitDictionaryLiteral(swiftrt.KV[K, V]{…}...)`.
  - Its classes are pointers (`*grdb.Database`) and its protocols interfaces, nil for none.
  - A local type or extension that conforms to its protocols registers in `init()`:
    `grdb.RegisterConformance[T](swiftrt.Conformance{Type, Protocols, Witnesses})`. Witnesses map each
    protocol requirement the type implements, itself or through a local protocol's default, to a Go
    function, keyed as Swift names the requirement (`"didInsert(_:)"`); the requirements come from the
    module's symbol graph. See `swiftrt.Conformance` for the function shapes.
- **A C module that is not transpiled** (such as `CEngine`) is referenced the way any C module is
  (`swiftrt.CF<function>` for C functions, `swiftrt.CV<name>` for C macros and globals). Each such name
  that the mapped package exports is taken from that package, imported under its package name. Names it
  does not export stay in `swiftrt`. When two mapped packages export a name, the module that sorts first
  wins.
- A mapped package's declarations decide a few shapes at call sites:
  - a type the package declares generic keeps its Swift type arguments (`Box<Int>` becomes
    `glue.Box[int]`, and its initializers are called as `BoxInit…[int]`); other foreign types drop them;
  - a `for` loop over a value a mapped function returns, other than an array, dictionary or set, ranges
    over it directly, so the function can return a lazy `iter.Seq`.

From a checkout, `just check <Target> <TestTarget>` transpiles a target and its tests and runs them
with `go test`. Set `SWIFT_PKG` to the package directory.

### Replacing a Swift file with hand-written Go

`-overlay Target=dir` (repeatable) takes hand-written Go files for a target from `dir`. A Go file there
replaces the Swift file of the same name, which is then not translated: `socket_lifecycle.go` replaces
`SocketLifecycle.swift`. The other Go files in the directory are copied into the package as well.

    go run github.com/ncmud/swift2go/cmd/swift2go -pkg path/to/SwiftPackage \
        -overlay Server=path/to/overlay/server transpile Core Server

- Use it for a file that is mostly calls into the platform (sockets, signals, process control), where
  a Go implementation on the standard library is clearer than a translation of the C calls. The rest of
  the target stays translated.
- The replacement declares, under the names swift2go generates, everything the rest of the program uses
  from the Swift file. Transpile once without `-overlay` to see those names.
- A replacement belongs to the generated package, so it imports the other generated packages by their
  paths in the output module. Keep overlay directories out of that module's own packages, for example
  under a directory whose name starts with `_`.
- The target's report lists the Swift files it left to the overlay. swift2go does not check that a
  replacement still matches its Swift file: when the Swift file changes, the replacement is yours to update.
- A test target takes the `_test.go` files of its target's overlay the same way.
- To replace a whole module, use `-map`. A target cannot have both.

## What it translates

| Swift | Go |
|---|---|
| struct / class | struct (value semantics through copies) / `*struct` |
| class inheritance and `override` | embedding, upcast methods, dynamic dispatch hooks |
| enum without associated values | named integer type and constants |
| enum with associated values | sealed interface and one struct per case, generic when the enum is |
| protocol, extensions, default implementations | interface, forwarding methods, package functions taking `self` |
| static protocol requirements, `Self` | type parameters and `(*new(T)).StaticX()` |
| generics, constrained extensions | Go generics, with pinned parameters dropped |
| `T?` | `*T`, with class references kept as the pointer itself |
| `throws`, `do`/`catch`, typed throws | panics carrying the error, caught by `swiftrt.Catch` |
| `Int` arithmetic | `swiftrt.Add`/`Sub`/`Mul`, which trap on overflow as Swift does |
| `Int128` | `swiftrt.Int128`, checked big-integer arithmetic |
| `String`, `Character`, `Substring`, `String.Index` | Go strings, grapheme clusters, a base with a range, byte offsets |
| `Array`, `Dictionary`, `Set` | slices, maps, `map[T]struct{}` (or `swiftrt.HSet` for elements Go can't key on) |
| `inout`, mutating methods, `didSet`/`willSet` | pointers, pointer receivers, setters that run observers |
| `switch` with patterns, `if case`, `guard`, labeled loops | Go control flow with pattern tests and bindings |
| closures, method references, `@autoclosure` | function literals |
| `Codable` (synthesized or hand-written) | a JSON-shaped tree in `swiftrt`, Foundation-compatible output |
| `Regex` literals | RE2 with Swift's Unicode word boundaries emulated |
| `async`/`await`, `Task`, `AsyncStream`, `for await` | goroutines and channels in `swiftrt` |
| swift-testing `@Test`, `#expect`, `#require`, `arguments:` | `func TestX(t *testing.T)` and subtests |
| freestanding expression macros | the compiler's recorded expansion |

The runtime also covers the parts of Foundation, Dispatch, Synchronization, swift-collections,
swift-log, CryptoKit and a few C library calls that the driving codebase uses.

## Design decisions

These are choices made for the codebase that drives the project. Each fits it, and each could become an
option.

- **Overflow traps.** `Int` arithmetic panics on overflow, as a Swift debug build does. `&+` and the other wrapping operators wrap.
- **Optionals are pointers.** `T?` is `*T`, and a class's Optional is the class pointer itself. A generic `T?` instantiated with a class is unwrapped at call sites, so callers see `*C`, not `**C`.
- **Value semantics by copying.** Struct values, arrays, dictionaries and sets are deep-copied (`swiftrt.Copy`) wherever Swift would copy them. Class references, which embed a marker, are never copied.
- **Statics are lazy.** Static and global stored properties become `swiftrt.Lazy` cells initialized on first use, as Swift does, so initialization cycles that Swift allows still work in Go.
- **Errors are panics.** `throw` panics with the error, and `do`/`catch`, `try?` and `try!` recover it. Functions keep their Go signatures without an `error` result.
- **Observers run on in-place mutation.** `didSet`/`willSet` fire for whole-value assignments and for mutations of part of the value (`dict[k] = v`, `array.append(x)`), except where Swift accesses the storage directly: inside initializers and when a property's own observers assign it on `self`.
- **Names follow Swift.** Methods are named after their base name and argument labels (`pad(_:to:)` becomes `PadTo`). Runtime calls are `swiftrt.<Receiver><Member><Labels>`. Objective-C classes keep their Objective-C names as receivers (`NSFileManagerFileExistsAtPath`). Clashes get a type or `Fn` suffix.
- **Imported option sets are listed by name.** Foreign `OptionSet` types the runtime models (`JSONEncoder.OutputFormatting`, YYJSON's options) are named in the emitter, so array literals of them build option sets.
- **Mapped names come from the Swift declarations.** A replacement package matches the names the emitter generates, rather than the emitter reading names from the package, so the same Swift source always produces the same Go calls.
- **Targets come in dependency order.** A dependency must be transpiled before, or in the same run as, the targets that import it. Otherwise its types are treated as foreign, and calls go to the runtime.

## Limits

- Matching for Swift `Regex` is scalar-based, apart from the trailing `\b` handled above.
- Values in an `HSet` are keyed by a canonical rendering of their fields, not by a custom `hash(into:)`.
- Pointers and C interop cover what the code needs (`gettimeofday`, `clock_gettime`, C strings), not `UnsafePointer` in general.
- Every untranslated construct is reported, never silently dropped. `swift2go stats` and the
  per-target report list them by kind.

## Layout

- `cmd/swift2go`: the CLI (`transpile`, `dump`, `stats`)
- `cmd/gocheck`: type-checks generated packages and lists errors even when imports fail
- `internal/swiftmod`: `swift package describe`, module emission, per-file AST dumps
- `internal/ast`: the JSON AST loader
- `internal/types`: batched demangling and a parser for demangled types
- `internal/emit`: declaration, statement and expression emitters
- `swiftrt`: the runtime the generated code calls
