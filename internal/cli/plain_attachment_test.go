package cli

import (
	"testing"
	"time"
)

// Human byte sizes must use a unit label consistent with the divisor.
// The formatter uses a binary IEC divisor, so labels must be KiB/MiB,
// never decimal SI labels.
func TestAttachmentHumanBytesUsesBinaryUnitsForBinaryDivisor(t *testing.T) {
	const kib int64 = 1 << 10
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{kib, "1.00 KiB"},
		{kib + kib/2, "1.50 KiB"},
		{kib * kib, "1.00 MiB"},
		{5 * kib * kib * kib, "5.00 GiB"},
	}
	for _, c := range cases {
		if got := attachmentHumanBytes(c.n); got != c.want {
			t.Fatalf("attachmentHumanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// A timestamp in the future (clock skew, a bad upstream value) must not
// render as a negative relative age like "-5m ago". The renderer falls
// back to the absolute date instead.
func TestAttachmentHumanCreatedHandlesFutureTimestamp(t *testing.T) {
	future := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	got := attachmentHumanCreated(future)
	if got == "" {
		t.Fatal("future timestamp produced empty output")
	}
	if got[0] == '-' || got == "just now" {
		t.Fatalf("future timestamp rendered as a negative/now age: %q", got)
	}
}

func TestAttachmentHumanCreatedUsesReferenceTime(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-2 * time.Hour).Format(time.RFC3339)
	got := attachmentHumanCreatedFrom(ts, now)
	if got != "2h ago" {
		t.Fatalf("attachmentHumanCreatedFrom = %q, want 2h ago", got)
	}
}
