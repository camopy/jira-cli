package jira

import (
	"context"
	"net/http"
	"testing"
)

// AutocompleteData GETs /jql/autocompletedata and maps the reference data into
// fields (including custom fields, via cfid), functions, and reserved words.
func TestJQLServiceAutocompleteData(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/rest/api/3/jql/autocompletedata" {
			t.Fatalf("path = %s, want /rest/api/3/jql/autocompletedata", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"visibleFieldNames":[
				{"value":"summary","displayName":"Summary - Summary"},
				{"value":"cf[10010]","displayName":"Story Points - cf[10010]","cfid":"cf[10010]"}
			],
			"visibleFunctionNames":[
				{"value":"currentUser()","displayName":"currentUser()"}
			],
			"jqlReservedWords":["and","or","empty"]
		}`))
	}))

	svc := NewJQLService(client)
	ref, _, err := svc.AutocompleteData(context.Background())
	if err != nil {
		t.Fatalf("AutocompleteData: %v", err)
	}
	if len(ref.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(ref.Fields))
	}
	if ref.Fields[0].Value != "summary" || ref.Fields[0].DisplayName != "Summary - Summary" {
		t.Fatalf("field[0] = %+v", ref.Fields[0])
	}
	if ref.Fields[1].CustomFieldID != "cf[10010]" {
		t.Fatalf("field[1] should carry the cfid, got %+v", ref.Fields[1])
	}
	if len(ref.Functions) != 1 || ref.Functions[0].Value != "currentUser()" {
		t.Fatalf("functions = %+v, want currentUser()", ref.Functions)
	}
	if len(ref.ReservedWords) != 3 {
		t.Fatalf("reserved words = %v, want 3", ref.ReservedWords)
	}
}
