package jira

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// GetFieldSchemaForProfile must resolve an issue-type NAME to its id via
// the createmeta issuetypes page before calling the field-metadata
// endpoint: the REST v3 field-metadata path takes an issueTypeId, not a
// name. A name-based lookup that skipped this step would 404 on real
// Cloud.
func TestGetFieldSchemaResolvesIssueTypeNameToID(t *testing.T) {
	var fieldMetaPath string
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/createmeta/JCT/issuetypes"):
			_, _ = w.Write([]byte(`{
				"startAt":0,"maxResults":50,"total":2,
				"issueTypes":[
					{"id":"10001","name":"Bug"},
					{"id":"10002","name":"Task"}
				]
			}`))
		case strings.Contains(r.URL.Path, "/createmeta/JCT/issuetypes/"):
			fieldMetaPath = r.URL.Path
			_, _ = w.Write([]byte(`{
				"startAt":0,"maxResults":50,"total":1,
				"fields":[
					{"fieldId":"summary","name":"Summary","schema":{"type":"string"}}
				]
			}`))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))

	svc := NewProjectService(client, 0)
	schema, _, err := svc.GetFieldSchemaForProfile(context.Background(), "default", "JCT", "Task")
	if err != nil {
		t.Fatalf("GetFieldSchemaForProfile: %v", err)
	}
	if !strings.HasSuffix(fieldMetaPath, "/createmeta/JCT/issuetypes/10002") {
		t.Fatalf("field-metadata call must use the resolved issue-type id 10002, got %q", fieldMetaPath)
	}
	if len(schema.Fields) != 1 || schema.Fields[0].ID != "summary" {
		t.Fatalf("schema fields wrong: %+v", schema.Fields)
	}
}

// An unknown issue-type name has no id to resolve — the lookup fails
// rather than calling the field-metadata endpoint with a bad path.
func TestGetFieldSchemaUnknownIssueTypeNameFails(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/createmeta/JCT/issuetypes") {
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":1,"issueTypes":[{"id":"10001","name":"Bug"}]}`))
			return
		}
		http.Error(w, "should not reach field metadata", http.StatusInternalServerError)
	}))

	svc := NewProjectService(client, 0)
	_, _, err := svc.GetFieldSchemaForProfile(context.Background(), "default", "JCT", "Nonexistent")
	if err == nil {
		t.Fatal("an unknown issue-type name must produce an error, not a bad-path call")
	}
}

// The issuetypes page is itself paginated — an issue type on page 2
// must still resolve.
func TestGetFieldSchemaResolvesIssueTypeOnSecondPage(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/createmeta/JCT/issuetypes"):
			if r.URL.Query().Get("startAt") == "0" {
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"total":2,"issueTypes":[{"id":"10001","name":"Bug"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"total":2,"issueTypes":[{"id":"10002","name":"Task"}]}`))
		case strings.Contains(r.URL.Path, "/createmeta/JCT/issuetypes/10002"):
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":1,"fields":[{"fieldId":"summary","name":"Summary","schema":{"type":"string"}}]}`))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))

	svc := NewProjectService(client, 0)
	if _, _, err := svc.GetFieldSchemaForProfile(context.Background(), "default", "JCT", "Task"); err != nil {
		t.Fatalf("issue type on page 2 must resolve: %v", err)
	}
}

// createmeta field metadata is paginated; GetFieldSchemaForProfile must
// loop every page. A custom field on page 2 must appear in the resolved
// schema, otherwise a valid field would be wrongly rejected.
func TestGetFieldSchemaWalksAllFieldPages(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/createmeta/JCT/issuetypes"):
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"total":1,"issueTypes":[{"id":"10002","name":"Task"}]}`))
		case strings.Contains(r.URL.Path, "/createmeta/JCT/issuetypes/10002"):
			if r.URL.Query().Get("startAt") == "0" {
				_, _ = w.Write([]byte(`{
					"startAt":0,"maxResults":1,"total":2,
					"fields":[{"fieldId":"summary","name":"Summary","schema":{"type":"string"}}]
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"startAt":1,"maxResults":1,"total":2,
				"fields":[{"fieldId":"customfield_60001","name":"Severity","schema":{"type":"option","custom":"com.atlassian.jira.plugin.system.customfieldtypes:select"}}]
			}`))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))

	svc := NewProjectService(client, 0)
	schema, _, err := svc.GetFieldSchemaForProfile(context.Background(), "default", "JCT", "Task")
	if err != nil {
		t.Fatalf("GetFieldSchemaForProfile: %v", err)
	}
	var found *FieldSchema
	for i := range schema.Fields {
		if schema.Fields[i].ID == "customfield_60001" {
			found = &schema.Fields[i]
		}
	}
	if found == nil {
		t.Fatalf("custom field on field-metadata page 2 must be in the schema, got %+v", schema.Fields)
	}
	if found.Custom != "select" {
		t.Fatalf("page-2 custom field token wrong: %q", found.Custom)
	}
}
