package jira

func nextOffset(startAt, valuesLen, requestedPageSize, responsePageSize int) int {
	if responsePageSize > 0 {
		return startAt + responsePageSize
	}
	if requestedPageSize > 0 {
		return startAt + requestedPageSize
	}
	if valuesLen > 0 {
		return startAt + valuesLen
	}
	return startAt + 1
}
