package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// FileCredentialStore.Put must write through a symlinked entry to the link's
// target, not replace the link with a regular file — the same guarantee Save
// gives config.toml. It is the credential-store sibling of the config write
// path, and both must share one symlink-aware atomic writer.
func TestFileCredentialStorePutWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	s := NewFileCredentialStore(dir)
	ref := SecretRef{Profile: "work", Backend: SecretBackendKeyring, Host: "x.example"}

	path, err := s.entryPath(ref)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "real.secret")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if err := s.Put(context.Background(), ref, "new-secret"); err != nil {
		t.Fatalf("Put through symlink: %v", err)
	}

	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Put replaced the symlinked credential entry with a regular file")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("write did not land on the link's target: %v", err)
	}
	if string(got) != "new-secret" {
		t.Fatalf("symlink target not updated; content = %q", string(got))
	}
	if v, gErr := s.Get(context.Background(), ref); gErr != nil || v != "new-secret" {
		t.Fatalf("Get through the link = %q, %v", v, gErr)
	}
}
