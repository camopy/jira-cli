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
	dir := "../../cmd/jira"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	// Two files are allowed to construct services directly:
	//
	//   services.go — the injected service factory itself. Building the
	//   concrete services is its whole purpose; it is the factory every
	//   other file is told to use.
	//
	//   cache.go — warms the local cache by constructing services
	//   directly. That predates this guard (which historically read only
	//   commands.go) and is a known, pre-existing gap the command file
	//   split neither introduced nor widened.
	//
	// Every other file in the command layer must go through the factory.
	allowedDirectConstruction := map[string]bool{
		"services.go": true,
		"cache.go":    true,
	}
	forbidden := []string{
		"jira.NewIssueService(",
		"jira.NewEpicService(",
		"jira.NewSearchService(",
		"jira.NewWorklogService(",
		"jira.NewProjectService(",
	}
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if allowedDirectConstruction[name] {
			continue
		}
		scanned++
		path := dir + "/" + name
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		for _, f := range forbidden {
			if strings.Contains(string(content), f) {
				t.Fatalf("%s directly constructs service %s; use injected service factory", path, f)
			}
		}
	}
	if scanned == 0 {
		t.Fatalf("no command files found under %s — guard is inert", dir)
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
