//go:build live

// Package issues holds the live Jira end-to-end suite for the issue and
// worklog command domains. It runs only under the `live` build tag and
// drives the real jira binary against the project named by
// JIRA_LIVETEST_PROJECT.
package issues

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matcra587/jira-cli/tests/live/internal/livekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	livekit.PrintSurvivors()
	livekit.CleanupBinary()
	os.Exit(code)
}

// liveCoveredMatrixIDs lists every issues_matrix.yaml row this suite
// exercises. matrix_test.go cross-checks it against the matrix file.
var liveCoveredMatrixIDs = map[string]struct{}{
	"issue.attachment.add":      {},
	"issue.attachment.delete":   {},
	"issue.attachment.download": {},
	"issue.attachment.list":     {},
	"issue.clone":               {},
	"issue.comment.add":         {},
	"issue.comment.delete":      {},
	"issue.comment.edit":        {},
	"issue.comment.legacy_add":  {},
	"issue.comment.list":        {},
	"issue.create.description":  {},
	"issue.create.dry_run":      {},
	"issue.create.minimal":      {},
	"issue.delete":              {},
	"issue.edit.assignee":       {},
	"issue.edit.description":    {},
	"issue.edit.dry_run":        {},
	"issue.edit.fields":         {},
	"issue.edit.labels":         {},
	"issue.edit.summary":        {},
	"issue.edit.unassign":       {},
	"issue.link.create":         {},
	"issue.link.delete":         {},
	"issue.link.list":           {},
	"issue.link.types":          {},
	"issue.list":                {},
	"issue.list.as_jql":         {},
	"issue.mine":                {},
	"issue.move":                {},
	"issue.transition.execute":  {},
	"issue.transition.list":     {},
	"issue.unwatch":             {},
	"issue.view":                {},
	"issue.watch":               {},
	"issue.watchers.add":        {},
	"issue.watchers.list":       {},
	"issue.watchers.remove":     {},
	"issue.weblink":             {},
	"worklog.add":               {},
	"worklog.list":              {},
}

func TestLiveIssuesEndToEnd(t *testing.T) {
	s := livekit.NewSuite(t)
	accountID := s.SelfAccountID(t)

	minimalKey := s.CreateIssue(t, s.Marker+" minimal disposable", nil)
	s.TrackCleanup(minimalKey)

	survivorKey := s.CreateIssue(t, livekit.SurvivorSummary(s.RunID), map[string]any{
		"description_markdown": "Live test survivor created with **Markdown** for run `" + s.RunID + "`.",
	})
	livekit.RecordSurvivor(survivorKey, s.BaseURL+"/browse/"+survivorKey)
	t.Logf("LIVE JIRA SURVIVOR: %s %s/browse/%s", survivorKey, s.BaseURL, survivorKey)

	linkPeerKey := s.CreateIssue(t, s.Marker+" link peer", map[string]any{
		"description": livekit.ADFDoc("Peer issue used only for link coverage in run " + s.RunID),
	})
	s.TrackCleanup(linkPeerKey)

	t.Run("create dry-run does not create an issue", func(t *testing.T) {
		searchToken := "dryruncreate-" + strings.ToLower(strings.ReplaceAll(s.RunID, "-", ""))
		drySummary := s.Marker + " " + searchToken
		env := s.Run(t, "issue", "create", "--dry-run", "--json-input", s.WriteJSON(t, "dry-run-create.json", map[string]any{
			"project_key":          s.Project,
			"issue_type":           s.IssueType,
			"summary":              drySummary,
			"description_markdown": "This dry-run issue must not exist.",
		}))
		assert.True(t, livekit.BoolField(t, env.Data, "dry_run"))
		assertNoIssueWithSummary(t, s, drySummary, searchToken)
	})

	t.Run("view and edit survivor", func(t *testing.T) {
		view := s.Run(t, "issue", "view", survivorKey)
		require.Contains(t, livekit.IssueSummary(view), "SURVIVOR "+s.RunID)

		drySummary := s.Marker + " dry-run edit should not persist"
		env := s.Run(t, "issue", "edit", survivorKey, "--summary", drySummary, "--dry-run")
		assert.True(t, livekit.BoolField(t, env.Data, "dry_run"))
		afterDryRun := s.Run(t, "issue", "view", survivorKey)
		assert.NotEqual(t, drySummary, livekit.IssueSummary(afterDryRun))

		editedSummary := livekit.SurvivorSummary(s.RunID) + " - summary edited"
		s.Run(t, "issue", "edit", survivorKey, "--summary", editedSummary)
		assert.Equal(t, editedSummary, livekit.IssueSummary(s.Run(t, "issue", "view", survivorKey)))

		s.Run(t, "issue", "edit", survivorKey, "--json-input", s.WriteJSON(t, "edit-description.json", map[string]any{
			"fields": map[string]any{
				"description": livekit.ADFDoc("ADF description edited by live issue test run " + s.RunID),
			},
		}))

		label := "jira-cli-livetest-" + strings.ToLower(strings.ReplaceAll(s.RunID, ":", "-"))
		s.Run(t, "issue", "edit", survivorKey, "--json-input", s.WriteJSON(t, "edit-fields-labels.json", map[string]any{
			"fields": map[string]any{
				"labels": []string{"jira-cli-livetest", label},
			},
		}))
		assert.Contains(t, livekit.IssueLabels(t, s.Run(t, "issue", "view", survivorKey)), label)

		s.Run(t, "issue", "edit", survivorKey, "--assignee", accountID)
		assertIssueAssignee(t, s.Run(t, "issue", "view", survivorKey), accountID)

		s.Run(t, "issue", "edit", survivorKey, "--assignee", "none")
		assertIssueUnassigned(t, s.Run(t, "issue", "view", survivorKey))

		s.Run(t, "issue", "edit", survivorKey, "--assignee", accountID)
	})

	t.Run("comments", func(t *testing.T) {
		legacy := s.Run(t, "issue", "comment", survivorKey, "--markdown", "Legacy comment alias for "+s.RunID)
		legacyComment := livekit.MapField(t, legacy.Data, "comment")
		legacyCommentID := livekit.StringField(t, legacyComment, "id")
		require.NotEmpty(t, legacyCommentID)
		s.Run(t, "issue", "comment", "delete", survivorKey, legacyCommentID, "--force")

		added := s.Run(t, "issue", "comment", "add", survivorKey, "--markdown", "Initial comment for "+s.RunID)
		comment := livekit.MapField(t, added.Data, "comment")
		commentID := livekit.StringField(t, comment, "id")
		require.NotEmpty(t, commentID)

		list := s.Run(t, "issue", "comment", "list", survivorKey, "--all")
		assertCommentPresent(t, list, commentID)

		edited := s.Run(t, "issue", "comment", "edit", survivorKey, commentID, "--markdown", "Edited comment for "+s.RunID)
		assert.Equal(t, commentID, livekit.StringField(t, livekit.MapField(t, edited.Data, "comment"), "id"))

		s.Run(t, "issue", "comment", "delete", survivorKey, commentID, "--force")
	})

	t.Run("worklogs", func(t *testing.T) {
		started := time.Now().UTC().Add(-30 * time.Minute).Format("2006-01-02T15:04:05.000-0700")
		added := s.Run(t, "worklog", "add", survivorKey, "--time-spent", "5m", "--started", started, "--markdown", "Live test worklog "+s.RunID)
		assert.False(t, livekit.BoolField(t, added.Data, "dry_run"))
		list := s.Run(t, "worklog", "list", survivorKey)
		require.NotEmpty(t, livekit.SliceField(t, list.Data, "worklogs"))
	})

	t.Run("attachments", func(t *testing.T) {
		attachmentPath := filepath.Join(t.TempDir(), "jira-live-attachment.txt")
		require.NoError(t, os.WriteFile(attachmentPath, []byte("jira-cli live attachment "+s.RunID+"\n"), 0o600))
		added := s.Run(t, "issue", "attachment", "add", survivorKey, "--file", attachmentPath)
		attachments := livekit.SliceField(t, added.Data, "attachments")
		require.NotEmpty(t, attachments)
		attachment, ok := attachments[0].(map[string]any)
		require.True(t, ok, "attachment row is not an object: %+v", attachments[0])
		attachmentID := livekit.StringField(t, attachment, "id")

		list := s.Run(t, "issue", "attachment", "list", survivorKey, "--all")
		assertAttachmentPresent(t, list, attachmentID)

		downloadPath := filepath.Join(t.TempDir(), "downloaded.txt")
		downloaded := s.Run(t, "issue", "attachment", "download", survivorKey, attachmentID, "--to", downloadPath)
		assert.Equal(t, downloadPath, livekit.StringField(t, downloaded.Data, "written_to"))
		assert.FileExists(t, downloadPath)

		s.Run(t, "issue", "attachment", "delete", survivorKey, attachmentID, "--force")
	})

	t.Run("links", func(t *testing.T) {
		linkType := pickLinkType(t, s)
		s.Run(t, "issue", "link", survivorKey, "--to", linkPeerKey, "--type", linkType)
		list := s.Run(t, "issue", "link", "list", survivorKey)
		linkID := findLinkID(t, list, linkPeerKey)
		require.NotEmpty(t, linkID)
		s.Run(t, "issue", "link", "delete", survivorKey, linkID, "--force")
		after := s.Run(t, "issue", "link", "list", survivorKey)
		assert.Empty(t, findLinkIDOrEmpty(after, linkPeerKey))
	})

	t.Run("watchers", func(t *testing.T) {
		s.Run(t, "issue", "watch", survivorKey)
		assert.True(t, livekit.BoolField(t, s.Run(t, "issue", "watchers", "list", survivorKey).Data, "is_watching"))

		s.Run(t, "issue", "unwatch", survivorKey)
		s.Run(t, "issue", "watchers", "add", survivorKey, "--user", "accountId:"+accountID)
		assertWatcherPresent(t, s.Run(t, "issue", "watchers", "list", survivorKey), accountID)
		s.Run(t, "issue", "watchers", "remove", survivorKey, "--user", "accountId:"+accountID)
	})

	t.Run("weblink", func(t *testing.T) {
		s.Run(t, "issue", "weblink", survivorKey, "--url", "https://example.com/jira-cli-live-"+s.RunID, "--title", "jira-cli live "+s.RunID)
	})

	t.Run("list and mine", func(t *testing.T) {
		list := s.Run(t, "issue", "list", "--jql", fmt.Sprintf("project = %s AND key = %s", s.Project, survivorKey), "--detail")
		assertIssueListed(t, list, survivorKey)

		asJQL := s.Run(t, "issue", "list", "--project", s.Project, "--as-jql")
		assert.Contains(t, livekit.StringField(t, asJQL.Data, "jql"), s.Project)

		mine := s.Run(t, "issue", "mine", "--project", s.Project)
		require.Contains(t, mine.Meta.Command, "issue.list")
	})

	t.Run("clone and move", func(t *testing.T) {
		cloneSummary := s.Marker + " cloned issue"
		clone := s.Run(t, "issue", "clone", survivorKey, "--force", "--json-input", s.WriteJSON(t, "clone.json", map[string]any{
			"fields": map[string]any{
				"summary": cloneSummary,
			},
		}))
		result := livekit.MapField(t, clone.Data, "result")
		cloneKey := livekit.StringField(t, result, "key")
		require.NotEmpty(t, cloneKey)
		s.TrackCleanup(cloneKey)
		assert.Equal(t, cloneSummary, livekit.IssueSummary(s.Run(t, "issue", "view", cloneKey)))

		movedSummary := livekit.SurvivorSummary(s.RunID) + " - moved via issue move"
		s.Run(t, "issue", "move", survivorKey, "--force", "--json-input", s.WriteJSON(t, "move.json", map[string]any{
			"fields": map[string]any{
				"summary": movedSummary,
			},
		}))
		assert.Equal(t, movedSummary, livekit.IssueSummary(s.Run(t, "issue", "view", survivorKey)))
	})

	t.Run("transition to closed state", func(t *testing.T) {
		transitionToClosedState(t, s, survivorKey)
	})

	t.Run("issue delete", func(t *testing.T) {
		require.NoError(t, s.SafeDeleteIssue(t, minimalKey))
		s.UntrackCleanup(minimalKey)
	})
}

// TestLiveIssuesFailureModes proves the CLI surfaces real Jira failures
// as structured JSON error envelopes — a non-zero exit and an ok=false
// body with a stable error code — instead of crashing or emitting
// unstructured output. Every case feeds a deliberately invalid input.
func TestLiveIssuesFailureModes(t *testing.T) {
	s := livekit.NewSuite(t)
	// A well-formed key in the real project whose issue number cannot
	// exist — Jira answers a clean 404 rather than a malformed-key 400.
	missingKey := s.Project + "-999999999"

	t.Run("view a missing issue reports jira_not_found", func(t *testing.T) {
		env := s.RunExpectError(t, "issue", "view", missingKey)
		requireJiraCode(t, env, "jira_not_found", 404)
	})

	t.Run("edit a missing issue reports jira_not_found", func(t *testing.T) {
		env := s.RunExpectError(t, "issue", "edit", missingKey, "--summary", "must not apply")
		requireJiraCode(t, env, "jira_not_found", 404)
	})

	t.Run("comment on a missing issue reports jira_not_found", func(t *testing.T) {
		env := s.RunExpectError(t, "issue", "comment", "add", missingKey, "--markdown", "must not post")
		requireJiraCode(t, env, "jira_not_found", 404)
	})

	t.Run("delete a missing issue reports jira_not_found", func(t *testing.T) {
		env := s.RunExpectError(t, "issue", "delete", missingKey, "--force")
		requireJiraCode(t, env, "jira_not_found", 404)
	})

	t.Run("an invalid JQL query reports jira_bad_request", func(t *testing.T) {
		env := s.RunExpectError(t, "issue", "list", "--jql", "this is not )( valid jql")
		requireJiraCode(t, env, "jira_bad_request", 400)
	})

	t.Run("create in a missing project is a structured failure", func(t *testing.T) {
		// Code is left unpinned: an unknown project can surface either
		// as the create call's 400 or as a 404 from project resolution
		// upstream of it. RunExpectError already pins the structured
		// contract — a non-zero exit, ok=false, a stable code, a message.
		s.RunExpectError(t, "issue", "create", "--json-input", s.WriteJSON(t, "missing-project.json", map[string]any{
			"project_key": "ZZNOSUCHPROJECT",
			"issue_type":  s.IssueType,
			"summary":     s.Marker + " must never be created",
		}))
	})

	t.Run("an invalid transition id is a structured failure", func(t *testing.T) {
		// Code is left unpinned for the same reason — the rejection may
		// come from the CLI's transition lookup or from Jira's 400.
		key := s.CreateIssue(t, s.Marker+" failure-mode transition target", nil)
		s.TrackCleanup(key)
		s.RunExpectError(t, "issue", "transition", key, "--transition", "99999999")
	})
}

// requireJiraCode asserts a failure envelope's first error carries the
// expected stable code and HTTP status, and is correctly marked
// non-retryable — Jira client errors (4xx) are never retryable.
func requireJiraCode(t *testing.T, env livekit.Envelope, wantCode string, wantHTTP int) {
	t.Helper()
	got := env.Errors[0]
	assert.Equal(t, wantCode, got.Code, "error code (full error: %+v)", got)
	assert.Equal(t, wantHTTP, got.HTTPStatus, "http_status (full error: %+v)", got)
	assert.False(t, got.Retryable, "a %d client error must not be retryable (full error: %+v)", wantHTTP, got)
	assert.NotEmpty(t, got.Hint, "a %s failure must carry a remediation hint (full error: %+v)", wantCode, got)
}

func assertNoIssueWithSummary(t *testing.T, s *livekit.Suite, summary, searchToken string) {
	t.Helper()
	env := s.Run(t, "issue", "list", "--jql", fmt.Sprintf("project = %s AND summary ~ %q", s.Project, searchToken))
	for _, raw := range livekit.SliceField(t, env.Data, "issues") {
		issue, ok := raw.(map[string]any)
		require.True(t, ok)
		assert.NotEqual(t, summary, livekit.StringField(t, issue, "summary"))
	}
}

func assertIssueAssignee(t *testing.T, env livekit.Envelope, accountID string) {
	t.Helper()
	fields := livekit.MapField(t, livekit.MapField(t, env.Data, "issue"), "fields")
	assignee := livekit.MapField(t, fields, "assignee")
	assert.Equal(t, accountID, livekit.StringField(t, assignee, "accountId"))
}

func assertIssueUnassigned(t *testing.T, env livekit.Envelope) {
	t.Helper()
	fields := livekit.MapField(t, livekit.MapField(t, env.Data, "issue"), "fields")
	assert.Nil(t, fields["assignee"])
}

func assertCommentPresent(t *testing.T, env livekit.Envelope, commentID string) {
	t.Helper()
	for _, raw := range livekit.SliceField(t, env.Data, "comments") {
		comment, ok := raw.(map[string]any)
		require.True(t, ok)
		if comment["id"] == commentID {
			return
		}
	}
	t.Fatalf("comment %s not found in %+v", commentID, env.Data["comments"])
}

func assertAttachmentPresent(t *testing.T, env livekit.Envelope, attachmentID string) {
	t.Helper()
	for _, raw := range livekit.SliceField(t, env.Data, "attachments") {
		attachment, ok := raw.(map[string]any)
		require.True(t, ok)
		if attachment["id"] == attachmentID {
			return
		}
	}
	t.Fatalf("attachment %s not found in %+v", attachmentID, env.Data["attachments"])
}

func pickLinkType(t *testing.T, s *livekit.Suite) string {
	t.Helper()
	env := s.Run(t, "issue", "link", "types", "--refresh")
	types := livekit.SliceField(t, env.Data, "link_types")
	require.NotEmpty(t, types, "Jira site returned no issue link types")
	fallback := ""
	for _, raw := range types {
		row, ok := raw.(map[string]any)
		require.True(t, ok)
		name := livekit.StringField(t, row, "name")
		if fallback == "" {
			fallback = name
		}
		if strings.EqualFold(name, "relates") {
			return name
		}
	}
	return fallback
}

func findLinkID(t *testing.T, env livekit.Envelope, otherKey string) string {
	t.Helper()
	id := findLinkIDOrEmpty(env, otherKey)
	require.NotEmpty(t, id, "link to %s not found in %+v", otherKey, env.Data["links"])
	return id
}

func findLinkIDOrEmpty(env livekit.Envelope, otherKey string) string {
	links, _ := env.Data["links"].([]any)
	for _, raw := range links {
		link, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		other, ok := link["other_issue"].(map[string]any)
		if !ok {
			continue
		}
		if other["key"] == otherKey {
			id, _ := link["id"].(string)
			return id
		}
	}
	return ""
}

func assertWatcherPresent(t *testing.T, env livekit.Envelope, accountID string) {
	t.Helper()
	for _, raw := range livekit.SliceField(t, env.Data, "watchers") {
		watcher, ok := raw.(map[string]any)
		require.True(t, ok)
		if watcher["account_id"] == accountID {
			return
		}
	}
	t.Fatalf("watcher account %s not found in %+v", accountID, env.Data["watchers"])
}

func assertIssueListed(t *testing.T, env livekit.Envelope, key string) {
	t.Helper()
	for _, raw := range livekit.SliceField(t, env.Data, "issues") {
		issue, ok := raw.(map[string]any)
		require.True(t, ok)
		if issue["key"] == key {
			return
		}
	}
	t.Fatalf("issue %s not found in %+v", key, env.Data["issues"])
}

func transitionToClosedState(t *testing.T, s *livekit.Suite, key string) {
	t.Helper()
	initialStatus := livekit.IssueStatus(s.Run(t, "issue", "view", key))
	for step := 0; step < 6; step++ {
		available := s.Run(t, "issue", "transition", key)
		transitions := livekit.SliceField(t, available.Data, "transitions")
		if len(transitions) == 0 {
			t.Logf("issue %s has no further transitions after status %q", key, livekit.IssueStatus(s.Run(t, "issue", "view", key)))
			return
		}
		transition := chooseClosingTransition(t, transitions)
		id := livekit.StringField(t, transition, "id")
		name := livekit.StringField(t, transition, "name")
		t.Logf("transitioning %s using discovered transition id=%s name=%q", key, id, name)
		s.Run(t, "issue", "transition", key, "--transition", id)
		status := livekit.IssueStatus(s.Run(t, "issue", "view", key))
		if looksClosedStatus(status) {
			t.Logf("issue %s reached closed-looking status %q from initial status %q", key, status, initialStatus)
			return
		}
	}
	t.Fatalf("issue %s did not reach a closed-looking state after discovered transitions; initial status %q, current status %q", key, initialStatus, livekit.IssueStatus(s.Run(t, "issue", "view", key)))
}

func chooseClosingTransition(t *testing.T, transitions []any) map[string]any {
	t.Helper()
	require.NotEmpty(t, transitions)
	var fallback map[string]any
	for _, raw := range transitions {
		row, ok := raw.(map[string]any)
		require.True(t, ok)
		if fallback == nil {
			fallback = row
		}
		name := strings.ToLower(livekit.StringField(t, row, "name"))
		if strings.Contains(name, "close") ||
			strings.Contains(name, "resolve") ||
			strings.Contains(name, "complete") ||
			strings.Contains(name, "finish") ||
			strings.Contains(name, "done") {
			return row
		}
	}
	return fallback
}

func looksClosedStatus(status string) bool {
	v := strings.ToLower(strings.TrimSpace(status))
	return strings.Contains(v, "closed") ||
		strings.Contains(v, "resolved") ||
		strings.Contains(v, "complete") ||
		strings.Contains(v, "done")
}
