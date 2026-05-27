package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// verifyCredential pings /rest/api/3/myself with a Jira Cloud credential
// pair and returns the authenticated user, so `auth login` can confirm a
// token works before reporting success.
func TestVerifyCredentialReturnsUserOnSuccess(t *testing.T) {
	var gotUser, gotPass, gotPath string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, gotOK = r.BasicAuth()
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"5b10ac","displayName":"Test User","emailAddress":"user@example.com"}`))
	}))
	defer srv.Close()

	user, err := verifyCredential(context.Background(), srv.URL, "user@example.com", "tok123", 0)
	if err != nil {
		t.Fatalf("verifyCredential() error = %v", err)
	}
	if user.AccountID != "5b10ac" || user.DisplayName != "Test User" {
		t.Fatalf("verifyCredential() user = %+v", user)
	}
	if !gotOK || gotUser != "user@example.com" || gotPass != "tok123" {
		t.Fatalf("request basic auth = %q/%q ok=%v, want user@example.com/tok123", gotUser, gotPass, gotOK)
	}
	if gotPath != "/rest/api/3/myself" {
		t.Fatalf("request path = %q, want /rest/api/3/myself", gotPath)
	}
}

// A rejected token must surface as an error so the login fails instead of
// persisting an unusable credential.
func TestVerifyCredentialErrorsOnRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	defer srv.Close()

	if _, err := verifyCredential(context.Background(), srv.URL, "user@example.com", "wrong", 0); err == nil {
		t.Fatal("verifyCredential() error = nil, want an auth error for a rejected token")
	}
}

// verifyCredential must bound its request by the profile's configured
// timeout, matching every other client in the CLI, so login verification
// against a stalled server fails fast instead of hanging on the default.
func TestVerifyCredentialHonorsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(8 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	done := make(chan error, 1)
	go func() {
		_, err := verifyCredential(context.Background(), srv.URL, "user@example.com", "tok123", 1*time.Second)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("verifyCredential() error = nil, want a timeout against a stalled server")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verifyCredential did not honor the 1s timeout — it hung on the default client timeout")
	}
}
