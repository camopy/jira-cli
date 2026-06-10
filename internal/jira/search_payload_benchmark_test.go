package jira

import "testing"

func BenchmarkSearchRequestPayloadWithDetailExpand(b *testing.B) {
	req := &SearchRequest{
		JQL:           "project = JCT ORDER BY updated DESC",
		MaxResults:    50,
		NextPageToken: "cursor-1",
		Fields:        []string{"*all"},
		Expand:        []string{"renderedFields", "names", "schema", "transitions", "operations", "changelog"},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = req.payload()
	}
}
