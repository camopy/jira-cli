package contract

// Doc <-> code lockstep for the error taxonomy: the agent guide's core
// contract must document every exit code the mapper can produce and every
// field the error struct can emit. The field list is derived from
// cli.Error's json tags by reflection, so adding a field to the struct
// without documenting it fails here — and vice versa the exit table cannot
// silently drift from ExitCode.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
)

func coreContractText(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "internal", "cli", "agent", "guide", "core_contract.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read core_contract.md: %v", err)
	}
	return string(raw)
}

// TestCoreContractDocumentsEveryExitCode pins the exit table to the full
// 0-7 set the code emits (canceled=6 and timeout=7 included).
func TestCoreContractDocumentsEveryExitCode(t *testing.T) {
	doc := coreContractText(t)
	for _, row := range []string{
		"| 0 ", "| 1 ", "| 2 ", "| 3 ", "| 4 ", "| 5 ", "| 6 ", "| 7 ",
	} {
		if !strings.Contains(doc, row) {
			t.Errorf("exit table is missing the %q row", strings.TrimSpace(row))
		}
	}
	for _, marker := range []string{"`code=canceled`", "`code=timeout`", "`code=read_only`"} {
		if !strings.Contains(doc, marker) {
			t.Errorf("contract does not document %s", marker)
		}
	}
}

// TestCoreContractDocumentsEveryErrorField derives the emittable envelope
// error fields from cli.Error's json tags and requires each to appear in
// the contract's errors[] field list.
func TestCoreContractDocumentsEveryErrorField(t *testing.T) {
	doc := coreContractText(t)
	typ := reflect.TypeOf(cli.Error{})
	for i := range typ.NumField() {
		tag := typ.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		if !strings.Contains(doc, "`"+name+"`") {
			t.Errorf("error field %q is emitted by cli.Error but not documented in core_contract.md", name)
		}
	}
}
