package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

func TestAuthStatusFailsTopLevelWhenRemoteProbeFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/3/myself", "/rest/api/3/mypermissions":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	bin := buildJiraBinary(t)
	cfg := jiraConfig(t, srv.URL)
	cmd := exec.Command(bin, "--config", cfg, "--output=json", "auth", "status")
	cmd.Env = append(os.Environ(), "JIRA_TOKEN_DEFAULT=bogus")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("auth status succeeded despite remote 401:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Profiles []struct {
				Profile string `json:"profile"`
				Valid   bool   `json:"valid"`
				Remote  struct {
					Myself struct {
						OK     bool `json:"ok"`
						Status int  `json:"status"`
					} `json:"myself"`
				} `json:"remote"`
			} `json:"profiles"`
		} `json:"data"`
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"errors"`
	}
	decodeErrorEnvelopeFromStderr(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	if env.OK {
		t.Fatalf("top-level ok should be false for remote auth failure:\n%s", stderr.String())
	}
	if len(env.Errors) == 0 {
		t.Fatalf("top-level errors should summarize unhealthy auth:\n%s", stderr.String())
	}
	if len(env.Data.Profiles) != 1 || env.Data.Profiles[0].Valid {
		t.Fatalf("profile validity should track failed remote probe:\n%+v", env.Data.Profiles)
	}
	if env.Data.Profiles[0].Remote.Myself.OK || env.Data.Profiles[0].Remote.Myself.Status != http.StatusUnauthorized {
		t.Fatalf("remote myself probe not reflected in data:\n%+v", env.Data.Profiles[0].Remote.Myself)
	}
}

func TestAuthStatusFailsTopLevelWhenCredentialMissing(t *testing.T) {
	bin := buildJiraBinary(t)
	cfg := jiraConfig(t, "https://jira.example.invalid")
	cmd := exec.Command(bin, "--config", cfg, "--output=json", "auth", "status")
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("auth status succeeded despite missing credential:\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Profiles []struct {
				Profile string `json:"profile"`
				Valid   bool   `json:"valid"`
				Error   string `json:"error"`
			} `json:"profiles"`
		} `json:"data"`
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	decodeErrorEnvelopeFromStderr(t, stdout.Bytes(), stderr.Bytes(), cmd.Args, &env)
	if env.OK {
		t.Fatalf("top-level ok should be false for missing credential:\n%s", stderr.String())
	}
	if len(env.Errors) == 0 {
		t.Fatalf("top-level errors should summarize missing credential:\n%s", stderr.String())
	}
	if len(env.Data.Profiles) != 1 || env.Data.Profiles[0].Valid || env.Data.Profiles[0].Error == "" {
		t.Fatalf("profile missing credential should be visible in data:\n%+v", env.Data.Profiles)
	}
}
