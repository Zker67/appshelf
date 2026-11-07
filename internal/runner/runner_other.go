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
