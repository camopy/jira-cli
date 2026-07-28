// `search jql --fields` narrows the flat per-issue summary projection — it
// must never switch the issues array to Jira's raw wire shape. A consumer
// parsing `.data.issues[].status` (the documented default) has to keep
// working the moment it adds --fields to trim context, so these tests pin
// the shape across the default and --fields paths against the same wire
// payload. --full remains the documented raw-record escape hatch.
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"reflect"
	"testing"
)

// searchProjectionServer serves one canned issue for every POST /search/jql.
func searchProjectionServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"id":"10001","key":"PROJ-1","self":"http://example.invalid/rest/api/3/issue/10001","fields":{"summary":"Checkout returns 500","status":{"name":"Done","statusCategory":{"key":"done","colorName":"green"}},"priority":{"name":"High"},"created":"2026-04-01T09:00:00Z","updated":"2026-05-03T10:00:00Z"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func searchJQLIssues(t *testing.T, bin, cfg string, extraArgs ...string) []map[string]any {
	t.Helper()
	args := append([]string{"--config", cfg, "--output=json", "search", "jql", "project = PROJ"}, extraArgs...)
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("search jql %v error = %v\n%s", extraArgs, err, out)
	}
	var env struct {
		Data struct {
			Issues []map[string]any `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if len(env.Data.Issues) != 1 {
		t.Fatalf("issues = %#v, want exactly one\n%s", env.Data.Issues, out)
	}
	return env.Data.Issues
}

func TestSearchFieldsNarrowsSummaryProjection(t *testing.T) {
	bin := buildJiraBinary(t)
	srv := searchProjectionServer(t)
	cfg := jiraConfig(t, srv.URL)

	byDefault := searchJQLIssues(t, bin, cfg)[0]
	wantDefault := map[string]any{
		"key":             "PROJ-1",
		"summary":         "Checkout returns 500",
		"status":          "Done",
		"status_category": "done",
		"status_color":    "green",
		"assignee":        nil,
		"priority":        "High",
		"created":         "2026-04-01T09:00:00Z",
		"updated":         "2026-05-03T10:00:00Z",
	}
	if !reflect.DeepEqual(byDefault, wantDefault) {
		t.Fatalf("default issue = %#v, want %#v", byDefault, wantDefault)
	}

	narrowed := searchJQLIssues(t, bin, cfg, "--fields", "summary,status")[0]
	wantNarrowed := map[string]any{
		"key":             "PROJ-1",
		"summary":         "Checkout returns 500",
		"status":          "Done",
		"status_category": "done",
		"status_color":    "green",
	}
	if !reflect.DeepEqual(narrowed, wantNarrowed) {
		t.Fatalf("--fields issue = %#v, want the narrowed summary shape %#v", narrowed, wantNarrowed)
	}
}

// --full is the only raw-wire mode: records keep Jira's nested fields object.
func TestSearchFullStaysRawWireShape(t *testing.T) {
	bin := buildJiraBinary(t)
	srv := searchProjectionServer(t)
	cfg := jiraConfig(t, srv.URL)

	full := searchJQLIssues(t, bin, cfg, "--full")[0]
	if full["id"] != "10001" || full["key"] != "PROJ-1" {
		t.Fatalf("--full issue lost the wire identity: %#v", full)
	}
	fields, ok := full["fields"].(map[string]any)
	if !ok {
		t.Fatalf("--full issue fields = %#v, want the nested wire object", full["fields"])
	}
	if fields["summary"] != "Checkout returns 500" {
		t.Fatalf("--full fields.summary = %#v", fields["summary"])
	}
}
