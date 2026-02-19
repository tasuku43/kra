//go:build vterm

package cli

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	vterm "github.com/mattn/go-libvterm"
)

func proxyAgentAttachIOWithVTerm(
	root string,
	sessionID string,
	conn *net.UnixConn,
	in io.Reader,
	out io.Writer,
	mode agentAttachMode,
) error {
	if conn == nil {
		return fmt.Errorf("broker connection is nil")
	}
	if mode.fullscreen && isTerminalWriter(out) {
		writeAttachTerminalEnter(out)
	} else if mode.clearOnEnter && isTerminalWriter(out) {
		writeAttachTerminalClear(out)
	}
	if !mode.fullscreen && mode.writeBoundary && isTerminalWriter(out) {
		writeAttachSessionBoundary(out)
	}
	if mode.flushInput && isTerminalReader(in) {
		flushTerminalInputBuffer(in)
	}

	restore, err := maybeEnterRawMode(in, out)
	if err != nil {
		return err
	}
	if mode.flushInput && isTerminalReader(in) {
		defer flushTerminalInputBuffer(in)
	}
	if restore != nil {
		defer restore()
	}
	if mode.restoreShell && isTerminalWriter(out) {
		defer writeAttachTerminalRestore(out)
	}

	cols, rows := terminalSize(in, out)
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	vt := vterm.New(rows, cols)
	if vt == nil {
		return fmt.Errorf("init vterm: %w", errAgentAttachRendererUnavailable)
	}
	defer func() { _ = vt.Close() }()
	vt.SetUTF8(true)
	screen := vt.ObtainScreen()
	screen.Reset(true)

	stopResizeWatcher := startAttachResizeWatcher(root, sessionID, in, out)
	defer stopResizeWatcher()

	readErrCh := make(chan error, 1)
	chunkCh := make(chan []byte, 32)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, rerr := conn.Read(buf)
			if n > 0 {
				payload := append([]byte(nil), buf[:n]...)
				chunkCh <- payload
			}
			if rerr != nil {
				close(chunkCh)
				readErrCh <- rerr
				return
			}
		}
	}()

	inputResCh := make(chan attachInputResult, 1)
	go func() {
		inputResCh <- forwardAttachInput(conn, in, mode.localDetach)
	}()

	var sigintCh chan os.Signal
	if mode.localDetach {
		sigintCh = make(chan os.Signal, 1)
		signal.Notify(sigintCh, os.Interrupt, syscall.SIGINT)
		defer signal.Stop(sigintCh)
	}

	for {
		select {
		case <-sigintCh:
			_ = conn.Close()
			return errAgentAttachDetached
		case inputRes := <-inputResCh:
			if inputRes.detached {
				_ = conn.Close()
				return errAgentAttachDetached
			}
			if isAgentAttachIOError(inputRes.err) {
				_ = conn.Close()
				return inputRes.err
			}
		case chunk, ok := <-chunkCh:
			if !ok {
				readErr := <-readErrCh
				if isAgentAttachIOError(readErr) {
					return readErr
				}
				return nil
			}
			if len(chunk) == 0 {
				continue
			}
			curCols, curRows := terminalSize(in, out)
			if curCols > 0 && curRows > 0 && (curCols != cols || curRows != rows) {
				cols = curCols
				rows = curRows
				vt.SetSize(rows, cols)
			}
			_, _ = vt.Write(chunk)
			_ = screen.Flush()
			renderVTermScreen(out, screen, rows, cols)
		}
	}
}

func renderVTermScreen(out io.Writer, screen *vterm.Screen, rows int, cols int) {
	if out == nil || screen == nil || rows <= 0 || cols <= 0 {
		return
	}
	var b strings.Builder
	b.Grow((rows * cols) + (rows * 4) + 16)
	b.WriteString("\r\x1b[2J\x1b[H")
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell, err := screen.GetCellAt(r, c)
			if err != nil || cell == nil {
				b.WriteByte(' ')
				continue
			}
			chars := cell.Chars()
			if len(chars) == 0 || chars[0] == 0 {
				b.WriteByte(' ')
				continue
			}
			b.WriteRune(chars[0])
		}
		if r != rows-1 {
			b.WriteByte('\n')
		}
	}
	_, _ = io.WriteString(out, b.String())
}
