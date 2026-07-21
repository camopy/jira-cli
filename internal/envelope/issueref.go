package envelope

// IssueRef is the envelope contract's issue identity: every issue-scoped
// envelope carries `data.issue` as an object with at least `key`, never a
// bare string, so `.data.issue.key` reads identically across create, edit,
// view, transition, comment, worklog, and epic envelopes. Richer payloads
// (create's POST response, view's full issue) satisfy the same minimum by
// carrying key at the same place. Introduced in contract v2 — v1 let
// mutation envelopes ship the key as a bare string, which made `.data.issue`
// change type between commands.
type IssueRef struct {
	ID   string `json:"id,omitempty"`
	Key  string `json:"key"`
	Self string `json:"self,omitempty"`
}

// String renders the ref as its key so human/plain output shows the issue
// key where machine output carries the identity object.
func (r IssueRef) String() string {
	return r.Key
}
