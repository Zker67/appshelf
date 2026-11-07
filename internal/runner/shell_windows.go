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
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}

	verb, _ := windows.UTF16PtrFromString("open")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	var p, d *uint16
	if params != "" {
		if p, err = windows.UTF16PtrFromString(params); err != nil {
			return 0, err
		}
	}
	if dir != "" {
		if d, err = windows.UTF16PtrFromString(dir); err != nil {
			return 0, err
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
			return 0, ErrElevationCanceled
		}
		return 0, fmt.Errorf("无法启动安装程序：%w", callErr)
	}
	if info.hProcess == 0 {
		return 0, errors.New("安装程序没有返回进程，无法等待它结束")
	}
	defer windows.CloseHandle(info.hProcess)
	if _, err := windows.WaitForSingleObject(info.hProcess, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, err
	}
	return code, nil
}
