//go:build windows

package procs

import (
	"errors"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Paths returns the executables of the processes running, those this user
// can query; processes of other users and the system are left out.
func Paths() ([]string, error) {
	list, err := Snapshot()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range list {
		if path, ok := imagePath(p.PID); ok {
			out = append(out, path)
		}
	}
	return out, nil
}

// Snapshot lists the processes running, those of other users and the
// system too.
func Snapshot() ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var out []Proc
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID == 0 {
			continue
		}
		out = append(out, Proc{PID: entry.ProcessID, Parent: entry.ParentProcessID, Name: windows.UTF16ToString(entry.ExeFile[:])})
	}
	return out, nil
}

// Path returns the executable of a process, when this user can query it.
func Path(pid uint32) (string, bool) { return imagePath(pid) }

func imagePath(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:n]), true
}

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessIoCounters = kernel32.NewProc("GetProcessIoCounters")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")

	user32                  = windows.NewLazySystemDLL("user32.dll")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procPostMessageW        = user32.NewProc("PostMessageW")
)

// ioCounters is IO_COUNTERS.
type ioCounters struct {
	ReadOperations, WriteOperations, OtherOperations uint64
	ReadBytes, WriteBytes, OtherBytes                uint64
}

// UsageOf returns how much a process has worked since it started, when
// this user can query it.
func UsageOf(pid uint32) (Usage, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Usage{}, false
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return Usage{}, false
	}
	var io ioCounters
	if r, _, _ := procGetProcessIoCounters.Call(uintptr(h), uintptr(unsafe.Pointer(&io))); r == 0 {
		return Usage{}, false
	}
	ticks := func(f windows.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return Usage{
		CPU: time.Duration(ticks(kernel)+ticks(user)) * 100, // 100 ns ticks
		IO:  io.ReadBytes + io.WriteBytes + io.OtherBytes,
	}, true
}

// EnumWindows calls back into Go; the callbacks are made once, as Windows
// keeps a limited number of them.
var (
	enumMu    sync.Mutex
	enumFound []windows.HWND
	enumAdd   = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		enumFound = append(enumFound, h)
		return 1
	})
)

func enumerate(parent windows.HWND) []windows.HWND {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFound = nil
	if parent == 0 {
		_ = windows.EnumWindows(enumAdd, nil)
	} else {
		windows.EnumChildWindows(parent, enumAdd, nil)
	}
	out := enumFound
	enumFound = nil
	return out
}

const (
	gwlExStyle      = -20
	wsExToolWindow  = 0x00000080
	wmGetText       = 0x000D
	smtoBlock       = 0x0001
	smtoAbortIfHung = 0x0002
	swRestore       = 9
)

// Windows returns the visible top-level windows of the processes, with
// their titles and, where Windows lets this user read them, their texts.
func Windows(pids map[uint32]bool) []Window {
	if len(pids) == 0 {
		return nil
	}
	var out []Window
	for _, h := range enumerate(0) {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(h, &pid); err != nil || !pids[pid] || !shown(h) {
			continue
		}
		w := Window{PID: pid, Handle: uintptr(h), Title: windowTitle(h)}
		for _, c := range enumerate(h) {
			if len(w.Text) >= 6 {
				break
			}
			if !windows.IsWindowVisible(c) || !isLabel(c) {
				continue
			}
			if s := controlText(c); s != "" && s != w.Title {
				w.Text = append(w.Text, s)
			}
		}
		out = append(out, w)
	}
	return out
}

// shown reports a window the user sees on the taskbar or the screen: not a
// tool window, not empty.
func shown(h windows.HWND) bool {
	if !windows.IsWindowVisible(h) {
		return false
	}
	ex, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyleArg())
	if ex&wsExToolWindow != 0 {
		return false
	}
	var r struct{ Left, Top, Right, Bottom int32 }
	if ok, _, _ := procGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}
	return r.Right-r.Left > 0 && r.Bottom-r.Top > 0
}

// gwlExStyleArg passes GWL_EXSTYLE, a negative int, as the uintptr Call
// takes.
func gwlExStyleArg() uintptr {
	v := int32(gwlExStyle)
	return uintptr(v)
}

func windowTitle(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return strings.TrimSpace(windows.UTF16ToString(buf[:n]))
}

// isLabel reports a control showing text to read: Static, and the static
// texts of Delphi programs as Inno Setup (TNewStaticText).
func isLabel(h windows.HWND) bool {
	buf := make([]uint16, 64)
	n, err := windows.GetClassName(h, &buf[0], int32(len(buf)))
	if err != nil || n == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(windows.UTF16ToString(buf[:n])), "static")
}

// controlText reads a control of another program, waiting at most 200
// milliseconds for a program that does not answer.
func controlText(h windows.HWND) string {
	buf := make([]uint16, 512)
	var copied uintptr
	r, _, _ := procSendMessageTimeoutW.Call(uintptr(h), wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])),
		smtoBlock|smtoAbortIfHung, 200, uintptr(unsafe.Pointer(&copied)))
	if r == 0 || copied == 0 {
		return ""
	}
	return strings.Join(strings.Fields(windows.UTF16ToString(buf[:min(int(copied), len(buf))])), " ")
}

// Front brings a window to the front, restoring it when minimized. Windows
// allows it while AppShelf's own window is in front.
func Front(handle uintptr) bool {
	if iconic, _, _ := procIsIconic.Call(handle); iconic != 0 {
		procShowWindow.Call(handle, swRestore)
	}
	r, _, _ := procSetForegroundWindow.Call(handle)
	return r != 0
}

// wmClose asks a window to close, as its close button does.
const wmClose = 0x0010

// CloseWindow asks a window to close, as its close button does, without
// waiting: its program may first ask to save.
func CloseWindow(handle uintptr) bool {
	r, _, _ := procPostMessageW.Call(handle, wmClose, 0, 0)
	return r != 0
}

// BootTime is when Windows last started.
func BootTime() time.Time {
	ms, _, _ := procGetTickCount64.Call()
	return time.Now().Add(-time.Duration(ms) * time.Millisecond)
}

// End ends a process. A process of an administrator, or of another user,
// gives ErrDenied.
func End(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return ErrDenied
		}
		return err
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return ErrDenied
		}
		return err
	}
	return nil
}
