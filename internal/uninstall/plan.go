// Package uninstall removes installed programs: those of a package manager
// through it (winget, scoop, Remove-AppxPackage for app packages), the
// others by the uninstall command of their registry key, quietly when the
// installer that made it is known to allow it.
//
// The rules that tell the installer and its quiet switches follow Bulk Crap
// Uninstaller (UninstallerTypeAdder, InnoSetupQuietUninstallStringGenerator;
// Apache-2.0); see references/external-repos.
package uninstall

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"appshelf/internal/detect"
)

// Method is how a program is removed.
type Method string

const (
	// ByWinget runs winget uninstall, from the winget or the msstore source.
	ByWinget Method = "winget"
	// ByScoop runs scoop uninstall.
	ByScoop Method = "scoop"
	// ByStore removes an app package with Remove-AppxPackage.
	ByStore Method = "store"
	// ByRegistry runs the uninstall command of the program's registry key.
	ByRegistry Method = "registry"
)

// Kind is the installer that made a program of the registry.
type Kind string

const (
	MSI           Kind = "msi"
	Burn          Kind = "burn" // a WiX bundle, in the Package Cache
	Inno          Kind = "inno"
	NSIS          Kind = "nsis"
	InstallShield Kind = "installshield"
	Other         Kind = "other"
)

// Target is a program to remove, as AppShelf read it.
type Target struct {
	// Key is the program's key in the installed list.
	Key                      string
	Name, Version, Publisher string
	Location                 string // its directory, when known
	Scope                    string // "Machine" or "User", when known
	Method                   Method
	WingetID, WingetSource   string // of ByWinget
	ScoopName                string // of ByScoop
	Global                   bool   // of ByScoop: installed for all users
	PackageFullName          string // of ByStore
	Reg                      *detect.Entry
	// CatalogIDs are the catalog's apps that are this program, to read
	// their state again once it is removed.
	CatalogIDs []string
}

// Plan is what removing a program does.
type Plan struct {
	Method Method
	Kind   Kind // of ByRegistry
	// File and Params are the command run, of ByRegistry.
	File, Params string
	// Command is the command as shown to the user.
	Command string
	// Quiet means it runs without asking; else the uninstaller's window
	// opens for the user.
	Quiet bool
	// Interactive means it can run again with the uninstaller's window,
	// when it ran quietly.
	Interactive bool
	// Elevate means administrator rights are asked for before it runs:
	// msiexec without its window cannot ask for them itself.
	Elevate bool
	// Copy means File runs from a copy in the temporary directory, Params
	// ending with _?= naming File's directory: an NSIS uninstaller then
	// does the work itself, rather than in a copy it leaves running, and
	// returns its exit code. Bulk Crap Uninstaller does the same
	// (UninstallManager.UpdateNsisStartInfo).
	Copy bool
	// Blocked is why it must not run, "" when it can.
	Blocked string
	// Notes are what to know before it runs.
	Notes []string
}

// Options are what plans need of AppShelf.
type Options struct {
	// SelfDir is AppShelf's own directory, never removed from here.
	SelfDir string
}

var (
	innoFile    = regexp.MustCompile(`(?i)^unins\d{3}\.exe$`)
	guid        = regexp.MustCompile(`\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}`)
	runtimeName = regexp.MustCompile(`(?i)(redistributable|runtime|运行库|运行时|\.net\b|visual c\+\+|webview2|directx|driver|驱动)`)
)

// winget's and scoop's own packages: AppShelf needs them.
var needed = map[string]string{
	"winget:microsoft.appinstaller":                     "这是 winget（应用安装程序），软件图书馆依赖它",
	"store:microsoft.desktopappinstaller_8wekyb3d8bbwe": "这是 winget（应用安装程序），软件图书馆依赖它",
	"scoop:scoop": "这是 scoop 本身",
}

// PlanFor works out how to remove t, from what AppShelf read of it:
// quietly when it can, or with the uninstaller's window when interactive
// asks for it. Inspect adds what it finds of the machine now.
func PlanFor(t Target, interactive bool, opt Options) Plan {
	p := Plan{Method: t.Method, Quiet: true}
	if why := neededBy(t); why != "" {
		p.Blocked = why
		return p
	}
	if opt.SelfDir != "" && t.Location != "" && samePath(t.Location, opt.SelfDir) {
		p.Blocked = "这是软件图书馆自己，请在 Windows 的“已安装的应用”里卸载"
		return p
	}
	if t.Reg != nil && t.Reg.NoRemove {
		p.Blocked = "这个程序在注册表里标明不允许卸载"
		return p
	}
	switch t.Method {
	case ByWinget:
		p.Interactive = true
		p.Quiet = !interactive
		mode := "--silent"
		if interactive {
			mode = "--interactive"
		}
		p.Command = "winget uninstall --id " + t.WingetID + " --exact --source " + orDefault(t.WingetSource, "winget") + " " + mode
	case ByScoop:
		p.Command = "scoop uninstall " + t.ScoopName
		if t.Global {
			p.Command += " --global"
		}
	case ByStore:
		p.Command = "Remove-AppxPackage -Package " + t.PackageFullName
	case ByRegistry:
		planRegistry(&p, t, interactive)
	default:
		p.Blocked = "没有找到卸载这个程序的方式"
	}
	if p.Blocked != "" {
		return p
	}
	if !p.Quiet {
		p.Notes = append(p.Notes, "会打开卸载程序的窗口，请在窗口里完成卸载")
	}
	if t.Scope == "Machine" && t.Method != ByStore {
		p.Notes = append(p.Notes, "为所有用户安装，可能会请求管理员权限")
	}
	if runtimeName.MatchString(t.Name) {
		p.Notes = append(p.Notes, "这是运行库或驱动，其他软件可能依赖它")
	}
	return p
}

func neededBy(t Target) string {
	switch t.Method {
	case ByWinget:
		return needed["winget:"+strings.ToLower(t.WingetID)]
	case ByScoop:
		return needed["scoop:"+strings.ToLower(t.ScoopName)]
	case ByStore:
		return needed["store:"+strings.ToLower(familyOf(t.PackageFullName))]
	}
	return ""
}

// familyOf returns the family name of a package's full name,
// Name_Version_Arch_ResourceID_PublisherID: Name_PublisherID.
func familyOf(full string) string {
	parts := strings.Split(full, "_")
	if len(parts) < 5 {
		return full
	}
	return parts[0] + "_" + parts[len(parts)-1]
}

// planRegistry chooses the command of a program of the registry.
func planRegistry(p *Plan, t Target, interactive bool) {
	e := t.Reg
	if e == nil {
		p.Blocked = "没有找到这个程序的注册表项"
		return
	}
	p.Kind = KindOf(*e)
	msiexec := filepath.Join(orDefault(os.Getenv("SystemRoot"), `C:\Windows`), "System32", "msiexec.exe")
	if p.Kind == MSI {
		code := ProductCode(*e)
		if code == "" {
			p.Blocked = "无法确定这个 Windows Installer 程序的产品代码"
			return
		}
		p.File, p.Params, p.Interactive = msiexec, "/x "+code+" /qn /norestart", true
		p.Elevate = e.Scope == "Machine"
		if interactive {
			p.Params, p.Quiet, p.Elevate = "/x "+code, false, false
		}
		p.Command = "msiexec " + p.Params
		return
	}
	cmd := e.UninstallString
	if cmd == "" {
		cmd = e.QuietUninstallString
	}
	if cmd == "" {
		p.Blocked = "注册表里没有卸载命令"
		return
	}
	file, params := SplitCommand(cmd)
	if file == "" {
		p.Blocked = "注册表里的卸载命令无效"
		return
	}
	given := params
	quiet := ""
	switch {
	case interactive:
	case p.Kind == Inno:
		// Before the quiet string: Inno Setup's is often /SILENT, which
		// still shows the uninstaller's progress window.
		params, quiet = join(params, "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART"), "inno"
	case e.QuietUninstallString != "":
		file, params = SplitCommand(e.QuietUninstallString)
		quiet = "registry"
	case p.Kind == NSIS:
		params, quiet = nsisQuiet(params), "nsis"
	case p.Kind == Burn:
		params, quiet = join(params, "/quiet /norestart"), "burn"
	}
	if p.Kind == NSIS && (interactive || quiet == "nsis") && runsInPlace(file, given) {
		params, p.Copy = join(params, "_?="+filepath.Dir(file)), true
	}
	p.File, p.Params = file, params
	p.Quiet = quiet != ""
	// A quiet command can run again with the window: the uninstall string
	// is the one the program gives for that.
	p.Interactive = p.Quiet && e.UninstallString != ""
	p.Command = quoteIfSpaced(file)
	if params != "" {
		p.Command += " " + params
	}
}

// nsisQuiet adds /S to an NSIS uninstaller's arguments: before _?=, which
// must come last, its directory taking the rest of the line.
func nsisQuiet(params string) string {
	if i := strings.Index(params, "_?="); i >= 0 {
		return join(strings.TrimSpace(params[:i]), "/S "+params[i:])
	}
	return join(params, "/S")
}

// runsInPlace reports an NSIS uninstaller that can run from a copy with
// _?=, as Bulk Crap Uninstaller tells: the command has no arguments of its
// own, the file is named as an uninstaller, and its directory is not
// itself named uninstall.
func runsInPlace(file, params string) bool {
	if params != "" || !filepath.IsAbs(file) {
		return false
	}
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
	return strings.Contains(name, "uninst") && !strings.EqualFold(filepath.Base(filepath.Dir(file)), "uninstall")
}

// withoutInPlace drops the _?= of a copy's arguments, for the uninstaller
// to run as it is when it cannot be copied.
func withoutInPlace(params string) string {
	if i := strings.Index(params, "_?="); i >= 0 {
		return strings.TrimSpace(params[:i])
	}
	return params
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

func quoteIfSpaced(s string) string {
	if strings.Contains(s, " ") {
		return `"` + s + `"`
	}
	return s
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// KindOf tells the installer that made a program, from its key and its
// uninstaller's file.
func KindOf(e detect.Entry) Kind {
	cmd := e.UninstallString
	if cmd == "" {
		cmd = e.QuietUninstallString
	}
	low := strings.ToLower(cmd)
	switch {
	case strings.Contains(low, "msiexec"):
		return MSI
	case strings.Contains(low, `\package cache\{`) && strings.Contains(low, `}\`):
		return Burn
	case e.WindowsInstaller && guid.MatchString(e.Name):
		return MSI
	case strings.Contains(low, `installshield installation information\{`):
		return InstallShield
	}
	file, _ := SplitCommand(cmd)
	if file == "" {
		return Other
	}
	base := filepath.Base(file)
	if innoFile.MatchString(base) {
		if _, err := os.Stat(strings.TrimSuffix(file, filepath.Ext(file)) + ".dat"); err == nil {
			return Inno
		}
	}
	if isNSIS(file) {
		return NSIS
	}
	return Other
}

// isNSIS looks for NSIS's mark in an uninstaller: its manifest names
// Nullsoft. Only the first 8 MiB are read.
func isNSIS(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	data, _ := io.ReadAll(io.LimitReader(f, 8<<20))
	return bytes.Contains(data, []byte("Nullsoft"))
}

// ProductCode returns a Windows Installer program's product code: its key's
// name, or the GUID of its msiexec command.
func ProductCode(e detect.Entry) string {
	if guid.MatchString(e.Name) && len(e.Name) == 38 {
		return strings.ToUpper(e.Name)
	}
	for _, cmd := range []string{e.UninstallString, e.QuietUninstallString} {
		if strings.Contains(strings.ToLower(cmd), "msiexec") {
			if m := guid.FindString(cmd); m != "" {
				return strings.ToUpper(m)
			}
		}
	}
	return ""
}

// SplitCommand splits a command line into the program and its arguments.
func SplitCommand(cmd string) (file, params string) {
	cmd = strings.TrimSpace(detect.ExpandEnv(cmd))
	if cmd == "" {
		return "", ""
	}
	if cmd[0] == '"' {
		end := strings.IndexByte(cmd[1:], '"')
		if end < 0 {
			return strings.Trim(cmd, `"`), ""
		}
		return cmd[1 : end+1], strings.TrimSpace(cmd[end+2:])
	}
	file = detect.CommandFile(cmd)
	if !strings.HasPrefix(cmd, file) {
		return file, ""
	}
	return file, strings.TrimSpace(cmd[len(file):])
}

func samePath(a, b string) bool {
	clean := func(p string) string {
		return strings.ToLower(strings.TrimRight(filepath.Clean(strings.Trim(p, `"`)), `\`))
	}
	return clean(a) == clean(b)
}
