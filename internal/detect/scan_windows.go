//go:build windows

package detect

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// Scan lists the programs under the Uninstall keys of the machine, its
// 32-bit view (WOW6432Node), and the user, leaving out system components
// and updates of other programs.
func Scan() ([]Entry, error) {
	views := []struct {
		root        registry.Key
		name        string
		access      uint32
		scope, arch string
	}{
		{registry.LOCAL_MACHINE, `HKLM`, registry.WOW64_64KEY, "Machine", "X64"},
		{registry.LOCAL_MACHINE, `HKLM (32 位)`, registry.WOW64_32KEY, "Machine", "X86"},
		{registry.CURRENT_USER, `HKCU`, 0, "User", "X64"},
	}
	var out []Entry
	seen := map[string]bool{}
	read := 0
	for _, v := range views {
		k, err := registry.OpenKey(v.root, uninstallPath, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE|v.access)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("读取注册表 %s 失败：%w", v.name, err)
		}
		names, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			return nil, fmt.Errorf("读取注册表 %s 失败：%w", v.name, err)
		}
		read++
		for _, name := range names {
			sub, err := registry.OpenKey(v.root, uninstallPath+`\`+name, registry.QUERY_VALUE|v.access)
			if err != nil {
				continue
			}
			e, ok := readEntry(sub)
			sub.Close()
			if !ok {
				continue
			}
			e.Key = v.name + `\` + name
			e.Name, e.Scope, e.Arch = name, v.scope, v.arch
			id := strings.ToLower(e.DisplayName + "\x00" + e.DisplayVersion + "\x00" + name)
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, e)
		}
	}
	if read == 0 {
		return nil, errors.New("找不到注册表的 Uninstall 键")
	}
	return out, nil
}

func readEntry(k registry.Key) (Entry, bool) {
	name, _, err := k.GetStringValue("DisplayName")
	if err != nil || strings.TrimSpace(name) == "" {
		return Entry{}, false
	}
	if n, _, err := k.GetIntegerValue("SystemComponent"); err == nil && n == 1 {
		return Entry{}, false
	}
	if p, _, err := k.GetStringValue("ParentKeyName"); err == nil && p != "" {
		return Entry{}, false
	}
	str := func(name string) string {
		v, _, _ := k.GetStringValue(name)
		return strings.TrimSpace(v)
	}
	num := func(name string) uint64 {
		v, _, _ := k.GetIntegerValue(name)
		return v
	}
	e := Entry{
		DisplayName:          strings.TrimSpace(name),
		DisplayVersion:       str("DisplayVersion"),
		Publisher:            str("Publisher"),
		DisplayIcon:          str("DisplayIcon"),
		InstallLocation:      str("InstallLocation"),
		UninstallString:      str("UninstallString"),
		QuietUninstallString: str("QuietUninstallString"),
		WindowsInstaller:     num("WindowsInstaller") == 1,
		NoRemove:             num("NoRemove") == 1,
		InstallDate:          installDate(str("InstallDate")),
		InstallSource:        str("InstallSource"),
	}
	// EstimatedSize is in KiB; some installers write nonsense.
	if kb := num("EstimatedSize"); kb > 0 && kb < 1<<32 {
		e.EstimatedSize = int64(kb) << 10
	}
	fillLocation(&e)
	if strings.Contains(e.InstallLocation, "~") {
		e.InstallLocation = longPath(e.InstallLocation)
	}
	return e, true
}

// longPath expands an 8.3 path, as C:\PROGRA~1, to its long form, for paths
// to compare; a path that cannot be expanded stays.
func longPath(p string) string {
	in, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetLongPathName(in, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}

// Lookup reads one program's key again: scope "Machine" or "User", arch
// "X64" or "X86", and the key's name. ok is false when the key is gone or
// is not a program Scan lists.
func Lookup(scope, arch, name string) (Entry, bool, error) {
	root, access := registry.CURRENT_USER, uint32(0)
	switch {
	case strings.EqualFold(scope, "Machine") && strings.EqualFold(arch, "X86"):
		root, access = registry.LOCAL_MACHINE, registry.WOW64_32KEY
	case strings.EqualFold(scope, "Machine"):
		root, access = registry.LOCAL_MACHINE, registry.WOW64_64KEY
	}
	if name == "" || strings.ContainsAny(name, `\/`) {
		return Entry{}, false, errors.New("注册表键名无效")
	}
	k, err := registry.OpenKey(root, uninstallPath+`\`+name, registry.QUERY_VALUE|access)
	if errors.Is(err, registry.ErrNotExist) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("读取注册表失败：%w", err)
	}
	defer k.Close()
	e, ok := readEntry(k)
	if !ok {
		return Entry{}, false, nil
	}
	e.Name, e.Scope, e.Arch = name, scope, arch
	return e, true, nil
}
