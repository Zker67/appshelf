// Package watch tells when what is installed may have changed outside
// AppShelf: a registry key of installed programs, or a directory as
// scoop's apps, changed.
package watch

import (
	"os"
	"path/filepath"
)

// Key is a registry key to watch.
type Key struct {
	// Machine is HKEY_LOCAL_MACHINE, else HKEY_CURRENT_USER; X86 its 32-bit
	// view.
	Machine, X86 bool
	Path         string
	// Subtree watches the subkeys too; Values watches values changing,
	// besides subkeys added and removed.
	Subtree, Values bool
}

const uninstall = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// InstalledKeys are the keys that change when programs are installed,
// updated or removed: the three Uninstall keys, and the user's repository
// of app packages.
func InstalledKeys() []Key {
	return []Key{
		{Machine: true, Path: uninstall, Subtree: true, Values: true},
		{Machine: true, X86: true, Path: uninstall, Subtree: true, Values: true},
		{Path: uninstall, Subtree: true, Values: true},
		{Path: `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`},
	}
}

// ScoopDirs are scoop's apps directories, the user's and the global one,
// those that exist.
func ScoopDirs() []string {
	var out []string
	user := os.Getenv("SCOOP")
	if user == "" {
		if home, err := os.UserHomeDir(); err == nil {
			user = filepath.Join(home, "scoop")
		}
	}
	global := os.Getenv("SCOOP_GLOBAL")
	if global == "" && os.Getenv("ProgramData") != "" {
		global = filepath.Join(os.Getenv("ProgramData"), "scoop")
	}
	for _, root := range []string{user, global} {
		if root == "" {
			continue
		}
		dir := filepath.Join(root, "apps")
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			out = append(out, dir)
		}
	}
	return out
}
