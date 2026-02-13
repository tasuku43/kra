package cli

import "testing"

func TestCLI_IsExperimentEnabled(t *testing.T) {
	t.Setenv(experimentsEnvKey, "foo, Agent ,bar")
	c := New(nil, nil)

	if !c.isExperimentEnabled("agent") {
		t.Fatalf("agent should be enabled")
	}
	if !c.isExperimentEnabled("AGENT") {
		t.Fatalf("AGENT should be enabled case-insensitively")
	}
	if c.isExperimentEnabled("unknown") {
		t.Fatalf("unknown should not be enabled")
	}
}
