package adf

import (
	"regexp"
	"strings"
)

// RenderOptions configure RenderActivatable.
type RenderOptions struct {
	// IsTerminal toggles OSC 8 hyperlink emission. When false, the
	// renderer emits plain text.
	IsTerminal bool
	// BaseURL is the active profile's Jira base URL, used to expand
	// issue keys (KAN-7) into browseable links.
	BaseURL string
}

// RenderActivatable renders an ADF document as terminal text with issue
// keys and bare URLs marked as OSC 8 hyperlinks. When IsTerminal is
// false, the same call returns plain text without escapes.
func RenderActivatable(doc Document, opts RenderOptions) string {
	plain := ToPlain(doc)
	if !opts.IsTerminal {
		return plain
	}
	return activate(plain, opts.BaseURL)
}

// issueKeyPattern matches Jira issue keys: 1+ uppercase letters, dash,
// digits. Conservative enough to avoid false positives like "PRE-2026".
var issueKeyPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9_]+)-(\d+)\b`)

// urlPattern matches bare http/https URLs.
var urlPattern = regexp.MustCompile(`https?://[^\s]+`)

// activate wraps every issue-key and bare URL with an OSC 8 hyperlink
// escape. The OSC 8 sequence is:
//
//	\x1b]8;;<URL>\x1b\\<TEXT>\x1b]8;;\x1b\\
//
// Modern terminals render <TEXT> as a clickable link to <URL>; older
// terminals strip the escapes and show only <TEXT>.
func activate(text, baseURL string) string {
	// URLs first so issue-key matches inside URLs don't double-wrap.
	text = urlPattern.ReplaceAllStringFunc(text, func(m string) string {
		return osc8(m, m)
	})
	// Issue keys (skip those already wrapped in OSC 8).
	text = issueKeyPattern.ReplaceAllStringFunc(text, func(m string) string {
		if strings.HasPrefix(m, "\x1b]8") {
			return m
		}
		base := strings.TrimRight(baseURL, "/")
		if base == "" {
			return m
		}
		return osc8(base+"/browse/"+m, m)
	})
	return text
}

// osc8 returns the OSC 8 hyperlink escape for url displayed as text.
func osc8(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
