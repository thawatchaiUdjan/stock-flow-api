// Command openapi writes the API contract, generated from the code, to the file named
// by its argument. CI fails when the committed openapi.yaml differs from its output
// (ADR 0013).
package main

//go:generate go run . ../../openapi.yaml

import (
	"fmt"
	"os"

	"github.com/thawatchaiUdjan/stock-flow-api/internal/api"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: openapi <output file>")
		os.Exit(2)
	}
	// The contract describes routes only; no handler runs, so no store is needed.
	_, contract := api.New(nil)
	spec, err := contract.OpenAPI().YAML()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi: render:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[1], spec, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "openapi:", err)
		os.Exit(1)
	}
}
