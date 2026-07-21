package contract

// Guardrail: the output schemas `jira agent schema` publishes must describe
// the envelopes the binary actually emits. Each case runs a command whose
// envelope is producible hermetically (dry-run or local-only), then checks
// data against the declared schema with a minimal JSON-Schema-subset walker
// (type, required, properties, items). Declared-but-conditional fields are
// fine; an emitted field missing from the declaration, a wrong type, or an
// absent required field fails. This is the test the data.issue string/object
// drift would have failed.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmittedEnvelopesConformToDeclaredSchemas(t *testing.T) {
	editPayload := filepath.Join(t.TempDir(), "edit.json")
	if err := os.WriteFile(editPayload, []byte(`{"summary":"renamed"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	createPayload := filepath.Join(t.TempDir(), "create.json")
	if err := os.WriteFile(createPayload, []byte(`{"summary":"Hi","project_key":"PROJ","issue_type":"Task"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cases := []struct {
		schemaKey string
		args      []string
		stub      bool // serve canned Jira responses instead of failing on any request
	}{
		{"issue.create", []string{"issue", "create", "--dry-run", "--no-input", "--json-input", createPayload}, false},
		{"issue.edit", []string{"issue", "edit", "PROJ-1", "--dry-run", "--no-input", "--json-input", editPayload}, false},
		{"issue.transition", []string{"issue", "transition", "PROJ-1", "--dry-run", "--transition", "31"}, false},
		{"issue.rank", []string{"issue", "rank", "PROJ-1", "PROJ-2", "--before", "PROJ-9", "--dry-run"}, false},
		{"worklog.add", []string{"worklog", "add", "PROJ-1", "--dry-run", "--time-spent", "1h"}, false},
		{"cache.clear", []string{"cache", "clear", "--dry-run"}, false},
		{"issue.view", []string{"issue", "view", "PROJ-1"}, true},
		{"issue.view", []string{"issue", "view", "PROJ-1", "PROJ-2"}, true},
		{"issue.list", []string{"issue", "list", "--jql", "project = PROJ"}, true},
		{"issue.comment.list", []string{"issue", "comment", "list", "PROJ-1"}, true},
		{"issue.attachment.list", []string{"issue", "attachment", "list", "PROJ-1"}, true},
		{"issue.link.list", []string{"issue", "link", "list", "PROJ-1"}, true},
		{"issue.link.types", []string{"issue", "link", "types"}, true},
		{"worklog.list", []string{"worklog", "list", "PROJ-1"}, true},
	}

	schemas := declaredOutputSchemas(t)
	for _, c := range cases {
		t.Run(c.schemaKey, func(t *testing.T) {
			var baseURL string
			if c.stub {
				baseURL = stubReadServer(t).URL
			} else {
				srv, _ := failOnAnyRequestServer(t)
				baseURL = srv.URL
			}
			cfg := jiraConfig(t, baseURL)
			args := append([]string{"--config", cfg, "--output=json"}, c.args...)
			stdout, stderr, code := runJira(t, args...)
			if code != 0 {
				t.Fatalf("%v exit = %d\nstdout=%s\nstderr=%s", c.args, code, stdout, stderr)
			}
			var env struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("envelope decode: %v\n%s", err, stdout)
			}
			schema, ok := schemas[c.schemaKey].(map[string]any)
			if !ok {
				t.Fatalf("no declared output schema for %q", c.schemaKey)
			}
			if errs := conformanceErrors("data", env.Data, schema); len(errs) > 0 {
				t.Fatalf("%s envelope does not conform to its declared schema:\n%s\nstdout=%s", c.schemaKey, errs, stdout)
			}
		})
	}
}

// stubReadServer serves the minimal canned Jira responses the read-op
// conformance cases need. Shapes mirror the fixtures the dedicated contract
// tests use; a path with no handler 404s and fails the case loudly.
func stubReadServer(t *testing.T) *httptest.Server {
	t.Helper()
	issueJSON := `{"id":"10001","key":"PROJ-1","self":"https://example/rest/api/3/issue/10001","fields":{` +
		`"summary":"Stub issue","updated":"2026-07-01T10:00:00.000+0000",` +
		`"status":{"name":"To Do","statusCategory":{"key":"new","colorName":"blue-gray"}},` +
		`"issuetype":{"name":"Task"},"project":{"key":"PROJ"},"priority":{"name":"Medium"},` +
		`"issuelinks":[{"id":"9001","self":"https://example/rest/api/3/issueLink/9001",` +
		`"type":{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"},` +
		`"outwardIssue":{"key":"PROJ-2","fields":{"summary":"Other","status":{"name":"To Do"}}}}]}}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(issueJSON))
	})
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-2", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.ReplaceAll(issueJSON, "PROJ-1", "PROJ-2")))
	})
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/comment", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"comments":[{"id":"10042","body":{"type":"doc","version":1,"content":[]},` +
			`"author":{"accountId":"712020:abc","displayName":"Alice"},` +
			`"updateAuthor":{"accountId":"712020:abc","displayName":"Alice"},` +
			`"created":"2026-07-01T10:00:00.000+0000","updated":"2026-07-01T10:00:00.000+0000"}],` +
			`"startAt":0,"maxResults":50,"total":1}`))
	})
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1/worklog", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"worklogs":[{"id":"10169","timeSpentSeconds":3600,` +
			`"started":"2026-07-01T09:00:00.000+0000"}],"startAt":0,"maxResults":50,"total":1}`))
	})
	mux.HandleFunc("GET /rest/api/3/issueLinkType", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issueLinkTypes":[{"id":"10000","name":"Blocks",` +
			`"inward":"is blocked by","outward":"blocks"}]}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/search/jql" {
			_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"PROJ-1","fields":{` +
				`"summary":"Stub issue","updated":"2026-07-01T10:00:00.000+0000",` +
				`"status":{"name":"To Do","statusCategory":{"key":"new","colorName":"blue-gray"}}}}]}`))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// declaredOutputSchemas reads output_schemas from the built binary's own
// `agent schema` output, so the test pins the published contract rather
// than an internal package view.
func declaredOutputSchemas(t *testing.T) map[string]any {
	t.Helper()
	stdout, stderr, code := runJira(t, "--output=compact", "agent", "schema")
	if code != 0 {
		t.Fatalf("agent schema exit = %d\nstderr=%s", code, stderr)
	}
	var data struct {
		OutputSchemas map[string]any `json:"output_schemas"`
	}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		t.Fatalf("agent schema decode: %v", err)
	}
	if len(data.OutputSchemas) == 0 {
		t.Fatal("agent schema published no output_schemas")
	}
	return data.OutputSchemas
}

// conformanceErrors walks a decoded JSON value against the declared schema
// subset. Undeclared emitted keys are violations: the schema must describe
// everything an agent can see.
func conformanceErrors(path string, value any, schema map[string]any) []string {
	var errs []string
	if !typeMatches(value, schema["type"]) {
		return []string{fmt.Sprintf("%s: value %v does not match declared type %v", path, value, schema["type"])}
	}
	obj, isObj := value.(map[string]any)
	if !isObj {
		if list, isList := value.([]any); isList {
			if items, ok := schema["items"].(map[string]any); ok {
				for i, item := range list {
					errs = append(errs, conformanceErrors(fmt.Sprintf("%s[%d]", path, i), item, items)...)
				}
			}
		}
		return errs
	}
	props, hasProps := schema["properties"].(map[string]any)
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			name, _ := r.(string)
			if _, present := obj[name]; !present {
				errs = append(errs, fmt.Sprintf("%s: required field %q absent", path, name))
			}
		}
	}
	// A schema with no properties map declares an opaque object (e.g. the
	// edit payload's fields echo): its contents are caller data, not
	// contract surface, so nothing inside it can be "undeclared".
	if !hasProps {
		return errs
	}
	for key, val := range obj {
		sub, declared := props[key].(map[string]any)
		if !declared {
			errs = append(errs, fmt.Sprintf("%s.%s: emitted but not declared in the schema", path, key))
			continue
		}
		errs = append(errs, conformanceErrors(path+"."+key, val, sub)...)
	}
	return errs
}

// typeMatches maps decoded-JSON Go types onto the declared JSON Schema type,
// which may be a single string or a list of alternatives. An absent type
// declaration matches anything.
func typeMatches(value, declared any) bool {
	switch d := declared.(type) {
	case nil:
		return true
	case string:
		return jsonTypeOf(value) == d || (d == "number" && jsonTypeOf(value) == "integer")
	case []any:
		for _, alt := range d {
			if s, ok := alt.(string); ok && typeMatches(value, s) {
				return true
			}
		}
		return false
	}
	return true
}

func jsonTypeOf(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case float64:
		if v == float64(int64(v)) {
			return "integer"
		}
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}
