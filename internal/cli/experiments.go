package cli

import (
	"os"
	"strings"
)

const experimentsEnvKey = "KRA_EXPERIMENTS"

const (
	experimentInsightCapture = "insight-capture"
	experimentAgentSkillpack = "agent-skillpack"
)

func (c *CLI) isExperimentEnabled(name string) bool {
	needle := strings.ToLower(strings.TrimSpace(name))
	if needle == "" {
		return false
	}
	raw := strings.TrimSpace(os.Getenv(experimentsEnvKey))
	if raw == "" {
		return false
	}
	for _, token := range strings.Split(raw, ",") {
		if strings.ToLower(strings.TrimSpace(token)) == needle {
			return true
		}
	}
	return false
}
