package envelope

// Typed Output structs for the issue-core operation family. Each struct is
// the single declaration of its envelope's `data` shape: the builder emits
// it, SchemaOf derives the published schema from it, and the registration
// beside it feeds the exhaustiveness guardrail. Registration uses a
// package-level var (not init) so the wiring is explicit and greppable.

// IssueRankOutput is `issue rank`'s envelope data: the requested ordering
// echoed back with the chunking the transport applied.
type IssueRankOutput struct {
	Anchor   string   `json:"anchor"`
	Position string   `json:"position"`
	Order    []string `json:"order"`
	Chunks   int      `json:"chunks"`
	DryRun   bool     `json:"dry_run"`
	// Ranked is present (false) only on the no-profile degraded path, where
	// the command validates and reports without a client to submit through.
	Ranked *bool `json:"ranked,omitempty"`
}

var _ = register("issue.rank", IssueRankOutput{}, nil)

// WebOpenIssueOutput is the data payload of a --web browser open scoped to
// one issue (issue view --web, jira open): the identity of what was opened;
// the URL rides in the web envelope itself.
type WebOpenIssueOutput struct {
	Issue IssueRef `json:"issue"`
}

// WebOpenSearchOutput is the data payload of a --web browser open for a JQL
// search: the query that was opened.
type WebOpenSearchOutput struct {
	Source string `json:"source"`
	Jql    string `json:"jql"`
}
