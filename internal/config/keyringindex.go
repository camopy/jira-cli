package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	xos "github.com/gechr/x/os"
	xstrings "github.com/gechr/x/strings"
)

// The keyring index is the small persisted record of every {host, profile}
// credential identity this CLI has stored in the OS keyring. It exists
// because the keyring backend (zalando/go-keyring) exposes no key listing,
// so without it stored credentials are undiscoverable once their profile
// leaves the config file — the orphan a future purge needs to find. The
// index holds identities only (the same metadata the config file itself
// carries), never credential material.
//
// Entries written by releases that predate the index are not represented;
// they are exactly as discoverable as they always were, so the index can
// only improve on the status quo. Maintenance is best-effort by design: a
// failed index write never fails the credential operation that triggered
// it, because the credential state (the thing that matters) already
// changed. Like the metadata cache, the index takes no lock: concurrent
// writers would race, but in practice each machine is driven by one user
// shell, and a lost write costs at most one enumeration entry.

// keyringIndexEntry is one stored credential identity.
type keyringIndexEntry struct {
	Host    string `json:"host"`
	Profile string `json:"profile"`
}

// keyringIndexPath resolves the index file location. The test credential
// store confinement dir wins when set — the same env that keeps test
// credential machinery out of the real keyring keeps the index out of the
// real config dir. The file name carries the keyring service override so a
// throwaway test namespace never touches the production index.
func keyringIndexPath() string {
	name := "keyring-index.json"
	if svc := keyringServiceName(); svc != defaultKeyringService {
		name = "keyring-index-" + sanitizeIndexSegment(svc) + ".json"
	}
	if dir := strings.TrimSpace(os.Getenv(TestCredentialStoreDirEnv)); dir != "" {
		return filepath.Join(dir, name)
	}
	return filepath.Join(configRoot(), "jira-cli", name)
}

// sanitizeIndexSegment keeps a service-name override filesystem-safe.
func sanitizeIndexSegment(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case xstrings.IsAlphanumericChar(r), r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, s)
}

// readKeyringIndex loads the index, returning nil on any miss — no file,
// unreadable, wrong shape. The index is advisory; a broken file reads as
// empty rather than failing the caller.
func readKeyringIndex() []keyringIndexEntry {
	raw, err := os.ReadFile(keyringIndexPath())
	if err != nil {
		return nil
	}
	var entries []keyringIndexEntry
	if json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	return entries
}

// writeKeyringIndex persists entries sorted and deduplicated, atomically.
func writeKeyringIndex(entries []keyringIndexEntry) error {
	seen := make(map[keyringIndexEntry]bool, len(entries))
	out := make([]keyringIndexEntry, 0, len(entries))
	for _, e := range entries {
		if e.Profile == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		return out[i].Profile < out[j].Profile
	})
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	path := keyringIndexPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return xos.AtomicWrite(path, append(body, '\n'), 0o600)
}

// keyringIndexAdd records ref's identity in the index.
func keyringIndexAdd(ref SecretRef) error {
	return writeKeyringIndex(append(readKeyringIndex(), keyringIndexEntry{Host: ref.Host, Profile: ref.Profile}))
}

// keyringIndexRemove drops ref's identity from the index. Removing an
// absent entry is a no-op write of the same content — cheap, and it keeps
// delete-side self-healing unconditional.
func keyringIndexRemove(ref SecretRef) error {
	entries := readKeyringIndex()
	out := entries[:0]
	for _, e := range entries {
		if e.Host == ref.Host && e.Profile == ref.Profile {
			continue
		}
		out = append(out, e)
	}
	return writeKeyringIndex(out)
}

// ListStoredRefs enumerates the {host, profile} identities this CLI has
// recorded storing in the OS keyring, as SecretRefs with the keyring
// backend set. The listing reflects the persisted index: writes from
// releases before the index existed are absent, and the caller (the orphan
// purge this enables) should treat it as "known stored", not "everything
// ever stored".
func (KeyringStore) ListStoredRefs() []SecretRef {
	entries := readKeyringIndex()
	refs := make([]SecretRef, 0, len(entries))
	for _, e := range entries {
		refs = append(refs, SecretRef{Host: e.Host, Profile: e.Profile, Backend: SecretBackendKeyring})
	}
	return refs
}
