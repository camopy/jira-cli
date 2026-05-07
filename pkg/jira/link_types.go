package jira

import (
	"context"
	"net/http"
)

type IssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
	Self    string `json:"self,omitempty"`
}

type IssueLinkTypeService interface {
	List(context.Context) ([]IssueLinkType, *Response, error)
}

type issueLinkTypeService struct {
	client *Client
}

func NewIssueLinkTypeService(client *Client) IssueLinkTypeService {
	return &issueLinkTypeService{client: client}
}

func (s *issueLinkTypeService) List(ctx context.Context) ([]IssueLinkType, *Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodGet, "rest/api/3/issueLinkType", nil)
	if err != nil {
		return nil, nil, err
	}
	var page struct {
		IssueLinkTypes []IssueLinkType `json:"issueLinkTypes"`
	}
	resp, err := s.client.Do(req, &page)
	return page.IssueLinkTypes, resp, err
}
