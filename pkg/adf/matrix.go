package adf

import (
	"bytes"
	"fmt"
)

// GenerateMatrix renders the ADF support matrix as Markdown derived
// from the registry. Output is deterministic — sorted by kind then
// name — so byte-equality tests catch any drift.
func GenerateMatrix() []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintln(buf, "# ADF Support Matrix")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "Generated from `pkg/adf` registry. Do not edit by hand.")
	fmt.Fprintln(buf, "Re-generate with `go run ./pkg/adf/genmatrix > docs/adf-support-matrix.md`.")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "Every `official_url` points to an Atlassian source only.")
	fmt.Fprintln(buf, "The same envelope shape is shared with the customfield registry.")
	fmt.Fprintln(buf, "`submit_description` clarifies what `submit=true` means in this registry.")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "## Nodes")
	fmt.Fprintln(buf)
	writeMatrixSection(buf, KindNode)
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "## Marks")
	fmt.Fprintln(buf)
	writeMatrixSection(buf, KindMark)
	return buf.Bytes()
}

func writeMatrixSection(buf *bytes.Buffer, kind Kind) {
	fmt.Fprintln(buf, "| Name | Status | author | render | preserve | validate | submit | Official URL | Notes |")
	fmt.Fprintln(buf, "|------|--------|--------|--------|----------|----------|--------|--------------|-------|")
	for _, e := range Registry().All() {
		if e.Kind != kind {
			continue
		}
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
