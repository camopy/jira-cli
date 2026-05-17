package unit

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestJiraServicesAreExportedInterfaces(t *testing.T) {
	services := map[string]reflect.Kind{
		"IssueService":   reflect.TypeOf((*jira.IssueService)(nil)).Elem().Kind(),
		"EpicService":    reflect.TypeOf((*jira.EpicService)(nil)).Elem().Kind(),
		"SearchService":  reflect.TypeOf((*jira.SearchService)(nil)).Elem().Kind(),
		"WorklogService": reflect.TypeOf((*jira.WorklogService)(nil)).Elem().Kind(),
		"ProjectService": reflect.TypeOf((*jira.ProjectService)(nil)).Elem().Kind(),
	}
	for name, kind := range services {
		if kind != reflect.Interface {
			t.Fatalf("%s kind = %s, want interface", name, kind)
		}
	}
}

func TestCommandLayerDoesNotConstructConcreteJiraServicesDirectly(t *testing.T) {
	content, err := os.ReadFile("../../cmd/jira/commands.go")
	if err != nil {
		t.Fatalf("ReadFile(commands.go) error = %v", err)
	}
	for _, forbidden := range []string{
		"jira.NewIssueService(",
		"jira.NewEpicService(",
		"jira.NewSearchService(",
		"jira.NewWorklogService(",
		"jira.NewProjectService(",
	} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("command layer directly constructs service %s; use injected service factory", forbidden)
		}
	}
}

type fakeIssueService struct{}

func (fakeIssueService) List(context.Context, *jira.IssueListOptions) ([]*jira.Issue, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Get(context.Context, string, *jira.IssueGetOptions) (*jira.Issue, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Create(context.Context, *jira.IssueCreateRequest) (*jira.Issue, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Update(context.Context, string, *jira.IssueUpdateRequest) (*jira.Issue, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Delete(context.Context, string, *jira.IssueDeleteOptions) (*jira.Response, error) {
	return nil, nil
}

func (fakeIssueService) Link(context.Context, *jira.IssueLinkRequest) (*jira.Response, error) {
	return nil, nil
}

func (fakeIssueService) AddRemoteLink(context.Context, string, *jira.RemoteLinkRequest) (*jira.Response, error) {
	return nil, nil
}

func (fakeIssueService) Clone(context.Context, string, *jira.IssueCloneRequest) (*jira.Issue, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Move(context.Context, string, *jira.IssueMoveRequest) (*jira.Issue, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Transitions(context.Context, string) ([]*jira.Transition, *jira.Response, error) {
	return nil, nil, nil
}

func (fakeIssueService) Transition(context.Context, string, *jira.TransitionRequest) (*jira.Response, error) {
	return nil, nil
}

func (fakeIssueService) AddComment(context.Context, string, *jira.CommentAddRequest) (*jira.Comment, *jira.Response, error) {
	return nil, nil, nil
}

var _ jira.IssueService = fakeIssueService{}
