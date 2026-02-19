package cli

import (
	"strings"
	"unicode"
)

const (
	agentTerminalSequenceMaxPayload = 8 * 1024
	agentRuntimeSignalMaxDetails    = 256
)

type agentRuntimeSignalEvent struct {
	Name      string
	StateHint string
	Details   string
}

type agentTerminalSequenceParser struct {
	escPending bool
	inOSC      bool
	oscEsc     bool
	oscPayload []byte
}

func newAgentTerminalSequenceParser() *agentTerminalSequenceParser {
	return &agentTerminalSequenceParser{}
}

func (p *agentTerminalSequenceParser) Feed(payload []byte) []agentRuntimeSignalEvent {
	if len(payload) == 0 {
		return nil
	}
	out := make([]agentRuntimeSignalEvent, 0, 2)
	for _, b := range payload {
		if p.inOSC {
			if p.oscEsc {
				if b == '\\' {
					if ev, ok := parseOSCPayload(p.oscPayload); ok {
						out = append(out, ev)
					}
					p.resetOSC()
					continue
				}
				p.appendOSCByte(0x1b)
				p.oscEsc = false
			}
			switch b {
			case 0x07: // BEL terminator
				if ev, ok := parseOSCPayload(p.oscPayload); ok {
					out = append(out, ev)
				}
				p.resetOSC()
			case 0x1b:
				p.oscEsc = true
			default:
				p.appendOSCByte(b)
			}
			continue
		}

		if p.escPending {
			p.escPending = false
			if b == ']' {
				p.inOSC = true
				p.oscEsc = false
				p.oscPayload = p.oscPayload[:0]
				continue
			}
			if b == 0x1b {
				p.escPending = true
				continue
			}
			if b == 0x07 {
				out = append(out, agentRuntimeSignalEvent{Name: "bell"})
				continue
			}
			continue
		}

		switch b {
		case 0x1b:
			p.escPending = true
		case 0x07:
			out = append(out, agentRuntimeSignalEvent{Name: "bell"})
		}
	}
	return out
}

func (p *agentTerminalSequenceParser) appendOSCByte(b byte) {
	if len(p.oscPayload) < agentTerminalSequenceMaxPayload {
		p.oscPayload = append(p.oscPayload, b)
	}
}

func (p *agentTerminalSequenceParser) resetOSC() {
	p.inOSC = false
	p.oscEsc = false
	p.oscPayload = p.oscPayload[:0]
}

func parseOSCPayload(raw []byte) (agentRuntimeSignalEvent, bool) {
	if len(raw) == 0 {
		return agentRuntimeSignalEvent{}, false
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return agentRuntimeSignalEvent{}, false
	}
	code, rest := splitFirst(s, ';')
	switch code {
	case "9":
		return agentRuntimeSignalEvent{
			Name:      "osc_9_notify",
			StateHint: "waiting_input",
			Details:   sanitizeSignalDetails(rest),
		}, true
	case "777":
		sub, body := splitFirst(rest, ';')
		if strings.EqualFold(strings.TrimSpace(sub), "notify") {
			return agentRuntimeSignalEvent{
				Name:      "osc_777_notify",
				StateHint: "waiting_input",
				Details:   sanitizeSignalDetails(body),
			}, true
		}
		return agentRuntimeSignalEvent{
			Name:    "osc_777",
			Details: sanitizeSignalDetails(rest),
		}, true
	case "133":
		op, body := splitFirst(rest, ';')
		switch strings.TrimSpace(strings.ToUpper(op)) {
		case "C":
			return agentRuntimeSignalEvent{
				Name:      "osc_133_c",
				StateHint: "running",
				Details:   sanitizeSignalDetails(body),
			}, true
		case "D":
			return agentRuntimeSignalEvent{
				Name:      "osc_133_d",
				StateHint: "waiting_input",
				Details:   sanitizeSignalDetails(body),
			}, true
		case "A":
			return agentRuntimeSignalEvent{Name: "osc_133_a"}, true
		case "B":
			return agentRuntimeSignalEvent{Name: "osc_133_b"}, true
		default:
			return agentRuntimeSignalEvent{
				Name:    "osc_133",
				Details: sanitizeSignalDetails(rest),
			}, true
		}
	default:
		return agentRuntimeSignalEvent{}, false
	}
}

func applyRuntimeStateHints(current string, events []agentRuntimeSignalEvent) string {
	state := normalizeAgentRuntimeStateValue(current)
	for _, ev := range events {
		switch strings.TrimSpace(strings.ToLower(ev.StateHint)) {
		case "running":
			state = "running"
		case "waiting_input":
			state = "waiting_input"
		case "idle":
			state = "idle"
		}
	}
	return state
}

func normalizeAgentRuntimeStateValue(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "running":
		return "running"
	case "idle":
		return "idle"
	case "waiting_input":
		return "waiting_input"
	case "exited":
		return "exited"
	default:
		return "unknown"
	}
}

func sanitizeSignalDetails(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	clean := strings.TrimSpace(b.String())
	if len(clean) > agentRuntimeSignalMaxDetails {
		return clean[:agentRuntimeSignalMaxDetails]
	}
	return clean
}

func splitFirst(s string, sep rune) (string, string) {
	idx := strings.IndexRune(s, sep)
	if idx < 0 {
		return strings.TrimSpace(s), ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])
}
