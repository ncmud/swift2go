ncmud := env_var_or_default("NCMUD", env_var("HOME") + "/dev/nc/ncmud")

build:
    go build ./cmd/... ./internal/... ./swiftrt/...

test:
    go test ./internal/... ./swiftrt/...

# Transpile targets in dependency order, e.g. `just transpile TermCore TermCoreTests`
transpile +targets:
    go run ./cmd/swift2go -pkg {{ncmud}} transpile {{targets}}

# Transpile TermCore and its tests, then build and run them as Go
termcore:
    go run ./cmd/swift2go -pkg {{ncmud}} transpile TermCore TermCoreTests
    cd out/termcore && go vet . && go test -count=1 .

# Count Go compile errors in a transpiled package
errors pkg:
    cd out/{{pkg}} && go test -c -o /dev/null -gcflags=-e . 2>&1 | sed 's/^[^ ]*: //' | sort | uniq -c | sort -rn
