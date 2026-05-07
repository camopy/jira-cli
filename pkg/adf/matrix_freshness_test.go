package adf_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/pkg/adf"
)

// contracts/adf-support-matrix.md MUST regenerate byte-identically
// from the registry. The CI gate is "run the generator, read the file
// on disk, fail if they differ".
//
// Drift is a stale-doc smell — the planning artifact must always reflect
// the registered support set.
func TestADFMatrixIsFresh(t *testing.T) {
	want := adf.GenerateMatrix()

	matrixPath := filepath.Join("..", "..", "docs", "adf-support-matrix.md")
	got, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read %s: %v\n\nRun `go run ./pkg/adf/genmatrix > %s`.", matrixPath, err, matrixPath)
	}

	if string(want) != string(got) {
		t.Fatalf("contracts/adf-support-matrix.md is stale. Re-run the generator:\n\n\tgo run ./pkg/adf/genmatrix > %s\n\nfirst-bytes diff (want vs got):\n  want bytes: %d\n  got  bytes: %d", matrixPath, len(want), len(got))
	}
}
