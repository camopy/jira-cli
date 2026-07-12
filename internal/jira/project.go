package jira

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProjectService resolves the field schemas the mutation pipeline validates
// against and lists projects for discovery. Schemas come from two Jira
// sources — createmeta for create/clone and editmeta for edit/move — and are
// memoized per profile in a ProjectSchemaCache so a multi-field mutation does
// not refetch the same screen.
type ProjectService interface {
	GetFieldSchema(context.Context, string, string) (*ProjectFieldSchema, *Response, error)
	GetFieldSchemaForProfile(context.Context, string, string, string) (*ProjectFieldSchema, *Response, error)
	// GetEditSchemaForProfile resolves the edit-screen field schema for
	// one issue via GET /rest/api/3/issue/{idOrKey}/editmeta. The
	// edit/move flows validate against this; createmeta covers
	// create/clone. Cached per profile + issue key.
	GetEditSchemaForProfile(context.Context, string, string) (*ProjectFieldSchema, *Response, error)
	List(context.Context, *ListOptions) ([]ProjectSummary, *Response, error)
}

// ProjectSummary is the discovery shape for `jira cache projects` (and shell
// completion). Subset of /rest/api/3/project/search; only the keys agents
// and humans need.
type ProjectSummary struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	ProjectType string `json:"project_type,omitempty"`
	Lead        string `json:"lead,omitempty"`
}

type projectSearchPage struct {
	StartAt    int  `json:"startAt"`
	MaxResults int  `json:"maxResults"`
	Total      int  `json:"total"`
	IsLast     bool `json:"isLast"`
	Values     []struct {
		ID             string `json:"id"`
		Key            string `json:"key"`
		Name           string `json:"name"`
		ProjectTypeKey string `json:"projectTypeKey"`
		Lead           struct {
			DisplayName string `json:"displayName"`
		} `json:"lead"`
	} `json:"values"`
}

type projectService struct {
	client *Client
	cache  *ProjectSchemaCache
}

// NewProjectService constructs a ProjectService with its own schema cache. ttl
// bounds how long a resolved schema is trusted before a refetch; a non-positive
// ttl falls back to the cache's default.
func NewProjectService(client *Client, ttl time.Duration) ProjectService {
	return &projectService{
		client: client,
		cache:  NewProjectSchemaCache(ttl),
	}
}

// GetFieldSchema resolves the create schema under the "default" profile. It is
// the profile-less shorthand for GetFieldSchemaForProfile; callers that manage
// multiple profiles pass the profile explicitly so cache entries stay separated.
func (s *projectService) GetFieldSchema(ctx context.Context, projectKey, issueType string) (*ProjectFieldSchema, *Response, error) {
	return s.GetFieldSchemaForProfile(ctx, "default", projectKey, issueType)
}

// List walks /rest/api/3/project/search until isLast=true and returns the
// merged project list. Suitable for the cache command + completion.
func (s *projectService) List(ctx context.Context, opts *ListOptions) ([]ProjectSummary, *Response, error) {
	page := 50
	startAt := 0
	if opts != nil {
		if opts.MaxResults > 0 {
			page = opts.MaxResults
		}
		if opts.StartAt > 0 {
			startAt = opts.StartAt
		}
	}
	var out []ProjectSummary
	var lastResp *Response
	pages := 0
	for {
		q := url.Values{}
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", strconv.Itoa(page))
		path := withQuery(RESTPath("project", "search"), q)
		req, err := s.client.NewRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, nil, err
		}
		var p projectSearchPage
		resp, err := s.client.Do(req, &p)
		if err != nil {
			return nil, resp, err
		}
		pages++
		for _, v := range p.Values {
			out = append(out, ProjectSummary{
				ID:          v.ID,
				Key:         v.Key,
				Name:        v.Name,
				ProjectType: v.ProjectTypeKey,
				Lead:        v.Lead.DisplayName,
			})
		}
		lastResp = resp
		if p.IsLast {
			break
		}
		if pages >= defaultMaxPages || len(out) >= defaultMaxResults {
			return nil, lastResp, fmt.Errorf("project pagination exceeded default bounds")
		}
		startAt = nextOffset(startAt, len(p.Values), page, p.MaxResults)
	}
	return out, lastResp, nil
}

// createMetaFieldPage is one page of the paginated REST v3
// createmeta/{project}/issuetypes/{issueTypeId} field-metadata endpoint.
type createMetaFieldPage struct {
	StartAt    int `json:"startAt"`
	MaxResults int `json:"maxResults"`
	Total      int `json:"total"`
	Fields     []struct {
		ID       string `json:"id"`
		FieldID  string `json:"fieldId"`
		Key      string `json:"key"`
		Name     string `json:"name"`
		Required bool   `json:"required"`
		Type     string `json:"type"`
		Schema   struct {
			Type   string `json:"type"`
			Custom string `json:"custom"`
		} `json:"schema"`
	} `json:"fields"`
}

// createMetaIssueTypePage is one page of the paginated REST v3
// createmeta/{project}/issuetypes endpoint. It carries both the id and
// the name of every issue type on the project's create screen — the
// field-metadata endpoint is keyed by id, not name.
type createMetaIssueTypePage struct {
	StartAt    int `json:"startAt"`
	MaxResults int `json:"maxResults"`
	Total      int `json:"total"`
	IssueTypes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"issueTypes"`
}

// GetFieldSchemaForProfile resolves the create-screen field schema for a
// project + issue-type pair, serving from the per-profile cache on a hit. The
// createmeta field endpoint is keyed by issue-type id while this repo addresses
// types by name, so the name is resolved to an id first; an unresolvable name
// surfaces as *IssueTypeUnknownError (a validation miss, not a 404). The result
// is paginated in and cached before return.
func (s *projectService) GetFieldSchemaForProfile(ctx context.Context, profile, projectKey, issueType string) (*ProjectFieldSchema, *Response, error) {
	if schema, ok := s.cache.Get(profile, projectKey, issueType); ok {
		return schema, &Response{IsLast: true}, nil
	}
	// The REST v3 field-metadata endpoint is keyed by issueTypeId. This
	// repo addresses issue types by NAME (e.g. "Task"), so the name is
	// resolved to its id via the issuetypes page before the
	// field-metadata call.
	issueTypeID, err := s.resolveIssueTypeID(ctx, projectKey, issueType)
	if err != nil {
		return nil, nil, err
	}
	schema := ProjectFieldSchema{
		ProjectKey: projectKey,
		IssueType:  issueType,
		Fields:     make([]FieldSchema, 0),
	}
	var lastResp *Response
	startAt := 0
	for {
		q := url.Values{}
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", "50")
		path := withQuery(RESTPath("issue", "createmeta", projectKey, "issuetypes", issueTypeID), q)
		req, err := s.client.NewRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, nil, err
		}
		var page createMetaFieldPage
		resp, err := s.client.Do(req, &page)
		if err != nil {
			return nil, resp, err
		}
		lastResp = resp
		for _, field := range page.Fields {
			id := firstNonEmpty(field.ID, field.FieldID, field.Key)
			fieldType := firstNonEmpty(field.Type, field.Schema.Type)
			schema.Fields = append(schema.Fields, FieldSchema{
				ID:       id,
				Name:     field.Name,
				Required: field.Required,
				Type:     fieldType,
				Custom:   customFieldToken(field.Schema.Custom),
			})
		}
		startAt += len(page.Fields)
		if len(page.Fields) == 0 || startAt >= page.Total {
			break
		}
	}
	s.cache.Set(profile, projectKey, issueType, &schema)
	return &schema, lastResp, nil
}

// IssueTypeUnknownError reports a --type value that matches no issue type on
// a project's create screen. The name is resolved against the fetched
// issuetypes list in-code, so a miss is a bad input value, NOT a Jira 404 —
// the CLI maps this to a validation error (exit 3) and surfaces Available as
// the envelope's "did you mean" suggestions. Do not give it a synthetic HTTP
// status: a hand-built 404 would drag a local lookup miss into the not-found
// bucket (see .claude/rules/output.md).
type IssueTypeUnknownError struct {
	IssueType  string
	ProjectKey string
	Available  []string // creatable issue-type names, for suggestions
}

// Error names the unmatched type and project; the valid alternatives ride on
// Available for the envelope's suggestions rather than being crammed into the
// string.
func (e *IssueTypeUnknownError) Error() string {
	return "issue type " + e.IssueType + " not found on the create screen for project " + e.ProjectKey
}

// resolveIssueTypeID walks the paginated createmeta issuetypes endpoint
// and returns the id of the issue type whose name matches issueType. An
// issueType value that already looks like a numeric id is returned
// as-is. An unknown name returns *IssueTypeUnknownError carrying the
// project's valid type names — a validation failure, not a 404.
func (s *projectService) resolveIssueTypeID(ctx context.Context, projectKey, issueType string) (string, error) {
	if isNumericID(issueType) {
		return issueType, nil
	}
	var available []string
	startAt := 0
	for {
		q := url.Values{}
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", "50")
		path := withQuery(RESTPath("issue", "createmeta", projectKey, "issuetypes"), q)
		req, err := s.client.NewRequest(ctx, "GET", path, nil)
		if err != nil {
			return "", err
		}
		var page createMetaIssueTypePage
		if _, err := s.client.Do(req, &page); err != nil {
			return "", err
		}
		for _, it := range page.IssueTypes {
			if it.Name == issueType {
				return it.ID, nil
			}
			available = append(available, it.Name)
		}
		startAt += len(page.IssueTypes)
		if len(page.IssueTypes) == 0 || startAt >= page.Total {
			break
		}
	}
	return "", &IssueTypeUnknownError{IssueType: issueType, ProjectKey: projectKey, Available: available}
}

// isNumericID reports whether s is a non-empty all-digit string — the
// shape of a Jira issue-type id.
func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// customFieldToken reduces a Jira schema.custom value to its trailing
// type token. Atlassian system custom fields report a fully-qualified
// identifier (com.atlassian.jira.plugin.system.customfieldtypes:select);
// the pipeline branches on the bare token (select). Marketplace fields
// use vendor-namespaced identifiers — their trailing token is kept as-is
// and the pipeline treats it as unknown, forwarding the value opaquely.
func customFieldToken(custom string) string {
	if custom == "" {
		return ""
	}
	if i := strings.LastIndexByte(custom, ':'); i >= 0 {
		return custom[i+1:]
	}
	return custom
}

// GetEditSchemaForProfile resolves the edit screen for one issue via
// GET /rest/api/3/issue/{idOrKey}/editmeta. EditMetaBean carries a
// `fields` map (NOT a paginated list) keyed by field id. The result is
// cached per profile under a synthetic "issue type" of the issue key so
// edit/move share the per-profile ProjectSchemaCache without colliding
// with createmeta entries.
func (s *projectService) GetEditSchemaForProfile(ctx context.Context, profile, issueKey string) (*ProjectFieldSchema, *Response, error) {
	const editScope = "@editmeta"
	if schema, ok := s.cache.Get(profile, issueKey, editScope); ok {
		return schema, &Response{IsLast: true}, nil
	}
	req, err := s.client.NewRequest(ctx, "GET", RESTPath("issue", issueKey, "editmeta"), nil)
	if err != nil {
		return nil, nil, err
	}
	var raw struct {
		Fields map[string]struct {
			Name     string `json:"name"`
			Key      string `json:"key"`
			FieldID  string `json:"fieldId"`
			Required bool   `json:"required"`
			Schema   struct {
				Type   string `json:"type"`
				Custom string `json:"custom"`
			} `json:"schema"`
		} `json:"fields"`
	}
	resp, err := s.client.Do(req, &raw)
	if err != nil {
		return nil, resp, err
	}
	schema := ProjectFieldSchema{
		IssueType: issueKey,
		Fields:    make([]FieldSchema, 0, len(raw.Fields)),
	}
	for id, field := range raw.Fields {
		fieldID := firstNonEmpty(field.FieldID, field.Key, id)
		schema.Fields = append(schema.Fields, FieldSchema{
			ID:       fieldID,
			Name:     field.Name,
			Required: field.Required,
			Type:     field.Schema.Type,
			Custom:   customFieldToken(field.Schema.Custom),
		})
	}
	s.cache.Set(profile, issueKey, editScope, &schema)
	return &schema, resp, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ProjectSchemaCache is a TTL cache of resolved field schemas, keyed by
// profile + project + issue-type (or a synthetic edit scope). It is safe for
// concurrent use; Get and Set copy the schema in and out so a cached entry
// cannot be mutated by a caller holding a returned pointer. The edit and create
// schemas coexist here — edit entries use a synthetic issue-type key.
type ProjectSchemaCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]schemaEntry
}

type schemaEntry struct {
	schema    *ProjectFieldSchema
	expiresAt time.Time
}

// NewProjectSchemaCache builds a schema cache with the given entry TTL, falling
// back to 30 minutes when ttl is non-positive.
func NewProjectSchemaCache(ttl time.Duration) *ProjectSchemaCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &ProjectSchemaCache{ttl: ttl, entries: make(map[string]schemaEntry)}
}

// Get returns a cached schema and true when a fresh entry exists. An expired
// entry reports a miss without being evicted (Set overwrites it on the next
// resolve). The returned schema is a copy the caller may freely mutate.
func (c *ProjectSchemaCache) Get(profile, projectKey, issueType string) (*ProjectFieldSchema, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[schemaKey(profile, projectKey, issueType)]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return cloneProjectFieldSchema(entry.schema), true
}

// Set stores a copy of schema under the profile/project/issue-type key with a
// fresh TTL. Copying on store means a later caller mutation of the passed schema
// does not reach the cached entry.
func (c *ProjectSchemaCache) Set(profile, projectKey, issueType string, schema *ProjectFieldSchema) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[schemaKey(profile, projectKey, issueType)] = schemaEntry{
		schema:    cloneProjectFieldSchema(schema),
		expiresAt: time.Now().Add(c.ttl),
	}
}

// InvalidateProfile drops every cached schema for one profile, leaving other
// profiles' entries intact. Used when a profile's config changes in a way that
// could alter its screens.
func (c *ProjectSchemaCache) InvalidateProfile(profile string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := profile + "\x00"
	for key := range c.entries {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.entries, key)
		}
	}
}

// InvalidateAll clears every cached schema across all profiles.
func (c *ProjectSchemaCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]schemaEntry)
}

func schemaKey(profile, projectKey, issueType string) string {
	return profile + "\x00" + projectKey + "\x00" + issueType
}

func cloneProjectFieldSchema(schema *ProjectFieldSchema) *ProjectFieldSchema {
	if schema == nil {
		return nil
	}
	out := *schema
	if schema.Fields != nil {
		out.Fields = append([]FieldSchema(nil), schema.Fields...)
	}
	return &out
}
