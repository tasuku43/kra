package cli

import "testing"

func TestCLI_IsExperimentEnabled(t *testing.T) {
	t.Setenv(experimentsEnvKey, "foo, Insight-Capture ,bar")
	c := New(nil, nil)

	if !c.isExperimentEnabled("insight-capture") {
		t.Fatalf("insight-capture should be enabled")
	}
	if !c.isExperimentEnabled("INSIGHT-CAPTURE") {
		t.Fatalf("INSIGHT-CAPTURE should be enabled case-insensitively")
	}
	if c.isExperimentEnabled("unknown") {
		t.Fatalf("unknown should not be enabled")
	}
}
