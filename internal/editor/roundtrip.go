package editor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"
)

// nonBlockingSpawnThreshold is the wall-clock floor below which an
// editor exit is considered suspicious (likely a non-blocking launcher
// like vanilla `code` that returns before the user can read, let alone
// edit). Real editors take seconds to load even when the user
// immediately quits without changes.
const nonBlockingSpawnThreshold = 500 * time.Millisecond

func WriteTemp(issueKey, fieldName, markdown string) (string, error) {
	f, err := os.CreateTemp("", "jira-edit-*.md")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "---\nissue_key: %s\nfield_name: %s\n---\n\n%s\n", issueKey, fieldName, markdown)
	if err != nil {
		return "", err
	}
	return f.Name(), nil
}

func ReadMarkdown(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return stripFrontmatter(string(data)), nil
}

// EditMarkdown writes a temp file, spawns the configured editor, and
// reads the edited content back. The temp file is preserved on every
// error path (per kubectl's pattern) so the user can recover in-flight
// edits — the path is named in the error message.
//
// A sub-second editor exit combined with byte-identical content trips
// the non-blocking-spawn safety check: that's the signature of
// `EDITOR=code` (without --wait) returning before VS Code has finished
// opening the buffer. Treating it as success would race the parent's
// cleanup against the editor's save, causing the strikethrough-on-
// filename + silent-data-loss bug.
func EditMarkdown(ctx context.Context, issueKey, fieldName, markdown, editorCommand string) (string, error) {
	path, err := WriteTemp(issueKey, fieldName, markdown)
	if err != nil {
		return "", err
	}
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		// File missing immediately after WriteTemp shouldn't happen, but
		// preserve the path in the error so a follow-up can investigate.
		return "", fmt.Errorf("read temp file before edit: %w (preserved at %s)", err, path)
	}
	originalHash := sha256.Sum256(originalBytes)

	spawnedAt := time.Now()
	if runErr := Run(ctx, editorCommand, path); runErr != nil {
		return "", fmt.Errorf("editor failed: %w (preserved at %s)", runErr, path)
	}
	elapsed := time.Since(spawnedAt)

	editedBytes, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read temp file after edit: %w (preserved at %s)", err, path)
	}
	editedHash := sha256.Sum256(editedBytes)

	if elapsed < nonBlockingSpawnThreshold && editedHash == originalHash {
		return "", fmt.Errorf(
			"editor returned in %s without modifying the file — likely a non-blocking launcher (e.g. EDITOR=code without --wait); set EDITOR with a blocking flag (e.g. EDITOR='code --wait') and retry, original preserved at %s",
			elapsed.Round(time.Millisecond), path,
		)
	}

	// Safe to delete: editor blocked properly OR the user actually
	// changed something.
	defer func() { _ = os.Remove(path) }()
	return stripFrontmatter(string(editedBytes)), nil
}

func stripFrontmatter(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return strings.TrimSpace(content)
	}
	rest := strings.TrimPrefix(content, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return strings.TrimSpace(content)
	}
	return strings.TrimSpace(rest[end+len("\n---\n"):])
}
