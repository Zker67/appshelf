//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps console programs such as winget and PowerShell from
// opening a console window over the app; the installers they start still
// show their own windows.
const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
