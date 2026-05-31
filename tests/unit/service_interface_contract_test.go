package unit

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
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
	root := "../../internal/cli"
	// services.go is the service factory itself: building the concrete
	// services is its whole purpose, and it is the one seam every command
	// reaches them through. Every other file in the command layer must go
	// through cmdutil.ServicesForClient(client) rather than calling
	// jira.NewXService directly, so construction stays in one place.
	allowedDirectConstruction := map[string]bool{
		"services.go": true,
	}
	forbidden := []string{
		"jira.NewIssueService(",
		"jira.NewSearchService(",
		"jira.NewWorklogService(",
		"jira.NewProjectService(",
		"jira.NewUserService(",
		"jira.NewBoardService(",
		"jira.NewLabelService(",
		"jira.NewEpicService(",
		"jira.NewCommentService(",
		"jira.NewAttachmentService(",
		"jira.NewWatcherService(",
		"jira.NewIssueLinkService(",
		"jira.NewIssueLinkTypeService(",
	}
	scanned := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if allowedDirectConstruction[name] {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		for _, f := range forbidden {
			if strings.Contains(string(content), f) {
				t.Errorf("%s directly constructs a Jira service with %s; reach it through cmdutil.ServicesForClient(client) instead", path, f)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("WalkDir(%s) error = %v", root, walkErr)
	}
	if scanned == 0 {
		t.Fatalf("no command files found under %s — guard is inert", root)
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
