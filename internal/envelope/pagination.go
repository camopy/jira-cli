package envelope

// Pagination is the canonical pagination block, identical on every paginated
// command: meta.pagination on single-target reads, and the same shape inside
// keyed multi-key results[].data. It lives in this leaf package so typed
// Output structs can carry it without importing the renderer layer;
// internal/cli aliases it for its many existing call sites.
type Pagination struct {
	StartAt    int    `json:"startAt"` //nolint:revive // pagination-exempt
	MaxResults int    `json:"maxResults"`
	Total      *int   `json:"total,omitempty"`
	IsLast     bool   `json:"isLast"`
	NextCursor string `json:"nextCursor,omitempty"`
}
