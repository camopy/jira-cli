package docs

import (
	"fmt"
	"io"
)

// llmsConceptualPages is the conceptual-page set surfaced in llms.txt, kept in
// lockstep with zensical.toml's nav. Title + site path (Zensical serves pages
// at <name>/).
var llmsConceptualPages = []struct{ Title, Path string }{
	{"Overview", "/"},
	{"Installation", "/installation/"},
	{"Auth", "/auth/"},
	{"Output and exit codes", "/output/"},
	{"Issues", "/issues/"},
	{"ADF authoring", "/adf/"},
	{"Custom fields", "/custom-fields/"},
	{"Search", "/search/"},
	{"JQL", "/jql/"},
	{"Cache", "/cache/"},
	{"Agent tooling", "/agent/"},
}

// GenLLMsTxt writes a deterministic llms.txt: a short project summary, the
// conceptual page map, and a pointer to the canonical agent surface (the
// embedded runbook and schema), which are the source of truth for agents.
func GenLLMsTxt(w io.Writer) error {
	var err error
	emit := func(format string, a ...any) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, a...)
		}
	}
	emit("# jira-cli\n\n")
	emit("Terminal-first Jira CLI for developer and agent workflows. Output is one\n")
	emit("`--output` flag (auto|human|json|compact); non-TTY and agent environments\n")
	emit("emit JSON envelopes.\n\n")
	emit("## Agent surface (canonical)\n\n")
	emit("Agents should drive the tool through its embedded, version-matched runbook\n")
	emit("rather than these pages:\n\n")
	emit("- `jira agent guide <slug>` — lifecycle runbooks (create/read/edit/transition issues, comments, links, worklog, destructive ops).\n")
	emit("- `jira agent schema` — JSON wire shapes for inputs and envelopes.\n\n")
	emit("## Concept guides\n\n")
	for _, pg := range llmsConceptualPages {
		emit("- [%s](%s)\n", pg.Title, pg.Path)
	}
	emit("\n## Command reference\n\n")
	emit("Generated per-command pages live under `/reference/` on the docs site.\n")
	return err
}
