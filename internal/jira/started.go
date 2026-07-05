package jira

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gechr/x/human"
)

// startedLayout is the exact form Jira's worklog API requires for `started`:
// milliseconds plus a no-colon UTC offset (yyyy-MM-dd'T'HH:mm:ss.SSSZ). Jira
// rejects anything looser, so every accepted input normalizes to this.
const startedLayout = "2006-01-02T15:04:05.000-0700"

// startedOffsetLayouts are the offset-carrying ISO-8601 forms; the value's own
// offset is kept. The no-colon variants match what Jira itself emits.
var startedOffsetLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.000-0700",
	"2006-01-02T15:04:05-0700",
}

// startedNaiveLayouts carry no offset and are interpreted in the caller's
// location. A bare date starts at midnight.
var startedNaiveLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// ParseStarted parses a worklog start timestamp and normalizes it to the
// strict form Jira requires. Accepted forms: ISO-8601 with an explicit offset
// (kept as given), a naive date/time without one (interpreted in loc), and the
// relative keywords `now`, `yesterday`, and `<duration> ago` (e.g. `2h ago`),
// all resolved against now. Anything else is an error, so an unparseable
// value fails local validation instead of a Jira submit.
func ParseStarted(input string, now time.Time, loc *time.Location) (string, error) {
	value := strings.TrimSpace(input)
	switch strings.ToLower(value) {
	case "":
		return "", errors.New("started timestamp is empty")
	case "now":
		return now.In(loc).Format(startedLayout), nil
	case "yesterday":
		return now.In(loc).AddDate(0, 0, -1).Format(startedLayout), nil
	}
	if rel, ok := strings.CutSuffix(strings.ToLower(value), " ago"); ok {
		d, err := human.ParseDuration(strings.TrimSpace(rel))
		if err != nil {
			return "", fmt.Errorf("relative started %q: %w", input, err)
		}
		if d < 0 {
			return "", fmt.Errorf("relative started %q: duration must be positive", input)
		}
		return now.In(loc).Add(-d).Format(startedLayout), nil
	}
	for _, layout := range startedOffsetLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Format(startedLayout), nil
		}
	}
	for _, layout := range startedNaiveLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.Format(startedLayout), nil
		}
	}
	return "", fmt.Errorf("started %q must be ISO-8601 (e.g. 2026-06-26T10:00:00, offset optional) or relative (now, yesterday, 2h ago)", input)
}
