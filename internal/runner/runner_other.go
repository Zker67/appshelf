//go:build !windows

package runner

import (
	"errors"
	"os/exec"
)

// ErrElevationCanceled reports that the user declined the administrator
// prompt of an installer.
var ErrElevationCanceled = errors.New("已取消管理员授权")

func hideConsole(*exec.Cmd) {}

// ShellRun starts Windows installers, which other systems do not run.
func ShellRun(file, params, dir string) (uint32, error) {
	return 0, errors.ErrUnsupported
}

// ShellRunElevated starts Windows installers, which other systems do not run.
func ShellRunElevated(file, params, dir string) (uint32, error) {
	return 0, errors.ErrUnsupported
}

// ShellStart starts Windows installers, which other systems do not run.
func ShellStart(file, params, dir string, elevate bool) (*Process, error) {
	return nil, errors.ErrUnsupported
}

// Process is a program ShellStart started; other systems have none.
type Process struct{}

// PID is the process's ID.
func (p *Process) PID() uint32 { return 0 }

// Exited reports whether the process has exited.
func (p *Process) Exited() (uint32, bool, error) { return 0, true, errors.ErrUnsupported }

// Close lets the process go.
func (p *Process) Close() {}

// InstallerBusy reports whether Windows Installer is busy; other systems
// have none.
func InstallerBusy() bool { return false }
