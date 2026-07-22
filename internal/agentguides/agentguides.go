package agentguides

import (
	"embed"
	"io/fs"

	"github.com/matcra587/docent"
)

// ContractVersion is the semver of the agent-facing contract: the command
// tree, envelope, exit codes, and guide semantics published by `jira agent
// schema` and stamped on the guide index. Bump major for a breaking change
// to the envelope, exit codes, or an existing flag/field; minor for
// additive surface; patch for wording-only changes. Independent of the
// binary version. The move from the hand-rolled surface to docent (new
// schema shape, new guide set) is the 2.x → 3.0.0 break.
const ContractVersion = "3.0.0"

//go:embed guides/*.md
var guidesFS embed.FS

// FS returns the guides directory as its own filesystem root, the view
// docenttest.Validate expects (the raw embed FS is rooted one level up).
func FS() (fs.FS, error) {
	return fs.Sub(guidesFS, "guides")
}

// Load parses and validates the embedded guide set.
func Load() (*docent.GuideSet, error) {
	sub, err := FS()
	if err != nil {
		return nil, err
	}
	return docent.LoadGuides(sub)
}
