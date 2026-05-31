package cmdutil

import (
	"time"

	"github.com/matcra587/jira-cli/internal/jira"
)

// ServiceFactory builds the typed Jira services a command needs from a single
// client. Centralizing construction here keeps every command's service wiring
// identical: a command resolves one client through JiraClientForCommand, then
// reaches every domain service through the factory rather than calling
// jira.NewXService itself.
//
// It is a concrete type, not an interface. There is one implementation, so an
// interface here would be indirection without a second implementer; a command
// that wants a test seam declares a narrow interface of just the accessors it
// uses (for example interface{ Issue() jira.IssueService }) and this concrete
// factory satisfies it structurally.
//
// Accessors are stateless: each builds a fresh service from the bound client.
// The services are thin wrappers over one *jira.Client, so construction is
// free and there is no shared state to cache or guard.
type ServiceFactory struct {
	client *jira.Client
}

// ServicesForClient returns the factory bound to client.
func ServicesForClient(client *jira.Client) *ServiceFactory {
	return &ServiceFactory{client: client}
}

func (f *ServiceFactory) Issue() jira.IssueService {
	return jira.NewIssueService(f.client)
}

func (f *ServiceFactory) Search() jira.SearchService {
	return jira.NewSearchService(f.client)
}

func (f *ServiceFactory) Worklog() jira.WorklogService {
	return jira.NewWorklogService(f.client)
}

// Project carries a schema cache scoped by ttl, so it takes the lifetime the
// caller wants for that cache.
func (f *ServiceFactory) Project(ttl time.Duration) jira.ProjectService {
	return jira.NewProjectService(f.client, ttl)
}

func (f *ServiceFactory) User() jira.UserService {
	return jira.NewUserService(f.client)
}

func (f *ServiceFactory) Board() jira.BoardService {
	return jira.NewBoardService(f.client)
}

func (f *ServiceFactory) Label() jira.LabelService {
	return jira.NewLabelService(f.client)
}

func (f *ServiceFactory) Epic() jira.EpicService {
	return jira.NewEpicService(f.client)
}

func (f *ServiceFactory) Comment() jira.CommentService {
	return jira.NewCommentService(f.client)
}

func (f *ServiceFactory) Attachment() jira.AttachmentService {
	return jira.NewAttachmentService(f.client)
}

func (f *ServiceFactory) Watcher() jira.WatcherService {
	return jira.NewWatcherService(f.client)
}

func (f *ServiceFactory) IssueLink() jira.IssueLinkService {
	return jira.NewIssueLinkService(f.client)
}

func (f *ServiceFactory) IssueLinkType() jira.IssueLinkTypeService {
	return jira.NewIssueLinkTypeService(f.client)
}

// IssueService is a shorthand for ServicesForClient(client).Issue(). It is
// retained only until its callers move to the factory accessor directly, at
// which point this shim and its siblings are removed so there is one way to
// reach a service.
func IssueService(client *jira.Client) jira.IssueService {
	return ServicesForClient(client).Issue()
}

// SearchService is a shorthand for ServicesForClient(client).Search(), retained
// until its callers move to the factory accessor.
func SearchService(client *jira.Client) jira.SearchService {
	return ServicesForClient(client).Search()
}

// WorklogService is a shorthand for ServicesForClient(client).Worklog(),
// retained until its callers move to the factory accessor.
func WorklogService(client *jira.Client) jira.WorklogService {
	return ServicesForClient(client).Worklog()
}
