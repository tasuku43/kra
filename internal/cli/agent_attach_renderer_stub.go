//go:build !vterm

package cli

import (
	"io"
	"net"
)

func proxyAgentAttachIOWithVTerm(
	root string,
	sessionID string,
	conn *net.UnixConn,
	in io.Reader,
	out io.Writer,
	mode agentAttachMode,
) error {
	return errAgentAttachRendererUnavailable
}
