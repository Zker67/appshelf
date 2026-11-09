// Package usage tells when the user last opened each installed program,
// from what Windows records of the programs started from the Start menu,
// the taskbar, the desktop and Explorer (UserAssist), and from the
// programs running now.
//
// Windows records no launch from a command line, at sign-in or by another
// program, nor any once the user turned off tracking app launches: a
// program without a record may still be used.
package usage

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"time"

	"appshelf/internal/detect"
)

// Launch is a program the user started, as Windows recorded it.
type Launch struct {
	// Path is the program's file; "" when only AppID is known.
	Path string
	// AppID is an AppUserModelID: "<PackageFamilyName>!<App>" of an app
	// package, or one a desktop program gives itself.
	AppID string
	At    time.Time
}

// Record is what Read found.
type Record struct {
	Launches []Launch
	// Running are the executables of the processes running.
	Running []string
	// At is when it was read: a running program counts as opened then.
	At time.Time
}

// Tracked reports whether Windows records launches at all: none means the
// user turned tracking off, or never opened anything.
func (r Record) Tracked() bool { return len(r.Launches) > 0 }

// Program is what tells a program's launches apart.
type Program struct {
	// Dirs are the program's own directories; those detect.OwnDirectory
	// rejects are left out.
	Dirs []string
	// Family is the family name of its app package.
	Family string
	// ScoopName is its name among scoop's apps.
	ScoopName string
}

// LastUsed returns when p was last opened, zero when no record says.
func (r Record) LastUsed(p Program) time.Time {
	var dirs []string
	for _, d := range p.Dirs {
		if d = clean(d); d != "" && detect.OwnDirectory(d) {
			dirs = append(dirs, strings.TrimSuffix(d, `\`)+`\`)
		}
	}
	family := strings.ToLower(p.Family)
	scoop := ""
	if p.ScoopName != "" {
		scoop = `\scoop\apps\` + strings.ToLower(p.ScoopName) + `\`
	}
	if len(dirs) == 0 && family == "" && scoop == "" {
		return time.Time{}
	}
	owns := func(path string) bool {
		path = clean(path)
		if path == "" {
			return false
		}
		for _, d := range dirs {
			if strings.HasPrefix(path, d) {
				return true
			}
		}
		if scoop != "" && strings.Contains(path, scoop) {
			return true
		}
		return family != "" && packageFamily(path) == family
	}
	for _, p := range r.Running {
		if owns(p) {
			return r.At
		}
	}
	var last time.Time
	for _, l := range r.Launches {
		hit := false
		switch {
		case l.Path != "":
			hit = !isUninstaller(l.Path) && owns(l.Path)
		case family != "":
			app, _, ok := strings.Cut(strings.ToLower(l.AppID), "!")
			hit = ok && app == family
		}
		if hit && l.At.After(last) {
			last = l.At
		}
	}
	return last
}

// packageFamily returns the family name of the app package a file is in,
// under WindowsApps\Name_Version_Arch_ResourceID_PublisherID\, in lower
// case; "" for other files.
func packageFamily(path string) string {
	_, rest, ok := strings.Cut(path, `\windowsapps\`)
	if !ok {
		return ""
	}
	dir, _, _ := strings.Cut(rest, `\`)
	parts := strings.Split(dir, "_")
	if len(parts) < 5 {
		return ""
	}
	return parts[0] + "_" + parts[len(parts)-1]
}

// isUninstaller reports an uninstaller's file: opening it is no use.
func isUninstaller(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(base, "unins") || strings.HasPrefix(base, "uninst")
}

// clean makes a Windows path comparable: backslashes, lower case; "" for
// what is not an absolute path.
func clean(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if len(p) < 3 || p[1] != ':' {
		return ""
	}
	return strings.ToLower(filepath.Clean(strings.ReplaceAll(p, "/", `\`)))
}

// rot13 decodes UserAssist's value names.
func rot13(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = 'a' + (c-'a'+13)%26
		case c >= 'A' && c <= 'Z':
			b[i] = 'A' + (c-'A'+13)%26
		}
	}
	return string(b)
}

// lastRun reads the time of the last run in a UserAssist value of
// Windows 7 and later: 72 bytes, a FILETIME at 60. Zero when it has none.
func lastRun(data []byte) time.Time {
	if len(data) < 68 {
		return time.Time{}
	}
	ft := int64(binary.LittleEndian.Uint64(data[60:68]))
	if ft <= 0 {
		return time.Time{}
	}
	// FILETIME counts 100 ns from 1601-01-01; Unix time from 1970.
	const epoch = 116444736000000000
	if ft < epoch {
		return time.Time{}
	}
	return time.Unix(0, (ft-epoch)*100)
}

// entryName is what a UserAssist value names, its name decoded.
type entryName struct {
	// Folder is the Known Folder GUID the path starts with, "{…}"; Rest
	// the path below it, or the whole path when Folder is "".
	Folder, Rest string
	// AppID is an AppUserModelID, when the name is no path.
	AppID string
}

// parseName tells a path, maybe under a Known Folder, from an
// AppUserModelID. Windows' own counters (UEME_…) are neither.
func parseName(name string) (entryName, bool) {
	switch {
	case name == "" || strings.HasPrefix(name, "UEME_"):
		return entryName{}, false
	case strings.HasPrefix(name, "{"):
		end := strings.Index(name, "}")
		if end < 0 {
			return entryName{}, false
		}
		return entryName{Folder: name[:end+1], Rest: strings.TrimPrefix(name[end+1:], `\`)}, true
	case len(name) > 2 && name[1] == ':':
		return entryName{Rest: name}, true
	case strings.ContainsAny(name, `\/`):
		return entryName{}, false
	}
	return entryName{AppID: name}, true
}

// isShortcut reports a shortcut's path.
func isShortcut(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".lnk")
}
