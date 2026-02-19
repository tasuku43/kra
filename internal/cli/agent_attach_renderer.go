package cli

import (
	"io"
	"net"
	"strings"
)

func normalizeAgentAttachRenderer(v string) string {
	normalized := strings.ToLower(strings.TrimSpace(v))
	switch normalized {
	case "", attachRendererAuto:
		return attachRendererAuto
	case attachRendererRaw:
		return attachRendererRaw
	case attachRendererVTerm:
		return attachRendererVTerm
	default:
		return normalized
	}
}

func proxyAgentAttachIOWithRenderer(
	root string,
	sessionID string,
	conn *net.UnixConn,
	in io.Reader,
	out io.Writer,
	mode agentAttachMode,
	renderer string,
) error {
	switch normalizeAgentAttachRenderer(renderer) {
	case attachRendererVTerm:
		return proxyAgentAttachIOWithVTerm(root, sessionID, conn, in, out, mode)
	case attachRendererAuto:
		return proxyAgentAttachIOWithVTerm(root, sessionID, conn, in, out, mode)
	default:
		return errAgentAttachRendererUnavailable
	}
}
