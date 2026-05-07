package unit

import (
	"testing"
	"time"

	"github.com/matcra587/jira-cli/internal/refresh"
)

func TestRefreshTickerDefaultsPauseResumeAndBackoff(t *testing.T) {
	ticker := refresh.NewTicker(0)
	if ticker.Interval() != 30*time.Second {
		t.Fatalf("default interval = %v", ticker.Interval())
	}
	ticker.Pause()
	if !ticker.Paused() {
		t.Fatal("ticker not paused")
	}
	ticker.Resume()
	if ticker.Paused() {
		t.Fatal("ticker still paused")
	}
	ticker.BackoffRateLimit(2 * time.Second)
	if ticker.Interval() <= 30*time.Second {
		t.Fatalf("backoff interval = %v", ticker.Interval())
	}
}
