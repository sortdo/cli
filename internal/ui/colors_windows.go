package ui

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableColors turns on escape sequences in the Windows console. Consoles older than Windows 10 get plain text.
func enableColors(file *os.File) bool {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
