package cache

import (
	"encoding/json"
	"strings"
)

// IssueKeysResource names the per-profile MRU list of recently used issue
// keys. Unlike the fetched metadata resources it is never primed from Jira:
// commands write it as a side effect of touching keys, and shell completion
// reads it, so freshness windows do not apply — the newest entry is by
// definition current.
const IssueKeysResource = "issuekeys"

// issueKeysCap bounds the MRU list. Fifty keys covers several screens of
// recent work without letting one big list command evict everything the
// user actually typed.
const issueKeysCap = 50

// RecordIssueKeys merges keys into the profile's most-recent-first list:
// incoming keys (already normalized by the caller's parse) move to or enter
// the front in the order given, duplicates collapse onto their newest
// position, and the tail truncates at the cap. Recording is a side effect
// of real work — callers ignore the error by design, so a broken cache can
// never fail a command.
func RecordIssueKeys(profile string, keys []string) error {
	incoming := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys)+issueKeysCap)
	for _, key := range keys {
		key = strings.ToUpper(strings.TrimSpace(key))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		incoming = append(incoming, key)
	}
	if len(incoming) == 0 {
		return nil
	}
	merged := incoming
	for _, key := range IssueKeys(profile) {
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, key)
	}
	if len(merged) > issueKeysCap {
		merged = merged[:issueKeysCap]
	}
	body, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	_, err = Write(profile, IssueKeysResource, body)
	return err
}

// IssueKeys returns the profile's recently used issue keys, newest first.
// Any miss — no cache, unreadable file, wrong shape — returns nil so
// completion and rendering degrade to nothing rather than erroring.
func IssueKeys(profile string) []string {
	entry, ok, err := ReadCachedOrEmpty(profile, IssueKeysResource)
	if !ok || err != nil {
		return nil
	}
	var keys []string
	if json.Unmarshal(entry.Data, &keys) != nil {
		return nil
	}
	return keys
}
