package auth

import (
	"context"
	"time"

	"github.com/matcra587/jira-cli/internal/jira"
)

// verifyCredential pings GET /rest/api/3/myself with a Jira Cloud credential
// pair (account email + API token) and returns the authenticated user. It
// lets `auth login` confirm a token actually works before reporting success,
// rather than persisting a credential that only fails on first use. The
// request is bounded by timeout (when positive) so verification honors the
// same per-profile timeout as every other client; a non-positive timeout
// leaves the client default in place.
func verifyCredential(ctx context.Context, baseURL, email, token string, timeout time.Duration) (*jira.CurrentUser, error) {
	opts := []jira.Option{jira.WithBaseURL(baseURL), jira.WithBasicAuth(email, token)}
	if timeout > 0 {
		opts = append(opts, jira.WithHTTPTimeout(timeout))
	}
	client, err := jira.NewClientE(opts...)
	if err != nil {
		return nil, err
	}
	user, _, err := jira.NewUserService(client).Myself(ctx)
	if err != nil {
		return nil, err
	}
	return user, nil
}
