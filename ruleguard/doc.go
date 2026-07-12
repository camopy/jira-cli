//go:build ruleguard

// Package gorules holds the ruleguard rules golangci-lint runs via gocritic.
// The build tag keeps `go build`/`go vet` from compiling this file — only the
// linter's bundled ruleguard engine reads it.
package gorules
