package unit

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"company", "https://company.atlassian.net"},
		{"  company  ", "https://company.atlassian.net"},
		{"company.atlassian.net", "https://company.atlassian.net"},
		{"https://company.atlassian.net", "https://company.atlassian.net"},
		{"https://company.atlassian.net/", "https://company.atlassian.net"},
		{"https://company.atlassian.net/jira/", "https://company.atlassian.net/jira"},
		{"company.example.com", "https://company.example.com"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"localhost:8080", "https://localhost:8080"},
		{"http://127.0.0.1:9000/", "http://127.0.0.1:9000"},
	}
	for _, c := range cases {
		got := config.NormalizeBaseURL(c.in)
		if got != c.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
