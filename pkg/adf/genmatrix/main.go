// Package main: regenerate contracts/adf-support-matrix.md from the registry.
//
// Usage: go run ./pkg/adf/genmatrix
package main

import (
	"os"

	"github.com/matcra587/jira-cli/pkg/adf"
)

func main() {
	if _, err := os.Stdout.Write(adf.GenerateMatrix()); err != nil {
		panic(err)
	}
}
