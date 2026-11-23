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

- macOS with the Swift 6.4.2 toolchain (`swiftc`, `swift-demangle`), and a Swift SDK of the same
  version to compile for another platform
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
- An attached macro that adds an extension (`@attached(extension)`) is expanded for the dump: the
  compiler's JSON AST leaves such extensions out, so a file that uses one is dumped from a copy,
  written under `out/derived/<Target>/macros`, in which the extension stands as source in place of
  the attribute. The macro's plugin must be built (`swift build`). Members that macros add already
  arrive in the AST.
- Sources that build-tool plugins generate (for example swift-openapi-generator) are read from
  `<build>/plugins/outputs`, where `swift build` writes them. swift2go does not run plugins. Run
  `swift build --target <Target>` first; if the outputs are missing, or older than a non-Swift file in
  the target's directory (such as the plugin's input), the target fails and names the command to run.

### Files the compiler fails on

A file the compiler can't type-check is left out of its target, and the run prints `swift2go: skipped` with
the file and the first line of the compiler's error. With `-strict` (or `SWIFT2GO_STRICT=1`), such a file fails
its target instead, and the run prints every distinct error the compiler gave and exits non-zero. An
`-incremental` run dumps every file again when it switches between strict and lenient, so a strict run never
reuses a file that failed in a lenient one.

### Incremental runs

`-incremental` caches each target's ASTs under `out/.cache/<Target>`, and the next run dumps only what
changed:

```sh
go run github.com/ncmud/swift2go/cmd/swift2go -incremental -pkg path/to/SwiftPackage transpile Core App
```

- A file is dumped again when its content changes. The target's other files keep their cached ASTs
  as long as the changed files' fingerprints hold. A fingerprint covers a file's declarations with
  their types, attributes and conformances, but not the bodies of functions, initializers and
  accessors, nor the expressions that give variables their initial values. The body or value that
  decides an opaque type (`some P`) counts.
- When a fingerprint changes, a file is added or removed, or the compiler or its flags change, every
  file of the target is dumped again. A target's `.swiftmodule` is emitted again only when its
  fingerprints or its dependencies' change, and every file of a target that imports it is then dumped
  again.
- A Go file is emitted again only when its Swift file's AST changed, or when something it read from
  other files did: whether a function it calls can write shared state, the initial value of a
  property whose default it spells out, or the names and once-per-package declarations that earlier
  files took. The other Go files are left as they are, so the output still matches a clean run's, and
  a file emitted with the content it already has is not written.
  `out/.cache/<Target>/emit.gob` records what each Go file read.
- Every Go file of a target is emitted again when the declarations of the target or of a target it
  depends on change, when a function body declares a type or stops declaring one, when an initial
  value changes whether a static or global is initialized lazily, and when the swift2go binary
  changes.
- `SWIFT2GO_CHECK_INCREMENTAL=1` emits the files a run would keep as well, and fails the target if any
  of them would differ from the kept file.
- The package graph that `swift package describe` reports is cached in `out/.cache/describe.gob` until
  the toolchain, a manifest, a `Package.resolved` or the names of the files in a target's directory change.
- An incremental run does not write `out/ast/<Target>.json`.

### Compiling for another platform

The AST holds the branches of `#if os(...)`, `#if canImport(...)` and `#if arch(...)` that are true
for the platform the Swift sources are compiled for, and that is the host unless `-swift-sdk` names an
installed [Swift SDK](https://www.swift.org/documentation/articles/static-linux-getting-started.html).
The Go output runs anywhere Go does; the option decides only which Swift branches it is translated from.

```sh
go run github.com/ncmud/swift2go/cmd/swift2go -pkg path/to/SwiftPackage \
    -swift-sdk swift-6.4.2-RELEASE_static-linux-0.1.0 -triple aarch64-swift-linux-musl \
    -build path/to/linux-build transpile Core App
```

- Any Swift SDK of the compiler's version works: swift.org's static Linux SDK (musl), or a glibc SDK for
  a distribution made with [swift-sdk-generator](https://github.com/swiftlang/swift-sdk-generator)
  (`make-linux-sdk --distribution-name ubuntu --distribution-version 24.04`). Pick the C library the
  program ships with, since `canImport(Glibc)` and `canImport(Musl)` select different code.
- `-swift-sdk` takes an SDK's id or one of its target triples, as `swift build --swift-sdk` does, and
  `-triple` picks one of the SDK's triples when it has more than one. `swift sdk list` shows the
  installed SDKs.
- Every compiler run gets the triple's `-target`, `-sdk`, `-resource-dir` and search paths. Xcode's
  compiler refuses the modules of a Swift SDK, which a swift.org compiler of the same version built,
  by their revision; swift2go sets `SWIFT_IGNORE_SWIFTMODULE_REVISION` so it reads them.
- Before transpiling, swift2go runs `swift build --build-system native` for the SDK, with its products
  in the `-build` directory, for each Swift dependency of the listed targets that is not itself listed,
  and for each listed target that uses a build-tool plugin. Give the platform a build directory of its
  own. Macro plugins are built for the host there and loaded from it. The builds are skipped when the
  swift executable, the SDK, the manifest, `Package.resolved` and the files of the targets built and of their
  dependencies are unchanged since they last all succeeded there (`swift2go-dependencies.stamp`).
- `-Xcc <flag>` (repeatable) passes a flag to the C compiler, in swift2go's compiler runs and in the
  `swift build` it runs: for example `-Xcc -I<dir>` for headers that a system library target needs and
  the SDK lacks.
- Foundation on Linux declares in Swift the classes and options that Darwin imports from Objective-C
  (`Bundle`, `FileManager`, `JSONSerialization.WritingOptions`, ...), some of them in
  `FoundationEssentials`. Their names are respelled as Darwin's, so the output calls the same runtime.
- The resource accessor looks for `<package>_<Target>.resources` beside the executable, as SwiftPM's
  does on Linux, instead of `.bundle`.
- `-incremental` caches dumps per compiler command, which names the triple, so a cache made for one
  platform is not reused for another.
- Test targets need the swift-testing module from the SDK; the static Linux SDK does not ship it.

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
    read with `<Owner><Prop>(recv)` and written with `<Owner>Set<Prop>(&v, x)`. A subscript is read with
    `<Owner>Subscript<suffix>(recv, args...)` and written with `<Owner>SetSubscript<suffix>(&v, args..., x)`.
    A static property is a package variable, or `<Protocol><Prop>[T]()` when a protocol declares it. A
    payload case is a constructor function and a payload-free case a value that `case` patterns compare with.
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
  - An instance property whose Go name is also the name of a type in the package, nested in its owner
    (`value.storage` beside `DatabaseValue.Storage`) or not (`request.body` beside `RequestBody`), is
    read with `<Owner><Prop>Of(recv)`, since the two would share a name.
  - An enum case with a payload is a struct type `<Enum><Case>` with fields `F0`, `F1`, …, which `case`
    patterns match by type and bind from.
  - Its classes are pointers (`*grdb.Database`) and its protocols interfaces, nil for none.
  - A closure passed for a parameter whose function type takes the declaration's generic parameters
    (`memberfn<A>(_ fn: (Self, A) -> R)`) arrives as a `swiftrt.Closure`: the Go function, and whether
    Swift declared each of its parameters Optional. Go gives a class `C` and `C?` the same pointer type, so
    a package that converts values for such a closure reads the flags to tell a required argument from an
    optional one.
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
| `T?` | `*T`, with class references kept as the pointer itself; a function's result is `swiftrt.Opt[T]` for a value type `T` |
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
| `NSRegularExpression` | RE2 with ICU's Unicode classes, or `regexp2`'s backtracking engine for lookaround and backreferences |
| `async`/`await`, `Task`, `AsyncStream`, `for await` | goroutines and channels in `swiftrt` |
| swift-testing `@Test`, `#expect`, `#require`, `arguments:` | `func TestX(t *testing.T)` and subtests |
| freestanding expression macros | the compiler's recorded expansion |
| `main.swift` top-level code | `func main` in `package main`, its variables package variables assigned in source order |
| `any P.Type`, `C.Type` with C known only as P | a value of the conforming type; `init` requirements are `StaticInit…` methods |
| `extension DependencyType: LocalProtocol` | methods written into the dependency's package (`retroactive_<module>.go`), calling hooks this module sets |

The runtime also covers the parts of Foundation, Dispatch, Synchronization, swift-collections,
swift-log, CryptoKit and a few C library calls that the driving codebase uses.

## Design decisions

These are choices made for the codebase that drives the project. Each fits it, and each could become an
option.

- **Overflow traps.** `Int` arithmetic and negation panic on overflow, as a Swift debug build does. `&+` and the other wrapping operators wrap. A range whose upper bound is below its lower bound, and a zero `stride`, trap as in Swift. Shifts are Swift's smart shifts: a negative count shifts the other way.
- **Optionals are pointers.** `T?` is `*T`, and a class's Optional is the class pointer itself. A generic `T?` instantiated with a class is unwrapped at call sites, so callers see `*C`, not `**C`.
- **Optional results are inline.** A function or method that returns a value type's Optional returns `swiftrt.Opt[T]`, the value and a presence flag, so returning one does not allocate. A call takes `Ptr()` of the result, which stays on the stack unless the caller keeps the pointer. An optional chain of a value type builds an `Opt` the same way. A nested Optional (`T??`), an Optional of a tuple or of `Void`, a result that names a generic parameter or stands for an associated type, a local function, a function of a mapped module and one declared in a file an overlay replaces keep `*T`. An override returns what the method it overrides returns, so an overlay that overrides an inline method returns `swiftrt.Opt[T]` too.
- **Value semantics by ownership.** Every variable, property and element holds its own storage, so a mutation writes in place. A struct, array, dictionary, set, optional or protocol value read from one place is copied (`swiftrt.Copy`) when it is stored in another, returned, or captured as a constant by an escaping closure. A function's own locals move to its caller without a copy. A value lent for the length of a call or a `for … in` loop is copied only when the place it was read from can change meanwhile: a class property or a global, when the callee or the loop body can write shared state, or a local that the loop body or a closure writes. A pass over every function body decides which functions can write shared state. Class references, which embed a marker, are never copied.
- **Arrays copy on write.** Copies of an array share its storage, as in Swift, until one of them is written. A slice with spare capacity owns its storage; one whose length equals its capacity may share it, so every in-place write goes through `swiftrt.Mut(&xs)`, which moves such a slice to storage of its own first. `swiftrt.Copy` shares an array when no array inside it owns spare capacity, and copies it otherwise. An Optional of a value type is shared too, and every write through it goes through `swiftrt.MutPtr(&opt)`, which first points it at a copy of its own. Dictionaries, sets and arrays of them are still copied whole. See [docs/COPY_ON_WRITE.md](docs/COPY_ON_WRITE.md).
- **Statics are lazy.** Static and global stored properties become `swiftrt.Lazy` cells initialized on first use, as Swift does, so initialization cycles that Swift allows still work in Go.
- **Swift's evaluation order.** Operands run left to right. Where Go would read a variable after a call to its right, the read is wrapped in `swiftrt.Eval`, a call Go must order first. The right side of `??` runs only when the left is `nil` (`swiftrt.CoalesceFunc`), unless it is a literal or a plain read. A compound assignment evaluates its subscripts and class-returning calls once.
- **Errors are panics.** `throw` panics with the error, and `do`/`catch`, `try?` and `try!` recover it. Functions keep their Go signatures without an `error` result.
- **Observers run on in-place mutation.** `didSet`/`willSet` fire for whole-value assignments and for mutations of part of the value (`dict[k] = v`, `array.append(x)`), except where Swift accesses the storage directly: inside initializers and when a property's own observers assign it on `self`.
- **Names follow Swift.** Methods are named after their base name and argument labels (`pad(_:to:)` becomes `PadTo`). Runtime calls are `swiftrt.<Receiver><Member><Labels>`. Objective-C classes keep their Objective-C names as receivers (`NSFileManagerFileExistsAtPath`). Clashes get a type or `Fn` suffix.
- **Imported option sets are listed by name.** Foreign `OptionSet` types the runtime models (`JSONEncoder.OutputFormatting`, YYJSON's options) are named in the emitter, so array literals of them build option sets.
- **Mapped names come from the Swift declarations.** A replacement package matches the names the emitter generates, rather than the emitter reading names from the package, so the same Swift source always produces the same Go calls.
- **Targets come in dependency order.** A dependency must be transpiled before, or in the same run as, the targets that import it. Otherwise its types are treated as foreign, and calls go to the runtime.

## Limits

- Matching for Swift `Regex` is scalar-based, apart from the trailing `\b` handled above.
- Values in an `HSet` are keyed by a canonical rendering of their fields, not by a custom `hash(into:)`.
- The pass that decides which functions can write shared state assumes library code does not call back
  into the program except through the closures it is handed, and it does not follow other tasks: a value
  lent across an `await` is not copied.
- A nested function that outlives its parent sees the parent's constants as they are when it runs, not as
  they were when it was made. Closures copy them.
- Pointers and C interop cover what the code needs (`gettimeofday`, `clock_gettime`, C strings, pointer arithmetic over C characters, `argv`), not `UnsafePointer` in general. A pointer to C characters cannot move backwards.
- A generic function that takes `swiftrt.TypeT[T]` for a protocol with static requirements cannot be called with a metatype known only at run time (`Self.self` in a protocol extension, an associated type's metatype); the call compiles and traps if it runs.
- A `weak` stored property of a class or struct is a `swiftrt.Weak` field, which reads nil once the
  collector frees the object. The collector frees it some time after Swift would. A `weak` static,
  local or closure capture (`[weak self]`) is still a strong reference, and `unowned` is always strong.
- A `deinit` becomes the method `SwiftDeinit`, which nothing calls, because Go has no deterministic
  destruction. A deinit that only deallocates pointer memory needs no call, since the collector frees it;
  any other deinit is reported, and code that relies on it has to call `SwiftDeinit` itself.
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
