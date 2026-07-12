// Package docs renders a Cobra command tree to byte-stable Markdown for the
// jira-cli documentation site. It mirrors the GitHub CLI's internal/docs
// generator: GenMarkdownTreeCustom walks the tree and GenMarkdownCustom renders
// one page per command, taking a filePrepender for site front-matter and a
// linkHandler for relative links between pages. Output is deterministic — no
// generation timestamp, stable ordering — so a CI drift gate only ever sees
// real changes.
//
// Flags are rendered from clib's metadata (github.com/gechr/clib), not raw
// pflag, so the reference surfaces the same groups, authored placeholders,
// allowed values, aliases and negatable variants that the terminal help shows.
package docs
