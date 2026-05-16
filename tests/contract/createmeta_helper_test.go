package contract

import (
	"io"
	"net/http"
	"strconv"
)

// registerCreatemeta wires the two-call createmeta sequence onto a
// ServeMux for a single project + issue type: the paginated issuetypes
// page that resolves the issue-type NAME to its id, then the
// field-metadata page keyed by that id. fieldsJSON is the JSON array
// for the field-metadata page's "fields" property.
//
// The CLI resolves the issue-type id before the field-metadata call —
// a mock that only serves the name-keyed path no longer matches.
func registerCreatemeta(mux *http.ServeMux, project, issueType, issueTypeID, fieldsJSON string) {
	total := strconv.Itoa(countTopLevelObjects(fieldsJSON))
	mux.HandleFunc("GET /rest/api/3/issue/createmeta/"+project+"/issuetypes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"startAt":0,"maxResults":50,"total":1,"issueTypes":[{"id":"`+issueTypeID+`","name":"`+issueType+`"}]}`)
	})
	mux.HandleFunc("GET /rest/api/3/issue/createmeta/"+project+"/issuetypes/"+issueTypeID, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"startAt":0,"maxResults":50,"total":`+total+`,"fields":`+fieldsJSON+`}`)
	})
}

// countTopLevelObjects counts the top-level {...} objects in a JSON
// array literal so the mock's reported "total" matches the field count
// it serves — the pagination loop stops once startAt reaches total.
func countTopLevelObjects(arr string) int {
	depth, count := 0, 0
	for _, r := range arr {
		switch r {
		case '{':
			if depth == 0 {
				count++
			}
			depth++
		case '}':
			depth--
		}
	}
	return count
}
