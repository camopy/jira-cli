package issue

import (
	"context"
	"errors"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

type fakeScreenSchemaLookup struct {
	schema *jira.ProjectFieldSchema
	err    error
	calls  int
}

func (f *fakeScreenSchemaLookup) GetFieldSchemaForProfile(_ context.Context, _, _, _ string) (*jira.ProjectFieldSchema, *jira.Response, error) {
	f.calls++
	return f.schema, nil, f.err
}

func (f *fakeScreenSchemaLookup) GetEditSchemaForProfile(_ context.Context, _, _ string) (*jira.ProjectFieldSchema, *jira.Response, error) {
	f.calls++
	return f.schema, nil, f.err
}

// A create-screen fetcher converts a jira field schema into a pipeline
// ScreenSchema: valid-field whitelist plus the custom-type map the
// stage-4 encoder branches on.
func TestScreenSchemaFetcherConvertsFieldSchema(t *testing.T) {
	lookup := &fakeScreenSchemaLookup{schema: &jira.ProjectFieldSchema{
		ProjectKey: "KAN",
		IssueType:  "Story",
		Fields: []jira.FieldSchema{
			{ID: "summary", Type: "string"},
			{ID: "customfield_10001", Type: "number", Custom: "float"},
		},
	}}
	fetch := newScreenSchemaFetcher(context.Background(), lookup, "default", "KAN", "Story")
	got, err := fetch()
	if err != nil {
		t.Fatalf("fetch error = %v", err)
	}
	if !got.ValidFields["summary"] || !got.ValidFields["customfield_10001"] {
		t.Fatalf("valid fields not populated: %+v", got.ValidFields)
	}
	if got.FieldTypes["customfield_10001"] != "float" {
		t.Fatalf("custom type not carried: %+v", got.FieldTypes)
	}
}

// A fetcher built without a project key or issue type cannot identify a
// screen — it reports ErrSchemaUnknown without calling the API.
func TestScreenSchemaFetcherUnknownWhenProjectMissing(t *testing.T) {
	lookup := &fakeScreenSchemaLookup{}
	fetch := newScreenSchemaFetcher(context.Background(), lookup, "default", "", "Story")
	_, err := fetch()
	if !errors.Is(err, pipeline.ErrSchemaUnknown) {
		t.Fatalf("expected ErrSchemaUnknown, got %v", err)
	}
	if lookup.calls != 0 {
		t.Fatalf("fetcher must not call the API without a project key, calls=%d", lookup.calls)
	}
}

// A lookup transport failure is reported as ErrSchemaUnknown so the
// pipeline's strict/best-effort policy governs the outcome.
func TestScreenSchemaFetcherWrapsLookupError(t *testing.T) {
	lookup := &fakeScreenSchemaLookup{err: errors.New("boom")}
	fetch := newScreenSchemaFetcher(context.Background(), lookup, "default", "KAN", "Story")
	_, err := fetch()
	if !errors.Is(err, pipeline.ErrSchemaUnknown) {
		t.Fatalf("transport failure must surface as ErrSchemaUnknown, got %v", err)
	}
}

// An edit-screen fetcher resolves the editmeta schema for an issue.
func TestEditScreenSchemaFetcherConvertsFieldSchema(t *testing.T) {
	lookup := &fakeScreenSchemaLookup{schema: &jira.ProjectFieldSchema{
		IssueType: "PROJ-1",
		Fields: []jira.FieldSchema{
			{ID: "summary", Type: "string"},
			{ID: "customfield_20002", Type: "option", Custom: "select"},
		},
	}}
	fetch := newEditScreenSchemaFetcher(context.Background(), lookup, "default", "PROJ-1")
	got, err := fetch()
	if err != nil {
		t.Fatalf("fetch error = %v", err)
	}
	if !got.ValidFields["customfield_20002"] {
		t.Fatalf("edit-screen field missing: %+v", got.ValidFields)
	}
	if got.FieldTypes["customfield_20002"] != "select" {
		t.Fatalf("edit-screen custom type missing: %+v", got.FieldTypes)
	}
}

// An empty schema (no fields) is treated as unknown so stage 3 is
// skipped rather than rejecting every field.
func TestEditScreenSchemaFetcherEmptySchemaIsUnknown(t *testing.T) {
	lookup := &fakeScreenSchemaLookup{schema: &jira.ProjectFieldSchema{IssueType: "PROJ-1"}}
	fetch := newEditScreenSchemaFetcher(context.Background(), lookup, "default", "PROJ-1")
	_, err := fetch()
	if !errors.Is(err, pipeline.ErrSchemaUnknown) {
		t.Fatalf("empty schema must be ErrSchemaUnknown, got %v", err)
	}
}
