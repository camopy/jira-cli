package changelog

import (
	"strings"
	"testing"

	"golang.org/x/mod/semver"
)

func TestHeader(t *testing.T) {
	if !strings.Contains(Header(), "Release Notes") {
		t.Fatalf("header missing title: %q", Header())
	}
	if strings.TrimSpace(Header()) != Header() {
		t.Fatal("header should be trimmed")
	}
}

func TestReleasesSortedDescending(t *testing.T) {
	releases := Releases()
	if len(releases) < 2 {
		t.Fatalf("expected several embedded releases, got %d", len(releases))
	}
	for i := 1; i < len(releases); i++ {
		if semver.Compare(releases[i-1].Tag, releases[i].Tag) < 0 {
			t.Fatalf("releases not newest-first: %s before %s", releases[i-1].Tag, releases[i].Tag)
		}
	}
	for _, r := range releases {
		if !semver.IsValid(r.Tag) {
			t.Fatalf("invalid tag: %q", r.Tag)
		}
		if r.Version != strings.TrimPrefix(r.Tag, "v") {
			t.Fatalf("version %q and tag %q disagree", r.Version, r.Tag)
		}
		if strings.TrimSpace(r.Markdown) == "" {
			t.Fatalf("empty notes for %s", r.Tag)
		}
	}
}

func TestFind(t *testing.T) {
	got, ok := Find("0.3.3")
	if !ok || got.Tag != "v0.3.3" {
		t.Fatalf("Find(0.3.3): ok=%v tag=%q", ok, got.Tag)
	}
	if withV, ok := Find("v0.3.3"); !ok || withV.Tag != got.Tag {
		t.Fatalf("Find(v0.3.3) should match Find(0.3.3)")
	}
	if _, ok := Find("9.9.9"); ok {
		t.Fatal("Find(9.9.9) should not match")
	}
}

func TestParsedSections(t *testing.T) {
	r, ok := Find("0.7.7")
	if !ok {
		t.Fatal("expected 0.7.7 to be embedded")
	}
	if !strings.Contains(r.URL, "tag/v0.7.7") {
		t.Fatalf("URL not parsed: %q", r.URL)
	}
	if r.Date != "2026-07-05" {
		t.Fatalf("date not parsed: %q", r.Date)
	}

	kinds := map[string][]string{}
	for _, s := range r.Sections {
		kinds[s.Kind] = s.Changes
	}
	for _, want := range []string{"Changed", "Fixed", "Dependencies"} {
		if _, ok := kinds[want]; !ok {
			t.Fatalf("missing section %q in %+v", want, r.Sections)
		}
	}
	// The Dependencies section lists each bump as its own change.
	if deps := kinds["Dependencies"]; len(deps) != 4 {
		t.Fatalf("Dependencies should have 4 changes, got %d: %v", len(deps), deps)
	}
	if fixed := kinds["Fixed"]; len(fixed) != 1 || !strings.Contains(fixed[0], "inline-code span") {
		t.Fatalf("Fixed change not parsed cleanly: %v", fixed)
	}
	// Bullet text is captured without the leading marker.
	for _, s := range r.Sections {
		for _, c := range s.Changes {
			if strings.HasPrefix(c, "- ") {
				t.Fatalf("change kept its bullet marker: %q", c)
			}
		}
	}
}

func TestFull(t *testing.T) {
	full := Full()
	if !strings.Contains(full, Header()) {
		t.Fatal("full changelog should start with the header")
	}
	for _, want := range []string{"## [0.1.0]", "## [0.3.3]"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full changelog missing %q", want)
		}
	}
	newest := Releases()[0]
	if strings.Index(full, Header()) > strings.Index(full, newest.Markdown) {
		t.Fatal("header should precede the newest release")
	}
}
