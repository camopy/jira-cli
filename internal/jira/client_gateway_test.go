package jira

import (
	"context"
	"net/http"
	"testing"
)

// A scoped-token profile gives the client a gateway base URL whose path
// carries the /ex/jira/<cloudId> prefix. The client builds its REST paths as
// relative references, so they must resolve UNDER that prefix rather than
// replacing it — otherwise every scoped request would 404 at the gateway root.
func TestClientRoutesRelativePathsUnderGatewayPrefix(t *testing.T) {
	client := NewClient(WithBaseURL("https://api.atlassian.com/ex/jira/cloud-xyz/"))

	req, err := client.NewRequest(context.Background(), http.MethodGet, RESTPath("myself"), nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	want := "https://api.atlassian.com/ex/jira/cloud-xyz/rest/api/3/myself"
	if got := req.URL.String(); got != want {
		t.Fatalf("gateway request URL = %q, want %q", got, want)
	}
}

// The POST /search/jql read endpoint is whitelisted out of the mutation gate by
// its path. That check compares the request path RELATIVE to the base, so it
// must keep working when the base carries a gateway prefix — a scoped read must
// not be misclassified as a mutation (which would break --read-only/--dry-run
// reads).
func TestSearchJQLReadWhitelistHoldsUnderGatewayPrefix(t *testing.T) {
	client := NewClient(WithBaseURL("https://api.atlassian.com/ex/jira/cloud-xyz/"))

	req, err := client.NewRequest(context.Background(), http.MethodPost, RESTPath("search", "jql"), map[string]any{"jql": "order by created"})
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if client.isMutationRequest(req) {
		t.Fatal("POST /search/jql under the gateway prefix was misclassified as a mutation")
	}

	// A genuine mutation under the same prefix must still be caught.
	mut, err := client.NewRequest(context.Background(), http.MethodPost, RESTPath("issue"), map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if !client.isMutationRequest(mut) {
		t.Fatal("POST /issue under the gateway prefix should be treated as a mutation")
	}
}
