package jira

import (
	"context"
	"errors"
	"net/http"
	"sort"

	xstrings "github.com/gechr/x/strings"
)

// IssueLinkService reads and writes the links between issues. List flattens
// Jira's nested issuelinks into a directional view; Create and Delete manage
// individual links.
type IssueLinkService interface {
	List(context.Context, string) ([]IssueLinkView, *Response, error)
	Create(context.Context, *IssueLinkRequest) (*Response, error)
	Delete(context.Context, string) (*Response, error)
}

// IssueLinkView is one link flattened for display: the CLI's own shape, not a
// Jira wire body. Jira returns each link once with the far end in an inward or
// outward slot; flattening turns that into one row per link with Direction
// ("inward"/"outward") naming which relationship phrase (Type.Inward vs
// Type.Outward) applies and OtherIssue naming the far end.
type IssueLinkView struct {
	ID         string        `json:"id"`
	Self       string        `json:"self,omitempty"`
	Type       IssueLinkType `json:"type"`
	Direction  string        `json:"direction"`
	OtherIssue IssueRef      `json:"other_issue"`
}

// IssueRef is the minimal reference to the issue at the far end of a link: key,
// summary, and status name, enough to render a link without a second fetch.
type IssueRef struct {
	Key     string `json:"key"`
	Summary string `json:"summary,omitempty"`
	Status  string `json:"status,omitempty"`
}

// Sentence renders the line selfKey's page displays for this link: the
// type's outward phrase when the link points outward from selfKey, the
// inward phrase otherwise. It is the read-side twin of
// IssueLinkType.PreviewSentences — the two must agree so a create preview
// matches what `issue link list` later reports.
func (v IssueLinkView) Sentence(selfKey string) string {
	phrase := v.Type.Inward
	if v.Direction == "outward" {
		phrase = v.Type.Outward
	}
	return selfKey + " " + phrase + " " + v.OtherIssue.Key
}

type issueLinkService struct {
	client *Client
}

// NewIssueLinkService constructs an IssueLinkService bound to the given client.
func NewIssueLinkService(client *Client) IssueLinkService {
	return &issueLinkService{client: client}
}

// List returns the issue's links, flattened to one row per direction and sorted
// for stable output. Links come back embedded in the issue body under the
// issuelinks field — there is no standalone list endpoint — so this reads the
// issue with a fields=issuelinks selector.
func (s *issueLinkService) List(ctx context.Context, key string) ([]IssueLinkView, *Response, error) {
	if xstrings.IsBlank(key) {
		return nil, nil, errors.New("issue link list: key is required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodGet, RESTPath("issue", key)+"?fields=issuelinks", nil)
	if err != nil {
		return nil, nil, err
	}
	var raw struct {
		Fields struct {
			IssueLinks []issueLinkWire `json:"issuelinks"`
		} `json:"fields"`
	}
	resp, err := s.client.Do(req, &raw)
	if err != nil {
		return nil, resp, err
	}
	views := flattenIssueLinks(raw.Fields.IssueLinks)
	SortIssueLinks(views)
	return views, resp, nil
}

// Create links two issues. The link type may be named (Type) or given by id
// (TypeID); one of them plus both issue keys are required. Which issue is inward
// versus outward determines how the link reads, so callers assign them
// deliberately.
func (s *issueLinkService) Create(ctx context.Context, reqBody *IssueLinkRequest) (*Response, error) {
	if reqBody == nil || xstrings.AllEmpty(reqBody.Type, reqBody.TypeID) || xstrings.AnyEmpty(reqBody.InwardIssue, reqBody.OutwardIssue) {
		return nil, errors.New("issue link create: type, inwardIssue, and outwardIssue are required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodPost, RESTPath("issueLink"), reqBody.payload())
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

// Delete removes a single link by its link id (not an issue key) — the id from
// an IssueLinkView.
func (s *issueLinkService) Delete(ctx context.Context, linkID string) (*Response, error) {
	if xstrings.IsBlank(linkID) {
		return nil, errors.New("issue link delete: linkID is required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodDelete, RESTPath("issueLink", linkID), nil)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

type issueLinkWire struct {
	ID           string         `json:"id"`
	Self         string         `json:"self"`
	Type         issueLinkType  `json:"type"`
	InwardIssue  *issueLinkSide `json:"inwardIssue,omitempty"`
	OutwardIssue *issueLinkSide `json:"outwardIssue,omitempty"`
}

type issueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
	Self    string `json:"self,omitempty"`
}

type issueLinkSide struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
	} `json:"fields"`
}

func flattenIssueLinks(wires []issueLinkWire) []IssueLinkView {
	if len(wires) == 0 {
		return []IssueLinkView{}
	}
	out := make([]IssueLinkView, 0, len(wires))
	for _, w := range wires {
		linkType := IssueLinkType{
			ID:      w.Type.ID,
			Name:    w.Type.Name,
			Inward:  w.Type.Inward,
			Outward: w.Type.Outward,
			Self:    w.Type.Self,
		}
		if w.OutwardIssue != nil {
			out = append(out, IssueLinkView{
				ID:         w.ID,
				Self:       w.Self,
				Type:       linkType,
				Direction:  "outward",
				OtherIssue: IssueRef{Key: w.OutwardIssue.Key, Summary: w.OutwardIssue.Fields.Summary, Status: w.OutwardIssue.Fields.Status.Name},
			})
		}
		if w.InwardIssue != nil {
			out = append(out, IssueLinkView{
				ID:         w.ID,
				Self:       w.Self,
				Type:       linkType,
				Direction:  "inward",
				OtherIssue: IssueRef{Key: w.InwardIssue.Key, Summary: w.InwardIssue.Fields.Summary, Status: w.InwardIssue.Fields.Status.Name},
			})
		}
	}
	return out
}

// SortIssueLinks orders links deterministically by direction, then link-type
// name, then far-end key, so repeated reads render in a stable order. It sorts
// in place.
func SortIssueLinks(links []IssueLinkView) {
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].Direction != links[j].Direction {
			return links[i].Direction < links[j].Direction
		}
		if links[i].Type.Name != links[j].Type.Name {
			return links[i].Type.Name < links[j].Type.Name
		}
		return links[i].OtherIssue.Key < links[j].OtherIssue.Key
	})
}
