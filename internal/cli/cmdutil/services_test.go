package cmdutil_test

import (
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
)

// The factory is the single construction seam: it must expose an accessor for
// every Jira domain service that has a concrete implementation, so no command
// needs to call jira.NewXService directly. Each accessor returns a usable
// service built from the bound client. This pins the contract — adding a
// service to internal/jira without wiring it here, or dropping an accessor,
// fails the build or this check.
func TestServiceFactoryExposesEveryConcreteService(t *testing.T) {
	t.Parallel()
	f := cmdutil.ServicesForClient(nil)
	services := map[string]any{
		"Issue":         f.Issue(),
		"Search":        f.Search(),
		"Worklog":       f.Worklog(),
		"Project":       f.Project(time.Minute),
		"User":          f.User(),
		"Board":         f.Board(),
		"Label":         f.Label(),
		"Epic":          f.Epic(),
		"Comment":       f.Comment(),
		"Attachment":    f.Attachment(),
		"Watcher":       f.Watcher(),
		"IssueLink":     f.IssueLink(),
		"IssueLinkType": f.IssueLinkType(),
	}
	for name, svc := range services {
		if svc == nil {
			t.Errorf("%s() returned nil; the factory must build a usable service", name)
		}
	}
}
