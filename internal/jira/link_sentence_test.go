package jira

import "testing"

// The create-side preview (IssueLinkType.PreviewSentences) and the read-side
// rendering (IssueLinkView.Sentence over flattened links) must agree on the
// sentence each issue's page displays — the incident this pins: a caller
// inferred the direction from the raw inward/outward echo, guessed the
// crossover backwards, and created a link that read the wrong way on both
// pages. Covers an asymmetric type (Blocks) and a symmetric one (Relates).
func TestLinkPreviewSentencesAgreeWithListRendering(t *testing.T) {
	tests := []struct {
		name                string
		linkType            issueLinkType
		wantInwardSentence  string
		wantOutwardSentence string
	}{
		{
			name:                "asymmetric blocks",
			linkType:            issueLinkType{ID: "10000", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
			wantInwardSentence:  "PROJ-1 blocks PROJ-2",
			wantOutwardSentence: "PROJ-2 is blocked by PROJ-1",
		},
		{
			name:                "symmetric relates",
			linkType:            issueLinkType{ID: "10003", Name: "Relates", Inward: "relates to", Outward: "relates to"},
			wantInwardSentence:  "PROJ-1 relates to PROJ-2",
			wantOutwardSentence: "PROJ-2 relates to PROJ-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := IssueLinkType{ID: tt.linkType.ID, Name: tt.linkType.Name, Inward: tt.linkType.Inward, Outward: tt.linkType.Outward}
			gotInward, gotOutward := created.PreviewSentences("PROJ-1", "PROJ-2")
			if gotInward != tt.wantInwardSentence || gotOutward != tt.wantOutwardSentence {
				t.Fatalf("PreviewSentences() = %q / %q, want %q / %q", gotInward, gotOutward, tt.wantInwardSentence, tt.wantOutwardSentence)
			}

			// Simulate the same link as each endpoint later reads it back:
			// Jira embeds only the FAR end, keeping its role, so the inward
			// issue's read carries an outwardIssue slot and vice versa.
			var onInward, onOutward issueLinkSide
			onInward.Key = "PROJ-2"
			onOutward.Key = "PROJ-1"
			inwardRead := flattenIssueLinks([]issueLinkWire{{ID: "1", Type: tt.linkType, OutwardIssue: &onInward}})
			outwardRead := flattenIssueLinks([]issueLinkWire{{ID: "1", Type: tt.linkType, InwardIssue: &onOutward}})
			if len(inwardRead) != 1 || len(outwardRead) != 1 {
				t.Fatalf("flattenIssueLinks() rows = %d / %d, want 1 / 1", len(inwardRead), len(outwardRead))
			}
			if got := inwardRead[0].Sentence("PROJ-1"); got != tt.wantInwardSentence {
				t.Errorf("inward issue list sentence = %q, want create preview %q", got, tt.wantInwardSentence)
			}
			if got := outwardRead[0].Sentence("PROJ-2"); got != tt.wantOutwardSentence {
				t.Errorf("outward issue list sentence = %q, want create preview %q", got, tt.wantOutwardSentence)
			}
		})
	}
}
