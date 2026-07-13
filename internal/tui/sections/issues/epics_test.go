package issues

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

func newEpicsModel(t *testing.T) *EpicsModel {
	t.Helper()
	ctx := newTestCtx(fakeServices{})
	m := NewEpics(ctx).(*EpicsModel)
	m.ctx = ctx
	m.applySize(2)
	return m
}

func TestEpicsCarouselDrivesChildQuery(t *testing.T) {
	m := newEpicsModel(t)
	m.applyEpics(epicsResult{epics: []*jira.Issue{
		mkIssue("JCT-100", "To Do", "Alpha epic"),
		mkIssue("JCT-200", "To Do", "Beta epic"),
	}})
	if !strings.Contains(m.lastJQL, "parent = JCT-100") {
		t.Fatalf("child query after load = %q, want the first epic", m.lastJQL)
	}
	m.cycleEpic(1)
	if !strings.Contains(m.lastJQL, "parent = JCT-200") {
		t.Fatalf("child query after cycle = %q, want the second epic", m.lastJQL)
	}
	m.cycleEpic(1) // wraps
	if !strings.Contains(m.lastJQL, "parent = JCT-100") {
		t.Fatalf("cycle did not wrap: %q", m.lastJQL)
	}
}

func TestEpicsSelectionSurvivesRefresh(t *testing.T) {
	m := newEpicsModel(t)
	a, b := mkIssue("JCT-100", "To Do", "Alpha"), mkIssue("JCT-200", "To Do", "Beta")
	m.applyEpics(epicsResult{epics: []*jira.Issue{a, b}})
	m.cycleEpic(1) // select Beta
	// A background refresh reorders the strip; the selection must follow the
	// key, not the index, so the visible children never silently swap.
	m.applyEpics(epicsResult{epics: []*jira.Issue{b, a}})
	if got := m.activeEpicKey(); got != "JCT-200" {
		t.Fatalf("selection after reorder = %q, want JCT-200", got)
	}
}

func TestEpicsEmptyStateRendersHint(t *testing.T) {
	m := newEpicsModel(t)
	if cmd := m.applyEpics(epicsResult{}); cmd != nil {
		t.Fatal("empty epic list still fetched children")
	}
	if !strings.Contains(m.header(), "no open epics") {
		t.Fatalf("empty header = %q", m.header())
	}
}

func TestEpicsJQLScopesToProject(t *testing.T) {
	m := newEpicsModel(t)
	if strings.Contains(m.epicsJQL(), "project =") {
		t.Fatal("unscoped profile got a project clause")
	}
	m.ctx.Project = "JCT"
	jql := m.epicsJQL()
	for _, want := range []string{"project = JCT", "issuetype = Epic", "statusCategory != Done"} {
		if !strings.Contains(jql, want) {
			t.Errorf("epics JQL %q missing %q", jql, want)
		}
	}
}

var _ core.Section = (*EpicsModel)(nil)

// TestRestyleKeepsSpinnerTicking pins the in-place spinner restyle: a theme
// preview mid-fetch must not orphan the in-flight tick chain (a replaced
// model would reject the old chain's id and the spinner would freeze).
func TestRestyleKeepsSpinnerTicking(t *testing.T) {
	m := newEpicsModel(t)
	m.loading = true
	tick := m.spin.Tick()
	m.restyle()
	if cmd := m.handleSpinner(tick.(spinner.TickMsg)); cmd == nil {
		t.Fatal("pre-restyle tick was rejected; the spinner chain died")
	}
}
