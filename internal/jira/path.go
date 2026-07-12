package jira

import (
	"net/url"
	"strings"
)

// RESTPath builds a Jira Cloud REST v3 path by prefixing parts with the
// rest/api/3 base and percent-escaping each segment. Escaping is why callers
// pass raw values (issue keys, ids) as separate parts rather than pre-joining:
// a segment containing a slash or reserved character stays a single path
// element and cannot break out of the intended resource.
func RESTPath(parts ...string) string {
	segments := make([]string, 0, len(parts)+3)
	segments = append(segments, "rest", "api", "3")
	segments = append(segments, parts...)
	return joinPathSegments(segments...)
}

func withQuery(path string, q url.Values) string {
	if encoded := q.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func joinPathSegments(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		escaped = append(escaped, pathSegment(part))
	}
	return strings.Join(escaped, "/")
}

func pathSegment(value string) string {
	switch value {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	default:
		return url.PathEscape(value)
	}
}
