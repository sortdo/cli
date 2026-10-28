package selfupdate

import (
	"os"
	"os/exec"
	"syscall"
)

// Remove deletes the program. A running program cannot be deleted on Windows, so it moves aside to <name>.old, and
// cmd deletes that a moment after sortdo has exited.
func Remove(executable string) error {
	old := executable + ".old"
	_ = os.Remove(old)
	if err := os.Rename(executable, old); err != nil {
		return err
	}
	command := exec.Command("cmd.exe")
	command.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /c ping -n 3 127.0.0.1 >nul & del /f /q "` + old + `"`, HideWindow: true}
	_ = command.Start()
	return nil
}
