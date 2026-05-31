//go:build windows

package tui

import "golang.org/x/sys/windows"

func isTTY(r any) bool {
	fd, ok := extractFD(r)
	if !ok {
		return false
	}
	if hasCharDevice(r) {
		return true
	}

	var mode uint32
	err := windows.GetConsoleMode(windows.Handle(fd), &mode)
	return err == nil
}
