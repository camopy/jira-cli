package main

import (
	"context"
	"time"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui"
	"github.com/spf13/cobra"
)

func tuiRun(cmd *cobra.Command) (any, error) {
	return tui.RunWithOptions(cmd.Context(), tuiOptionsForCommand(cmd))
}

func tuiOptionsForCommand(cmd *cobra.Command) tui.Options {
	opts := tui.Options{}
	client, profile, ok, err := cmdutil.JiraClientForCommand(cmd)
	if err != nil {
		opts.InitialError = err.Error()
		return opts
	}
	if cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd))); err == nil {
		opts.ActiveProfile = cmdutil.ActiveProfile(cmd, cfg).Name
		for _, profile := range cfg.Profiles {
			opts.Profiles = append(opts.Profiles, profile.Name)
		}
	}
	if opts.ActiveProfile == "" {
		opts.ActiveProfile = profile.Name
	}
	opts.BaseURL = profile.BaseURL
	opts.Email = profile.Email
	opts.AccountID = profile.AccountID
	opts.TeamAccountIDs = append([]string(nil), profile.TeamAccountIDs...)
	if profile.RefreshInterval > 0 {
		opts.RefreshEvery = time.Duration(profile.RefreshInterval) * time.Second
	}
	if ok {
		// Auto-resolve account_id via /myself when the profile doesn't have
		// it. Jira Cloud requires an accountId for assign-by-user (email
		// is silently ignored) so without this the TUI's "A" key would
		// look like it succeeded but never set the assignee.
		if opts.AccountID == "" {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if user, _, err := jira.NewUserService(client).Myself(ctx); err == nil && user.AccountID != "" {
				opts.AccountID = user.AccountID
				if opts.Email == "" && user.EmailAddress != "" {
					opts.Email = user.EmailAddress
				}
			}
		}
		opts.IssueProvider = tui.IssueProviderFunc(func(ctx context.Context) ([]*jira.Issue, error) {
			issues, _, err := issueService(client).List(ctx, &jira.IssueListOptions{ListOptions: jira.ListOptions{MaxResults: 50}})
			return issues, err
		})
		opts.MutationService = tuiJiraMutations{issues: issueService(client), worklogs: worklogService(client)}
	}
	return opts
}

type tuiJiraMutations struct {
	issues   jira.IssueService
	worklogs jira.WorklogService
}

func (m tuiJiraMutations) UpdateIssue(ctx context.Context, key string, fields map[string]any) (*jira.Issue, error) {
	issue, _, err := m.issues.Update(ctx, key, &jira.IssueUpdateRequest{Fields: fields})
	return issue, err
}

func (m tuiJiraMutations) CreateIssue(ctx context.Context, req *jira.IssueCreateRequest) (*jira.Issue, error) {
	issue, _, err := m.issues.Create(ctx, req)
	return issue, err
}

func (m tuiJiraMutations) TransitionIssue(ctx context.Context, key string, req *jira.TransitionRequest) error {
	_, err := m.issues.Transition(ctx, key, req)
	return err
}

func (m tuiJiraMutations) AddComment(ctx context.Context, key string, req *jira.CommentAddRequest) (*jira.Comment, error) {
	comment, _, err := m.issues.AddComment(ctx, key, req)
	return comment, err
}

func (m tuiJiraMutations) AddWorklog(ctx context.Context, key string, req *jira.WorklogAddRequest) (*jira.Worklog, error) {
	worklog, _, err := m.worklogs.Add(ctx, key, req)
	return worklog, err
}

func (m tuiJiraMutations) CloneIssue(ctx context.Context, key string, req *jira.IssueCloneRequest) (*jira.Issue, error) {
	issue, _, err := m.issues.Clone(ctx, key, req)
	return issue, err
}

func (m tuiJiraMutations) MoveIssue(ctx context.Context, key string, req *jira.IssueMoveRequest) (*jira.Issue, error) {
	issue, _, err := m.issues.Move(ctx, key, req)
	return issue, err
}

func (m tuiJiraMutations) DeleteIssue(ctx context.Context, key string) error {
	_, err := m.issues.Delete(ctx, key, nil)
	return err
}
