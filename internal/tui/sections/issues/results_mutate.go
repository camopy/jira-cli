// Writes from the results list: creates, comments, transitions, edits,
// assignments, labels, and worklogs, plus the optimistic row updates and
// rollbacks that cover a write until the server confirms it.

package issues

import (
	"context"
	"errors"
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
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// canMutate reports that no single transition or bulk batch is mid-flight, so a
// new mutation won't race the optimistic rollback or a pending reconcile.
func (r *results) canMutate() bool { return r.rollback == nil && !r.bulkPending && !r.writing }

// openTextForm opens a free-text action on the dialog stack. The controller
// renders the box-less form; the section's dialog stack frames and drives it.
func (r *results) openTextForm(mode action.Mode, key, initial string) {
	var c action.Controller
	c.OpenText(mode, key, initial)
	r.dialogs.Push(newFormDialog(c, submittingGlyph()))
}

// closeForm pops the text/create form when it is the top dialog — the seam the
// external-editor round-trip uses to dismiss the overlay once it has the draft.
// It no-ops under any other dialog, so a stray call can't disturb a pick.
func (r *results) closeForm() {
	if _, ok := r.dialogs.Top().(*formDialog); ok {
		r.dialogs.Update(formFinishMsg{})
	}
}

// openComment starts a comment in the in-TUI overlay. When an external editor
// is configured ($JIRA_EDITOR/$EDITOR), ctrl+e inside the overlay hands the
// draft to it — the modal is the default, the editor the escape hatch.
func (r *results) openComment() tea.Cmd {
	iss := r.selected()
	if iss == nil || !r.canMutate() {
		return nil
	}
	r.openTextForm(action.ModeComment, issueKey(iss), "")
	return nil
}

// handleEditor resumes a flow that went through the external editor. On an
// editor failure the comment overlay (still open, still holding the draft)
// stays up so nothing typed is lost; otherwise the overlay closes and the
// editor's text submits exactly like its in-modal equivalent — an emptied
// buffer is a deliberate discard.
func (r *results) handleEditor(msg input.EditorFinishedMsg) tea.Cmd {
	if msg.Err != nil {
		r.err = msg.Err
		return nil
	}
	kind, ident, ok := strings.Cut(msg.ID, ":")
	if !ok {
		return nil
	}
	if kind != "comment" {
		return nil
	}
	r.closeForm()
	if xstrings.IsBlank(msg.Text) {
		return r.flashNotice("empty comment discarded", false)
	}
	body, _, err := adf.FromMarkdownLossy(msg.Text)
	if err != nil {
		r.err = err
		return nil
	}
	return r.mutate(activityDesc{pending: "commenting on " + ident, done: ident + " commented", key: ident}, func(svc core.Services, base context.Context) error {
		_, _, e := svc.Issues().AddComment(base, ident, &jira.CommentAddRequest{Body: body})
		return e
	})
}

// openCreate starts the new-issue overlay. The target project is the profile
// default, falling back to the selected issue's project so a scoped list "just
// works" without config. The overlay itself opens only once the project's issue
// types load (loadIssueTypes → createMetaScope → openCreateForm), since the
// type cycle field needs its options up front.
func (r *results) openCreate() tea.Cmd {
	if !r.canMutate() {
		return nil
	}
	proj := r.ctx.Project
	if proj == "" {
		if iss := r.selected(); iss != nil {
			proj = projectOf(issueKey(iss))
		}
	}
	if proj == "" {
		return r.flashNotice("no target project: set default_project or select an issue", true)
	}
	// Stash the target now so a failed type load can still open the form on the
	// default type (see the createMetaScope arm in handleTask).
	r.createProject = proj
	return r.loadIssueTypes(proj, false)
}

// createIssue submits the new-issue request. The picked type overrides the
// profile default (still Task when neither is set); an accepted assignee rides
// as its accountId and labels as a plain list. The Markdown description
// converts to ADF here, the same way comments do — the service's create payload
// does not translate Markdown — and the refetch after the mutation surfaces the
// new issue.
func (r *results) createIssue(req action.Request) (tea.Cmd, error) {
	if req.Summary == "" {
		return r.flashNotice("issue needs a summary on the first line", true), nil
	}
	issueType := req.IssueType
	if issueType == "" {
		issueType = r.ctx.DefaultIssueType
	}
	if issueType == "" {
		issueType = "Task"
	}
	create := &jira.IssueCreateRequest{Project: req.IssueKey, IssueType: issueType, Summary: req.Summary}
	fields := map[string]any{}
	if desc := strings.TrimSpace(req.Text); desc != "" {
		doc, _, err := adf.FromMarkdownLossy(desc)
		if err != nil {
			return nil, err
		}
		fields["description"] = doc
	}
	if req.Assignee != "" {
		fields["assignee"] = map[string]any{"accountId": req.Assignee}
	}
	if len(req.Labels) > 0 {
		fields["labels"] = req.Labels
	}
	if len(fields) > 0 {
		create.Fields = fields
	}
	return r.mutate(activityDesc{pending: "creating issue in " + req.IssueKey, done: "created in " + req.IssueKey}, func(svc core.Services, base context.Context) error {
		_, _, err := svc.Issues().Create(base, create)
		return err
	}), nil
}

// transitionItems builds picker items from an issue's valid transitions: the
// name is shown, the id rides the value. Only the transitions Jira allows for
// the issue are passed in, so the user can never pick an invalid one.
func transitionItems(transitions []*jira.Transition) []picker.Item {
	items := make([]picker.Item, 0, len(transitions))
	for _, t := range transitions {
		if t == nil {
			continue
		}
		items = append(items, picker.Item{Label: ptr.Deref(t.Name), Value: ptr.Deref(t.ID)})
	}
	return items
}

// applySingleTransition applies the picked transition to one issue: the row
// moves optimistically, then the write reconciles on the mutate scope.
func (r *results) applySingleTransition(key, id, name string) tea.Cmd {
	r.rollback = r.applyOptimisticTransition(key, name)
	// Record the move for the footer/log. Transitions run their own task rather
	// than r.mutate (they carry a bespoke optimistic update), so they must start
	// the activity entry here; handleTask's mutateScope arm resolves it.
	r.startOp(activityDesc{pending: "moving " + key + " to " + name, done: key + " → " + name, key: key})
	base := r.ctx.Base
	svc := r.ctx.Services
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

// handleFormSubmit routes a request the form dialog emitted. A bulk collect
// parks its y/N confirmation; a single-issue write starts on the mutate scope
// and is remembered as form-owned (formWriting) so its outcome resolves the
// open form rather than a detached toast. When the dispatch can't even start a
// write (a local encode failure), the form reopens with the reason inline.
func (r *results) handleFormSubmit(msg formSubmitMsg) tea.Cmd {
	if msg.req.Mode.Bulk() {
		// The form dialog stays open until now so the overlay never blinks
		// empty; swap it for the y/N confirmation the bulk write parks.
		r.closeForm()
		// Bulk dispatch only selects marked keys and parks a confirmation; its
		// local error path is reserved for single-issue encoding.
		cmd, _ := r.dispatchSubmit(msg.req)
		return cmd
	}
	r.formWriting = true
	cmd, err := r.dispatchSubmit(msg.req)
	if cmd == nil {
		// The dispatch bailed before contacting Jira (e.g. an unparseable
		// worklog or a Markdown-to-ADF failure). Surface it in the form and let
		// the draft stand instead of stranding it submitting.
		r.formWriting = false
		if err == nil {
			err = errors.New("could not submit")
		}
		r.dialogs.Update(formFinishMsg{err: err})
		return nil
	}
	return cmd
}

// dispatchSubmit executes a completed action request: it parks bulk writes
// behind a confirmation and runs single-issue writes on the mutate scope,
// applying any optimistic row change first. A nil cmd with a non-nil error
// means the dispatch failed locally, before contacting Jira; the caller
// surfaces the error in the open form.
func (r *results) dispatchSubmit(req action.Request) (tea.Cmd, error) {
	switch req.Mode {
	case action.ModeBulkAssign, action.ModeBulkComment:
		// The remaining bulk modes are text verbs collected by the controller;
		// bulk transition now flows through the transition pick dialog. Bulk
		// writes hit every marked issue at once, so they park behind a y/N
		// confirmation (default No) instead of running off the submit.
		keys := r.markedKeys()
		if len(keys) == 0 {
			return nil, nil
		}
		text := req.Text
		kind := bulkCommentKind
		if req.Mode == action.ModeBulkAssign {
			// Assignee queries are lookup keys; comments post verbatim
			// (leading whitespace is markdown).
			text = strings.TrimSpace(text)
			kind = bulkAssignKind
		}
		return r.parkBulkConfirm(&bulkConfirm{kind: kind, text: text, keys: keys}), nil
	case action.ModeComment:
		body, _, err := adf.FromMarkdownLossy(req.Text)
		if err != nil {
			return nil, err
		}
		issKey := req.IssueKey
		return r.mutate(activityDesc{pending: "commenting on " + issKey, done: issKey + " commented", key: issKey}, func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().AddComment(base, issKey, &jira.CommentAddRequest{Body: body})
			return e
		}), nil
	case action.ModeEdit:
		key, summary := req.IssueKey, req.Text
		r.rollback = r.applyOptimisticSummary(key, summary)
		return r.mutate(activityDesc{pending: "editing " + key, done: key + " updated", key: key}, func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().Update(base, key, &jira.IssueUpdateRequest{
				Fields: map[string]any{"summary": summary},
			})
			return e
		}), nil
	case action.ModeAssign:
		// One normalised value drives both the optimistic row and the write:
		// "" means unassign for each.
		display := strings.TrimSpace(req.Text)
		if clearsAssignee(display) {
			display = ""
		}
		r.rollback = r.applyOptimisticAssignee(req.IssueKey, display)
		return r.assignTo(req.IssueKey, display), nil
	case action.ModeLabels:
		// Full-replacement semantics: the field opened pre-filled, so what
		// comes back is the complete list — empty input clears every label.
		labels := xstrings.SplitCSV(req.Text)
		issKey := req.IssueKey
		return r.mutate(activityDesc{pending: "updating labels on " + issKey, done: issKey + " labels updated", key: issKey}, func(svc core.Services, base context.Context) error {
			_, _, e := svc.Issues().Update(base, issKey, &jira.IssueUpdateRequest{
				Fields: map[string]any{"labels": labels},
			})
			return e
		}), nil
	case action.ModeCreate:
		// IssueKey carries the project for this mode (see openCreate).
		return r.createIssue(req)
	case action.ModeWorklog:
		wd := r.ctx.WorkdaySeconds
		if wd <= 0 {
			wd = workdaySeconds
		}
		secs, err := jira.ParseDuration(req.Text, wd)
		if err != nil {
			return nil, err
		}
		key := req.IssueKey
		return r.mutate(activityDesc{pending: "logging work on " + key, done: key + " work logged", key: key}, func(svc core.Services, base context.Context) error {
			_, _, e := svc.Worklogs().Add(base, key, &jira.WorklogAddRequest{TimeSpentSeconds: secs})
			return e
		}), nil
	}
	return nil, nil
}

// activityDesc is the footer/log text for one mutation: pending shows while the
// write is in flight, done replaces it on success, and key (when set) renders
// as the entry's hyperlinked issue key.
type activityDesc struct {
	pending string
	done    string
	key     string
}

// startOp records a mutation on the activity registry and stashes the resolved
// text/key to apply when it lands. A nil registry (bare test context) is a
// no-op, so mutations still run without the footer.
func (r *results) startOp(desc activityDesc) {
	if r.ctx.Activity == nil {
		return
	}
	r.writeOpID = r.ctx.Activity.Start(desc.pending)
	r.writeOpDesc = desc
}

// finishOp resolves the recorded mutation: a non-nil error fails it, otherwise
// it lands with the stashed done text and key. It clears the handle so a later
// non-op completion on the scope can't re-resolve it.
func (r *results) finishOp(err error) {
	if r.ctx.Activity == nil || r.writeOpID == 0 {
		return
	}
	if err != nil {
		r.ctx.Activity.Fail(r.writeOpID, err)
	} else {
		r.ctx.Activity.Finish(r.writeOpID, r.writeOpDesc.done, r.writeOpDesc.key)
	}
	r.writeOpID = 0
}

// mutate runs a single-issue write on the mutate scope. On success handleTask
// reconciles by refetching; on error it rolls back any optimistic change the
// caller registered (r.rollback) and shows the failure toast. Verbs whose
// field shows in the row (edit, assign) apply optimistically before calling
// this; comment and worklog have nothing row-visible, so they just reconcile.
// desc records the write in the activity registry for the footer and log.
func (r *results) mutate(desc activityDesc, run func(svc core.Services, base context.Context) error) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	r.writing = true // block overlapping writes until this one reconciles
	r.startOp(desc)
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
	return r.mutate(activityDesc{pending: "assigning " + key, done: key + " assigned", key: key}, func(svc core.Services, base context.Context) error {
		me, _, err := svc.Users().Myself(base)
		if err != nil {
			return err
		}
		if me.AccountID == "" {
			return errors.New("could not resolve your account id for self-assign")
		}
		return setAssignee(svc, base, key, me.AccountID)
	})
}

// assignTo resolves a user query (name/email) to an account id and assigns it.
// The keywords "none"/"unassigned" (or an empty query) clear the assignee
// instead of being treated as a user search.
func (r *results) assignTo(key, query string) tea.Cmd {
	q := strings.TrimSpace(query)
	return r.mutate(activityDesc{pending: "assigning " + key, done: key + " assigned", key: key}, func(svc core.Services, base context.Context) error {
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
		browser.Open(base, url) //nolint:errcheck // browser launch is intentionally fire-and-forget
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
