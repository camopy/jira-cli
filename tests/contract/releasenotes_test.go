// Contract tests for the `jira release-notes` command, which prints jira-cli's
// own embedded changelog. It reads nothing over the network, so these tests do
// not stub Jira; they assert against the notes compiled into the binary.
//
//   - JSON: the full changelog exposes a markdown document and a versions list.
//   - Single version / --latest: narrow the notes to one release.
//   - Human: raw Markdown when piped, ready to paste into a release body.
//   - Validation: an unknown version, and version-plus-latest, are errors.
package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A config is passed so the root command's config load succeeds; the loopback
// URL is never contacted because release-notes makes no Jira call.
func releaseNotesConfig(t *testing.T) string {
	t.Helper()
	return writeCacheTestConfig(t, "http://127.0.0.1:1")
}

func TestReleaseNotesFullJSON(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := releaseNotesConfig(t)

	out, err := runWithEnv(bin, os.Environ(), "--config", cfg, "release-notes", "--output=json")
	if err != nil {
		t.Fatalf("release-notes: %v\n%s", err, out)
	}
	var envel map[string]any
	if err := json.Unmarshal(out, &envel); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envel["data"].(map[string]any)

	if v, ok := data["version"].(string); ok && v != "" {
		t.Fatalf("full changelog should carry no single version, got %q", v)
	}
	// JSON is structured, not a Markdown blob.
	if _, ok := data["markdown"]; ok {
		t.Fatal("JSON output should not carry a raw markdown blob")
	}
	releases, _ := data["releases"].([]any)
	if len(releases) == 0 {
		t.Fatal("expected at least one release in the changelog")
	}

	// A known historical release is present and parsed into sections.
	var found map[string]any
	for _, entry := range releases {
		e, _ := entry.(map[string]any)
		if e["version"] == "0.3.3" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatalf("expected release 0.3.3 in releases list: %+v", releases)
	}
	sections, _ := found["sections"].([]any)
	if len(sections) == 0 {
		t.Fatalf("release 0.3.3 should have parsed sections: %+v", found)
	}
	first, _ := sections[0].(map[string]any)
	if _, ok := first["kind"].(string); !ok {
		t.Fatalf("section missing kind: %+v", first)
	}
	if changes, _ := first["changes"].([]any); len(changes) == 0 {
		t.Fatalf("section missing changes: %+v", first)
	}
}

func TestReleaseNotesLatestJSON(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := releaseNotesConfig(t)

	out, err := runWithEnv(bin, os.Environ(), "--config", cfg, "release-notes", "--latest", "--output=json")
	if err != nil {
		t.Fatalf("release-notes --latest: %v\n%s", err, out)
	}
	var envel map[string]any
	if err := json.Unmarshal(out, &envel); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envel["data"].(map[string]any)
	if v, _ := data["version"].(string); v == "" {
		t.Fatal("--latest should carry the newest version")
	}
	if releases, _ := data["releases"].([]any); len(releases) != 1 {
		t.Fatalf("--latest should return exactly one release, got %d", len(releases))
	}
}

func TestReleaseNotesSingleVersion(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := releaseNotesConfig(t)

	out, err := runWithEnv(bin, os.Environ(), "--config", cfg, "release-notes", "0.3.3", "--output=json")
	if err != nil {
		t.Fatalf("release-notes 0.3.3: %v\n%s", err, out)
	}
	var envel map[string]any
	if err := json.Unmarshal(out, &envel); err != nil {
		t.Fatalf("parse envelope: %v\n%s", err, out)
	}
	data, _ := envel["data"].(map[string]any)
	if data["version"] != "0.3.3" {
		t.Fatalf("data.version: want 0.3.3 got %v", data["version"])
	}
	releases, _ := data["releases"].([]any)
	if len(releases) != 1 {
		t.Fatalf("single version should return exactly one release, got %d", len(releases))
	}
	rel, _ := releases[0].(map[string]any)
	if rel["version"] != "0.3.3" {
		t.Fatalf("release.version: want 0.3.3 got %v", rel["version"])
	}
	if sections, _ := rel["sections"].([]any); len(sections) == 0 {
		t.Fatalf("release 0.3.3 should carry parsed sections: %+v", rel)
	}
}

func TestReleaseNotesHumanMarkdown(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := releaseNotesConfig(t)

	out, err := runWithEnv(bin, os.Environ(), "--config", cfg, "release-notes", "0.3.3", "--output=human")
	if err != nil {
		t.Fatalf("release-notes human: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"## [0.3.3]", "### Added", "### Security"} {
		if !strings.Contains(got, want) {
			t.Fatalf("human output missing %q:\n%s", want, got)
		}
	}
}

func TestReleaseNotesUnknownVersion(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := releaseNotesConfig(t)

	out, err := runWithEnv(bin, os.Environ(), "--config", cfg, "release-notes", "9.9.9", "--output=json")
	if err == nil {
		t.Fatalf("expected an error for an unknown version; got:\n%s", out)
	}
}

func TestReleaseNotesVersionAndLatestConflict(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := releaseNotesConfig(t)

	out, err := runWithEnv(bin, os.Environ(), "--config", cfg, "release-notes", "0.3.3", "--latest", "--output=json")
	if err == nil {
		t.Fatalf("expected an error when a version and --latest are combined; got:\n%s", out)
	}
}
