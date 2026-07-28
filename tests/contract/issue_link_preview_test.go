package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type linkPreviewEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		InwardIssue struct {
			Key string `json:"key"`
		} `json:"inward_issue"`
		OutwardIssue struct {
			Key string `json:"key"`
		} `json:"outward_issue"`
		Preview *struct {
			InwardIssueSentence  string `json:"inward_issue_sentence"`
			OutwardIssueSentence string `json:"outward_issue_sentence"`
		} `json:"preview"`
		DryRun bool `json:"dry_run"`
	} `json:"data"`
}

// The create and dry-run envelopes must carry the sentence each endpoint's
// page will display, so a caller can catch a backwards link before (or as)
// it is created. The dry run must resolve the phrases offline from the
// primed linktypes cache — the server is closed before it runs.
func TestIssueLinkCreateAndDryRunCarryPreviewSentences(t *testing.T) {
	var linkPosts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/issueLinkType":
			_, _ = w.Write([]byte(`{"issueLinkTypes":[
				{"id":"10000","name":"Blocks","inward":"is blocked by","outward":"blocks"},
				{"id":"10003","name":"Relates","inward":"relates to","outward":"relates to"}
			]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/issueLink":
			linkPosts++
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Setenv("JIRA_TOKEN_DEFAULT", "test-token")

	bin := buildJiraBinary(t)
	cfg := writeCacheTestConfig(t, srv.URL)
	cacheRoot := t.TempDir()
	env := append(os.Environ(), "XDG_CACHE_HOME="+cacheRoot)

	// Live create: cache is cold, so the preview resolution fetches the
	// link types (priming the cache for the offline dry run below).
	out, err := runWithEnv(bin, env, "--config", cfg, "--output=json", "issue", "link", "PROJ-1", "--to", "PROJ-2", "--type", "Blocks")
	if err != nil {
		t.Fatalf("live link create: %v\n%s", err, out)
	}
	var live linkPreviewEnvelope
	if err := json.Unmarshal(out, &live); err != nil {
		t.Fatalf("parse live envelope: %v\n%s", err, out)
	}
	if !live.OK || live.Data.DryRun || linkPosts != 1 {
		t.Fatalf("live create = ok %v dry_run %v posts %d\n%s", live.OK, live.Data.DryRun, linkPosts, out)
	}
	assertLinkPreview(t, live, "PROJ-1 blocks PROJ-2", "PROJ-2 is blocked by PROJ-1")

	// Offline dry run: the server is gone; the preview must come from the
	// primed cache and the request preview must never contact Jira.
	srv.Close()
	out, err = runWithEnv(bin, env, "--config", cfg, "--output=json", "issue", "link", "PROJ-1", "--to", "PROJ-2", "--type", "Blocks", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run link create: %v\n%s", err, out)
	}
	var dry linkPreviewEnvelope
	if err := json.Unmarshal(out, &dry); err != nil {
		t.Fatalf("parse dry-run envelope: %v\n%s", err, out)
	}
	if !dry.OK || !dry.Data.DryRun {
		t.Fatalf("dry run = ok %v dry_run %v\n%s", dry.OK, dry.Data.DryRun, out)
	}
	assertLinkPreview(t, dry, "PROJ-1 blocks PROJ-2", "PROJ-2 is blocked by PROJ-1")
}

func assertLinkPreview(t *testing.T, env linkPreviewEnvelope, wantInward, wantOutward string) {
	t.Helper()
	if env.Data.Preview == nil {
		t.Fatalf("envelope carries no data.preview: %+v", env.Data)
	}
	if env.Data.Preview.InwardIssueSentence != wantInward || env.Data.Preview.OutwardIssueSentence != wantOutward {
		t.Fatalf("preview sentences = %q / %q, want %q / %q",
			env.Data.Preview.InwardIssueSentence, env.Data.Preview.OutwardIssueSentence, wantInward, wantOutward)
	}
}
