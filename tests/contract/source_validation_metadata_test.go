package contract

import (
	"strings"
	"testing"
)

// Mutually exclusive input sources must be rejected by Cobra flag
// metadata, before the handler runs. The proof: the conflict is reported
// even when --json-input points at a path that does not exist — if the
// handler were doing the checking it would first try (and fail) to open
// the file, producing a file error instead of a flag-conflict error.
func TestCommentBodySourcesRejectedByMetadata(t *testing.T) {
	bin := buildJiraBinary(t)
	_ = bin

	missing := t.TempDir() + "/does-not-exist.json"

	cases := []struct {
		name string
		args []string
	}{
		{
			"comment add",
			[]string{"issue", "comment", "add", "PROJ-1", "--body-markdown", "hi", "--json-input", missing, "--output=json"},
		},
		{
			"comment edit",
			[]string{"issue", "comment", "edit", "PROJ-1", "55", "--body-markdown", "hi", "--json-input", missing, "--output=json"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runJira(t, tc.args...)
			if code != 3 {
				t.Fatalf("exit = %d; want 3 (validation)\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			body := strings.ToLower(string(stderr) + string(stdout))
			// Cobra's MarkFlagsMutuallyExclusive emits a distinctive
			// message naming the flag group; a handler-side string check
			// would not. Requiring the Cobra phrasing proves the rule
			// lives in declarative flag metadata, not buried in RunE.
			if !strings.Contains(body, "none of the others can be") {
				t.Fatalf("conflict not reported by Cobra flag-group metadata:\n%s", body)
			}
			if strings.Contains(body, "does-not-exist") {
				t.Fatalf("handler opened --json-input before flag validation fired:\n%s", body)
			}
		})
	}
}

// `issue link` needs both --to and --type. Passing one without the other
// must be rejected by Cobra's MarkFlagsRequiredTogether metadata before
// RunE constructs a Jira client.
func TestIssueLinkEndpointsRequiredTogetherByMetadata(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"only --to", []string{"issue", "link", "PROJ-1", "--to", "PROJ-2", "--output=json"}},
		{"only --type", []string{"issue", "link", "PROJ-1", "--type", "Blocks", "--output=json"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runJira(t, tc.args...)
			if code != 3 {
				t.Fatalf("exit = %d; want 3 (validation)\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			body := strings.ToLower(string(stderr) + string(stdout))
			if !strings.Contains(body, "they must all be set") {
				t.Fatalf("half-specified link not reported by Cobra required-together metadata:\n%s", body)
			}
		})
	}
}
