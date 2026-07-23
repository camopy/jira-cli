package cmdutil

import (
	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// RecordIssueKeys adds keys the command just touched to the active
// profile's recently-used cache, the source shell completion's issuekey
// predictor reads. Recording is strictly best-effort: it resolves the
// profile from config directly (never builds a client, so dry-run paths
// may call it too) and drops every failure — a broken cache must never
// fail the command that tripped it.
func RecordIssueKeys(cmd *cobra.Command, keys ...string) {
	if len(keys) == 0 {
		return
	}
	cfg, err := config.Load(config.WithPath(ConfigPath(cmd)))
	if err != nil {
		return
	}
	profile := ActiveProfile(cmd, cfg)
	if name := RequestedProfile(cmd); name != "" {
		if p, err := cfg.ResolveProfile(name); err == nil {
			profile = p
		}
	}
	_ = cache.RecordIssueKeys(CacheKeyForProfile(cmd, profile), keys) //nolint:errcheck // recency completion cache is best-effort
}

// RecordIssuesSeen records the keys of issues a list or search returned.
// Returned keys are weaker recency signals than typed ones, but they are
// exactly what the user is about to act on, so they feed the same cache.
func RecordIssuesSeen(cmd *cobra.Command, issues []*jira.Issue) {
	keys := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue != nil && issue.Key != nil {
			keys = append(keys, *issue.Key)
		}
	}
	RecordIssueKeys(cmd, keys...)
}
