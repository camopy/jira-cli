// Package adf — registry-backed strict-validation rules.
//
// These tables encode the structural contract of the pinned ADF JSON
// schema (@atlaskit/adf-schema 52.11.3, draft-04). They are derived
// strictly from the local JSON schema, not the Atlassian prose docs:
// the prose docs omit nodes, omit required attrs, and list narrower
// enums. Where the two
// disagree, the JSON schema is the contract Jira enforces.
//
// The rules drive ValidateDoc. They are intentionally a separate table
// from the public Entry registry: Entry describes author/render/submit
// capability for the agent matrix surface; these rules describe the
// structural shape a document must satisfy before submission.

package adf

// attrType enumerates the JSON value kinds an attr may hold.
type attrType int

const (
	attrAny attrType = iota
	attrString
	attrNumber
)

// attrRule describes one required attribute on a node or mark.
type attrRule struct {
	key string
	typ attrType
	// enum, when non-empty, constrains a string attr to this set.
	enum []string
	// min/max constrain a numeric attr (inclusive) when hasRange is set.
	min, max float64
	// hasRange marks min/max as a real bound. An explicit flag (rather
	// than a min!=0||max!=0 sentinel) so a legitimate min:0 bound is not
	// silently skipped.
	hasRange bool
	// nonEmpty requires a string attr to have length >= 1.
	nonEmpty bool
	// hexColor requires a string attr to match a #RRGGBB or #RRGGBBAA
	// pattern. allowAlpha selects which.
	hexColor   bool
	allowAlpha bool
}

// contentRule describes which child node types a node may contain.
type contentRule struct {
	// inlineOnly forbids block nodes; only inline nodes are permitted.
	inlineOnly bool
	// allowed, when non-nil, is an exact whitelist of permitted child
	// node types. nil means "no content restriction beyond inlineOnly".
	allowed map[string]bool
	// noMarks forbids any marks on direct text children (codeBlock).
	noMarks bool
	// minItems, when > 0, requires the node to have at least that many
	// child nodes — the schema's content `minItems` constraint.
	minItems int
}

// nodeRule is the strict-validation rule for one node type.
type nodeRule struct {
	requiredAttrs []attrRule
	// anyOfAttrs, when non-nil, encodes a schema `attrs.anyOf`: the node
	// is valid if it satisfies the requiredAttrs of at least one branch.
	// Used for media / inlineCard / blockCard / mediaSingle, whose
	// required attrs differ per variant.
	anyOfAttrs [][]attrRule
	// optionalAttrs constrains attrs that, when present, must satisfy a
	// type/enum/range — but are not required.
	optionalAttrs []attrRule
	content       contentRule
}

// markRule is the strict-validation rule for one mark type.
type markRule struct {
	requiredAttrs []attrRule
	// textNodeOnly restricts the mark to text nodes — the schema places
	// `code` only on text/code-inline nodes, never on mention/emoji.
	textNodeOnly bool
}

// inlineNodeTypes is the set of ADF inline node types. Anything not in
// this set is a block node and is forbidden inside paragraph/heading.
var inlineNodeTypes = map[string]bool{
	"text":            true,
	"hardBreak":       true,
	"emoji":           true,
	"date":            true,
	"mention":         true,
	"status":          true,
	"inlineCard":      true,
	"placeholder":     true,
	"inlineExtension": true,
	"mediaInline":     true,
}

// panelTypeEnum is the 7-value enum from the JSON schema. The Atlassian
// prose docs list only 5 — they omit "tip" and "custom".
var panelTypeEnum = []string{"info", "note", "tip", "warning", "error", "success", "custom"}

// tableCellContent is the JSON schema's `table_cell_content` whitelist —
// the exact set of block node types a tableCell/tableHeader may hold.
// Wider than the Atlassian prose docs, which underspecify it.
var tableCellContent = map[string]bool{
	"paragraph":    true,
	"panel":        true,
	"blockquote":   true,
	"orderedList":  true,
	"bulletList":   true,
	"rule":         true,
	"heading":      true,
	"codeBlock":    true,
	"mediaSingle":  true,
	"mediaGroup":   true,
	"decisionList": true,
	"taskList":     true,
	"blockCard":    true,
	"embedCard":    true,
	"extension":    true,
	"nestedExpand": true,
}

// cardLayoutEnum is the 7-value layout enum shared by mediaSingle,
// embedCard, and blockCard.
var cardLayoutEnum = []string{"wide", "full-width", "center", "wrap-right", "wrap-left", "align-end", "align-start"}

// nodeRules maps node type → strict-validation rule. Nodes absent from
// this map have no required attrs and no content restriction beyond the
// generic registry/known-type check in validate.go.
var nodeRules = map[string]nodeRule{
	"heading": {
		requiredAttrs: []attrRule{{key: "level", typ: attrNumber, min: 1, max: 6, hasRange: true}},
		content:       contentRule{inlineOnly: true},
	},
	"paragraph": {
		content: contentRule{inlineOnly: true},
	},
	"panel": {
		requiredAttrs: []attrRule{{key: "panelType", typ: attrString, enum: panelTypeEnum}},
		content:       contentRule{minItems: 1},
	},
	"status": {
		requiredAttrs: []attrRule{
			{key: "text", typ: attrString, nonEmpty: true},
			{key: "color", typ: attrString, enum: []string{"neutral", "purple", "blue", "red", "yellow", "green"}},
		},
	},
	"date": {
		requiredAttrs: []attrRule{{key: "timestamp", typ: attrString, nonEmpty: true}},
	},
	"mention": {
		// id must be a non-empty account reference; an empty id is a broken
		// mention that Jira rejects (parity with the other non-empty required
		// string attrs: status.text, date.timestamp, media.id, link.href, …).
		requiredAttrs: []attrRule{{key: "id", typ: attrString, nonEmpty: true}},
	},
	"emoji": {
		requiredAttrs: []attrRule{{key: "shortName", typ: attrString, nonEmpty: true}},
	},
	"taskItem": {
		requiredAttrs: []attrRule{
			{key: "localId", typ: attrString},
			{key: "state", typ: attrString, enum: []string{"TODO", "DONE"}},
		},
		content: contentRule{inlineOnly: true},
	},
	"blockTaskItem": {
		requiredAttrs: []attrRule{
			{key: "localId", typ: attrString},
			{key: "state", typ: attrString, enum: []string{"TODO", "DONE"}},
		},
		content: contentRule{minItems: 1, allowed: map[string]bool{
			"paragraph": true,
			"extension": true,
		}},
	},
	"decisionItem": {
		requiredAttrs: []attrRule{
			{key: "localId", typ: attrString},
			{key: "state", typ: attrString},
		},
		content: contentRule{inlineOnly: true},
	},
	"taskList": {
		requiredAttrs: []attrRule{{key: "localId", typ: attrString}},
		content: contentRule{minItems: 1, allowed: map[string]bool{
			"taskItem": true, "taskList": true, "blockTaskItem": true,
		}},
	},
	"decisionList": {
		requiredAttrs: []attrRule{{key: "localId", typ: attrString}},
		content:       contentRule{minItems: 1, allowed: map[string]bool{"decisionItem": true}},
	},
	"extension": {
		requiredAttrs: []attrRule{
			{key: "extensionKey", typ: attrString, nonEmpty: true},
			{key: "extensionType", typ: attrString, nonEmpty: true},
		},
	},
	"bodiedExtension": {
		requiredAttrs: []attrRule{
			{key: "extensionKey", typ: attrString, nonEmpty: true},
			{key: "extensionType", typ: attrString, nonEmpty: true},
		},
	},
	"inlineExtension": {
		requiredAttrs: []attrRule{
			{key: "extensionKey", typ: attrString, nonEmpty: true},
			{key: "extensionType", typ: attrString, nonEmpty: true},
		},
	},
	"table": {
		content: contentRule{minItems: 1, allowed: map[string]bool{"tableRow": true}},
	},
	"tableRow": {
		// tableRow.content has no minItems in the schema — an empty row
		// is degenerate but structurally valid.
		content: contentRule{allowed: map[string]bool{"tableCell": true, "tableHeader": true}},
	},
	"tableCell": {
		content: contentRule{minItems: 1, allowed: tableCellContent},
	},
	"tableHeader": {
		content: contentRule{minItems: 1, allowed: tableCellContent},
	},
	"blockquote": {
		content: contentRule{minItems: 1, allowed: map[string]bool{
			"paragraph":   true,
			"orderedList": true,
			"bulletList":  true,
			"codeBlock":   true,
			"mediaSingle": true,
			"mediaGroup":  true,
			"extension":   true,
		}},
	},
	"bulletList": {
		content: contentRule{minItems: 1, allowed: map[string]bool{"listItem": true}},
	},
	"orderedList": {
		optionalAttrs: []attrRule{{key: "order", typ: attrNumber, min: 0, max: 1e9, hasRange: true}},
		content:       contentRule{minItems: 1, allowed: map[string]bool{"listItem": true}},
	},
	"listItem": {
		content: contentRule{minItems: 1, allowed: map[string]bool{
			"paragraph":   true,
			"bulletList":  true,
			"orderedList": true,
			"taskList":    true,
			"codeBlock":   true,
			"mediaSingle": true,
			"extension":   true,
		}},
	},
	"codeBlock": {
		content: contentRule{allowed: map[string]bool{"text": true}, noMarks: true},
	},
	// media has two attr variants: a file/link node (type+id+collection)
	// and an external node (type+url). anyOfAttrs accepts either.
	"media": {
		anyOfAttrs: [][]attrRule{
			{
				{key: "type", typ: attrString, enum: []string{"link", "file"}},
				{key: "id", typ: attrString, nonEmpty: true},
				{key: "collection", typ: attrString},
			},
			{
				{key: "type", typ: attrString, enum: []string{"external"}},
				{key: "url", typ: attrString},
			},
		},
	},
	"mediaInline": {
		requiredAttrs: []attrRule{
			{key: "id", typ: attrString, nonEmpty: true},
			{key: "collection", typ: attrString},
		},
	},
	"mediaGroup": {
		content: contentRule{minItems: 1, allowed: map[string]bool{"media": true}},
	},
	"mediaSingle": {
		// Both attr branches require `layout`; the pixel branch also
		// requires width+widthType. Either branch satisfies the node.
		anyOfAttrs: [][]attrRule{
			{{key: "layout", typ: attrString, enum: cardLayoutEnum}},
			{
				{key: "width", typ: attrNumber},
				{key: "widthType", typ: attrString, enum: []string{"pixel"}},
				{key: "layout", typ: attrString, enum: cardLayoutEnum},
			},
		},
		content: contentRule{minItems: 1, allowed: map[string]bool{
			"media": true, "caption": true,
		}},
	},
	"embedCard": {
		requiredAttrs: []attrRule{
			{key: "url", typ: attrString, nonEmpty: true},
			{key: "layout", typ: attrString, enum: cardLayoutEnum},
		},
	},
	"inlineCard": {
		anyOfAttrs: [][]attrRule{
			{{key: "url", typ: attrString, nonEmpty: true}},
			{{key: "data", typ: attrAny}},
		},
	},
	"blockCard": {
		anyOfAttrs: [][]attrRule{
			{{key: "datasource", typ: attrAny}},
			{{key: "url", typ: attrString, nonEmpty: true}},
			{{key: "data", typ: attrAny}},
		},
	},
	// caption and nestedExpand are not root-legal but appear inside
	// mediaSingle / table cells. Registered with no constraints so a
	// valid document carrying them is not rejected as an unknown node.
	"caption":      {},
	"nestedExpand": {},
}

// markRules maps mark type → strict-validation rule.
var markRules = map[string]markRule{
	"strong":          {},
	"em":              {},
	"strike":          {},
	"underline":       {},
	"code":            {textNodeOnly: true},
	"link":            {requiredAttrs: []attrRule{{key: "href", typ: attrString, nonEmpty: true}}},
	"subsup":          {requiredAttrs: []attrRule{{key: "type", typ: attrString, enum: []string{"sub", "sup"}}}},
	"textColor":       {requiredAttrs: []attrRule{{key: "color", typ: attrString, hexColor: true}}},
	"backgroundColor": {requiredAttrs: []attrRule{{key: "color", typ: attrString, hexColor: true}}},
	"alignment":       {requiredAttrs: []attrRule{{key: "align", typ: attrString, enum: []string{"center", "end"}}}},
	"indentation":     {requiredAttrs: []attrRule{{key: "level", typ: attrNumber, min: 1, max: 6, hasRange: true}}},
	"border": {requiredAttrs: []attrRule{
		{key: "size", typ: attrNumber, min: 1, max: 3, hasRange: true},
		{key: "color", typ: attrString, hexColor: true, allowAlpha: true},
	}},
}

// blockLegalMarks is the set of marks the schema permits on block nodes
// via the marks-constrained node variants — alignment/indentation on
// paragraph & heading, border on media, breakout on root-only nodes.
// Every other mark is inline-only and illegal on a block node.
var blockLegalMarks = map[string]bool{
	"alignment":   true,
	"indentation": true,
	"border":      true,
	"breakout":    true,
	"fontSize":    true,
}

// codeExclusiveMarks is the set of decorative text marks the schema
// forbids alongside the `code` mark — `code` lives only in the
// code_inline_node whose allowed mark set is {code, link, annotation}.
var codeExclusiveMarks = map[string]bool{
	"strong":          true,
	"em":              true,
	"strike":          true,
	"underline":       true,
	"subsup":          true,
	"textColor":       true,
	"backgroundColor": true,
}

// docRootNodeTypes is the set of node types permitted as direct children
// of the document root. A bare `text` node at the root is invalid.
var docRootNodeTypes = map[string]bool{
	"blockCard":       true,
	"codeBlock":       true,
	"mediaSingle":     true,
	"paragraph":       true,
	"taskList":        true,
	"orderedList":     true,
	"bulletList":      true,
	"blockquote":      true,
	"decisionList":    true,
	"embedCard":       true,
	"extension":       true,
	"heading":         true,
	"mediaGroup":      true,
	"rule":            true,
	"panel":           true,
	"table":           true,
	"bodiedExtension": true,
	"expand":          true,
	"layoutSection":   true,
}
