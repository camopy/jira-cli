// Regenerate contracts/customfield-matrix.md from the registry.
//
// Usage: go run ./pkg/jira/customfield/genmatrix
package main

import (
	"os"

	"github.com/matcra587/jira-cli/pkg/jira/customfield"
)

func main() {
	if _, err := os.Stdout.Write(customfield.GenerateMatrix()); err != nil {
		panic(err)
	}
}
