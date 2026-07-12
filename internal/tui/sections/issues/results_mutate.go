// Writes from the results list: comments, transitions,
// edits, assignments, and worklogs, plus the optimistic row updates and
// rollbacks that cover a write until the server confirms it.

package issues

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gechr/x/ptr"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/browser"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// canMutate reports that no single transition or bulk batch is mid-flight, so a
// new mutation won't race the optimistic rollback or a pending reconcile.
func (r *results) canMutate() bool { return r.rollback == nil && !r.bulkPending && !r.writing }

// openComment starts a comment: through the external editor when one is
// configured ($JIRA_EDITOR/$EDITOR — the TUI suspends while it runs),
// otherwise the in-modal textarea.
func (r *results) openComment() tea.Cmd {
	iss := r.selected()
	if iss == nil || !r.canMutate() {
		return nil
	}
	if input.EditorCommand() != "" {
		return input.Edit("comment:"+issueKey(iss), "")
	}
	r.ctrl.OpenText(action.ModeComment, issueKey(iss), "")
	return nil
}

// handleEditor resumes a flow that went through the external editor: an error
// or empty buffer surfaces and nothing is written; otherwise the text submits
// exactly like its in-modal equivalent.
func (r *results) handleEditor(msg input.EditorFinishedMsg) tea.Cmd {
	if msg.Err != nil {
		r.err = msg.Err
		return nil
	}
	kind, issKey, ok := strings.Cut(msg.ID, ":")
	if !ok || kind != "comment" {
		return nil
	}
	if xstrings.IsBlank(msg.Text) {
		return r.flashNotice("empty comment discarded", false)
	}
	body, _, err := adf.FromMarkdownLossy(msg.Text)
	if err != nil {
		r.err = err
		return nil
	}
	return r.mutate(func(svc core.Services, base context.Context) error {
		_, _, e := svc.Issues().AddComment(base, issKey, &jira.CommentAddRequest{Body: body})
		return e
	})
}

func (r *results) fetchTransitions(key string, bulk bool) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.transitionsScope(),
		Run: func() (any, error) {
			if svc == nil {
				return transitionsResult{issueKey: key, bulk: bulk}, nil
			}
			ts, _, err := svc.Issues().Transitions(base, key)
			if err != nil {
				return nil, err
			}
			return transitionsResult{issueKey: key, transitions: ts, bulk: bulk}, nil
		},
	})
}

// workdaySeconds is the assumed length of a working day for parsing relative
// worklog durations like "1d". Jira's default is 8 hours.
const workdaySeconds = 8 * 60 * 60

func (r *results) submitAction() tea.Cmd {
	req, ok := r.ctrl.Submit()
	if !ok {
		return nil
	}
	switch req.Mode {
	case action.ModeTransition:
		r.rollback = r.applyOptimisticTransition(req.IssueKey, req.TransitionName)
		base := r.ctx.Base
		svc := r.ctx.Services
		key, id := req.IssueKey, req.TransitionID
		return r.ctx.StartTask(core.TaskSpec{
			Scope: r.mutateScope(),
			Run: func() (any, error) {
				if svc == nil {
					return nil, nil
				}
				_, err := svc.Issues().Transition(base, key, &jira.TransitionRequest{ID: id})
				return nil, err
			},
		})
	case action.ModeBulkTransition, action.ModeBulkAssign, action.ModeBulkComment:
		// Bulk writes hit every marked issue at once, so they park behind a
		// y/N confirmation (default No) instead of running off the submit.
		keys := r.markedKeys()
		if len(keys) == 0 {
			return nil
		}
		text := req.Text
		switch req.Mode {
		case action.ModeBulkTransition:
			// The picker carries the chosen name verbatim — no trim, because
			// the apply side matches it exactly against each issue's
			// transition names, whitespace included; per-issue ids resolve at
			// apply time because each issue exposes its own transitions.
			text = req.TransitionName
		case action.ModeBulkAssign:
			// Assignee queries are lookup keys; comments post verbatim
			// (leading whitespace is markdown).
			text = strings.TrimSpace(text)
		}
		r.confirm = &bulkConfirm{mode: req.Mode, text: text, keys: keys}
		return nil
	case action.ModeComment:
		body, _, err := adf.FromMarkdownLossy(req.Text)
		if err != nil {
			r.err = err
			return nil
		}
		issKey := req.IssueKey
		return r.mutate(func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().AddComment(base, issKey, &jira.CommentAddRequest{Body: body})
			return e
		})
	case action.ModeEdit:
		key, summary := req.IssueKey, req.Text
		r.rollback = r.applyOptimisticSummary(key, summary)
		return r.mutate(func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().Update(base, key, &jira.IssueUpdateRequest{
				Fields: map[string]any{"summary": summary},
			})
			return e
		})
	case action.ModeAssign:
		// One normalised value drives both the optimistic row and the write:
		// "" means unassign for each.
		display := strings.TrimSpace(req.Text)
		if clearsAssignee(display) {
			display = ""
		}
		r.rollback = r.applyOptimisticAssignee(req.IssueKey, display)
		return r.assignTo(req.IssueKey, display)
	case action.ModeWorklog:
		wd := r.ctx.WorkdaySeconds
		if wd <= 0 {
			wd = workdaySeconds
		}
		secs, err := jira.ParseDuration(req.Text, wd)
		if err != nil {
			r.err = err
			return nil
		}
		key := req.IssueKey
		return r.mutate(func(svc core.Services, base context.Context) error {
			_, _, e := svc.Worklogs().Add(base, key, &jira.WorklogAddRequest{TimeSpentSeconds: secs})
			return e
		})
	}
	return nil
}

// mutate runs a single-issue write on the mutate scope. On success handleTask
// reconciles by refetching; on error it rolls back any optimistic change the
// caller registered (r.rollback) and shows the failure toast. Verbs whose
// field shows in the row (edit, assign) apply optimistically before calling
// this; comment and worklog have nothing row-visible, so they just reconcile.
func (r *results) mutate(run func(svc core.Services, base context.Context) error) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	r.writing = true // block overlapping writes until this one reconciles
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.mutateScope(),
		Run: func() (any, error) {
			if svc == nil {
				return nil, nil
			}
			return nil, run(svc, base)
		},
	})
}

// assignMe resolves the current user's account id and assigns the issue to them.
func (r *results) assignMe(key string) tea.Cmd {
	return r.mutate(func(svc core.Services, base context.Context) error {
		me, _, err := svc.Users().Myself(base)
		if err != nil {
			return err
		}
		if me.AccountID == "" {
			return fmt.Errorf("could not resolve your account id for self-assign")
		}
		return setAssignee(svc, base, key, me.AccountID)
	})
}

// assignTo resolves a user query (name/email) to an account id and assigns it.
// The keywords "none"/"unassigned" (or an empty query) clear the assignee
// instead of being treated as a user search.
func (r *results) assignTo(key, query string) tea.Cmd {
	q := strings.TrimSpace(query)
	return r.mutate(func(svc core.Services, base context.Context) error {
		if q == "" || strings.EqualFold(q, "none") || strings.EqualFold(q, "unassigned") {
			return setAssignee(svc, base, key, "")
		}
		id, err := svc.Users().ResolveUser(base, q)
		if err != nil {
			return err
		}
		return setAssignee(svc, base, key, id)
	})
}

// setAssignee writes the assignee field; an empty id clears it (unassigned).
func setAssignee(svc core.Services, base context.Context, key, accountID string) error {
	var assignee any
	if accountID != "" {
		assignee = map[string]any{"accountId": accountID}
	}
	_, _, err := svc.Issues().Update(base, key, &jira.IssueUpdateRequest{
		Fields: map[string]any{"assignee": assignee},
	})
	return err
}

// openInBrowser opens the issue's web page. The open is fire-and-forget: a
// failure to launch a browser shouldn't disrupt the dashboard.
func (r *results) openInBrowser(key string) tea.Cmd {
	url := r.issueURL(key)
	if url == "" {
		return nil
	}
	base := r.ctx.Base
	return func() tea.Msg {
		_ = browser.Open(base, url)
		return nil
	}
}

// issueURL builds the browse URL for an issue via the browser helper (which
// trims the base and path-escapes the key), or "" if no base URL is known.
func (r *results) issueURL(key string) string {
	if xstrings.AnyEmpty(r.ctx.BaseURL, key) {
		return ""
	}
	return browser.IssueURL(r.ctx.BaseURL, key)
}

// transitionByName resolves the transition whose name matches status for one
// issue and applies it. Match is by transition name (jira.Transition models only
// id+name, not the destination status), which for default Jira workflows equals
// the target status. The available transitions are issue-specific, so a name
// that isn't reachable for this issue surfaces as a per-issue error.
func transitionByName(ctx context.Context, svc core.Services, key, status string) error {
	ts, _, err := svc.Issues().Transitions(ctx, key)
	if err != nil {
		return fmt.Errorf("list transitions: %w", err)
	}
	id := findTransitionID(ts, status)
	if id == "" {
		return fmt.Errorf("no transition to %q", status)
	}
	if _, err := svc.Issues().Transition(ctx, key, &jira.TransitionRequest{ID: id}); err != nil {
		return fmt.Errorf("apply transition: %w", err)
	}
	return nil
}

// findTransitionID returns the id of the transition named status (case-folded),
// or "" when none matches.
func findTransitionID(ts []*jira.Transition, status string) string {
	for _, t := range ts {
		if strings.EqualFold(ptr.Deref(t.Name), status) {
			return ptr.Deref(t.ID)
		}
	}
	return ""
}

// optimistic applies a local field change to the issue immediately (so the row
// reflects the write before the server confirms) and returns the rollback that
// restores it. change mutates the fields and returns its own undo; optimistic
// wraps both sides with the row rebuild.
func (r *results) optimistic(key string, change func(f *jira.IssueFields) func()) func() {
	iss := r.find(key)
	if iss == nil || iss.Fields == nil {
		return func() {}
	}
	undo := change(iss.Fields)
	r.applyFilter()
	return func() {
		undo()
		r.applyFilter()
	}
}

func (r *results) applyOptimisticTransition(key, newStatus string) func() {
	return r.optimistic(key, func(f *jira.IssueFields) func() {
		if f.Status == nil {
			f.Status = &jira.Status{}
		}
		prev := f.Status.Name
		ns := newStatus
		f.Status.Name = &ns
		return func() { f.Status.Name = prev }
	})
}

// applyOptimisticSummary swaps the row summary for an edit in flight.
func (r *results) applyOptimisticSummary(key, summary string) func() {
	return r.optimistic(key, func(f *jira.IssueFields) func() {
		prev := f.Summary
		s := summary
		f.Summary = &s
		return func() { f.Summary = prev }
	})
}

// applyOptimisticAssignee shows the assignee column as the typed query (or
// clears it for an unassign) while the write runs. The server's canonical
// display name replaces it when the post-write reconcile lands.
func (r *results) applyOptimisticAssignee(key, display string) func() {
	return r.optimistic(key, func(f *jira.IssueFields) func() {
		prev := f.Assignee
		if display == "" {
			f.Assignee = nil
		} else {
			d := display
			f.Assignee = &jira.User{DisplayName: &d}
		}
		return func() { f.Assignee = prev }
	})
}
