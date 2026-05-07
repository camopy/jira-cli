package customfield

import (
	"bytes"
	"fmt"
)

// GenerateMatrix renders the customfield support matrix as Markdown
// derived from the registry. Mirrors pkg/adf/matrix.go. Output is
// deterministic so byte-equality tests catch drift.
func GenerateMatrix() []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "# Customfield Type Support Matrix")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "Generated from `pkg/jira/customfield` registry. Do not edit by hand.")
	fmt.Fprintln(buf, "Re-generate with `go run ./pkg/jira/customfield/genmatrix > docs/customfield-matrix.md`.")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "Every `official_url` points to an Atlassian source only.")
	fmt.Fprintln(buf, "The same envelope shape is shared with the ADF registry.")
	fmt.Fprintln(buf, "`submit_description` clarifies what `submit=true` means in this registry.")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "## Field types")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "| Name | Status | author | render | preserve | validate | submit | Official URL | Notes |")
	fmt.Fprintln(buf, "|------|--------|--------|--------|----------|----------|--------|--------------|-------|")
	for _, e := range Registry().All() {
		fmt.Fprintf(buf,
			"| `%s` | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			e.Name,
			string(e.Status),
			boolMark(e.Capabilities.Author),
			boolMark(e.Capabilities.Render),
			boolMark(e.Capabilities.Preserve),
			boolMark(e.Capabilities.Validate),
			boolMark(e.Capabilities.Submit),
			e.OfficialURL,
			escapePipes(e.Notes),
		)
	}
	return buf.Bytes()
}

func boolMark(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func escapePipes(s string) string {
	if s == "" {
		return ""
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r == '|' {
			out = append(out, '\\', '|')
			continue
		}
		out = append(out, []byte(string(r))...)
	}
	return string(out)
}
