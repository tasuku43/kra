package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

type cliJSONError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type cliJSONResponse struct {
	OK          bool          `json:"ok"`
	Action      string        `json:"action"`
	WorkspaceID string        `json:"workspace_id,omitempty"`
	Result      any           `json:"result,omitempty"`
	Warnings    any           `json:"warnings,omitempty"`
	Error       *cliJSONError `json:"error,omitempty"`
}

func writeCLIJSON(w io.Writer, payload cliJSONResponse) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
