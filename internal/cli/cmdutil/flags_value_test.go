package cmdutil

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestFlagValueHelpersPreservePflagZeroValueFallback(t *testing.T) {
	fs := pflag.NewFlagSet("values", pflag.ContinueOnError)
	fs.Bool("debug", false, "")
	fs.String("profile", "", "")
	if err := fs.Set("debug", "true"); err != nil {
		t.Fatalf("Set(debug): %v", err)
	}
	if err := fs.Set("profile", "work"); err != nil {
		t.Fatalf("Set(profile): %v", err)
	}

	if !BoolValue(fs, "debug") {
		t.Fatal("BoolValue(debug) = false, want true")
	}
	if got := StringValue(fs, "profile"); got != "work" {
		t.Fatalf("StringValue(profile) = %q, want work", got)
	}
	if BoolValue(fs, "missing") || BoolValue(fs, "profile") {
		t.Fatal("BoolValue must return false for missing or non-bool flags")
	}
	if got := StringValue(fs, "missing"); got != "" {
		t.Fatalf("StringValue(missing) = %q, want empty", got)
	}
	if got := StringValue(fs, "debug"); got != "" {
		t.Fatalf("StringValue(debug) = %q, want empty for non-string flag", got)
	}
}
