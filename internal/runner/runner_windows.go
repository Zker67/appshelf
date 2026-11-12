//go:build windows

package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

// createNoWindow keeps console programs such as winget and PowerShell from
// opening a console window over the app; the installers they start still
// show their own windows.
const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}

// endTree makes ctx ending a command end what it started too, as aria2
// under scoop or an installer under winget: taskkill /T ends the tree.
func endTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		kill := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe"), "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		hideConsole(kill)
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
