package docs

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenLLMsTxtIsDeterministicAndPointsAtAgentSurface(t *testing.T) {
	var a, b bytes.Buffer
	if err := GenLLMsTxt(&a); err != nil {
		t.Fatalf("GenLLMsTxt: %v", err)
	}
	if err := GenLLMsTxt(&b); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if a.String() != b.String() {
		t.Fatal("llms.txt is not byte-stable")
	}
	out := a.String()
	for _, want := range []string{"# jira-cli", "jira agent guide", "jira agent schema", "/output/", "/auth/", "/search/", "/jql/"} {
		if !strings.Contains(out, want) {
			t.Errorf("llms.txt missing %q\n%s", want, out)
		}
	}
}
