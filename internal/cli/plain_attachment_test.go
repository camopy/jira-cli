package cli

import (
	"testing"
	"time"
)

// Human byte sizes must use a unit label consistent with the divisor.
// The renderer divides by 1024, so the label must be the binary IEC
// unit (KiB/MiB), never the decimal SI label (KB/MB) — a 1024-divisor
// with a "KB" label misreports the size.
func TestAttachmentHumanBytesUsesBinaryUnitsForBinaryDivisor(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{5 * 1024 * 1024 * 1024, "5.0 GiB"},
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
