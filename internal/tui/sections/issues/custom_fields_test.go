package issues

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

type recordingIssueFieldsService struct {
	jira.IssueService
	fields    []string
	getFields []string
}

func (s *recordingIssueFieldsService) List(_ context.Context, opts *jira.IssueListOptions) ([]*jira.Issue, *jira.Response, error) {
	s.fields = append([]string(nil), opts.Fields...)
	return nil, nil, nil
}

func (s *recordingIssueFieldsService) Get(_ context.Context, key string, opts *jira.IssueGetOptions) (*jira.Issue, *jira.Response, error) {
	s.getFields = append([]string(nil), opts.Fields...)
	return mkIssue(key, "To Do", "summary"), nil, nil
}

func TestIssueFetchFieldsAppendsCustomFieldsWithoutMutatingBase(t *testing.T) {
	before := append([]string(nil), fetchFields...)
	fields := []core.CustomField{
		{ID: "customfield_10010"},
		{ID: "customfield_10020"},
	}

	got := issueFetchFields(fields)
	want := append(append([]string(nil), before...), "customfield_10010", "customfield_10020")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("issueFetchFields() = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(fetchFields, before) {
		t.Fatalf("issueFetchFields mutated fetchFields: %v", fetchFields)
	}
}

func TestRunFetchSnapshotsCustomFieldProjection(t *testing.T) {
	svc := &recordingIssueFieldsService{}
	ctx := newTestCtx(fakeServices{issue: svc})
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10010"}}
	m := New(ctx).(*Model)
	cmd := m.Init(ctx)
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10020"}}

	taskMsg(t, cmd)
	if !slices.Contains(svc.fields, "customfield_10010") || slices.Contains(svc.fields, "customfield_10020") {
		t.Fatalf("fetch fields = %v, want launch-time customfield_10010 only", svc.fields)
	}
}

func TestFullDetailRequestsConfiguredFieldsAndComments(t *testing.T) {
	svc := &recordingIssueFieldsService{}
	ctx := newTestCtx(fakeServices{issue: svc})
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10010"}}
	m := New(ctx).(*Model)
	m.Init(ctx)

	taskMsg(t, m.openDetail(mkIssue("JCT-1", "To Do", "summary")))
	for _, field := range []string{"description", "comment", "customfield_10010"} {
		t.Run(field, func(t *testing.T) {
			if !slices.Contains(svc.getFields, field) {
				t.Errorf("detail fields = %v, want %q", svc.getFields, field)
			}
		})
	}
}

func TestCustomFieldReloadRefetchesDuringReadOnlyInteractions(t *testing.T) {
	ctx := newTestCtx(nil)
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10010"}}

	t.Run("local filter", func(t *testing.T) {
		m := New(ctx).(*Model)
		m.Init(ctx)
		m.filtering = true
		if _, cmd := m.Update(core.ConfigReloadedMsg{Config: &config.Config{}}); cmd == nil {
			t.Fatal("custom-field reload must refetch while the local filter is open")
		}
	})

	t.Run("search editor", func(t *testing.T) {
		s := NewSearch(ctx).(*SearchModel)
		s.Init(ctx)
		s.jql = "project = JCT"
		s.editing = true
		if _, cmd := s.Update(core.ConfigReloadedMsg{Config: &config.Config{}}); cmd == nil {
			t.Fatal("custom-field reload must refetch the committed query while its editor is open")
		}
	})
}

func TestEmptySearchWithConfiguredFieldsDoesNotRefetchForever(t *testing.T) {
	ctx := newTestCtx(nil)
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10010"}}
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)

	msg := taskMsg(t, s.fetch()).(core.TaskFinishedMsg)
	cmd, handled := s.handleTask(msg)
	if !handled || cmd != nil {
		t.Fatalf("empty search handled = %v, cmd = %v; want settled fetch", handled, cmd)
	}
}

func TestConfiguredFieldsWithoutServicesDoNotRefetchForever(t *testing.T) {
	ctx := newTestCtx(nil)
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10010"}}
	m := New(ctx).(*Model)

	msg := taskMsg(t, m.Init(ctx)).(core.TaskFinishedMsg)
	cmd, handled := m.handleTask(msg)
	if !handled || cmd != nil {
		t.Fatalf("nil-service fetch handled = %v, cmd = %v; want settled fetch", handled, cmd)
	}
}

func TestStaleFieldProjectionRefetchesAfterHotReload(t *testing.T) {
	ctx := newTestCtx(fakeServices{})
	m := New(ctx).(*Model)
	m.Init(ctx)
	ctx.CustomFields = []core.CustomField{{ID: "customfield_10010"}}

	cmd, handled := m.handleTask(core.TaskFinishedMsg{Scope: m.fetchScope(), Result: fetchResult{}})
	if !handled || cmd == nil {
		t.Fatal("a fetch launched before the custom-field reload must refetch")
	}

	cmd, handled = m.handleTask(core.TaskFinishedMsg{
		Scope:  m.fetchScope(),
		Result: fetchResult{},
		Err:    errors.New("stale request failed"),
	})
	if !handled || cmd == nil {
		t.Fatal("a failed fetch launched before the custom-field reload must refetch")
	}
}

func TestIssueFlaggedUsesRawConfiguredJiraField(t *testing.T) {
	field := core.CustomField{ID: "customfield_10021", Name: "Flagged"}
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "impediment option", raw: `[{"value":"Impediment"}]`, want: true},
		{name: "null", raw: `null`, want: false},
		{name: "empty array", raw: `[]`, want: false},
		{name: "invalid json", raw: `{`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issue := mkIssue("JCT-1", "To Do", tc.name)
			issue.Fields.CustomFields = map[string]json.RawMessage{field.ID: json.RawMessage(tc.raw)}
			if got := issueFlagged(issue, []core.CustomField{field}); got != tc.want {
				t.Errorf("issueFlagged(%s) = %t, want %t", tc.raw, got, tc.want)
			}
		})
	}
}

func TestCustomFieldValueFormatsJiraShapes(t *testing.T) {
	iss := mkIssue("JCT-1", "To Do", "summary")
	iss.Fields.CustomFields = map[string]json.RawMessage{
		"customfield_1": json.RawMessage(`5.5`),
		"customfield_2": json.RawMessage(`{"value":"Enterprise"}`),
		"customfield_3": json.RawMessage(`[{"name":"Alpha"},{"name":"Beta"}]`),
		"customfield_4": json.RawMessage(`null`),
		"customfield_5": json.RawMessage(`"line\n\u0000\u0007\u001b[31mred"`),
	}
	cases := []struct {
		name string
		id   string
		want string
	}{
		{name: "number", id: "customfield_1", want: "5.5"},
		{name: "object", id: "customfield_2", want: "Enterprise"},
		{name: "array", id: "customfield_3", want: "Alpha, Beta"},
		{name: "null", id: "customfield_4", want: "—"},
		{name: "sanitized string", id: "customfield_5", want: "line red"},
		{name: "missing", id: "customfield_6", want: "—"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := customFieldValue(iss, core.CustomField{ID: tc.id}); got != tc.want {
				t.Errorf("customFieldValue(%s) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

func TestCustomFieldsRenderInPreviewAndOverview(t *testing.T) {
	iss := mkIssue("JCT-1", "To Do", "summary")
	iss.Fields.CustomFields = map[string]json.RawMessage{"customfield_1": json.RawMessage(`5`)}
	fields := []core.CustomField{{ID: "customfield_1", Name: "Story Points", Label: "Points", Column: true}}

	preview := sidebar(iss, 60, mdr(), "", fields...)
	overview := renderDetail(iss, false, 60, detailOverview, mdr(), "", "", fields...)
	for _, tc := range []struct{ name, out string }{{"preview", preview}, {"overview", overview}} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.out, "\nPoints: 5\n") || strings.Contains(tc.out, "Story Points: 5") {
				t.Errorf("custom field did not use configured label: %q", tc.out)
			}
		})
	}
}

func TestCustomFieldColumnsRespectPriorityAndWidth(t *testing.T) {
	iss := mkIssue("JCT-1", "To Do", "fix the flux capacitor")
	iss.Fields.CustomFields = map[string]json.RawMessage{
		"customfield_1": json.RawMessage(`5`),
		"customfield_2": json.RawMessage(`"Mobile"`),
	}
	fields := []core.CustomField{
		{ID: "customfield_1", Name: "Story Points", Label: "Points", Column: true},
		{ID: "customfield_2", Name: "Customer", Column: true},
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	wide := rowText(iss, 100, widestStatus([]*jira.Issue{iss}), now, fields...)
	if lipgloss.Width(wide) != 100 || !strings.Contains(wide, "Mobile") {
		t.Fatalf("wide custom-field row = %q (width %d)", wide, lipgloss.Width(wide))
	}
	header := columnHeader(100, fields...)
	for _, label := range []string{"points", "customer"} {
		t.Run(label+" heading", func(t *testing.T) {
			if !strings.Contains(strings.ToLower(header), label) {
				t.Errorf("header missing %q: %q", label, header)
			}
		})
	}

	oneColumnWidth := rowFixed + minSummary + customColumnWidth(fields[0]) + 2
	narrow := columnHeader(oneColumnWidth, fields...)
	if !strings.Contains(strings.ToUpper(narrow), "POINTS") || strings.Contains(strings.ToUpper(narrow), "CUSTOMER") {
		t.Fatalf("narrow header should keep only the first configured column: %q", narrow)
	}
	for _, width := range []int{rowFixed + 4, rowFixed - 4} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			row := rowText(iss, width, widestStatus([]*jira.Issue{iss}), now, fields...)
			if got := lipgloss.Width(row); got > width {
				t.Fatalf("narrow row width = %d, budget %d: %q", got, width, row)
			}
		})
	}
}
