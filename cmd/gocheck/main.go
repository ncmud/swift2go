// Command gocheck type-checks Go packages and prints every error, even in packages whose imports fail.
package main

import (
	"fmt"
	"os"

	"golang.org/x/tools/go/packages"
)

func main() {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports, Tests: true}
	pkgs, err := packages.Load(cfg, os.Args[1:]...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	seen := map[string]bool{}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			s := e.Error()
			if !seen[s] {
				seen[s] = true
				fmt.Println(s)
			}
		}
	}
}
