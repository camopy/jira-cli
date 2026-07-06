// Object-valued system fields accept flat-string spellings ("project":
// "PROJ", "priority": "Medium") on create and edit: the CLI lifts each bare
// string to its one fixed identity key before the pipeline, so the shape
// that used to pass --dry-run and then 400 on the wire now submits what
// Jira actually accepts. Explicit wire objects must pass through untouched.
package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func writeJSONPayload(t *testing.T, name, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestCreateDryRunLiftsFlatStringSystemFields(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := jiraConfigNoServer(t)
	// Wire spellings with bare-string values — the exact flat shape agents
	// write from REST muscle memory. OTHER differs from the profile default
	// so the preview provably read the lifted object, not the fallback.
	payload := writeJSONPayload(t, "create.json", `{
		"summary": "Flat strings",
		"project": "OTHER",
		"issuetype": "Bug",
		"parent": "PROJ-70",
		"priority": "Medium",
		"components": ["ui", "api"]
	}`)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"issue", "create", "--no-input", "--dry-run", "--json-input", payload).CombinedOutput()
	if err != nil {
		t.Fatalf("create dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Preview map[string]any `json:"preview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	preview := env.Data.Preview
	if preview["project_key"] != "OTHER" || preview["issue_type"] != "Bug" {
		t.Fatalf("preview identity = %v/%v, want OTHER/Bug (bare strings were not lifted)\n%s", preview["project_key"], preview["issue_type"], out)
	}
	for key, want := range map[string]any{
		"parent":     map[string]any{"key": "PROJ-70"},
		"priority":   map[string]any{"name": "Medium"},
		"components": []any{map[string]any{"name": "ui"}, map[string]any{"name": "api"}},
	} {
		if !reflect.DeepEqual(preview[key], want) {
			t.Fatalf("preview %s = %#v, want wire shape %#v\n%s", key, preview[key], want, out)
		}
	}
}

func TestEditDryRunLiftsFlatStringSystemFields(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := jiraConfigNoServer(t)
	payload := writeJSONPayload(t, "edit.json", `{
		"fields": {
			"parent": "PROJ-70",
			"priority": "Medium",
			"assignee": "712020:abc",
			"reporter": "712020:def",
			"components": ["ui"],
			"fixVersions": ["1.1.0"],
			"versions": ["1.0.0"]
		}
	}`)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"issue", "edit", "PROJ-1", "--no-input", "--dry-run", "--json-input", payload).CombinedOutput()
	if err != nil {
		t.Fatalf("edit dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Fields map[string]any `json:"fields"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	want := map[string]any{
		"parent":      map[string]any{"key": "PROJ-70"},
		"priority":    map[string]any{"name": "Medium"},
		"assignee":    map[string]any{"accountId": "712020:abc"},
		"reporter":    map[string]any{"accountId": "712020:def"},
		"components":  []any{map[string]any{"name": "ui"}},
		"fixVersions": []any{map[string]any{"name": "1.1.0"}},
		"versions":    []any{map[string]any{"name": "1.0.0"}},
	}
	if !reflect.DeepEqual(env.Data.Fields, want) {
		t.Fatalf("edit fields = %#v, want every system field in wire shape %#v\n%s", env.Data.Fields, want, out)
	}
}

// Explicit wire objects — including id addressing — pass through untouched;
// the lift never rewrites a shape the caller chose.
func TestEditDryRunKeepsExplicitWireObjects(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := jiraConfigNoServer(t)
	payload := writeJSONPayload(t, "edit.json", `{
		"fields": {
			"priority": {"id": "3"},
			"components": [{"id": "10001"}]
		}
	}`)

	out, err := exec.Command(bin, "--config", cfg, "--output=json",
		"issue", "edit", "PROJ-1", "--no-input", "--dry-run", "--json-input", payload).CombinedOutput()
	if err != nil {
		t.Fatalf("edit dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			Fields map[string]any `json:"fields"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	want := map[string]any{
		"priority":   map[string]any{"id": "3"},
		"components": []any{map[string]any{"id": "10001"}},
	}
	if !reflect.DeepEqual(env.Data.Fields, want) {
		t.Fatalf("edit fields = %#v, want explicit objects untouched %#v\n%s", env.Data.Fields, want, out)
	}
}
