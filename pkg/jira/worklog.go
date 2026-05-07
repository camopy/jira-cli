package jira

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"unicode"

	"github.com/matcra587/jira-cli/pkg/adf"
)

type WorklogService interface {
	List(context.Context, string, *ListOptions) ([]*Worklog, *Response, error)
	Add(context.Context, string, *WorklogAddRequest) (*Worklog, *Response, error)
	Delete(context.Context, string, string) (*Response, error)
}

type worklogService struct {
	client *Client
}

type WorklogAddRequest struct {
	TimeSpentSeconds int           `json:"timeSpentSeconds"`
	Started          string        `json:"started,omitempty"`
	Comment          *adf.Document `json:"comment,omitempty"`
	DryRun           bool          `json:"-"`
}

func NewWorklogService(client *Client) WorklogService {
	return &worklogService{client: client}
}

func (s *worklogService) List(ctx context.Context, issueKey string, opts *ListOptions) ([]*Worklog, *Response, error) {
	path := "rest/api/3/issue/" + issueKey + "/worklog"
	if opts != nil {
		if q := opts.QueryValues().Encode(); q != "" {
			path += "?" + q
		}
	}
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var result struct {
		Worklogs []*Worklog `json:"worklogs"`
	}
	resp, err := s.client.Do(req, &result)
	return result.Worklogs, resp, err
}

func (s *worklogService) Add(ctx context.Context, issueKey string, reqBody *WorklogAddRequest) (*Worklog, *Response, error) {
	if reqBody == nil || reqBody.TimeSpentSeconds <= 0 {
		return nil, nil, errors.New("positive time spent is required")
	}
	if reqBody.DryRun {
		return &Worklog{ID: String("DRY-RUN"), TimeSpentSeconds: Int(reqBody.TimeSpentSeconds)}, &Response{IsLast: true}, nil
	}
	req, err := s.client.NewRequest(ctx, http.MethodPost, "rest/api/3/issue/"+issueKey+"/worklog", reqBody)
	if err != nil {
		return nil, nil, err
	}
	var worklog Worklog
	resp, err := s.client.Do(req, &worklog)
	return &worklog, resp, err
}

func (s *worklogService) Delete(ctx context.Context, issueKey, worklogID string) (*Response, error) {
	req, err := s.client.NewRequest(ctx, http.MethodDelete, "rest/api/3/issue/"+issueKey+"/worklog/"+worklogID, nil)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

func ParseDuration(input string, workdaySeconds int) (int, error) {
	if workdaySeconds <= 0 {
		workdaySeconds = DefaultWorkdaySeconds
	}
	total := 0
	i := 0
	for i < len(input) {
		if input[i] == '-' {
			return 0, errors.New("duration must be positive")
		}
		start := i
		for i < len(input) && unicode.IsDigit(rune(input[i])) {
			i++
		}
		if start == i || i >= len(input) {
			return 0, errors.New("invalid duration")
		}
		n, _ := strconv.Atoi(input[start:i])
		unit := input[i]
		i++
		switch unit {
		case 'd':
			total += n * workdaySeconds
		case 'h':
			total += n * 3600
		case 'm':
			total += n * 60
		default:
			return 0, errors.New("unsupported duration unit")
		}
	}
	if total <= 0 {
		return 0, errors.New("duration must be positive")
	}
	return total, nil
}

const DefaultWorkdaySeconds = 28800
