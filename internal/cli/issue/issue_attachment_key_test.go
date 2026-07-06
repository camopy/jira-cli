package issue

import "testing"

// The single-issue attachment verbs (download, delete) must run their key
// argument through the shared issue-key parser: a traversal path or
// hallucinated key fails fast, and a list/range expansion is rejected
// because the verbs address exactly one issue.
func TestAttachmentIssueKeyAcceptsOneCanonicalKey(t *testing.T) {
	got, err := attachmentIssueKey("PROJ-123")
	if err != nil {
		t.Fatalf("attachmentIssueKey(PROJ-123) error = %v", err)
	}
	if got != "PROJ-123" {
		t.Fatalf("attachmentIssueKey(PROJ-123) = %q, want PROJ-123", got)
	}
}

func TestAttachmentIssueKeyRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{"path traversal", "../PROJ-1"},
		{"lowercase", "proj-1"},
		{"free text", "the checkout bug"},
		{"empty", ""},
		{"range expansion", "PROJ-1:3"},
		{"comma list", "PROJ-1,PROJ-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := attachmentIssueKey(tc.arg); err == nil {
				t.Fatalf("attachmentIssueKey(%q) = %q, want error", tc.arg, got)
			}
		})
	}
}
