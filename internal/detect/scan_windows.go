//go:build windows

package detect

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// Scan lists the programs under the Uninstall keys of the machine, its
// 32-bit view (WOW6432Node), and the user, leaving out system components
// and updates of other programs.
func Scan() ([]Entry, error) {
	views := []struct {
		root   registry.Key
		name   string
		access uint32
	}{
		{registry.LOCAL_MACHINE, `HKLM`, registry.WOW64_64KEY},
		{registry.LOCAL_MACHINE, `HKLM (32 位)`, registry.WOW64_32KEY},
		{registry.CURRENT_USER, `HKCU`, 0},
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
	ver, _, _ := k.GetStringValue("DisplayVersion")
	pub, _, _ := k.GetStringValue("Publisher")
	return Entry{DisplayName: strings.TrimSpace(name), DisplayVersion: strings.TrimSpace(ver), Publisher: strings.TrimSpace(pub)}, true
}
