//go:build windows

package cli

import "io"

func flushTerminalInputBuffer(in io.Reader) {}
