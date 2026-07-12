// Package boardscope resolves the `--board NAME` / `--board-id N` flag pair
// against the local board cache and renders the result into JQL clauses and
// envelope data. FromFlags is the single place that decides which precedence
// path won (flag, default, or none); it is shared by issue list and jql build.
package boardscope
