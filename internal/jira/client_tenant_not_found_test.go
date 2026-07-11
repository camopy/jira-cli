package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/errtax"
)

// Atlassian answers a nonexistent tenant with a 404 whose body says "Site
// temporarily unavailable" — wording that reads as an outage, not a base_url
// typo. The Atl-Missing-Tcs header is the structured tenant-not-found signal,
// so the transport must branch on it: mark the error, refine the code to
// jira_site_not_found, and replace the misleading display message while
// keeping the upstream text in ErrorMessages. An ordinary in-site 404 must be
// untouched.
func TestAPIErrorDetectsTenantNotFound(t *testing.T) {
	tests := map[string]struct {
		headers        map[string]string
		wantTenantMiss bool
		wantCode       errtax.Code
	}{
		"404 with Atl-Missing-Tcs is site-not-found": {
			headers:        map[string]string{"Atl-Missing-Tcs": "true"},
			wantTenantMiss: true,
			wantCode:       errtax.CodeJiraSiteNotFound,
		},
		"plain 404 stays resource-not-found": {
			headers:        nil,
			wantTenantMiss: false,
			wantCode:       errtax.CodeJiraNotFound,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errorCode":"OTHER","errorMessage":"Site temporarily unavailable"}`))
			}))
			req, err := client.NewRequest(context.Background(), http.MethodGet, "/rest/api/3/myself", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			_, err = client.Do(req, nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *APIError, got %T: %v", err, err)
			}
			if apiErr.TenantNotFound != tc.wantTenantMiss {
				t.Fatalf("TenantNotFound = %v, want %v", apiErr.TenantNotFound, tc.wantTenantMiss)
			}
			if apiErr.Code() != tc.wantCode {
				t.Fatalf("Code() = %q, want %q", apiErr.Code(), tc.wantCode)
			}
			if tc.wantTenantMiss {
				if !strings.Contains(apiErr.Message, "no Atlassian site exists at") {
					t.Fatalf("message should name the missing site, got %q", apiErr.Message)
				}
				if strings.Contains(apiErr.Message, "temporarily unavailable") {
					t.Fatalf("message kept Atlassian's misleading outage wording: %q", apiErr.Message)
				}
			}
		})
	}
}
