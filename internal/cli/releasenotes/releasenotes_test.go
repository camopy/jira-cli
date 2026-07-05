package releasenotes

import (
	"strings"
	"testing"

	changelog "github.com/matcra587/jira-cli"
)

func TestBuildResultFull(t *testing.T) {
	res, err := buildResult("", false)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !strings.Contains(res.Markdown, changelog.Header()) {
		t.Fatal("full markdown should include the header")
	}
	all := changelog.Releases()
	if len(res.Releases) != len(all) {
		t.Fatalf("full should list every release: want %d got %d", len(all), len(res.Releases))
	}
	if res.Releases[0].Version != all[0].Version {
		t.Fatalf("full releases not newest-first: %q", res.Releases[0].Version)
	}
}

func TestBuildResultLatest(t *testing.T) {
	res, err := buildResult("", true)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	newest := changelog.Releases()[0]
	if len(res.Releases) != 1 {
		t.Fatalf("latest should list one release, got %d", len(res.Releases))
	}
	if res.Releases[0].Version != newest.Version {
		t.Fatalf("latest: want %q got %q", newest.Version, res.Releases[0].Version)
	}
	if res.Markdown != newest.Markdown {
		t.Fatal("latest markdown should be the newest release's notes")
	}
}

func TestBuildResultSingleVersion(t *testing.T) {
	res, err := buildResult("0.3.3", false)
	if err != nil {
		t.Fatalf("single: %v", err)
	}
	if len(res.Releases) != 1 || res.Releases[0].Version != "0.3.3" {
		t.Fatalf("single should list only 0.3.3: %+v", res.Releases)
	}
	if len(res.Releases[0].Sections) == 0 {
		t.Fatal("single release should carry parsed sections")
	}
	if !strings.Contains(res.Markdown, "0.3.3") {
		t.Fatalf("markdown should mention the release:\n%s", res.Markdown)
	}
}

func TestBuildResultVPrefixTolerated(t *testing.T) {
	res, err := buildResult("v0.3.3", false)
	if err != nil {
		t.Fatalf("v-prefixed: %v", err)
	}
	if len(res.Releases) != 1 || res.Releases[0].Version != "0.3.3" {
		t.Fatalf("want 0.3.3, got %+v", res.Releases)
	}
}

func TestBuildResultUnknownVersion(t *testing.T) {
	_, err := buildResult("9.9.9", false)
	if err == nil {
		t.Fatal("unknown version should be an error")
	}
	if !strings.Contains(err.Error(), "0.3.3") {
		t.Fatalf("error should list available versions: %v", err)
	}
}
