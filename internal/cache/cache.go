// Package cache is a tiny per-profile JSON file store for Jira metadata
// (labels, epics, projects, …) that's cheap to look up — used by the
// `jira cache <resource>` commands and (eventually) by Cobra shell
// completion functions.
//
// The store is intentionally dumb: one JSON file per resource, atomic
// write, read-time freshness check. No locking — concurrent writers
// would race, but in practice each profile is driven by one user shell.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gechr/x/shell"
)

// DefaultTTL is the freshness window before `Read` reports stale=true.
// Callers can still use the cached value; this just signals "consider
// refreshing" to commands and completion functions.
const DefaultTTL = 1 * time.Hour

// Entry wraps a cached value with its fetch timestamp + source profile.
// Stored verbatim on disk so consumers can introspect age and provenance.
type Entry struct {
	Profile   string          `json:"profile"`
	Resource  string          `json:"resource"`
	FetchedAt time.Time       `json:"fetched_at"`
	Data      json.RawMessage `json:"data"`
}

// Key returns the stable namespace component for one config/site/profile
// identity. The human profile name stays visible in command output; this key
// is only for cache storage so profiles with the same name in different Jira
// sites or config files do not share metadata.
func Key(profile, siteURL, configPath string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "default"
	}
	siteURL = strings.TrimRight(strings.ToLower(strings.TrimSpace(siteURL)), "/")
	configPath = strings.TrimSpace(configPath)
	if configPath != "" {
		if abs, err := filepath.Abs(configPath); err == nil {
			configPath = abs
		}
	}
	sum := sha256.Sum256([]byte(configPath + "\x00" + siteURL + "\x00" + profile))
	return profile + "-" + hex.EncodeToString(sum[:])[:16]
}

// Path returns the on-disk location for a (profile, resource) pair. The
// directory is created lazily; callers should treat the returned path
// purely as input to Read/Write.
func Path(profile, resource string) (string, error) {
	root := dirRoot()
	return filepath.Join(root, sanitize(profile), sanitize(resource)+".json"), nil
}

// Read returns the cached entry. ok=false when no cache file exists.
// stale=true when the entry is older than ttl (or DefaultTTL when ttl<=0)
// — the value is still returned so callers can use stale data as a
// fallback while triggering a refresh.
func Read(profile, resource string, ttl time.Duration) (entry Entry, ok, stale bool, err error) {
	path, err := Path(profile, resource)
	if err != nil {
		return Entry{}, false, false, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Entry{}, false, false, nil
		}
		return Entry{}, false, false, fmt.Errorf("read cache %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &entry); err != nil {
		return Entry{}, false, false, fmt.Errorf("decode cache %s: %w", path, err)
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	stale = time.Since(entry.FetchedAt) > ttl
	return entry, true, stale, nil
}

// Write atomically stores `data` (already JSON-encoded) under the (profile,
// resource) key. Uses a temp-file-then-rename to avoid leaving a half-written
// file when the process is killed mid-write.
func Write(profile, resource string, data json.RawMessage) (Entry, error) {
	path, err := Path(profile, resource)
	if err != nil {
		return Entry{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Entry{}, err
	}
	entry := Entry{
		Profile:   profile,
		Resource:  resource,
		FetchedAt: time.Now().UTC(),
		Data:      data,
	}
	body, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return Entry{}, err
	}
	defer func() {
		// Best-effort cleanup if rename below failed.
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return Entry{}, err
	}
	if err := tmp.Close(); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Clear removes the cache file for (profile, resource). Returns ok=true
// when a file existed and was removed; ok=false silently when none.
func Clear(profile, resource string) (bool, error) {
	path, err := Path(profile, resource)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ClearProfile wipes every cache file under a profile (the parent directory).
// Returns the number of files removed.
func ClearProfile(profile string) (int, error) {
	dir := filepath.Join(dirRoot(), sanitize(profile))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func dirRoot() string {
	root, err := shell.XDGCacheHome()
	if err != nil || root == "" {
		root = ".cache"
	}
	return filepath.Join(root, "jira-cli")
}

// sanitize turns a profile/resource name into a filesystem-safe component.
// Doesn't try to be clever — just replaces path separators and the rare
// character that would let a user-supplied name escape the cache root.
func sanitize(name string) string {
	if name == "" {
		return "_"
	}
	r := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	return r.Replace(name)
}
