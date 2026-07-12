package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	xhttp "github.com/gechr/x/http"
	xstrings "github.com/gechr/x/strings"
)

// tenantInfoPath is Atlassian's cloudId probe. It is not part of the
// documented REST API, but it is the method Atlassian's own support docs point
// users to, it is unauthenticated, and it returns {"cloudId":"..."} for a
// Cloud site. Token auto-detection uses it: when a token is rejected at the
// site, the discovered cloudId lets login re-verify against the gateway to
// decide whether the token is scoped.
const tenantInfoPath = "/_edge/tenant_info"

// maxTenantInfoBody caps the discovery response read. The real body is a few
// hundred bytes; the cap stops a misbehaving endpoint from streaming forever.
const maxTenantInfoBody = 64 << 10

// discoverCloudID fetches the Atlassian cloudId for a site by GETting
// <site>/_edge/tenant_info. Scoped (granular) API tokens must address Jira via
// https://api.atlassian.com/ex/jira/<cloudId>/..., so auto-detection needs the
// cloudId to re-verify a site-rejected token against the gateway. The probe is
// unauthenticated. Bounded by timeout when positive.
func discoverCloudID(ctx context.Context, siteBaseURL string, timeout time.Duration) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(siteBaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("a site base URL is required to discover the cloudId")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+tenantInfoPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// TrimSpace guards the unregistered-code case, where the reason phrase
		// is empty and xhttp.Status would leave a trailing space.
		return "", fmt.Errorf("%s returned HTTP %s", base+tenantInfoPath, strings.TrimSpace(xhttp.Status(resp.StatusCode)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTenantInfoBody))
	if err != nil {
		return "", err
	}
	var body struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", fmt.Errorf("could not parse %s response: %w", tenantInfoPath, err)
	}
	if xstrings.IsBlank(body.CloudID) {
		return "", fmt.Errorf("%s response did not include a cloudId", tenantInfoPath)
	}
	return strings.TrimSpace(body.CloudID), nil
}
