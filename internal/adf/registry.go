// Package adf — registry of supported ADF nodes/marks.
//
// This table is the single source of truth for ADF support metadata.
// The contracts/adf-support-matrix.md doc is generated from it, and
// `jira agent adf-matrix --json` emits these rows. Every row's
// official_url points to an Atlassian source only. Every MVP node/mark
// MUST have a row. The envelope shape is shared with pkg/jira/customfield;
// submit_description disambiguates the submit capability per registry.

package adf

import (
	"encoding/json"
	"maps"
	"slices"
)

// nodeRuleNames / markRuleNames are the validated universes in stable
// order — the names Registry() synthesizes rows for.
func nodeRuleNames() []string { return slices.Sorted(maps.Keys(nodeRules)) }
func markRuleNames() []string { return slices.Sorted(maps.Keys(markRules)) }

// Kind tags whether a registry row describes a node or a mark.
type Kind int

const (
	// KindNode marks a row that describes an ADF node.
	KindNode Kind = iota
	// KindMark marks a row that describes an ADF mark.
	KindMark
)

func (k Kind) String() string {
	switch k {
	case KindNode:
		return "node"
	case KindMark:
		return "mark"
	default:
		return "unknown"
	}
}

func (k Kind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// Capabilities is the shared envelope's per-row capability struct.
// All booleans default to false; explicit support is opt-in.
type Capabilities struct {
	Author   bool `json:"author"`
	Render   bool `json:"render"`
	Preserve bool `json:"preserve"`
	Validate bool `json:"validate"`
	Submit   bool `json:"submit"`
}

// Status tags the support tier a row currently sits in. New rows start at
// "preserve-only" and graduate to "mvp" or higher tiers as authoring lands.
type Status string

const (
	// StatusMVP tags a node/mark jira-cli fully authors.
	StatusMVP Status = "mvp"
	// StatusPreserveOnly tags the entry tier: parsed and preserved on
	// round-trip, but not yet authored.
	StatusPreserveOnly Status = "preserve-only"
)

// Entry is one row in the registry — the shared envelope shape.
// input_shape and output_shape are JSON Schema 2020-12 fragments.
type Entry struct {
	Kind              Kind            `json:"kind"`
	Name              string          `json:"name"`
	Status            Status          `json:"status"`
	Capabilities      Capabilities    `json:"capabilities"`
	InputShape        json.RawMessage `json:"input_shape,omitempty"`
	OutputShape       json.RawMessage `json:"output_shape,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
	OfficialURL       string          `json:"official_url"`
	Notes             string          `json:"notes,omitempty"`
	SubmitDescription string          `json:"submit_description"`
}

// RegistryView is the read-only handle commands and tests use.
type RegistryView struct {
	entries []Entry
	index   map[string]Entry // key = "kind:name"
}

// All returns a copy of every registry entry in stable order, so a caller may
// retain or mutate the result without disturbing the shared registry.
func (r RegistryView) All() []Entry {
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Lookup returns the entry for a kind+name pair, and whether one exists.
func (r RegistryView) Lookup(kind Kind, name string) (Entry, bool) {
	e, ok := r.index[kindKey(kind, name)]
	return e, ok
}

func kindKey(k Kind, name string) string { return k.String() + ":" + name }

const (
	adfDocsBase  = "https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/"
	adfMarksBase = "https://developer.atlassian.com/cloud/jira/platform/apis/document/marks/"
	// adfStructureURL documents nodes the Atlassian prose docs omit (the
	// task/decision family and other schema-only nodes) — the structure
	// page is the closest official source.
	adfStructureURL = "https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/"
)

// authorableNodes is the set of node types the Markdown convenience layer
// (FromMarkdownLossy) can emit. It feeds the computed Author capability in
// the matrix, and the corpus contract test in registry_capability_test.go
// fails when it drifts from the converter's actual behavior in either
// direction.
var authorableNodes = map[string]bool{
	"doc":         true,
	"paragraph":   true,
	"text":        true,
	"heading":     true,
	"bulletList":  true,
	"orderedList": true,
	"listItem":    true,
	"codeBlock":   true,
	"blockquote":  true,
	"hardBreak":   true,
	"rule":        true,
	"table":       true,
	"tableRow":    true,
	"tableCell":   true,
	"tableHeader": true,
	"taskList":    true,
	"taskItem":    true,
}

// authorableMarks is the mark counterpart of authorableNodes, under the
// same corpus contract test.
var authorableMarks = map[string]bool{
	"strong": true,
	"em":     true,
	"strike": true,
	"code":   true,
	"link":   true,
}

// nodeRow / markRow keep the curated table below visually scannable.
// Capabilities are not curated: Registry() computes them from the
// authorable/renderable sets so a row can never claim more or less than
// the code does.
func nodeRow(name string, status Status, notes string) Entry {
	return Entry{
		Kind:              KindNode,
		Name:              name,
		Status:            status,
		OfficialURL:       adfDocsBase + name + "/",
		Notes:             notes,
		SubmitDescription: "ADF: included in a Jira rich-text field payload after validation passes.",
	}
}

// nodeRowAt is nodeRow with an explicit official URL, for nodes the
// Atlassian prose docs omit.
func nodeRowAt(name, url string, status Status, notes string) Entry {
	e := nodeRow(name, status, notes)
	e.OfficialURL = url
	return e
}

func markRow(name string, status Status, notes string) Entry {
	return Entry{
		Kind:              KindMark,
		Name:              name,
		Status:            status,
		OfficialURL:       adfMarksBase + name + "/",
		Notes:             notes,
		SubmitDescription: "ADF: applied to text inside a rich-text field payload after validation passes.",
	}
}

// registryEntries holds the curated rows: status, notes, and doc URL.
// Rows for everything else in the validated universe are synthesized by
// Registry() as preserve-only, so the matrix always covers every node and
// mark the validator accepts.
var registryEntries = []Entry{
	// MVP nodes.
	nodeRow("doc", StatusMVP, "ADF root."),
	nodeRow("paragraph", StatusMVP, ""),
	nodeRow("text", StatusMVP, ""),
	nodeRow("heading", StatusMVP, "attrs.level required (1-6)."),
	nodeRow("bulletList", StatusMVP, ""),
	nodeRow("orderedList", StatusMVP, ""),
	nodeRow("listItem", StatusMVP, "Authored as part of a bullet/ordered list."),
	nodeRow("codeBlock", StatusMVP, "attrs.language optional."),
	nodeRow("blockquote", StatusMVP, ""),
	nodeRow("hardBreak", StatusMVP, ""),
	nodeRow("rule", StatusMVP, ""),
	nodeRow("mention", StatusMVP, "attrs.id (accountId) and attrs.text required."),
	nodeRow("emoji", StatusMVP, "attrs.shortName required."),
	nodeRow("date", StatusMVP, "attrs.timestamp required (epoch ms as string)."),
	nodeRow("status", StatusMVP, "attrs.text and attrs.color required."),
	nodeRow("inlineCard", StatusMVP, "Field compatibility enforced; degrades to text+link in best-effort."),
	nodeRow("panel", StatusMVP, "attrs.panelType required (info/warning/error/success/note)."),
	nodeRow("table", StatusMVP, ""),
	nodeRow("tableRow", StatusMVP, "Authored as part of a table."),
	nodeRow("tableCell", StatusMVP, "Authored as part of a table."),
	nodeRow("tableHeader", StatusMVP, "Authored as part of a table."),
	nodeRowAt("taskList", adfStructureURL, StatusMVP,
		"Action-item (checkbox) list. Authored from GFM task-list Markdown (`- [ ]` / `- [x]`); attrs.localId is generated during conversion."),
	nodeRowAt("taskItem", adfStructureURL, StatusMVP,
		"Authored as part of a task list; attrs.state (TODO|DONE) carries the checkbox state."),
	nodeRowAt("blockTaskItem", adfStructureURL, StatusMVP,
		"Jira-authored task item with block content; rendered and preserved, not authored from Markdown."),
	nodeRowAt("decisionList", adfStructureURL, StatusMVP,
		"Decision list. No Markdown authoring surface — author as native ADF; renders as `- <>` items."),
	nodeRowAt("decisionItem", adfStructureURL, StatusMVP,
		"Rendered with the `<>` decision marker; authored as part of a native-ADF decisionList."),

	// MVP marks.
	markRow("strong", StatusMVP, ""),
	markRow("em", StatusMVP, ""),
	markRow("strike", StatusMVP, ""),
	markRow("code", StatusMVP, "Combines only with the link mark; any other mark on code text is invalid."),
	markRow("link", StatusMVP, "attrs.href required."),
	markRow("textColor", StatusMVP, "attrs.color required (#RRGGBB)."),
	markRow("backgroundColor", StatusMVP, "attrs.color required (#RRGGBB)."),
	markRow("subsup", StatusMVP, "attrs.type=sub|sup."),
	markRow("underline", StatusMVP, ""),
}

// capabilitiesFor computes a row's capability flags from the code that
// implements them: Author from the Markdown converter's emit set, Render
// from the Markdown renderer's node set, and Preserve/Validate/Submit
// unconditionally — knownNodeType/knownMarkType grants exactly those three
// to every registered type.
func capabilitiesFor(kind Kind, name string) Capabilities {
	caps := Capabilities{Preserve: true, Validate: true, Submit: true}
	switch kind {
	case KindNode:
		caps.Author = authorableNodes[name]
		caps.Render = renderableMarkdownNodes[name]
	case KindMark:
		caps.Author = authorableMarks[name]
		caps.Render = renderableMarkdownMarks[name]
	}
	return caps
}

// Registry returns the read-only registry view: the curated rows with
// computed capabilities, plus a synthesized preserve-only row for every
// validated node/mark that has no curated entry. The matrix therefore
// covers the validator's full universe by construction — it cannot
// under-report what the CLI accepts.
func Registry() RegistryView {
	entries := make([]Entry, 0, len(nodeRules)+len(markRules))
	curated := make(map[string]bool, len(registryEntries))
	for _, e := range registryEntries {
		e.Capabilities = capabilitiesFor(e.Kind, e.Name)
		curated[kindKey(e.Kind, e.Name)] = true
		entries = append(entries, e)
	}
	entries = append(entries, synthesizedRows(KindNode, nodeRuleNames(), curated)...)
	entries = append(entries, synthesizedRows(KindMark, markRuleNames(), curated)...)

	idx := make(map[string]Entry, len(entries))
	for _, e := range entries {
		idx[kindKey(e.Kind, e.Name)] = e
	}
	return RegistryView{entries: entries, index: idx}
}

// synthesizedRows builds sorted preserve-only rows for every name without
// a curated entry.
func synthesizedRows(kind Kind, names []string, curated map[string]bool) []Entry {
	var out []Entry
	for _, name := range names {
		if curated[kindKey(kind, name)] {
			continue
		}
		out = append(out, Entry{
			Kind:         kind,
			Name:         name,
			Status:       StatusPreserveOnly,
			Capabilities: capabilitiesFor(kind, name),
			OfficialURL:  adfStructureURL,
			Notes:        "Accepted, validated, and preserved as native ADF; no Markdown authoring surface.",
			SubmitDescription: "ADF: included in a Jira rich-text field payload after " +
				"validation passes.",
		})
	}
	return out
}
