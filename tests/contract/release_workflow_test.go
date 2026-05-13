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
		"install_args: --locked",
		"id: release-state",
		"gh release view",
		"gh release download",
		"goreleaser/goreleaser-action@1a80836c5c9d9e5755a25cb59ec6f45a3b5f41a8",
		"actions/create-github-app-token@1b10c78c7865c340bc4f6099eb2f838309f1e8c3",
		"vars.APP_CLIENT_ID",
		"secrets.APP_PRIVATE_KEY",
		"matcra587/github-actions/packages/homebrew-publish-formula@6e3053f35d4ec31adc4f82c04a2c43209f3b36e6",
		"steps.app-token.outputs.token",
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
	if strings.Contains(string(release), "HOMEBREW_TAP_TOKEN") {
		t.Fatalf("release workflow should use the Slack-aligned GitHub App token path, not HOMEBREW_TAP_TOKEN")
	}
}
