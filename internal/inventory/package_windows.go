//go:build windows

package inventory

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetPackagePathByFullName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetPackagePathByFullName")

// PackagePath is the directory of an installed app package, by its full
// name; "" when it is not installed or cannot be found.
func PackagePath(fullName string) string {
	if fullName == "" || procGetPackagePathByFullName.Find() != nil {
		return ""
	}
	name, err := windows.UTF16PtrFromString(fullName)
	if err != nil {
		return ""
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	r, _, _ := procGetPackagePathByFullName.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 || n == 0 || int(n) > len(buf) {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
