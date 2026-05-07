package integration

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/matcra587/jira-cli/internal/tui"
	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestTUIActionsSubmitThroughMutationService(t *testing.T) {
	service := &recordingMutationService{}
	model := tui.NewWithOptions(t.Context(), tui.Options{
		IssueProvider: tui.IssueProviderFunc(func(context.Context) ([]*jira.Issue, error) {
			return []*jira.Issue{{
				Key: jira.String("PROJ-1"),
				Fields: &jira.IssueFields{
					Summary: jira.String("Original summary"),
					Status:  &jira.Status{Name: jira.String("To Do")},
				},
			}}, nil
		}),
		MutationService: service,
	})

	submitAndApply := func(msg tea.Msg) tui.App {
		t.Helper()
		updated, cmd := model.Update(msg)
		model = updated.(tui.App)
		if cmd == nil {
			t.Fatalf("submit message %T did not return a command", msg)
		}
		updated, _ = model.Update(cmd())
		model = updated.(tui.App)
		return model
	}

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'e'})
	model = updated.(tui.App)
	submitAndApply(tui.SubmitEditMsg{Fields: map[string]any{"summary": "Edited summary"}})
	if service.updatedKey != "PROJ-1" || service.updatedFields["summary"] != "Edited summary" {
		t.Fatalf("edit submission did not call mutation service: %+v", service)
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: 'n'})
	model = updated.(tui.App)
	submitAndApply(tui.SubmitCreateMsg{Request: &jira.IssueCreateRequest{Project: "PROJ", IssueType: "Task", Summary: "Created issue"}})
	if service.createdSummary != "Created issue" {
		t.Fatalf("create submission did not call mutation service: %+v", service)
	}

	submitAndApply(tui.SubmitTransitionMsg{IssueKey: "PROJ-1", TransitionID: "31"})
	if service.transitionKey != "PROJ-1" || service.transitionID != "31" {
		t.Fatalf("transition submission did not call mutation service: %+v", service)
	}

	doc, err := adf.FromMarkdown("hello")
	if err != nil {
		t.Fatalf("FromMarkdown() error = %v", err)
	}
	submitAndApply(tui.SubmitCommentMsg{IssueKey: "PROJ-1", Body: doc})
	if service.commentKey != "PROJ-1" {
		t.Fatalf("comment submission did not call mutation service: %+v", service)
	}

	submitAndApply(tui.SubmitWorklogMsg{IssueKey: "PROJ-1", TimeSpentSeconds: 900})
	if service.worklogKey != "PROJ-1" || service.worklogSeconds != 900 {
		t.Fatalf("worklog submission did not call mutation service: %+v", service)
	}

	submitAndApply(tui.SubmitCloneMsg{IssueKey: "PROJ-1", Request: &jira.IssueCloneRequest{Fields: map[string]any{"summary": "Clone"}}})
	if service.cloneKey != "PROJ-1" {
		t.Fatalf("clone submission did not call mutation service: %+v", service)
	}

	submitAndApply(tui.SubmitMoveMsg{IssueKey: "PROJ-1", Request: &jira.IssueMoveRequest{Fields: map[string]any{"project": map[string]string{"key": "NEXT"}}}})
	if service.moveKey != "PROJ-1" {
		t.Fatalf("move submission did not call mutation service: %+v", service)
	}

	submitAndApply(tui.SubmitDeleteMsg{IssueKey: "PROJ-1", Confirm: true})
	if service.deleteKey != "PROJ-1" {
		t.Fatalf("delete submission did not call mutation service: %+v", service)
	}

	if rendered := model.View().Content; !strings.Contains(rendered, "submitted delete PROJ-1") {
		t.Fatalf("mutation completion was not rendered:\n%s", rendered)
	}
}

type recordingMutationService struct {
	updatedKey     string
	updatedFields  map[string]any
	createdSummary string
	transitionKey  string
	transitionID   string
	commentKey     string
	worklogKey     string
	worklogSeconds int
	cloneKey       string
	moveKey        string
	deleteKey      string
}

func (s *recordingMutationService) UpdateIssue(_ context.Context, key string, fields map[string]any) (*jira.Issue, error) {
	s.updatedKey = key
	s.updatedFields = fields
	return &jira.Issue{Key: jira.String(key), Fields: &jira.IssueFields{Summary: jira.String("Edited summary")}}, nil
}

func (s *recordingMutationService) CreateIssue(_ context.Context, req *jira.IssueCreateRequest) (*jira.Issue, error) {
	s.createdSummary = req.Summary
	return &jira.Issue{Key: jira.String("PROJ-2"), Fields: &jira.IssueFields{Summary: jira.String(req.Summary)}}, nil
}

func (s *recordingMutationService) TransitionIssue(_ context.Context, key string, req *jira.TransitionRequest) error {
	s.transitionKey = key
	s.transitionID = req.ID
	return nil
}

func (s *recordingMutationService) AddComment(_ context.Context, key string, _ *jira.CommentAddRequest) (*jira.Comment, error) {
	s.commentKey = key
	return &jira.Comment{ID: jira.String("10001")}, nil
}

func (s *recordingMutationService) AddWorklog(_ context.Context, key string, req *jira.WorklogAddRequest) (*jira.Worklog, error) {
	s.worklogKey = key
	s.worklogSeconds = req.TimeSpentSeconds
	return &jira.Worklog{ID: jira.String("20001"), TimeSpentSeconds: jira.Int(req.TimeSpentSeconds)}, nil
}

func (s *recordingMutationService) CloneIssue(_ context.Context, key string, _ *jira.IssueCloneRequest) (*jira.Issue, error) {
	s.cloneKey = key
	return &jira.Issue{Key: jira.String("PROJ-3")}, nil
}

func (s *recordingMutationService) MoveIssue(_ context.Context, key string, _ *jira.IssueMoveRequest) (*jira.Issue, error) {
	s.moveKey = key
	return &jira.Issue{Key: jira.String(key)}, nil
}

func (s *recordingMutationService) DeleteIssue(_ context.Context, key string) error {
	s.deleteKey = key
	return nil
}
