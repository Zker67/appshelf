package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// installDate turns the registry's YYYYMMDD into YYYY-MM-DD; anything else
// is unknown.
func installDate(s string) string {
	if len(s) != 8 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	if s[:2] != "19" && s[:2] != "20" || s[4:6] < "01" || s[4:6] > "12" || s[6:] < "01" || s[6:] > "31" {
		return ""
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:]
}

// fillLocation guesses the install location of a program whose key has
// none: the directory of its icon, else of its uninstaller, when that is a
// directory of its own.
func fillLocation(e *Entry) {
	if e.InstallLocation != "" {
		return
	}
	for _, cmd := range []string{e.DisplayIcon, e.UninstallString} {
		file := CommandFile(cmd)
		if file == "" {
			continue
		}
		dir := filepath.Dir(file)
		if !OwnDirectory(dir) {
			continue
		}
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			e.InstallLocation, e.LocationGuessed = dir, true
			return
		}
	}
}

// CommandFile returns the program a command line runs, or an icon's file:
// quoted, or the longest prefix ending in .exe that exists, else the first
// word. ",index" after an icon's file is left out.
func CommandFile(cmd string) string {
	cmd = strings.TrimSpace(ExpandEnv(cmd))
	if cmd == "" {
		return ""
	}
	if cmd[0] == '"' {
		if end := strings.IndexByte(cmd[1:], '"'); end >= 0 {
			return cmd[1 : end+1]
		}
		return strings.Trim(cmd, `"`)
	}
	lower := strings.ToLower(cmd)
	best := ""
	for i := 0; ; {
		j := strings.Index(lower[i:], ".exe")
		if j < 0 {
			break
		}
		end := i + j + len(".exe")
		if end == len(cmd) || cmd[end] == ' ' || cmd[end] == ',' {
			cand := cmd[:end]
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				best = cand
			} else if best == "" && !strings.Contains(cand, " ") {
				best = cand
			}
		}
		i = end
	}
	if best != "" {
		return best
	}
	if i := strings.IndexByte(cmd, ','); i > 0 && !strings.Contains(cmd[:i], " ") {
		// An icon: "C:\App\app.ico,0".
		return cmd[:i]
	}
	if i := strings.IndexByte(cmd, ' '); i > 0 {
		return cmd[:i]
	}
	return cmd
}

// OwnDirectory reports whether dir can be a program's own directory: not a
// drive, Windows, a directory every program shares, or where installers
// keep their caches and copies.
func OwnDirectory(dir string) bool {
	d := strings.ToLower(filepath.Clean(dir))
	if len(d) <= 3 || !filepath.IsAbs(d) {
		return false
	}
	for _, shared := range SharedDirectories() {
		if d == shared {
			return false
		}
	}
	win := strings.ToLower(filepath.Clean(os.Getenv("SystemRoot")))
	if win != "." && (d == win || strings.HasPrefix(d, win+`\`)) {
		return false
	}
	for _, part := range []string{`\package cache\`, `\installer\`, `\temp\`, `\installshield installation information\`, `\microsoft\edge\`} {
		if strings.Contains(d+`\`, part) {
			return false
		}
	}
	return true
}

// SharedDirectories are the directories many programs share, in lower case:
// Program Files, ProgramData, the user's profile and its AppData.
func SharedDirectories() []string {
	var out []string
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "CommonProgramFiles", "CommonProgramFiles(x86)", "ProgramData", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PUBLIC", "SystemDrive"} {
		if p := os.Getenv(v); p != "" {
			out = append(out, strings.ToLower(filepath.Clean(p)))
		}
	}
	if p := os.Getenv("LOCALAPPDATA"); p != "" {
		for _, sub := range []string{"Programs", `Programs\Common`, "Packages", "Microsoft", "Temp"} {
			out = append(out, strings.ToLower(filepath.Join(p, sub)))
		}
	}
	if p := os.Getenv("APPDATA"); p != "" {
		out = append(out, strings.ToLower(filepath.Join(p, "Microsoft")))
	}
	return out
}
