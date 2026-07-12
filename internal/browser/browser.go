// Package browser builds Jira web URLs and opens them in the OS default
// browser. URL construction is pure and testable; Open is thin platform glue
// shared by the CLI commands that grew a --web affordance.
package browser

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	xstrings "github.com/gechr/x/strings"
)

// IssueURL returns the /browse/<key> URL for an issue, or "" when either the
// base URL or the key is empty.
func IssueURL(baseURL, key string) string {
	baseURL = trimBase(baseURL)
	key = strings.TrimSpace(key)
	if xstrings.AnyEmpty(baseURL, key) {
		return ""
	}
	return baseURL + "/browse/" + url.PathEscape(key)
}

// SearchURL returns the /issues/?jql=<query> URL for a JQL search, or "" when
// either the base URL or the query is empty.
func SearchURL(baseURL, jqlQuery string) string {
	baseURL = trimBase(baseURL)
	jqlQuery = strings.TrimSpace(jqlQuery)
	if xstrings.AnyEmpty(baseURL, jqlQuery) {
		return ""
	}
	return baseURL + "/issues/?jql=" + url.QueryEscape(jqlQuery)
}

func trimBase(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// Open launches url in the OS default browser, giving the launcher up to 10s.
// The URL must be built by the IssueURL/SearchURL helpers (callers never pass
// unvalidated input straight through).
func Open(ctx context.Context, rawURL string) error {
	if xstrings.IsBlank(rawURL) {
		return fmt.Errorf("browser: empty URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", rawURL) //nolint:gosec // URL built by IssueURL/SearchURL
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", rawURL) //nolint:gosec // URL built by IssueURL/SearchURL
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", rawURL) //nolint:gosec // URL built by IssueURL/SearchURL
	}
	return cmd.Run()
}
