package customfield_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/pkg/jira/customfield"
)

// contracts/customfield-matrix.md MUST regenerate byte-identically
// from the registry. Mirror of the pkg/adf matrix-freshness gate.
func TestCustomfieldMatrixIsFresh(t *testing.T) {
	want := customfield.GenerateMatrix()
	matrixPath := filepath.Join("..", "..", "..", "docs", "customfield-matrix.md")
	got, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read %s: %v\n\nRun `go run ./pkg/jira/customfield/genmatrix > %s`.", matrixPath, err, matrixPath)
	}
	if string(want) != string(got) {
		t.Fatalf("customfield-matrix.md is stale. Re-run the generator:\n\n\tgo run ./pkg/jira/customfield/genmatrix > %s\n", matrixPath)
	}
}
