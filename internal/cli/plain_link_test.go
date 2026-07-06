package cli

import (
	"bytes"
	"strings"
	"testing"

	termansi "github.com/gechr/x/ansi"
	"github.com/matcra587/jira-cli/internal/jira"
)

func TestPlainPadRightUsesDisplayWidth(t *testing.T) {
	got := padRight("界", 4)
	if width := termansi.StringWidth(got); width != 4 {
		t.Fatalf("display width = %d, want 4 for %q", width, got)
	}
	if spaces := strings.Count(got, " "); spaces != 2 {
		t.Fatalf("spaces = %d, want 2 for %q", spaces, got)
	}
}

func TestLinkListPlainRendersTypedLinks(t *testing.T) {
	var buf bytes.Buffer
	err := WriteLinkListPlain(&buf, "issue.link.list", map[string]any{
		"key": "PROJ-1",
		"links": []jira.IssueLinkView{
			{
				ID:        "9001",
				Direction: "outward",
				Type:      jira.IssueLinkType{Name: "Blocks", Outward: "blocks", Inward: "is blocked by"},
				OtherIssue: jira.IssueRef{
					Key:     "PROJ-2",
					Summary: "dependent work",
					Status:  "In Progress",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteLinkListPlain() error = %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "(no links)") {
		t.Fatalf("typed link data rendered as empty:\n%s", got)
	}
	for _, want := range []string{"Links on PROJ-1", "blocks", "PROJ-2", "dependent work"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain link output missing %q:\n%s", want, got)
		}
	}
}

func TestLinkTypesPlainRendersTypedLinkTypes(t *testing.T) {
	var buf bytes.Buffer
	err := WriteLinkTypesPlain(&buf, "issue.link.types", map[string]any{
		"count":      1,
		"from_cache": true,
		"link_types": []jira.IssueLinkType{
			{ID: "10000", Name: "Blocks", Outward: "blocks", Inward: "is blocked by"},
		},
	})
	if err != nil {
		t.Fatalf("WriteLinkTypesPlain() error = %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "(no link types configured)") {
		t.Fatalf("typed link type data rendered as empty:\n%s", got)
	}
	for _, want := range []string{"Link types", "Blocks", "blocks", "is blocked by"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain link-type output missing %q:\n%s", want, got)
		}
	}
}
