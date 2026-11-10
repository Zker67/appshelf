//go:build windows

package runner

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrElevationCanceled reports that the user declined the administrator
// prompt of an installer.
var ErrElevationCanceled = errors.New("已取消管理员授权")

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo is SHELLEXECUTEINFOW; Go aligns its fields as C does.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
	swShowNormal          = 1
)

// ShellRun starts file with params as Explorer would, so an installer that
// requires administrator rights asks for them through UAC rather than
// failing, then waits for it to exit and returns its exit code. params is
// passed as written, as the installer's command line.
func ShellRun(file, params, dir string) (uint32, error) {
	return shellWait(shellStart("open", file, params, dir))
}

// ShellRunElevated is ShellRun asking for administrator rights first, for
// a program that cannot ask for them itself, as msiexec without its window.
func ShellRunElevated(file, params, dir string) (uint32, error) {
	return shellWait(shellStart("runas", file, params, dir))
}

// ShellStart starts file as ShellRun does, asking for administrator rights
// first when elevate is set, and returns at once: the caller waits for the
// process, and closes it.
func ShellStart(file, params, dir string, elevate bool) (*Process, error) {
	verb := "open"
	if elevate {
		verb = "runas"
	}
	return shellStart(verb, file, params, dir)
}

// Process is a program ShellStart started.
type Process struct {
	handle windows.Handle
	pid    uint32
}

// PID is the process's ID.
func (p *Process) PID() uint32 { return p.pid }

// Exited reports whether the process has exited and then its exit code,
// without waiting.
func (p *Process) Exited() (code uint32, done bool, err error) {
	ev, err := windows.WaitForSingleObject(p.handle, 0)
	if err != nil {
		return 0, false, err
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return 0, false, nil
	}
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return 0, true, err
	}
	return code, true, nil
}

// Close lets the process go; it keeps running.
func (p *Process) Close() { windows.CloseHandle(p.handle) }

func shellWait(p *Process, err error) (uint32, error) {
	if err != nil {
		return 0, err
	}
	defer p.Close()
	if _, err := windows.WaitForSingleObject(p.handle, windows.INFINITE); err != nil {
		return 0, err
	}
	code, _, err := p.Exited()
	return code, err
}

func shellStart(verbName, file, params, dir string) (*Process, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}

	verb, _ := windows.UTF16PtrFromString(verbName)
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return nil, err
	}
	var p, d *uint16
	if params != "" {
		if p, err = windows.UTF16PtrFromString(params); err != nil {
			return nil, err
		}
	}
	if dir != "" {
		if d, err = windows.UTF16PtrFromString(dir); err != nil {
			return nil, err
		}
	}
	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       f,
		lpParameters: p,
		lpDirectory:  d,
		nShow:        swShowNormal,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return nil, ErrElevationCanceled
		}
		return nil, fmt.Errorf("无法启动安装程序：%w", callErr)
	}
	if info.hProcess == 0 {
		return nil, errors.New("安装程序没有返回进程，无法等待它结束")
	}
	pid, _ := windows.GetProcessId(info.hProcess)
	return &Process{handle: info.hProcess, pid: pid}, nil
}

// InstallerBusy reports whether Windows Installer is installing or
// removing something now: it holds the _MSIExecute mutex meanwhile.
func InstallerBusy() bool {
	name, _ := windows.UTF16PtrFromString(`Global\_MSIExecute`)
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err == nil {
		windows.CloseHandle(h)
		return true
	}
	// The mutex is there, held by the system, when access is denied.
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
