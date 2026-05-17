package jira

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
)

type IssueLinkService interface {
	List(context.Context, string) ([]IssueLinkView, *Response, error)
	Create(context.Context, *IssueLinkRequest) (*Response, error)
	Delete(context.Context, string) (*Response, error)
}

type IssueLinkView struct {
	ID         string        `json:"id"`
	Self       string        `json:"self,omitempty"`
	Type       IssueLinkType `json:"type"`
	Direction  string        `json:"direction"`
	OtherIssue IssueRef      `json:"other_issue"`
}

type IssueRef struct {
	Key     string `json:"key"`
	Summary string `json:"summary,omitempty"`
	Status  string `json:"status,omitempty"`
}

type issueLinkService struct {
	client *Client
}

func NewIssueLinkService(client *Client) IssueLinkService {
	return &issueLinkService{client: client}
}

func (s *issueLinkService) List(ctx context.Context, key string) ([]IssueLinkView, *Response, error) {
	if strings.TrimSpace(key) == "" {
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

func (s *issueLinkService) Create(ctx context.Context, reqBody *IssueLinkRequest) (*Response, error) {
	if reqBody == nil || reqBody.Type == "" || reqBody.InwardIssue == "" || reqBody.OutwardIssue == "" {
		return nil, errors.New("issue link create: type, inwardIssue, and outwardIssue are required")
	}
	body := map[string]any{
		"type":         map[string]string{"name": reqBody.Type},
		"inwardIssue":  map[string]string{"key": reqBody.InwardIssue},
		"outwardIssue": map[string]string{"key": reqBody.OutwardIssue},
	}
	req, err := s.client.NewRequest(ctx, http.MethodPost, RESTPath("issueLink"), body)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

func (s *issueLinkService) Delete(ctx context.Context, linkID string) (*Response, error) {
	if strings.TrimSpace(linkID) == "" {
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
