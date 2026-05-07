// Contract tests for the new
// `jira issue link list/delete/types` sub-commands.
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

// link list contract: derives from GET /issue/{key}?fields=issuelinks;
// outward+inward unified into single sorted array.
func TestIssueLinkListUnifiesInwardOutwardSortedEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-1") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"key": "PROJ-1",
			"fields": {
				"issuelinks": [
					{"id":"9001","type":{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"},"outwardIssue":{"key":"PROJ-200","fields":{"summary":"downstream","status":{"name":"In Progress"}}}},
					{"id":"9002","type":{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"},"inwardIssue":{"key":"PROJ-100","fields":{"summary":"upstream","status":{"name":"Done"}}}},
					{"id":"9003","type":{"id":"10001","name":"Relates","inward":"relates to","outward":"relates to"},"outwardIssue":{"key":"PROJ-150","fields":{"summary":"sibling","status":{"name":"To Do"}}}}
				]
			}
		}`))
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "--json", "issue", "link", "list", "PROJ-1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue link list error = %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue link list output is not JSON: %v\n%s", err, out)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing data: %+v", env)
	}
	links, ok := data["links"].([]any)
	if !ok {
		t.Fatalf("data.links is not array: %+v", data)
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d: %+v", len(links), links)
	}
	wantIDs := []string{"9002", "9001", "9003"}
	for i, raw := range links {
		link := raw.(map[string]any)
		if link["id"] != wantIDs[i] {
			t.Fatalf("position %d: got id=%v want=%s", i, link["id"], wantIDs[i])
		}
	}
	first := links[0].(map[string]any)
	if first["direction"] != "inward" {
		t.Fatalf("first link direction: got %v want inward", first["direction"])
	}
	other, _ := first["other_issue"].(map[string]any)
	if other["key"] != "PROJ-100" {
		t.Fatalf("first link other_issue: %+v", first["other_issue"])
	}
	tp, _ := first["type"].(map[string]any)
	if tp["name"] != "Blocks" {
		t.Fatalf("first link type: %+v", first["type"])
	}
}

// link delete: DELETE /issueLink/{id}, force-gate, response
// data.link_id echoes the supplied id verbatim regardless of source KEY.
func TestIssueLinkDeleteForceGateAndIDEcho(t *testing.T) {
	cfg := jiraConfig(t, "http://127.0.0.1:1")
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "issue", "link", "delete", "PROJ-1", "9001", "--no-input", "--json")
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected force-gate failure under --no-input without --force")
	}

	var deleted atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/rest/api/3/issueLink/9001" {
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfg2 := jiraConfig(t, srv.URL)
	cmd = exec.Command("go", "run", "../../cmd/jira", "--config", cfg2, "issue", "link", "delete", "PROJ-1", "9001", "--force", "--no-input", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue link delete error = %v\n%s", err, out)
	}
	if !deleted.Load() {
		t.Fatal("DELETE /issueLink/9001 was not called")
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("issue link delete output is not JSON: %v\n%s", err, out)
	}
	data, _ := env["data"].(map[string]any)
	if data["link_id"] != "9001" {
		t.Fatalf("data.link_id should echo 9001 verbatim; got %+v", data)
	}
	if data["deleted"] != true {
		t.Fatalf("data.deleted should be true; got %+v", data)
	}
}

// dry-run: --dry-run must not call DELETE.
func TestIssueLinkDeleteDryRunNoCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("dry-run must not call Jira; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "issue", "link", "delete", "PROJ-1", "9001", "--dry-run", "--no-input", "--force", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue link delete --dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"dry_run": true`) {
		t.Fatalf("dry-run envelope missing dry_run flag: %s", out)
	}
}

// link types: cache-backed, --refresh forces fetch.
func TestIssueLinkTypesCachedAndRefresh(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issueLinkType" {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"},{"id":"10001","name":"Cloners","inward":"is cloned by","outward":"clones"}]}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	out, err := runWithEnv(bin, env, "--config", cfg, "issue", "link", "types", "--json")
	if err != nil {
		t.Fatalf("link types first call: %v\n%s", err, out)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit after first call, got %d", hits.Load())
	}
	var env1 map[string]any
	if err := json.Unmarshal(out, &env1); err != nil {
		t.Fatalf("parse first envelope: %v\n%s", err, out)
	}
	data1, _ := env1["data"].(map[string]any)
	if data1["from_cache"] != false {
		t.Fatalf("first call should be from_cache=false; got %+v", data1)
	}
	types1, _ := data1["link_types"].([]any)
	if len(types1) != 2 {
		t.Fatalf("expected 2 link types, got %d", len(types1))
	}

	out, err = runWithEnv(bin, env, "--config", cfg, "issue", "link", "types", "--json")
	if err != nil {
		t.Fatalf("link types second call: %v\n%s", err, out)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected cached second call (still 1 hit), got %d", hits.Load())
	}
	var env2 map[string]any
	if err := json.Unmarshal(out, &env2); err != nil {
		t.Fatalf("parse second envelope: %v\n%s", err, out)
	}
	data2, _ := env2["data"].(map[string]any)
	if data2["from_cache"] != true {
		t.Fatalf("second call should be from_cache=true; got %+v", data2)
	}

	if _, err := runWithEnv(bin, env, "--config", cfg, "issue", "link", "types", "--refresh", "--json"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after --refresh, got %d", hits.Load())
	}
}

// extension: --raw returns Atlassian's native {issueLinkTypes:[...]} verbatim.
func TestIssueLinkTypesRawPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issueLinkTypes":[{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"}]}`))
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	env := append(os.Environ(), "XDG_CACHE_HOME="+t.TempDir())

	out, err := runWithEnv(bin, env, "--config", cfg, "--raw", "issue", "link", "types")
	if err != nil {
		t.Fatalf("link types --raw: %v\n%s", err, out)
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		t.Fatalf("--raw output is not JSON: %v\n%s", err, out)
	}
	if _, ok := raw["issueLinkTypes"]; !ok {
		t.Fatalf("--raw missing Atlassian's `issueLinkTypes` key: %+v", raw)
	}
	if _, ok := raw["data"]; ok {
		t.Fatalf("--raw must not wrap in CLI envelope; saw `data`: %+v", raw)
	}
}

// Existing `jira issue link KEY --to OTHER --type X` create form must still
// work after the sub-command refactor (preserved as default action).
func TestIssueLinkCreateBackCompat(t *testing.T) {
	var got atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issueLink" {
			got.Store(true)
			w.WriteHeader(http.StatusCreated)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	cfg := jiraConfig(t, srv.URL)
	cmd := exec.Command("go", "run", "../../cmd/jira", "--config", cfg, "issue", "link", "PROJ-1", "--to", "PROJ-2", "--type", "Blocks", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("issue link create back-compat: %v\n%s", err, out)
	}
	if !got.Load() {
		t.Fatal("legacy create form did not POST /issueLink")
	}
}
