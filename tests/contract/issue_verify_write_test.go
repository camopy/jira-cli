package contract

// Regression coverage for `issue create --verify`: Jira applies writes
// field-by-field and can silently drop values it cannot honor while still
// returning 2xx. With --verify the CLI re-fetches the issue and diffs the
// requested fields against the server's applied values — a create whose
// --label/--parent diverge from the re-fetch MUST surface field_not_applied
// warnings and a verification.dropped list, never a clean success. The
// matching-server twin pins the false-positive guard: identical applied
// fields produce ZERO warnings.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// verifyCreatemetaFields is the create-screen metadata for a project
// accepting the wire system fields plus labels and parent.
const verifyCreatemetaFields = `[` +
	`{"fieldId":"project","name":"Project","required":true,"schema":{"type":"project"}},` +
	`{"fieldId":"issuetype","name":"Issue Type","required":true,"schema":{"type":"issuetype"}},` +
	`{"fieldId":"summary","name":"Summary","required":true,"schema":{"type":"string"}},` +
	`{"fieldId":"labels","name":"Labels","required":false,"schema":{"type":"array","items":"string"}},` +
	`{"fieldId":"parent","name":"Parent","required":false,"schema":{"type":"issuelink"}}` +
	`]`

// verifyEnvelope is the envelope slice the --verify assertions read.
type verifyEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		Verification *struct {
			Applied map[string]any `json:"applied"`
			Dropped []struct {
				Field     string `json:"field"`
				Requested any    `json:"requested"`
				Applied   any    `json:"applied"`
			} `json:"dropped"`
		} `json:"verification"`
	} `json:"data"`
	Warnings []struct {
		Type    string `json:"type"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"warnings"`
}

// runVerifiedCreate runs `issue create --verify` against a mock whose GET
// answers with fetchedFields, returning the decoded envelope.
func runVerifiedCreate(t *testing.T, fetchedFields string) verifyEnvelope {
	t.Helper()
	mux := http.NewServeMux()
	registerCreatemeta(mux, "PROJ", "Task", "10002", verifyCreatemetaFields)
	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","key":"PROJ-1","self":"x"}`))
	})
	mux.HandleFunc("GET /rest/api/3/issue/PROJ-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","key":"PROJ-1","fields":` + fetchedFields + `}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := jiraConfig(t, srv.URL)
	stdout, stderr, code := runJira(t, "--config", cfg,
		"issue", "create", "--no-input", "--output=json", "--verify",
		"--project", "PROJ", "--type", "Task", "--summary", "Verified",
		"--label", "alpha", "--label", "beta", "--parent", "PROJ-100")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	var env verifyEnvelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout)
	}
	return env
}

// TestIssueCreateVerifySurfacesDroppedFields: the server kept none of the
// requested labels and nulled the parent — the envelope must stay ok:true
// (the write succeeded) but carry field_not_applied warnings and the
// dropped list.
func TestIssueCreateVerifySurfacesDroppedFields(t *testing.T) {
	env := runVerifiedCreate(t,
		`{"summary":"Verified","issuetype":{"id":"10002","name":"Task"},"labels":["gamma"],"parent":null}`)

	if !env.OK {
		t.Fatalf("ok = false; a verified drop is a warning, not a failure")
	}
	if env.Data.Verification == nil {
		t.Fatalf("envelope carries no verification block")
	}
	droppedFields := map[string]bool{}
	for _, d := range env.Data.Verification.Dropped {
		droppedFields[d.Field] = true
	}
	if !droppedFields["labels"] || !droppedFields["parent"] || len(droppedFields) != 2 {
		t.Fatalf("verification.dropped = %+v, want exactly labels and parent", env.Data.Verification.Dropped)
	}
	warned := map[string]bool{}
	for _, w := range env.Warnings {
		if w.Type == "field_not_applied" {
			warned[w.Field] = true
		}
	}
	if !warned["labels"] || !warned["parent"] {
		t.Fatalf("warnings = %+v, want field_not_applied for labels and parent", env.Warnings)
	}
}

// TestIssueCreateVerifyMatchingResultHasZeroWarnings is the false-positive
// guard end-to-end: the server applied everything requested (plus an
// automation-added label, which is not a drop) — zero warnings, empty
// dropped list.
func TestIssueCreateVerifyMatchingResultHasZeroWarnings(t *testing.T) {
	env := runVerifiedCreate(t,
		`{"summary":"Verified","issuetype":{"id":"10002","name":"Task"},"labels":["alpha","beta","automation-added"],"parent":{"id":"9","key":"PROJ-100"}}`)

	if !env.OK {
		t.Fatalf("ok = false for a fully applied write")
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("false positive: warnings = %+v, want none", env.Warnings)
	}
	if env.Data.Verification == nil {
		t.Fatalf("envelope carries no verification block")
	}
	if len(env.Data.Verification.Dropped) != 0 {
		t.Fatalf("false positive: dropped = %+v, want empty", env.Data.Verification.Dropped)
	}
}
