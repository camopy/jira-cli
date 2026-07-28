package jira

import (
	"context"
	"net/http"
)

// IssueLinkType mirrors an entry from GET /rest/api/3/issueLinkType. Name is
// the type's identity ("Blocks"), while Inward and Outward are the two
// human-readable phrases the same type reads as depending on direction ("is
// blocked by" / "blocks") — a link command needs both to render and to accept
// either phrasing.
type IssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
	Self    string `json:"self,omitempty"`
}

// PreviewSentences returns the sentence each endpoint's page will display
// once a link of this type joins inwardKey and outwardKey. The mapping is
// the crossover that makes link direction easy to get backwards: the inward
// issue's page shows the type's OUTWARD phrase (the link points outward
// from it toward the other issue), and the outward issue's page shows the
// INWARD phrase. For Blocks (inward "is blocked by", outward "blocks"),
// linking A to B renders "A blocks B" on A and "B is blocked by A" on B.
func (t IssueLinkType) PreviewSentences(inwardKey, outwardKey string) (inwardIssueSentence, outwardIssueSentence string) {
	return inwardKey + " " + t.Outward + " " + outwardKey,
		outwardKey + " " + t.Inward + " " + inwardKey
}

// IssueLinkTypeService lists the link types configured on the instance,
// typically to validate or complete a --type value before creating a link.
type IssueLinkTypeService interface {
	List(context.Context) ([]IssueLinkType, *Response, error)
}

type issueLinkTypeService struct {
	client *Client
}

// NewIssueLinkTypeService constructs an IssueLinkTypeService bound to the given
// client.
func NewIssueLinkTypeService(client *Client) IssueLinkTypeService {
	return &issueLinkTypeService{client: client}
}

// List returns every issue link type configured on the instance. The response
// is unpaged — Jira returns the full set in one envelope object.
func (s *issueLinkTypeService) List(ctx context.Context) ([]IssueLinkType, *Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, RESTPath("issueLinkType"), nil)
	if err != nil {
		return nil, nil, err
	}
	var page struct {
		IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
	}
	resp, err := s.client.Do(req, &page)
	return page.IssueLinkTypes, resp, err
}
