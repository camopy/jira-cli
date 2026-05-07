package contract

import (
	"os"
	"strings"
	"testing"
)

func TestReleaseWorkflowUsesPinnedGoReleaserAndHomebrewPublisher(t *testing.T) {
	release, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatalf("ReadFile(release) error = %v", err)
	}
	goreleaser, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatalf("ReadFile(goreleaser) error = %v", err)
	}
	combined := string(release) + "\n" + string(goreleaser)
	for _, want := range []string{
		"actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd",
		"sigstore/cosign-installer@cad07c2e89fa2edd6e2d7bab4c1aa38e53f76003",
		"jdx/mise-action@1648a7812b9aeae629881980618f079932869151",
		"goreleaser/goreleaser-action@1a80836c5c9d9e5755a25cb59ec6f45a3b5f41a8",
		"matcra587/github-actions/packages/homebrew-publish-formula@8b104684e72bef79fca78b294accb5f789d3f202",
		"HOMEBREW_TAP_TOKEN",
		"tap: matcra587/homebrew-tap",
		"name_template:",
		"{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}",
		"sign-blob",
		"internal/version.Version={{ .Version }}",
		"internal/version.BuildBy=goreleaser",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("release workflow/config missing %q\nrelease:\n%s\ngoreleaser:\n%s", want, release, goreleaser)
		}
	}
}
