package editor

import (
	"context"
	"fmt"
	"os"
	"strings"
)

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

func EditMarkdown(ctx context.Context, issueKey, fieldName, markdown, editorCommand string) (string, error) {
	path, err := WriteTemp(issueKey, fieldName, markdown)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(path) }()
	if err := Run(ctx, editorCommand, path); err != nil {
		return "", err
	}
	return ReadMarkdown(path)
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
