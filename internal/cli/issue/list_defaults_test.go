package issue

import (
	"reflect"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jql"
)

// issueListBuilderWithProfileDefaults scopes a list/mine query to the profile's
// default_project, but only when the caller supplied no --project of its own.
func TestIssueListBuilderWithProfileDefaults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		builder  jql.BuildOptions
		profile  config.Profile
		wantProj []string
	}{
		{
			name:     "default applied when no project given",
			profile:  config.Profile{DefaultProject: "ACME"},
			wantProj: []string{"ACME"},
		},
		{
			name:     "explicit project overrides default",
			builder:  jql.BuildOptions{Projects: []string{"OTHER"}},
			profile:  config.Profile{DefaultProject: "ACME"},
			wantProj: []string{"OTHER"},
		},
		{
			name:     "no default leaves projects empty",
			wantProj: nil,
		},
		{
			name:     "blank-only project falls back to default",
			builder:  jql.BuildOptions{Projects: []string{"  "}},
			profile:  config.Profile{DefaultProject: "ACME"},
			wantProj: []string{"ACME"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := issueListBuilderWithProfileDefaults(tc.builder, tc.profile)
			if !reflect.DeepEqual(got.Projects, tc.wantProj) {
				t.Fatalf("Projects = %#v, want %#v", got.Projects, tc.wantProj)
			}
		})
	}
}
