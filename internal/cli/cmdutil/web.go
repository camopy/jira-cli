package cmdutil

import (
	"maps"

	"github.com/matcra587/jira-cli/internal/browser"
	"github.com/spf13/cobra"
)

// WriteWebEnvelope opens url in the OS default browser when the session is
// interactive, then writes an envelope carrying the url and whether it was
// opened. The url is always reported, so an agent or piped invocation gets the
// link without a browser spawning, and a launch failure is surfaced as a
// warning rather than an error — the url is still usable by hand.
//
// extra may be nil; it is not mutated. The "url" and "opened" keys are added to
// a fresh copy.
func WriteWebEnvelope(cmd *cobra.Command, command, url string, extra any) error {
	// extra is a typed Output struct (or legacy map); view it as a map so
	// the url/opened fields fold in beside its fields.
	base, _ := dataAsMap(extra)
	data := make(map[string]any, len(base)+2)
	maps.Copy(data, base)
	data["url"] = url
	opened, err := openInBrowser(cmd, url)
	data["opened"] = opened
	if err != nil {
		return WriteEnvelopeWithRawWarnings(cmd, command, data, []map[string]any{
			{"type": "browser_open_failed", "message": err.Error()},
		})
	}
	return WriteEnvelope(cmd, command, data)
}

// openInBrowser launches url only when the session is interactive — not an
// agent harness and not driven by a piped/redirected stdin. The probe is stdin,
// not stdout: `jira open KEY > out.json` redirects stdout yet is still a human
// who wants the browser, so it must launch. Headless contexts return
// (false, nil) so the URL is reported without a pointless browser spawn.
func openInBrowser(cmd *cobra.Command, url string) (bool, error) {
	det := DetectorFromContext(cmd)
	if det.Agent || det.StdinPiped {
		return false, nil
	}
	if err := browser.Open(cmd.Context(), url); err != nil {
		return false, err
	}
	return true, nil
}
