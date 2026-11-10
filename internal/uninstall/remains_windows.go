//go:build windows

package uninstall

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// pendingRenames returns the files Windows removes or replaces when it
// restarts: PendingFileRenameOperations, which uninstallers fill with the
// files in use they could not delete.
func pendingRenames() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	list, _, err := k.GetStringsValue("PendingFileRenameOperations")
	if err != nil {
		return nil
	}
	var out []string
	for _, s := range list {
		// Pairs of a file and where it goes, "" to delete it; as \??\C:\…
		s = strings.TrimPrefix(strings.TrimPrefix(s, "!"), `\??\`)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
