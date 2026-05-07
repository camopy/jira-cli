package config

import (
	"os"
	"path/filepath"
	"strings"
)

type Query struct {
	Name        string
	Description string
	Project     string
	JQL         string
}

func LoadQueries(dir string) (map[string]Query, error) {
	out := map[string]Query{}
	dir = expandHome(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(entry.Name(), ".jql")
		q := Query{Name: key}
		body := string(b)
		if strings.HasPrefix(body, "---\n") {
			rest := strings.TrimPrefix(body, "---\n")
			head, tail, ok := strings.Cut(rest, "\n---\n")
			if ok {
				for _, line := range strings.Split(head, "\n") {
					k, v, ok := strings.Cut(line, ":")
					if !ok {
						continue
					}
					switch strings.TrimSpace(k) {
					case "name":
						q.Name = strings.TrimSpace(v)
					case "description":
						q.Description = strings.TrimSpace(v)
					case "project":
						q.Project = strings.TrimSpace(v)
					}
				}
				body = tail
			}
		}
		q.JQL = strings.TrimSpace(body)
		out[key] = q
	}
	return out, nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
