package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Save must write through a symlinked config path to the link's target, not
// replace the symlink itself. The atomic temp-file+rename idiom renames over
// `path`; when `path` is a symlink, an unresolved rename clobbers the link with
// a regular file. Resolving symlinks first keeps the link intact and updates
// the target it points at.
func TestSaveWritesThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real-config.toml")
	if err := os.WriteFile(target, []byte("default_profile = \"old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := Save(link, &Config{DefaultProfile: "updated-profile"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Save replaced the symlink with a regular file; it must write through to the target")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "updated-profile") {
		t.Fatalf("symlink target was not updated; content = %q", string(got))
	}
}

// A dangling symlink — one whose target does not exist yet, e.g. a user pointing
// config.toml at a shared file before the first `config set` — must be followed
// to its declared target on first write, not clobbered into a regular file.
func TestSaveWritesThroughDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "shared-config.toml") // intentionally absent
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := Save(link, &Config{DefaultProfile: "via-dangling-link"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("Save replaced the dangling symlink with a regular file")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("write did not land on the link's declared target: %v", err)
	}
	if !strings.Contains(string(got), "via-dangling-link") {
		t.Fatalf("target content = %q", string(got))
	}
}

// LoadOrInit must initialize a missing config through the same symlink-aware,
// directory-creating path as Save. A config.toml symlinked to a target whose
// directory does not exist yet (a dotfiles symlink staged before the real file)
// must have the target — and the target's directory — created, with the link
// left intact. The previous init path made the LINK's directory and wrote the
// raw path, so it could neither follow the link into a missing target dir nor
// guarantee the symlink survived.
func TestLoadOrInitInitializesThroughDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "jira.toml") // dir "dotfiles" absent
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadOrInit(WithPath(link)); err != nil {
		t.Fatalf("LoadOrInit through dangling symlink into a missing dir: %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("LoadOrInit replaced the symlink with a regular file")
	}
	if _, err := os.ReadFile(target); err != nil {
		t.Fatalf("init did not create the config at the link's target: %v", err)
	}
}
