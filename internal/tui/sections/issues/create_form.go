// The new-issue overlay's data plumbing: the session-scoped suggestion caches,
// the issue-type prefetch that has to land before the type cycle field can
// open, and the assignee/label fetch closures the form runs inline. Kept apart
// from results_mutate.go (which owns the write itself) so the create form's
// read side stays legible.

package issues

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gechr/primer/filter"
	"github.com/gechr/x/ptr"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/components/action"
	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/components/suggest"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

// createSuggestTTL is how long a create-form lookup (issue types, assignee
// search, labels) is reused before a refetch. Short: the create form is
// open briefly and metadata rarely changes within one session.
const createSuggestTTL = 5 * time.Minute

// assigneeKey scopes an assignable-user search to both the project and the
// query, so switching the create target never serves another project's people.
type assigneeKey struct{ project, query string }

// ensureCreateCaches builds the three suggestion caches on first use. Each
// source reads r.ctx.Services at call time (inside the form's fetch command),
// so a nil Services in a test degrades to empty suggestions rather than a
// panic. The caches persist on the section, so a second create reuses them.
func (r *results) ensureCreateCaches() {
	if r.issueTypeCache != nil {
		return
	}
	r.issueTypeCache = suggest.New(createSuggestTTL, func(ctx context.Context, project string) ([]jira.ProjectIssueType, error) {
		if r.ctx.Services == nil {
			return nil, nil
		}
		types, _, err := r.ctx.Services.Projects().ListIssueTypes(ctx, project)
		return types, err
	}, nil)
	r.assigneeCache = suggest.New(createSuggestTTL, func(ctx context.Context, key assigneeKey) ([]*jira.User, error) {
		if r.ctx.Services == nil {
			return nil, nil
		}
		users, _, err := r.ctx.Services.Users().AssignableSearch(ctx, key.query, key.project)
		return users, err
	}, nil)
	r.labelCache = suggest.New(createSuggestTTL, func(ctx context.Context, _ string) ([]string, error) {
		if r.ctx.Services == nil {
			return nil, nil
		}
		labels, _, err := r.ctx.Services.Labels().List(ctx, nil)
		return labels, err
	}, nil)
	r.projectCache = suggest.New(createSuggestTTL, func(ctx context.Context, _ struct{}) ([]string, error) {
		if r.ctx.Services == nil {
			return nil, nil
		}
		projects, _, err := r.ctx.Services.Projects().List(ctx, nil)
		if err != nil {
			return nil, err
		}
		return projectKeys(projects), nil
	}, nil)
}

// loadIssueTypes loads a project's create-screen issue types on the createmeta
// scope. The cycle field needs its full option set at construction, so unlike
// the assignee/label fields (which fetch inline) the types are prefetched
// before the overlay appears; handleTask opens the form once they land. With
// update set the load instead feeds a newly picked project's types back into
// the already-open form — same scope, same cache, so stepping to a project
// visited before updates instantly.
func (r *results) loadIssueTypes(project string, update bool) tea.Cmd {
	r.ensureCreateCaches()
	base := r.ctx.Base
	return r.ctx.StartTask(core.TaskSpec{
		Scope: r.createMetaScope(),
		Run: func() (any, error) {
			// The initial open also needs the project list for the pill; it is
			// independent of the type fetch, so the two run concurrently and the
			// open waits for the slower one rather than their sum. A failed
			// project list degrades to the current project alone — the form
			// still creates there — so it must not block the open.
			var projects []string
			done := make(chan struct{})
			if update {
				close(done)
			} else {
				go func() {
					defer close(done)
					var err error
					projects, err = r.projectCache.Get(base, struct{}{})
					if err != nil {
						projects = nil
					}
				}()
			}
			types, err := r.issueTypeCache.Get(base, project)
			<-done
			if err != nil {
				return nil, err
			}
			if update {
				return createMetaResult{project: project, types: types, update: true}, nil
			}
			return createMetaResult{project: project, types: types, projects: ensureContains(projects, project)}, nil
		},
	})
}

// openCreateForm builds the new-issue controller with the loaded types, the
// selectable projects, and the inline suggestion sources, then pushes it onto
// the dialog stack. The assignee source is bound to the opening project; a
// later project-pill change refetches the type list but keeps the assignee
// scope, since re-scoping the fetch closure (read on its own goroutine) would
// race the main loop's write of the current target — an accepted assignee's
// accountId stays valid across projects regardless.
func (r *results) openCreateForm(project string, types []jira.ProjectIssueType, projects []string) {
	r.createProject = project
	var c action.Controller
	c.OpenCreate(action.CreateConfig{
		Project:       project,
		Projects:      projects,
		IssueTypes:    issueTypeNames(types),
		DefaultType:   r.ctx.DefaultIssueType,
		AssigneeFetch: r.assigneeFetch(project),
		LabelFetch:    r.labelFetch(),
	})
	// The form opens when the type prefetch lands, not on the keypress itself,
	// so it takes the async-open grace: a keystroke in flight while the fetch
	// ran must not type into the summary of a form the user hasn't seen.
	r.dialogs.PushWithGrace(newFormDialog(c, submittingGlyph()))
}

// projectKeys projects the discovery summaries to their keys for the create
// pill's options, dropping any without one.
func projectKeys(projects []jira.ProjectSummary) []string {
	out := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.Key != "" {
			out = append(out, p.Key)
		}
	}
	return out
}

// ensureContains prepends want to list when the list does not already hold it,
// so the create pill always offers the target project even if the discovery
// list is stale or scoped away from it. A blank want is left out.
func ensureContains(list []string, want string) []string {
	if want == "" || slices.Contains(list, want) {
		return list
	}
	return append([]string{want}, list...)
}

// assigneeFetch returns the form's assignee suggestion source. It prefetches the
// project's assignable users once (the empty-query page) and filters that set
// locally, so the common case — assigning to a teammate already on the page —
// needs no per-keystroke round trip. Only when the local set has no match for a
// non-empty query does it fall back to a server search: Jira caps the
// empty-query page, so a name outside it (a large tenant, an unusual assignee)
// still resolves, at the cost of one round trip for that query. Each suggestion's
// Detail is the accountId, so an accepted name carries its id to the write
// without a re-resolve; errors yield no suggestions (the field stays usable).
func (r *results) assigneeFetch(project string) func(string) []form.Suggestion {
	return func(query string) []form.Suggestion {
		if base, err := r.assigneeCache.Get(r.ctx.Base, assigneeKey{project: project}); err == nil {
			if hits := matchUsers(base, query); len(hits) > 0 || strings.TrimSpace(query) == "" {
				return usersToSuggestions(hits)
			}
		}
		users, err := r.assigneeCache.Get(r.ctx.Base, assigneeKey{project: project, query: query})
		if err != nil {
			return nil
		}
		return usersToSuggestions(users)
	}
}

// labelFetch returns the form's label suggestion source. Jira's label list is
// global, so it is fetched whole (cache key "") and filtered to the typed token
// locally rather than per-query over the wire.
func (r *results) labelFetch() func(string) []form.Suggestion {
	return func(query string) []form.Suggestion {
		labels, err := r.labelCache.Get(r.ctx.Base, "")
		if err != nil {
			return nil
		}
		return matchLabels(labels, query)
	}
}

// issueTypeNames projects the createmeta types to their names for the cycle
// field's options, preserving the create screen's order.
func issueTypeNames(types []jira.ProjectIssueType) []string {
	out := make([]string, 0, len(types))
	for _, t := range types {
		if t.Name != "" {
			out = append(out, t.Name)
		}
	}
	return out
}

// usersToSuggestions maps assignable users to form suggestions: the label is
// the display name (falling back to email, then the id), and the Detail is the
// accountId. Users without an accountId are dropped — there is nothing to
// assign.
func usersToSuggestions(users []*jira.User) []form.Suggestion {
	out := make([]form.Suggestion, 0, len(users))
	for _, u := range users {
		if u == nil || u.AccountID == nil || *u.AccountID == "" {
			continue
		}
		name := ptr.Deref(u.DisplayName)
		if name == "" {
			name = ptr.Deref(u.EmailAddress)
		}
		if name == "" {
			name = *u.AccountID
		}
		out = append(out, form.Suggestion{Value: name, Label: name, Detail: *u.AccountID})
	}
	return out
}

// matchUsers filters assignable users to those whose display name or email
// contains query (case-insensitive substring via filter.Term), mirroring
// matchLabels for the prefetched assignee set. A blank query matches every
// user, so an empty field still offers the page.
func matchUsers(users []*jira.User, query string) []*jira.User {
	term := filter.Term{Text: strings.TrimSpace(query), Case: filter.CaseInsensitive}
	out := make([]*jira.User, 0, len(users))
	for _, u := range users {
		if u == nil {
			continue
		}
		if term.Match(ptr.Deref(u.DisplayName) + " " + ptr.Deref(u.EmailAddress)) {
			out = append(out, u)
		}
	}
	return out
}

// matchLabels filters the global label list to those containing the typed token
// (case-insensitive substring via filter.Term). A blank token matches every
// label, so an empty field still offers the list.
func matchLabels(labels []string, query string) []form.Suggestion {
	term := filter.Term{Text: query, Case: filter.CaseInsensitive}
	out := make([]form.Suggestion, 0, len(labels))
	for _, l := range labels {
		if term.Match(l) {
			out = append(out, form.Suggestion{Value: l, Label: l})
		}
	}
	return out
}
