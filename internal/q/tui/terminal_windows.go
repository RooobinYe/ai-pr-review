//go:build windows

package tui

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

const enableVirtualTerminalProcessing = 0x0004

func enableVirtualTerminal(out io.Writer) (func() error, error) {
	file, ok := out.(*os.File)
	if !ok || file == nil {
		return nil, nil
	}

	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil, err
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return func() error { return nil }, nil
	}

	if err := windows.SetConsoleMode(handle, mode|enableVirtualTerminalProcessing); err != nil {
		return nil, err
	}

	return func() error {
		return windows.SetConsoleMode(handle, mode)
	}, nil
}
