// Package goldens pins the dashboard's rendered frames at the two reference
// terminal sizes (80x24 and 120x40) per section. The App is driven
// synchronously through its Update loop with fake services and the final
// frame is asserted against golden files (teatest's golden helper; run with
// -update to regenerate).
//
// A full teatest program run is deliberately not used for the assertion:
// the loading spinner renders a timing-dependent number of frames into the
// cumulative output stream, so whole-stream goldens flake. Driving Update
// directly and asserting the settled frame keeps the golden byte-stable.
package goldens

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/sections/issues"
	"github.com/matcra587/jira-cli/internal/tui/sections/settings"
)

// fakeIssueSvc serves a fixed issue list; embedding satisfies the rest.
type fakeIssueSvc struct {
	jira.IssueService
	issues []*jira.Issue
}

func (f fakeIssueSvc) List(context.Context, *jira.IssueListOptions) ([]*jira.Issue, *jira.Response, error) {
	return f.issues, nil, nil
}

type fakeJQLSvc struct{ jira.JQLService }

func (fakeJQLSvc) Parse(context.Context, []string, string) ([]jira.ParsedQuery, *jira.Response, error) {
	return []jira.ParsedQuery{{}}, nil, nil
}

type fakeServices struct {
	core.Services
	issue jira.IssueService
}

func (f fakeServices) Issues() jira.IssueService { return f.issue }
func (f fakeServices) JQL() jira.JQLService      { return fakeJQLSvc{} }

func mkIssue(key, status, summary, issueType, priority, assignee string) *jira.Issue {
	k, st, su := key, status, summary
	f := &jira.IssueFields{Summary: &su, Status: &jira.Status{Name: &st}}
	if issueType != "" {
		f.IssueType = &jira.IssueType{Name: &issueType}
	}
	if priority != "" {
		f.Priority = &jira.Priority{Name: &priority}
	}
	if assignee != "" {
		f.Assignee = &jira.User{DisplayName: &assignee}
	}
	return &jira.Issue{Key: &k, Fields: f}
}

func fixtureIssues() []*jira.Issue {
	return []*jira.Issue{
		mkIssue("JCT-1", "To Do", "Wire the flux capacitor", "Task", "High", "Ann Example"),
		mkIssue("JCT-2", "In Progress", "Reticulate splines before launch window", "Bug", "Highest", "Bob Sample"),
		mkIssue("JCT-3", "Done", "Document the splines", "Subtask", "Low", ""),
	}
}

// buildApp wires a dashboard exactly like the real entrypoint, minus the
// live client: issues + search + settings (always last, as the real order
// resolver appends it), fake services, no config file.
func buildApp() core.App {
	ctx := core.NewProgramContext(fakeServices{issue: fakeIssueSvc{issues: fixtureIssues()}}, nil)
	reg := core.NewRegistry()
	reg.Register(issues.ID, issues.New)
	reg.Register(issues.SearchID, issues.NewSearch)
	reg.Register(settings.ID, settings.New)
	ctx.Version = "v0.0.0-test"
	ctx.ProfileName = "default"
	ctx.Project = "JCT"
	return core.NewApp(ctx, reg, []core.SectionID{issues.ID, issues.SearchID, settings.ID})
}

// drive runs the model's Update loop over the cmd graph until it settles:
// every cmd executes on its own goroutine (a 60s refresh tick simply never
// delivers), batches fan out, and the loop stops once no message has
// arrived for a quiet period.
func drive(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	msgs := make(chan tea.Msg, 256)
	var inflight atomic.Int64
	exec := func(c tea.Cmd) {
		if c == nil {
			return
		}
		inflight.Add(1)
		go func() {
			defer inflight.Add(-1)
			if msg := c(); msg != nil {
				// Non-blocking: a tick that fires after drive returned (the
				// refresh heartbeat, a flash clear) must not strand its
				// goroutine on a full channel across -count runs.
				select {
				case msgs <- msg:
				default:
				}
			}
		}()
	}
	exec(cmd)
	const quietWindow = 250 * time.Millisecond
	quiet := time.NewTimer(quietWindow)
	defer quiet.Stop()
	for {
		select {
		case msg := <-msgs:
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, c := range batch {
					exec(c)
				}
				continue
			}
			var c tea.Cmd
			m, c = m.Update(msg)
			exec(c)
			quiet.Reset(quietWindow)
		case <-quiet.C:
			// Don't settle while cmds are still running (a slow scheduler
			// could otherwise hand back a half-processed frame) — long-lived
			// timers (the 60s refresh tick) are the documented exception: two
			// consecutive quiet windows with the same in-flight count means
			// the rest are timers, not work.
			if inflight.Load() == 0 {
				return m
			}
			select {
			case msg := <-msgs:
				if batch, ok := msg.(tea.BatchMsg); ok {
					for _, c := range batch {
						exec(c)
					}
				} else {
					var c tea.Cmd
					m, c = m.Update(msg)
					exec(c)
				}
				quiet.Reset(quietWindow)
			case <-time.After(quietWindow):
				return m // only timers left in flight
			}
		}
	}
}

// frame renders the settled dashboard at the given size, optionally after
// extra key presses (e.g. tab to reach the search section).
func frame(t *testing.T, w, h int, keys ...tea.KeyPressMsg) string {
	t.Helper()
	app := buildApp()
	var m tea.Model = app
	init := m.(core.App).Init()
	m = drive(t, m, init)
	m = drive(t, m, func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} })
	for _, k := range keys {
		key := k
		m = drive(t, m, func() tea.Msg { return key })
	}
	return m.(core.App).View().Content
}

func assertGolden(t *testing.T, content string, wantHeight int) {
	t.Helper()
	// Every frame must be exactly the advertised height — a drifting footer
	// or an overflowing body is a layout regression even when the golden is
	// regenerated with -update.
	if got := len(strings.Split(strings.TrimRight(content, "\n"), "\n")); got != wantHeight {
		t.Errorf("frame height = %d lines, want %d", got, wantHeight)
	}
	teatest.RequireEqualOutput(t, []byte(content))
}

func tab() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyTab} }

func TestGoldenIssues80x24(t *testing.T) {
	assertGolden(t, frame(t, 80, 24), 24)
}

func TestGoldenIssues120x40(t *testing.T) {
	assertGolden(t, frame(t, 120, 40), 40)
}

func TestGoldenSearch80x24(t *testing.T) {
	assertGolden(t, frame(t, 80, 24, tab()), 24)
}

func TestGoldenSearch120x40(t *testing.T) {
	assertGolden(t, frame(t, 120, 40, tab()), 40)
}

func TestGoldenSettings80x24(t *testing.T) {
	assertGolden(t, frame(t, 80, 24, tab(), tab()), 24)
}

func TestGoldenSettings120x40(t *testing.T) {
	assertGolden(t, frame(t, 120, 40, tab(), tab()), 40)
}
