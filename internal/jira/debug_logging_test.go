package jira

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gechr/clog"
)

func newDebugLogContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()

	var logs bytes.Buffer
	logger := clog.New(clog.NewOutput(&logs, clog.ColorNever))
	logger.SetLevel(clog.LevelDebug)

	return logger.WithContext(context.Background()), &logs
}

func newHTTPHandlerClient(handler http.Handler, opts ...Option) *Client {
	options := []Option{
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			serverReq := req.Clone(req.Context())
			serverReq.RequestURI = req.URL.RequestURI()
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, serverReq)

			res := recorder.Result()
			res.Request = req
			return res, nil
		})}),
	}
	options = append(options, opts...)
	return NewClient(options...)
}
