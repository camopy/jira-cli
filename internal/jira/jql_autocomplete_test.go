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
				{"value":"summary","displayName":"Summary - Summary","operators":["~","!~"]},
				{"value":"status","displayName":"Status - Status","operators":["=","!=","in"],"auto":"true"},
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
	if len(ref.Fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(ref.Fields))
	}
	if ref.Fields[0].Value != "summary" || ref.Fields[0].DisplayName != "Summary - Summary" {
		t.Fatalf("field[0] = %+v", ref.Fields[0])
	}
	if len(ref.Fields[0].Operators) != 2 || ref.Fields[0].Operators[0] != "~" {
		t.Fatalf("field[0] operators = %v, want [~ !~]", ref.Fields[0].Operators)
	}
	if !ref.Fields[1].Auto {
		t.Fatalf("status should be value-suggestable (auto), got %+v", ref.Fields[1])
	}
	if ref.Fields[0].Auto {
		t.Fatal("summary must not be value-suggestable")
	}
	if ref.Fields[2].CustomFieldID != "cf[10010]" {
		t.Fatalf("field[2] should carry the cfid, got %+v", ref.Fields[2])
	}
	if len(ref.Functions) != 1 || ref.Functions[0].Value != "currentUser()" {
		t.Fatalf("functions = %+v, want currentUser()", ref.Functions)
	}
	if len(ref.ReservedWords) != 3 {
		t.Fatalf("reserved words = %v, want 3", ref.ReservedWords)
	}
}

// AutocompleteSuggestions GETs the suggestions endpoint with fieldName (and
// fieldValue when narrowing) and maps the results.
func TestJQLServiceAutocompleteSuggestions(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/jql/autocompletedata/suggestions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("fieldName"); got != "status" {
			t.Fatalf("fieldName = %q, want status", got)
		}
		if got := r.URL.Query().Get("fieldValue"); got != "in" {
			t.Fatalf("fieldValue = %q, want in", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"value":"In Progress","displayName":"<b>In</b> Progress"},
			{"value":"In Review","displayName":"<b>In</b> Review"}
		]}`))
	}))

	svc := NewJQLService(client)
	got, _, err := svc.AutocompleteSuggestions(context.Background(), "status", "in")
	if err != nil {
		t.Fatalf("AutocompleteSuggestions: %v", err)
	}
	if len(got) != 2 || got[0].Value != "In Progress" {
		t.Fatalf("suggestions = %+v", got)
	}
}

func TestJQLServiceAutocompleteSuggestionsRequiresField(t *testing.T) {
	svc := NewJQLService(nil)
	if _, _, err := svc.AutocompleteSuggestions(context.Background(), "", "x"); err == nil {
		t.Fatal("empty fieldName must error before any request")
	}
}
