package components

import "strings"

type Tabs struct {
	Items  []string
	Active int
}

func (t Tabs) Render() string {
	if len(t.Items) == 0 {
		return ""
	}
	items := make([]string, len(t.Items))
	copy(items, t.Items)
	if t.Active >= 0 && t.Active < len(items) {
		items[t.Active] = "[" + items[t.Active] + "]"
	}
	return strings.Join(items, " ")
}
