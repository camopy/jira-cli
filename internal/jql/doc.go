// Package jql builds and composes Jira Query Language strings from
// structured inputs. It is pure string logic — no cobra, config, or I/O —
// so the jql/search/issue command layers and shell completion can all share
// one query builder without import cycles.
//
// It is the single composition path: every command builds its query through
// Build (filters → ORDER BY) or IssueList (raw JQL + filters + ORDER BY); every
// value is quoted through the one quoter, Value (safe identifier bare,
// otherwise double-quoted with embedded quotes escaped); and top-level ORDER BY
// / OR detection goes through the one quote-aware tokenizer (topLevelWordTokens,
// SplitTopLevelOrderBy). Board scoping composes on top via CombineClauses /
// ParenthesizeIfTopLevelOR rather than re-implementing any of it. Keep new JQL
// construction here — command layers must not hand-build clause strings, so the
// quoting and precedence rules cannot drift per command.
package jql
