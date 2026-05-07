package components

import "fmt"

type Header struct {
	Profile string
	Tab     string
}

func (h Header) Render() string {
	if h.Profile == "" {
		h.Profile = "default"
	}
	return fmt.Sprintf("jira  profile:%s  tab:%s", h.Profile, h.Tab)
}
