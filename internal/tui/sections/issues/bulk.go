package issues

import (
	"context"
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/gechr/x/human"
	"golang.org/x/sync/errgroup"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// selectAllShown marks every visible row, or clears them all when every one
// is already marked — one key toggles between all and none.
func (r *results) selectAllShown() {
	if len(r.shown) == 0 {
		return
	}
	if r.marks == nil {
		r.marks = make(map[string]bool)
	}
	all := true
	for _, iss := range r.shown {
		if !r.marks[issueKey(iss)] {
			all = false
			break
		}
	}
	for _, iss := range r.shown {
		if all {
			delete(r.marks, issueKey(iss))
		} else {
			r.marks[issueKey(iss)] = true
		}
	}
}

// invertShown flips the mark on every visible row.
func (r *results) invertShown() {
	for _, iss := range r.shown {
		r.toggleMark(issueKey(iss))
	}
}

// selectRange marks every row between the anchor (the last space-toggled
// row) and the cursor, inclusive. With no anchor in the visible set the
// cursor row is marked and becomes the anchor, so x-x-x walks like a range.
func (r *results) selectRange() {
	cur := r.list.Cursor()
	if cur < 0 || cur >= len(r.shown) {
		return
	}
	anchor := -1
	for i, iss := range r.shown {
		if issueKey(iss) == r.markAnchor {
			anchor = i
			break
		}
	}
	if anchor == -1 {
		anchor = cur
		r.markAnchor = issueKey(r.shown[cur])
	}
	lo, hi := anchor, cur
	if lo > hi {
		lo, hi = hi, lo
	}
	if r.marks == nil {
		r.marks = make(map[string]bool)
	}
	for i := lo; i <= hi; i++ {
		r.marks[issueKey(r.shown[i])] = true
	}
}

// bulkConfirm is a bulk request parked behind a y/N prompt. Bulk writes hit
// every marked issue at once, so they never run straight off the modal
// submit; the prompt restates the verb and the target count first.
type bulkConfirm struct {
	mode action.Mode
	text string
	keys []string
}

// prompt is the confirmation line: the verb, how many issues it hits (with
// a key preview), defaulting to No.
func (b *bulkConfirm) prompt() string {
	n := len(b.keys)
	target := fmt.Sprintf("%s (%s)", human.Pluralize(n, "issue", "issues"), keysPreview(b.keys))
	switch b.mode {
	case action.ModeBulkTransition:
		return fmt.Sprintf("Transition %s to %q? (y/N)", target, b.text)
	case action.ModeBulkAssign:
		if clearsAssignee(b.text) {
			return fmt.Sprintf("Unassign %s? (y/N)", target)
		}
		return fmt.Sprintf("Assign %s to %q? (y/N)", target, b.text)
	case action.ModeBulkComment:
		return fmt.Sprintf("Comment on %s? (y/N)", target)
	}
	// Unreachable today (submitAction only parks the three modes above), but
	// an empty overlay would strand the user with no clue what y applies.
	return fmt.Sprintf("Apply %s to %s? (y/N)", b.mode, target)
}

// keysPreview shows the first few target keys so the user confirms against
// real issues, not just a count.
func keysPreview(keys []string) string {
	const max = 5
	if len(keys) <= max {
		return strings.Join(keys, ", ")
	}
	return strings.Join(keys[:max], ", ") + fmt.Sprintf(" +%d more", len(keys)-max)
}

// updateConfirm resolves the pending bulk confirmation: y applies, anything
// else cancels and keeps the selection so the user can adjust and retry.
func (r *results) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	c := r.confirm
	r.confirm = nil
	switch msg.String() {
	case "y", "Y":
		return r.runBulk(c)
	default:
		return nil
	}
}

// runBulk dispatches a confirmed bulk request, marking the batch in flight
// only when a task actually started (a comment that fails ADF conversion
// must not strand bulkPending).
func (r *results) runBulk(c *bulkConfirm) tea.Cmd {
	var cmd tea.Cmd
	switch c.mode {
	case action.ModeBulkTransition:
		cmd = r.bulkTransition(c.keys, c.text)
	case action.ModeBulkAssign:
		cmd = r.bulkAssign(c.keys, c.text)
	case action.ModeBulkComment:
		cmd = r.bulkComment(c.keys, c.text)
	}
	if cmd != nil {
		r.bulkPending = true
	}
	return cmd
}

// runBulkPool applies one mutation per key through a bounded worker pool.
// Each worker returns nil so one issue's failure never cancels its siblings;
// every outcome is collected into the bulkResult instead.
func runBulkPool(base context.Context, verb string, keys []string, apply func(ctx context.Context, key string) error) bulkResult {
	res := bulkResult{verb: verb, failed: make(map[string]error)}
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(base)
	g.SetLimit(4)
	for _, key := range keys {
		key := key // defensive copy; safe on go ≥1.22 but explicit
		g.Go(func() error {
			err := apply(ctx, key)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				res.failed[key] = err
			} else {
				res.succeeded = append(res.succeeded, key)
			}
			return nil
		})
	}
	_ = g.Wait() // workers never return an error, so Wait cannot either
	return res
}

// bulkTransition moves every marked issue to the named status concurrently,
// resolving the per-issue transition id at apply time (different issues
// expose different transitions).
func (r *results) bulkTransition(keys []string, status string) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	verb := "→ " + status
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.mutateScope(),
		Run: func() (any, error) {
			if svc == nil {
				return bulkResult{verb: verb}, nil
			}
			return runBulkPool(base, verb, keys, func(ctx context.Context, key string) error {
				return transitionByName(ctx, svc, key, status)
			}), nil
		},
	})
}

// clearsAssignee reports that an assign query means "unassign" rather than
// a user search (mirrors the single-issue assignTo keywords).
func clearsAssignee(query string) bool {
	q := strings.TrimSpace(query)
	return q == "" || strings.EqualFold(q, "none") || strings.EqualFold(q, "unassigned")
}

// bulkAssign resolves the user query once, then writes the same assignee to
// every marked issue. A resolution failure fails the whole batch before any
// write — half-assigning to a mistyped name helps nobody.
func (r *results) bulkAssign(keys []string, query string) tea.Cmd {
	base := r.ctx.Base
	svc := r.ctx.Services
	q := strings.TrimSpace(query)
	verb := "→ @" + q
	if clearsAssignee(q) {
		verb = "unassigned"
	}
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.mutateScope(),
		Run: func() (any, error) {
			if svc == nil {
				return bulkResult{verb: verb}, nil
			}
			id := ""
			if !clearsAssignee(q) {
				var err error
				id, err = svc.Users().ResolveUser(base, q)
				if err != nil {
					return nil, err
				}
			}
			return runBulkPool(base, verb, keys, func(ctx context.Context, key string) error {
				return setAssignee(svc, ctx, key, id)
			}), nil
		},
	})
}

// bulkComment posts the same comment on every marked issue. The markdown
// converts to ADF once, up front — a conversion error surfaces immediately
// instead of failing per issue. The document is a plain node tree that the
// workers only read (JSON marshaling), so sharing it across the pool is safe.
func (r *results) bulkComment(keys []string, text string) tea.Cmd {
	body, _, err := adf.FromMarkdownLossy(text)
	if err != nil {
		r.err = err
		return nil
	}
	base := r.ctx.Base
	svc := r.ctx.Services
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.mutateScope(),
		Run: func() (any, error) {
			if svc == nil {
				return bulkResult{verb: "commented"}, nil
			}
			return runBulkPool(base, "commented", keys, func(ctx context.Context, key string) error {
				_, _, e := svc.Issues().AddComment(ctx, key, &jira.CommentAddRequest{Body: body})
				return e
			}), nil
		},
	})
}
