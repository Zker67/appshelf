// Package detect recognizes installed software without asking a package
// manager: by the entries under the registry's Uninstall keys, which
// Windows' "Installed apps" lists, or by an executable that exists.
package detect

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"appshelf/internal/catalog"
	"appshelf/internal/version"
)

// Entry is a program listed under an Uninstall key.
type Entry struct {
	DisplayName    string
	DisplayVersion string
	Publisher      string
	Key            string // the registry key, for messages
	// Name is the key's own name under Uninstall, which winget's IDs of
	// programs of no source end with, and MSI product codes are.
	Name string
	// Scope is "Machine" or "User"; Arch the registry view, "X64" or "X86".
	Scope, Arch string
	// DisplayIcon is the program's icon: a file, maybe with ",index".
	DisplayIcon     string
	InstallLocation string
}

// Result is what a rule found.
type Result struct {
	Installed bool
	Version   string // empty when unknown
	Via       string // what matched, for the interface
}

// Match applies a rule to the entries of Scan. Matching the display name or
// finding the executable counts as installed; the version is the highest
// of the entries the name matches.
func Match(rule catalog.Detect, entries []Entry) (Result, error) {
	var r Result
	if rule.DisplayName != "" {
		re, err := regexp.Compile("(?i)" + rule.DisplayName)
		if err != nil {
			return Result{}, fmt.Errorf("注册表名称规则无效：%w", err)
		}
		for _, e := range entries {
			if !re.MatchString(e.DisplayName) {
				continue
			}
			if !r.Installed {
				r = Result{Installed: true, Version: e.DisplayVersion, Via: "注册表：" + e.DisplayName}
				continue
			}
			if cmp, ok := version.Compare(e.DisplayVersion, r.Version); ok && cmp > 0 {
				r.Version, r.Via = e.DisplayVersion, "注册表："+e.DisplayName
			}
		}
	}
	if rule.ExePath != "" && !r.Installed {
		path := ExpandEnv(rule.ExePath)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			r = Result{Installed: true, Via: "文件：" + path}
		}
	}
	return r, nil
}

var envPattern = regexp.MustCompile(`%([A-Za-z0-9_()]+)%`)

// ExpandEnv replaces Windows-style %VARIABLE% references; unknown ones stay.
func ExpandEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.Trim(m, "%")
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return m
	})
}
