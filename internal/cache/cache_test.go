package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadRejectsMismatchedSchemaVersion(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const profile, resource = "p-test", "labels"

	// A round-tripped write is stamped with the current schema and reads back.
	if _, err := Write(profile, resource, json.RawMessage(`["a","b"]`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, ok, _, err := Read(profile, resource, time.Hour); err != nil || !ok {
		t.Fatalf("fresh write should read back: ok=%v err=%v", ok, err)
	}

	// An entry from a future cache shape is discarded as absent so the
	// caller refetches instead of mis-parsing it.
	path, err := Path(profile, resource)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	future := Entry{
		Profile:   profile,
		Resource:  resource,
		Schema:    SchemaVersion + 1,
		FetchedAt: time.Now().UTC(),
		Data:      json.RawMessage(`["a"]`),
	}
	body, _ := json.Marshal(future)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write future entry: %v", err)
	}
	if _, ok, _, err := Read(profile, resource, time.Hour); err != nil || ok {
		t.Fatalf("future-schema entry should read as absent: ok=%v err=%v", ok, err)
	}
}

func TestReadGrandfathersLegacyUnversionedEntry(t *testing.T) {
	if SchemaVersion != 0 {
		t.Skip("grandfathering applies only while SchemaVersion is 0; bumping it intentionally wipes legacy caches")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const profile, resource = "p-test", "labels"
	path, err := Path(profile, resource)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A pre-schema entry has no "schema" field, so it decodes to Schema=0 ==
	// SchemaVersion and stays valid — no upgrade wipes a usable cache.
	legacy := `{"profile":"p-test","resource":"labels","fetched_at":"` +
		time.Now().UTC().Format(time.RFC3339) + `","data":["a","b"]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy entry: %v", err)
	}
	entry, ok, _, err := Read(profile, resource, time.Hour)
	if err != nil || !ok {
		t.Fatalf("legacy entry should read as valid: ok=%v err=%v", ok, err)
	}
	if string(entry.Data) != `["a","b"]` {
		t.Fatalf("legacy data mismatch: %s", entry.Data)
	}
}

func TestReadCachedOrEmptyIgnoresAge(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const profile, resource = "p-test", "boards"
	path, err := Path(profile, resource)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// An entry far past any resource TTL must still be served: NeverBlock
	// readers serve cached-or-empty and never refetch on age.
	old := Entry{
		Profile:   profile,
		Resource:  resource,
		Schema:    SchemaVersion,
		FetchedAt: time.Now().Add(-100 * 24 * time.Hour).UTC(),
		Data:      json.RawMessage(`["x"]`),
	}
	body, _ := json.Marshal(old)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write old entry: %v", err)
	}
	entry, ok, err := ReadCachedOrEmpty(profile, resource)
	if err != nil || !ok {
		t.Fatalf("a 100-day-old entry should still be served: ok=%v err=%v", ok, err)
	}
	if string(entry.Data) != `["x"]` {
		t.Fatalf("data mismatch: %s", entry.Data)
	}
}

func TestReadCachedOrEmptyDistinguishesBrokenFromAbsent(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	const profile, resource = "p-test", "boards"

	// Absent: no error, ok=false. Callers that distinguish "absent" from
	// "broken" (board-scope resolvers) rely on this split.
	if _, ok, err := ReadCachedOrEmpty(profile, resource); err != nil || ok {
		t.Fatalf("absent entry: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}

	// Broken: an undecodable cache file surfaces an error, not a silent miss.
	path, err := Path(profile, resource)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write broken entry: %v", err)
	}
	if _, ok, err := ReadCachedOrEmpty(profile, resource); err == nil || ok {
		t.Fatalf("broken entry: want ok=false err!=nil, got ok=%v err=%v", ok, err)
	}
}
