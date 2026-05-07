package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gechr/x/ansi"
)

func TestWriteWatcherListPlainHeaderAndRows(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"is_watching": true,
		"watch_count": float64(3),
		"watchers": []map[string]any{
			{"display_name": "Alice Smith", "account_id": "712020:abcd1234efgh5678", "email_address": "alice@example.com", "active": true},
			{"display_name": "Bob Jones", "account_id": "shortid", "email_address": "", "active": true},
			{"display_name": "Carol Lee", "account_id": "712020:carolXXXX", "email_address": "carol@example.com", "active": false},
		},
	}

	var buf bytes.Buffer
	if err := WriteWatcherListPlain(&buf, "issue.watchers.list", data, WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteWatcherListPlain() error = %v", err)
	}
	got := ansi.Strip(buf.String())

	wants := []string{
		"Watchers",
		"3 watchers",
		"(you are watching)",
		"Alice Smith",
		// truncated to last 8 runes with leading ellipsis
		"…efgh5678",
		"alice@example.com",
		"Bob Jones",
		// short id (≤ 8 chars) is preserved verbatim — no leading ellipsis
		"shortid",
		"(hidden)",
		"Carol Lee",
		"(inactive)",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("renderer output missing %q:\n%s", want, got)
		}
	}
}

func TestWriteWatcherListPlainEmptyShowsAffordance(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"is_watching": false,
		"watch_count": float64(0),
		"watchers":    []map[string]any{},
	}

	var buf bytes.Buffer
	if err := WriteWatcherListPlain(&buf, "issue.watchers.list", data, WithPlainTTY(false)); err != nil {
		t.Fatalf("WriteWatcherListPlain() error = %v", err)
	}
	got := ansi.Strip(buf.String())
	if !strings.Contains(got, "0 watchers") {
		t.Fatalf("empty list missing 0-count header: %s", got)
	}
	if !strings.Contains(got, "(no watchers visible)") {
		t.Fatalf("empty list missing affordance: %s", got)
	}
	if strings.Contains(got, "you are watching") {
		t.Fatalf("empty list should not advertise self-watch: %s", got)
	}
}

func TestTruncateAccountIDPreservesShortIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"", "(no id)"},
		{"abc", "abc"},
		{"abcdefgh", "abcdefgh"},                 // exactly 8 → preserved
		{"712020:abcdef", "…0:abcdef"},           // tail-of-8 with ellipsis
		{"712020:abcd1234efgh5678", "…efgh5678"}, // long ID → trailing 8
	}
	for _, tc := range cases {
		if got := truncateAccountID(tc.in); got != tc.want {
			t.Errorf("truncateAccountID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
