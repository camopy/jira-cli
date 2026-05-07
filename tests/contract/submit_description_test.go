package contract

import (
	"strings"
	"testing"
)

// The `submit` capability has registry-specific semantics. Each row's
// submit_description MUST disambiguate which registry's submit it refers
// to:
//   - ADF rows describe rich-text payload submission, mentioning the
//     ADF validation pipeline.
//   - Customfield rows describe REST field payload submission,
//     mentioning the customfield/schema validation pipeline.
//
// This test reads the agent surfaces and asserts the descriptive text
// references the relevant validation pipeline so an agent doesn't have
// to read prose to disambiguate.
func TestSubmitDescriptionsAreRegistrySpecific(t *testing.T) {
	bin := buildJiraBinary(t)

	adfRows := loadAgentRows(t, bin, "adf-matrix")
	cfRows := loadAgentRows(t, bin, "fieldtypes")

	for _, row := range adfRows {
		desc, _ := row["submit_description"].(string)
		if desc == "" {
			t.Errorf("adf-matrix %v missing submit_description", row["name"])
			continue
		}
		// ADF semantics MUST mention ADF validation.
		if !strings.Contains(strings.ToLower(desc), "adf") {
			t.Errorf("adf-matrix %v submit_description should mention ADF validation: %q", row["name"], desc)
		}
		if strings.Contains(strings.ToLower(desc), "customfield") {
			t.Errorf("adf-matrix %v submit_description leaks customfield reference: %q", row["name"], desc)
		}
	}

	for _, row := range cfRows {
		desc, _ := row["submit_description"].(string)
		if desc == "" {
			t.Errorf("fieldtypes %v missing submit_description", row["name"])
			continue
		}
		// Customfield semantics MUST mention customfield/schema validation.
		if !strings.Contains(strings.ToLower(desc), "customfield") && !strings.Contains(strings.ToLower(desc), "schema") {
			t.Errorf("fieldtypes %v submit_description should mention customfield/schema validation: %q", row["name"], desc)
		}
		if strings.Contains(strings.ToLower(desc), "adf") {
			t.Errorf("fieldtypes %v submit_description leaks ADF reference: %q", row["name"], desc)
		}
	}
}
