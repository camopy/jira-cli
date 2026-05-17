package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/jira"
)

// GetEditSchemaForProfile resolves the edit screen of one issue via the
// editmeta endpoint and reduces each field's schema.custom identifier to
// its bare type token.
func TestGetEditSchemaForProfileParsesEditmeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-1/editmeta" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fields":{
			"summary":{"name":"Summary","fieldId":"summary","required":true,"schema":{"type":"string"}},
			"customfield_10001":{"name":"Points","fieldId":"customfield_10001","required":false,"schema":{"type":"number","custom":"com.atlassian.jira.plugin.system.customfieldtypes:float"}}
		}}`))
	}))
	defer srv.Close()

	svc := jira.NewProjectService(jira.NewClient(jira.WithBaseURL(srv.URL+"/")), time.Minute)
	schema, _, err := svc.GetEditSchemaForProfile(context.Background(), "default", "PROJ-1")
	if err != nil {
		t.Fatalf("GetEditSchemaForProfile error = %v", err)
	}
	byID := map[string]jira.FieldSchema{}
	for _, f := range schema.Fields {
		byID[f.ID] = f
	}
	if _, ok := byID["summary"]; !ok {
		t.Fatalf("summary field missing: %+v", schema.Fields)
	}
	cf, ok := byID["customfield_10001"]
	if !ok {
		t.Fatalf("customfield_10001 missing: %+v", schema.Fields)
	}
	if cf.Custom != "float" {
		t.Fatalf("schema.custom not reduced to bare token: %q", cf.Custom)
	}
}

// The edit schema is cached per profile so a second lookup makes no
// second HTTP call.
func TestGetEditSchemaForProfileCachesPerProfile(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"fields":{"summary":{"name":"Summary","fieldId":"summary","schema":{"type":"string"}}}}`))
	}))
	defer srv.Close()

	svc := jira.NewProjectService(jira.NewClient(jira.WithBaseURL(srv.URL+"/")), time.Minute)
	for range 2 {
		if _, _, err := svc.GetEditSchemaForProfile(context.Background(), "default", "PROJ-1"); err != nil {
			t.Fatalf("GetEditSchemaForProfile error = %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("expected 1 HTTP call (second served from cache), got %d", calls)
	}
}
