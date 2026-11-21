# The SwiftPM package to transpile; set SWIFT_PKG to its directory.
swift_pkg := env_var_or_default("SWIFT_PKG", ".")

build:
    go build ./cmd/... ./internal/... ./swiftrt/...

test:
    go test ./internal/... ./swiftrt/...

lint:
    golangci-lint run ./cmd/... ./internal/... ./swiftrt/...

# Format, vet, lint and test, as CI does
precommit:
    golangci-lint fmt
    go vet ./cmd/... ./internal/... ./swiftrt/...
    golangci-lint run ./cmd/... ./internal/... ./swiftrt/...
    go test ./internal/... ./swiftrt/...

# Transpile targets in dependency order, e.g. `just transpile MyLib MyLibTests`
transpile +targets:
    go run ./cmd/swift2go -pkg {{swift_pkg}} transpile {{targets}}

# Transpile a target and its test target, then vet and run the tests as Go, e.g. `just check MyLib MyLibTests`
check target tests:
    go run ./cmd/swift2go -pkg {{swift_pkg}} transpile {{target}} {{tests}}
    cd out/{{lowercase(target)}} && go vet . && go test -count=1 .

# Count Go compile errors in a transpiled package
errors pkg:
    cd out/{{pkg}} && go test -c -o /dev/null -gcflags=-e . 2>&1 | sed 's/^[^ ]*: //' | sort | uniq -c | sort -rn
