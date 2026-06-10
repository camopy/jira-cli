package issues

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/matcra587/jira-cli/internal/jira"
)

func facetIssue(key, status, assignee string, labels ...string) *jira.Issue {
	i := mkIssue(key, status, "summary "+key)
	if assignee != "" {
		i.Fields.Assignee = &jira.User{DisplayName: &assignee}
	}
	i.Fields.Labels = labels
	return i
}

func facetModel(t *testing.T) *Model {
	t.Helper()
	m := changeModel(t)
	land(m,
		facetIssue("JCT-1", "To Do", "Ann", "infra"),
		facetIssue("JCT-2", "In Progress", "Ann", "infra", "urgent"),
		facetIssue("JCT-3", "In Progress", "Bob"),
		facetIssue("JCT-4", "Done", ""),
	)
	return m
}

func TestFacetsForCollectsDistinctValuesWithCounts(t *testing.T) {
	m := facetModel(t)
	items := facetsFor(m.all)
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.Label
	}
	joined := strings.Join(labels, "\n")
	for _, want := range []string{
		"status: In Progress (2)",
		"status: To Do (1)",
		"assignee: Ann (2)",
		"label: infra (2)",
		"label: urgent (1)",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("facets missing %q in:\n%s", want, joined)
		}
	}
}

func TestFacetNarrowsAndComposesWithTextFilter(t *testing.T) {
	m := facetModel(t)
	m.facet = facet{field: "status", value: "In Progress"}
	m.applyFilter()
	if len(m.shown) != 2 {
		t.Fatalf("status facet shown = %d, want 2", len(m.shown))
	}
	m.filter = "JCT-3"
	m.applyFilter()
	if len(m.shown) != 1 || issueKey(m.shown[0]) != "JCT-3" {
		t.Errorf("facet+filter shown = %v", m.shown)
	}
}

func TestLabelFacetMatchesAnyOfTheIssueLabels(t *testing.T) {
	m := facetModel(t)
	m.facet = facet{field: "label", value: "urgent"}
	m.applyFilter()
	if len(m.shown) != 1 || issueKey(m.shown[0]) != "JCT-2" {
		t.Errorf("label facet shown = %v", m.shown)
	}
}

func TestFacetKeyOpensPickerAndEnterApplies(t *testing.T) {
	m := facetModel(t)
	m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !m.faceting || !m.CapturesInput() {
		t.Fatal("f did not open the facet picker")
	}
	// Type to narrow to the Bob facet, then apply.
	for _, r := range "bob" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.faceting {
		t.Fatal("enter did not close the picker")
	}
	if m.facet.field != "assignee" || m.facet.value != "Bob" {
		t.Fatalf("applied facet = %+v", m.facet)
	}
	if len(m.shown) != 1 || issueKey(m.shown[0]) != "JCT-3" {
		t.Errorf("shown after facet = %v", m.shown)
	}
}

func TestFacetEscClosesWithoutApplying(t *testing.T) {
	m := facetModel(t)
	m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.faceting || m.facet != (facet{}) {
		t.Errorf("esc should close without applying: faceting=%v facet=%+v", m.faceting, m.facet)
	}
}

func TestActiveFacetShowsChipAndClearItemRestores(t *testing.T) {
	m := facetModel(t)
	m.facet = facet{field: "status", value: "Done"}
	m.applyFilter()
	if line := ansi.Strip(m.statusLine()); !strings.Contains(line, "status: Done") {
		t.Errorf("status line missing facet chip: %q", line)
	}

	// Reopen the picker: with a facet active its first item clears it.
	m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.facet != (facet{}) {
		t.Errorf("clear item did not reset the facet: %+v", m.facet)
	}
	if len(m.shown) != 4 {
		t.Errorf("shown after clear = %d, want all 4", len(m.shown))
	}
}

func TestUnassignedFacetSelectsIssuesWithoutAssignee(t *testing.T) {
	m := facetModel(t)
	items := facetsFor(m.all)
	found := false
	for _, it := range items {
		if strings.Contains(it.Label, "assignee: Unassigned (1)") {
			found = true
		}
	}
	if !found {
		t.Fatal("unassigned bucket missing from facets")
	}
	m.facet = facet{field: "assignee", value: unassignedFacet}
	m.applyFilter()
	if len(m.shown) != 1 || issueKey(m.shown[0]) != "JCT-4" {
		t.Errorf("unassigned facet shown = %v, want [JCT-4]", m.shown)
	}
}

func TestPasteTypesIntoOpenFacetPicker(t *testing.T) {
	m := facetModel(t)
	m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m.Update(tea.PasteMsg{Content: "urgent"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.facet.field != "label" || m.facet.value != "urgent" {
		t.Errorf("pasted facet filter applied %+v, want label/urgent", m.facet)
	}
}
