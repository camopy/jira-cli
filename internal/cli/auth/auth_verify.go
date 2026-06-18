package auth

import (
	"context"
	"errors"
	"time"

	"github.com/gechr/clog"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
)

// discoverVerb names the cloud-ID discovery sub-step in the operation-verb
// registry, so the spinner, the debug flow lines, and the human completion
// line all phrase it identically ("discovering"/"discovered cloud ID").
const discoverVerb = "auth.login.discover"

// verifyAndDetectCredential verifies a Jira Cloud credential and auto-detects
// whether it is a classic or scoped (granular) API token. The token string
// carries no type marker — classic and scoped tokens share a prefix — so the
// only reliable signal is behavioral: a classic token authenticates at the
// site host, while a scoped token is rejected there and works only through the
// Atlassian gateway (api.atlassian.com/ex/jira/<cloudId>).
//
// It therefore tries the site first. On success the token is classic and the
// returned cloudId is empty. On an AUTH rejection (401/403) it discovers the
// site's cloudId (unauthenticated tenant_info) and re-verifies against the
// gateway; a success there means the token is scoped and its cloudId is
// returned for the caller to persist. A non-auth failure (network, 5xx) is
// returned verbatim — it is not a "wrong token", so no gateway probe is made.
// When the token is rejected at BOTH the site and the gateway, or the cloudId
// cannot be discovered, the original site error is returned (the actionable
// one). gatewayBaseURL builds the gateway base for a cloudId; production passes
// config.GatewayBaseURL, tests inject a stub pointing at a local server.
func verifyAndDetectCredential(
	ctx context.Context,
	profile config.Profile,
	token string,
	timeout, maxRetryWait time.Duration,
	gatewayBaseURL func(cloudID string) string,
) (*jira.CurrentUser, string, error) {
	// Classic path: the site host.
	user, siteErr := verifyCredential(ctx, profile.BaseURL, profile.Email, token, timeout, maxRetryWait)
	if siteErr == nil {
		clog.Debug().Msg("confirmed classic token")
		return user, "", nil
	}
	// Only an auth rejection might mean a scoped token; a transport/server
	// error is not a token-type question, so surface it unchanged.
	var apiErr *jira.APIError
	if !errors.As(siteErr, &apiErr) || apiErr.Type != jira.ErrorTypeAuth {
		return nil, "", siteErr
	}
	// Scoped path: discover the cloudId and re-verify through the gateway.
	// These structured debug lines narrate WHY the flow leaves the site for the
	// gateway, so under --debug (where the spinner is suppressed) the raw HTTP
	// calls have context instead of appearing out of nowhere. Each line brackets
	// the request that follows it.
	verb := cli.VerbFor(discoverVerb)
	clog.Debug().Int("site_status", apiErr.StatusCode).Msg("site rejected token; " + verb.Gerundf())
	cloudID, discErr := discoverCloudID(ctx, profile.BaseURL, timeout)
	if discErr != nil {
		// Cannot probe the gateway (e.g. tenant_info blocked); the site auth
		// error is the actionable one to report.
		return nil, "", siteErr
	}
	clog.Debug().Str("id", cloudID).Msg(verb.Pastf() + "; verifying at gateway")
	gwUser, gwErr := verifyCredential(ctx, gatewayBaseURL(cloudID), profile.Email, token, timeout, maxRetryWait)
	if gwErr != nil {
		// Rejected at the site AND the gateway: the credential is simply bad.
		return nil, "", siteErr
	}
	clog.Debug().Str("id", cloudID).Msg("confirmed scoped token")
	return gwUser, cloudID, nil
}

// verifyCredential pings GET /rest/api/3/myself with a Jira Cloud credential
// pair (account email + API token) and returns the authenticated user. It
// lets `auth login` confirm a token actually works before reporting success,
// rather than persisting a credential that only fails on first use. The
// request is bounded by timeout (when positive) so verification honors the
// same per-profile timeout as every other client; a non-positive timeout
// leaves the client default in place. maxRetryWait carries the resolved
// rate-limit retry budget so this read rides out a 429 like every other
// command rather than failing login on a transient rate limit.
func verifyCredential(ctx context.Context, baseURL, email, token string, timeout, maxRetryWait time.Duration) (*jira.CurrentUser, error) {
	opts := []jira.Option{
		jira.WithBaseURL(baseURL),
		jira.WithBasicAuth(email, token),
		jira.WithMaxRetryWait(maxRetryWait),
	}
	if timeout > 0 {
		opts = append(opts, jira.WithHTTPTimeout(timeout))
	}
	client, err := jira.NewClientE(opts...)
	if err != nil {
		return nil, err
	}
	user, _, err := cmdutil.ServicesForClient(client).User().Myself(ctx)
	if err != nil {
		return nil, err
	}
	return user, nil
}
