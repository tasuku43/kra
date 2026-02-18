//go:build !windows

package cli

import (
	"errors"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/unix"
)

// flushTerminalInputBuffer discards pending stdin bytes at terminal handoff
// boundaries so selector/attach keypress leftovers do not leak to the next mode.
func flushTerminalInputBuffer(in io.Reader) {
	inFile, ok := in.(*os.File)
	if !ok {
		return
	}
	if !isatty.IsTerminal(inFile.Fd()) {
		return
	}

	fd := int(inFile.Fd())
	if err := unix.SetNonblock(fd, true); err != nil {
		return
	}
	defer func() {
		_ = unix.SetNonblock(fd, false)
	}()

	buf := make([]byte, 256)
	for {
		n, err := unix.Read(fd, buf)
		if n > 0 {
			continue
		}
		if err == nil {
			return
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			return
		}
		return
	}
}
