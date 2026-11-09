//go:build windows

package leftover

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"

	"appshelf/internal/runner"
)

func TestNormName(t *testing.T) {
	for in, want := range map[string]string{
		"Foo App 2.1 (x64)":         "foo app",
		"Foo App v3.0.1":            "foo app",
		"PiliPlus 版本 2.2.0+5598":    "piliplus",
		"7-Zip 23.01 (x64 edition)": "7 zip 23 01 x64 edition",
		"Visual Studio Code":        "visual studio code",
		"  ":                        "",
	} {
		if got := normName(in); got != want {
			t.Errorf("normName(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Foo Corp, Inc.": "foo", "Igor Pavlov": "igor pavlov", "Microsoft Corporation": "", "安徽海马云科技股份有限公司": "安徽海马云科技"} {
		if got := normPublisher(in); got != want {
			t.Errorf("normPublisher(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatch(t *testing.T) {
	f := &finder{names: nameForms("Foo App 2.1 (x64)", "")}
	for name, want := range map[string]*Reason{
		"Foo App":         &namePerfect,
		"FooApp":          &namePerfect,
		"foo-app":         &namePerfect,
		"Foo App Pro":     &nameDodgy,
		"Foo App Updater": nil, // too little of it is the name
		"Foo":             nil, // too short to tell
		"Bar":             nil,
		"Microsoft":       nil,
	} {
		got := f.match(name)
		if (got == nil) != (want == nil) || got != nil && *got != *want {
			t.Errorf("match(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestProtected(t *testing.T) {
	base := testBase(t)
	cfg := Config{DirRoots: []string{filepath.Join(base, "AppData")}, Protected: []string{filepath.Join(base, "AppData", "AppShelf")}}
	others := []Other{{Name: "Bar", Location: filepath.Join(base, "AppData", "Shared", "Bar")}}
	for path, want := range map[string]bool{
		filepath.Join(base, "AppData"):                         true,  // a root
		filepath.Join(base, "AppData", "AppShelf"):             true,  // AppShelf's
		filepath.Join(base, "AppData", "AppShelf", "x"):        true,  // inside AppShelf's
		filepath.Join(base, "AppData", "Shared"):               true,  // holds another program
		filepath.Join(base, "AppData", "Shared", "Bar", "sub"): true,  // inside another program
		filepath.Join(base, "AppData", "Foo"):                  false, // the program's
		os.Getenv("SystemRoot"):                                true,
		`C:\`:                                                  true,
		os.Getenv("ProgramFiles"):                              true,
		`relative`:                                             true,
	} {
		if got := Protected(cfg, path, others, ""); got != want {
			t.Errorf("Protected(%q) = %v, want %v", path, got, want)
		}
	}
	// The program's own location is not another program's.
	if Protected(cfg, filepath.Join(base, "AppData", "Shared", "Bar"), others, filepath.Join(base, "AppData", "Shared", "Bar")) {
		t.Error("the program's own location counted as another's")
	}
}

func TestRegPathOK(t *testing.T) {
	cfg := Config{RegRoot: "Software", RunKey: `Software\Microsoft\Windows\CurrentVersion\Run`}
	for _, c := range []struct {
		it   Item
		want bool
	}{
		{Item{Kind: RegKey, Path: `Software\Foo`}, true},
		{Item{Kind: RegKey, Path: `Software`}, false},
		{Item{Kind: RegKey, Path: `Software\Microsoft`}, false},
		{Item{Kind: RegKey, Path: `Software\Microsoft\Windows\CurrentVersion\Run`}, false},
		{Item{Kind: RegKey, Path: `Software\..\Foo`}, false},
		{Item{Kind: RegKey, Path: `System\Foo`}, false},
		{Item{Kind: RegValue, Path: `Software\Microsoft\Windows\CurrentVersion\Run`, Value: "Foo"}, true},
		{Item{Kind: RegValue, Path: `Software\Foo`, Value: "x"}, false},
		{Item{Kind: RegValue, Path: `Software\Microsoft\Windows\CurrentVersion\Run`}, false},
	} {
		if got := RegPathOK(cfg, c.it); got != c.want {
			t.Errorf("RegPathOK(%+v) = %v, want %v", c.it, got, c.want)
		}
	}
}

// testBase is a directory of the test's own outside the temporary one,
// which Protected refuses.
func testBase(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp(os.Getenv("LOCALAPPDATA"), "AppShelfTest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	return base
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func shortcut(t *testing.T, path, target string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	script := `$s = (New-Object -ComObject WScript.Shell).CreateShortcut([string]$P.Path); $s.TargetPath = [string]$P.Target; $s.Save(); Write-Result 1`
	if _, err := runner.PowerShell(context.Background(), script, map[string]any{"Path": path, "Target": target}, nil, runner.Options{Timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
}

func regKey(t *testing.T, path string, values map[string]string) {
	t.Helper()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	for n, v := range values {
		k.SetStringValue(n, v)
	}
	k.Close()
}

// TestFindAndClean builds what a removed program "Foo App" left in
// directories of its own, a registry key of its own and shortcuts, finds
// it, and cleans part of it.
func TestFindAndClean(t *testing.T) {
	base := testBase(t)
	b := make([]byte, 6)
	rand.Read(b)
	regRoot := `Software\AppShelfTest-` + hex.EncodeToString(b)
	runKey := regRoot + `-Run`
	t.Cleanup(func() {
		deleteTree(regRoot)
		deleteTree(runKey)
	})

	pf, appData, start := filepath.Join(base, "PF"), filepath.Join(base, "AppData"), filepath.Join(base, "Start")
	loc := filepath.Join(pf, "Foo App")
	write(t, filepath.Join(loc, "data.txt"), "left")
	write(t, filepath.Join(appData, "Foo App", "config.json"), "{}")
	write(t, filepath.Join(appData, "Foo Corp", "Foo App", "settings.ini"), "x")
	write(t, filepath.Join(appData, "FooApp", "helper.exe"), "MZ")
	write(t, filepath.Join(appData, "AppShelf", "Foo App", "x"), "AppShelf's")
	write(t, filepath.Join(pf, "Bar", "bar.exe"), "MZ")
	write(t, filepath.Join(appData, "Unrelated", "u.txt"), "u")
	shortcut(t, filepath.Join(start, "Foo App.lnk"), filepath.Join(loc, "foo.exe"))
	shortcut(t, filepath.Join(start, "Foo App", "Foo App.lnk"), filepath.Join(loc, "foo.exe"))
	shortcut(t, filepath.Join(start, "Notepad.lnk"), filepath.Join(os.Getenv("SystemRoot"), "notepad.exe"))
	regKey(t, regRoot+`\Foo App`, map[string]string{"Theme": "dark"})
	regKey(t, regRoot+`\Foo Corp\Foo App`, nil)
	regKey(t, regRoot+`\Microsoft`, nil)
	regKey(t, runKey, map[string]string{
		"FooApp": `"` + filepath.Join(loc, "foo.exe") + `" --tray`,
		"Other":  filepath.Join(os.Getenv("SystemRoot"), "notepad.exe"),
	})

	cfg := Config{
		DirRoots: []string{pf, appData}, RegRoot: regRoot, ShortcutDirs: []string{start}, RunKey: runKey,
		Protected: []string{filepath.Join(appData, "AppShelf")}, links: readLinks,
	}
	prog := Program{Name: "Foo App 2.1 (x64)", Publisher: "Foo Corp, Inc.", Location: loc}
	others := []Other{{Name: "Bar Tool", Publisher: "Bar", Location: filepath.Join(pf, "Bar")}}
	items := Find(context.Background(), cfg, prog, others)

	type want struct {
		kind  Kind
		path  string
		score int
	}
	wants := []want{
		{Dir, loc, 4},
		{Dir, filepath.Join(appData, "Foo App"), 2},
		{Dir, filepath.Join(appData, "Foo Corp", "Foo App"), 4},
		{Dir, filepath.Join(appData, "Foo Corp"), 2},
		{Dir, filepath.Join(appData, "FooApp"), -2},
		{Shortcut, filepath.Join(start, "Foo App.lnk"), 4},
		{Shortcut, filepath.Join(start, "Foo App", "Foo App.lnk"), 4},
		{Dir, filepath.Join(start, "Foo App"), 6},
		{RegKey, regRoot + `\Foo App`, 2},
		{RegKey, regRoot + `\Foo Corp\Foo App`, 4},
		{RegKey, regRoot + `\Foo Corp`, 4},
		{RegValue, runKey, 4},
	}
	byPath := map[string]Item{}
	for _, it := range items {
		byPath[string(it.Kind)+"|"+strings.ToLower(it.Path)+"|"+it.Value] = it
		t.Logf("%-8s %3d %-6s %s %s %v", it.Kind, it.Score, it.Level, it.Path, it.Value, it.Reasons)
	}
	for _, w := range wants {
		key := string(w.kind) + "|" + strings.ToLower(w.path) + "|"
		if w.kind == RegValue {
			key += "FooApp"
		}
		it, ok := byPath[key]
		if !ok {
			t.Errorf("not found: %s %s", w.kind, w.path)
			continue
		}
		if it.Score != w.score || it.Level != levelOf(w.score) || it.ID == "" {
			t.Errorf("%s: score %d %s, want %d", w.path, it.Score, it.Level, w.score)
		}
	}
	if len(items) != len(wants) {
		t.Errorf("found %d, want %d", len(items), len(wants))
	}

	// Clean the program's directory, a shortcut, its registry keys and its
	// startup value; the others stay.
	var chosen []Item
	for _, it := range items {
		if it.Path == loc || it.Kind == RegKey || it.Kind == RegValue || it.Path == filepath.Join(start, "Foo App.lnk") {
			chosen = append(chosen, it)
		}
	}
	// A find that has become protected since is not cleaned.
	chosen = append(chosen, Item{ID: "x", Kind: Dir, Path: filepath.Join(appData, "AppShelf")})
	backups := filepath.Join(base, "backups")
	outcomes := Clean(context.Background(), cfg, chosen, others, loc, backups)
	for i, o := range outcomes {
		it := chosen[i]
		if it.ID == "x" {
			if o.OK || !strings.Contains(o.Error, "受保护") {
				t.Errorf("protected path: %+v", o)
			}
			continue
		}
		if !o.OK {
			t.Errorf("%s %s: %s", it.Kind, it.Path, o.Error)
			continue
		}
		if (it.Kind == RegKey || it.Kind == RegValue) && !exists(o.Backup) {
			t.Errorf("%s: no backup at %q", it.Path, o.Backup)
		}
	}
	if exists(loc) || exists(filepath.Join(start, "Foo App.lnk")) {
		t.Error("files not moved to the Recycle Bin")
	}
	if !exists(filepath.Join(appData, "Foo App")) || !exists(filepath.Join(appData, "AppShelf")) {
		t.Error("cleaned what was not chosen")
	}
	if keyExists(regRoot+`\Foo App`) || keyExists(regRoot+`\Foo Corp`) || !keyExists(regRoot+`\Microsoft`) {
		t.Error("registry keys: chosen ones must go, others stay")
	}
	k, _ := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	_, _, errFoo := k.GetStringValue("FooApp")
	_, _, errOther := k.GetStringValue("Other")
	k.Close()
	if errFoo == nil || errOther != nil {
		t.Errorf("run values: FooApp %v, Other %v", errFoo, errOther)
	}
	data, _ := os.ReadFile(outcomes[0].Backup)
	if len(chosen) > 0 && chosen[0].Kind == RegKey && len(data) == 0 {
		t.Error("empty backup")
	}
}

func TestScoopPersist(t *testing.T) {
	base := testBase(t)
	persist := filepath.Join(base, "scoop", "persist")
	write(t, filepath.Join(persist, "ripgrep", "config"), "x")
	items := Find(context.Background(), Config{ScoopPersist: []string{persist}}, Program{Name: "ripgrep", ScoopName: "ripgrep"}, nil)
	if len(items) != 1 || items[0].Path != filepath.Join(persist, "ripgrep") || items[0].Level != High {
		t.Fatalf("items = %+v", items)
	}
}
