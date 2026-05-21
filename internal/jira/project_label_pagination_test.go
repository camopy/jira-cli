package jira

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProjectServiceListStopsAtDefaultPageBound(t *testing.T) {
	calls := 0
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/project/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		calls++
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"startAt":` + strconv.Itoa(startAt) + `,
			"maxResults":1,
			"isLast":false,
			"values":[{"id":"` + strconv.Itoa(calls) + `","key":"P` + strconv.Itoa(calls) + `","name":"Project ` + strconv.Itoa(calls) + `"}]
		}`))
	}))

	svc := NewProjectService(client, time.Minute)
	_, _, err := svc.List(context.Background(), &ListOptions{MaxResults: 1})
	if err == nil {
		t.Fatal("List returned nil error for never-ending project pagination")
	}
	if !strings.Contains(err.Error(), "project pagination exceeded default bounds") {
		t.Fatalf("List error = %v", err)
	}
	if calls != defaultMaxPages {
		t.Fatalf("calls = %d, want %d", calls, defaultMaxPages)
	}
}

func TestProjectServiceListContinuesPastEmptyNonFinalPage(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/project/search" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		w.Header().Set("Content-Type", "application/json")
		switch startAt {
		case 0:
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":50,"isLast":false,"values":[{"id":"1","key":"ENG","name":"Engineering"}]}`))
		case 50:
			_, _ = w.Write([]byte(`{"startAt":50,"maxResults":50,"isLast":false,"values":[]}`))
		case 100:
			_, _ = w.Write([]byte(`{"startAt":100,"maxResults":50,"isLast":true,"values":[{"id":"2","key":"OPS","name":"Operations"}]}`))
		default:
			t.Fatalf("unexpected startAt %d", startAt)
		}
	}))

	svc := NewProjectService(client, time.Minute)
	projects, _, err := svc.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 2 || projects[0].Key != "ENG" || projects[1].Key != "OPS" {
		t.Fatalf("projects = %+v, want ENG and OPS", projects)
	}
}

func TestLabelServiceListStopsAtDefaultPageBound(t *testing.T) {
	calls := 0
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/label" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		calls++
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"startAt":` + strconv.Itoa(startAt) + `,
			"maxResults":1,
			"isLast":false,
			"values":["label-` + strconv.Itoa(calls) + `"]
		}`))
	}))

	svc := NewLabelService(client)
	_, _, err := svc.List(context.Background(), &ListOptions{MaxResults: 1})
	if err == nil {
		t.Fatal("List returned nil error for never-ending label pagination")
	}
	if !strings.Contains(err.Error(), "label pagination exceeded default bounds") {
		t.Fatalf("List error = %v", err)
	}
	if calls != defaultMaxPages {
		t.Fatalf("calls = %d, want %d", calls, defaultMaxPages)
	}
}

func TestLabelServiceListContinuesPastEmptyNonFinalPage(t *testing.T) {
	client := newHTTPHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/label" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		startAt, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		w.Header().Set("Content-Type", "application/json")
		switch startAt {
		case 0:
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1000,"isLast":false,"values":["one"]}`))
		case 1000:
			_, _ = w.Write([]byte(`{"startAt":1000,"maxResults":1000,"isLast":false,"values":[]}`))
		case 2000:
			_, _ = w.Write([]byte(`{"startAt":2000,"maxResults":1000,"isLast":true,"values":["two"]}`))
		default:
			t.Fatalf("unexpected startAt %d", startAt)
		}
	}))

	svc := NewLabelService(client)
	labels, _, err := svc.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(labels) != 2 || labels[0] != "one" || labels[1] != "two" {
		t.Fatalf("labels = %+v, want [one two]", labels)
	}
}
