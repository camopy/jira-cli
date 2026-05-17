package jira

import (
	"net/url"
	"strconv"
	"strings"
)

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

func JQLValue(value string) string {
	return strconv.Quote(value)
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
