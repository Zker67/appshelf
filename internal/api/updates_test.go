package api

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/inventory"
	"appshelf/internal/source"
)

// check runs a check for updates and waits for it to end.
func (e *env) check() {
	e.t.Helper()
	_, before, _ := e.app.upd.times()
	e.updates.Check()
	e.until(func() bool {
		checking, at, _ := e.app.upd.times()
		return !checking && at.After(before)
	})
}

// setUpdate makes the installed list say Obsidian has latest.
func (e *env) setUpdate(latest string) {
	e.wapi.mu.Lock()
	e.wapi.pkgs[0].Update = latest
	e.wapi.mu.Unlock()
}

// installObsidianFor makes the registry list Obsidian, installed for the
// user in dir, and reads the installed list again.
func (e *env) installObsidianFor(dir string) {
	e.t.Helper()
	e.mu.Lock()
	e.registry = []detect.Entry{{DisplayName: "Obsidian", DisplayVersion: "1.8", Publisher: "Obsidian", Name: "Obsidian", Scope: "User", Arch: "X64", Key: `HKCU\Obsidian`, InstallLocation: dir}}
	e.mu.Unlock()
	e.check()
}

func (e *env) notes() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.notified)
}

func itemOf(t *testing.T, l UpdateList, key string) UpdateItem {
	t.Helper()
	for _, it := range l.Items {
		if it.Key == key {
			return it
		}
	}
	t.Fatalf("no update of %s in %+v", key, l.Items)
	return UpdateItem{}
}

func TestUpdatesMergeCatalogAndInstalled(t *testing.T) {
	e := newEnv(t)
	l := e.updates.List()
	if !l.Loaded || l.Count != 1 || len(l.Items) != 1 {
		t.Fatalf("list = %+v", l)
	}
	it := l.Items[0]
	if it.Key != installedObsidian || it.AppID != "" || it.Version != "1.8" || it.Latest != "1.9" || it.Mode != UpdateNotify ||
		it.CanAuto || !strings.Contains(it.AutoNote, "安装范围") || !strings.HasPrefix(it.Icon, "/i/") {
		t.Errorf("installed update = %+v", it)
	}

	// In the catalog too, it is the catalog's app, once.
	updateState(e, availableUpdate())
	app, err := e.shelf.SaveApp(catalog.App{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return e.updates.List().Items[0].AppID == app.ID })
	l = e.updates.List()
	if len(l.Items) != 1 || l.Items[0].Icon != "/w/Obsidian.Obsidian" || l.Count != 1 {
		t.Errorf("merged = %+v", l.Items)
	}

	// The catalog's state decides: up to date, there is no update, though
	// the installed list has one.
	updateState(e, source.Status{State: source.Installed, Version: "1.9"})
	e.check()
	if l := e.updates.List(); len(l.Items) != 0 || l.Count != 0 {
		t.Errorf("catalog up to date: %+v", l.Items)
	}
}

func TestUpdatesHoldAndRestore(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	app, err := e.shelf.SaveApp(catalog.App{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}})
	if err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return e.updates.List().Items[0].AppID == app.ID })

	if err := e.updates.Skip(installedObsidian); err != nil {
		t.Fatal(err)
	}
	l := e.updates.List()
	if it := itemOf(t, l, installedObsidian); it.Hold != HoldSkipped || it.Skip != "1.9" || l.Count != 0 {
		t.Errorf("skipped: %+v, count %d", it, l.Count)
	}
	if len(l.Rules) != 1 || l.Rules[0].Skip != "1.9" || l.Rules[0].Name != "Obsidian" || !l.Rules[0].Installed {
		t.Errorf("rules: %+v", l.Rules)
	}
	if a := e.shelf.Snapshot().Apps[0]; a.Hold != HoldSkipped {
		t.Errorf("catalog app hold: %q", a.Hold)
	}
	for _, en := range e.installed.List().Entries {
		if en.Kind == InstalledWinget && (en.Hold != HoldSkipped || en.Mode != UpdateNotify) {
			t.Errorf("installed entry: %+v", en)
		}
	}

	// A newer version is offered again.
	updateState(e, source.Status{State: source.UpdateAvailable, Version: "1.8", Latest: "2.0"})
	e.check()
	l = e.updates.List()
	if it := itemOf(t, l, installedObsidian); it.Hold != HoldNone || l.Count != 1 || len(l.Rules) != 0 {
		t.Errorf("newer version: %+v, rules %+v", it, l.Rules)
	}

	if err := e.updates.Snooze(installedObsidian, 7); err != nil {
		t.Fatal(err)
	}
	it := itemOf(t, e.updates.List(), installedObsidian)
	if it.Hold != HoldSnoozed || it.Until.Before(time.Now().Add(6*24*time.Hour)) {
		t.Errorf("snoozed: %+v", it)
	}
	if err := e.updates.Restore(installedObsidian); err != nil {
		t.Fatal(err)
	}
	if it := itemOf(t, e.updates.List(), installedObsidian); it.Hold != HoldNone || !it.Until.IsZero() {
		t.Errorf("restored: %+v", it)
	}

	if err := e.updates.SetMode(installedObsidian, UpdateIgnore); err != nil {
		t.Fatal(err)
	}
	l = e.updates.List()
	if it := itemOf(t, l, installedObsidian); it.Hold != HoldIgnored || it.Mode != UpdateIgnore || l.Count != 0 {
		t.Errorf("ignored: %+v", it)
	}
	if len(l.Rules) != 1 || l.Rules[0].Mode != UpdateIgnore {
		t.Errorf("ignored rule: %+v", l.Rules)
	}
	if err := e.updates.Restore(installedObsidian); err != nil {
		t.Fatal(err)
	}
	if l := e.updates.List(); l.Items[0].Mode != UpdateNotify || l.Count != 1 || len(l.Rules) != 0 {
		t.Errorf("restored from ignored: %+v", l)
	}
	if p := e.core.Store().Policies(); len(p) != 0 {
		t.Errorf("default policies kept: %+v", p)
	}

	for name, err := range map[string]error{
		"no days":         e.updates.Snooze(installedObsidian, 0),
		"too many days":   e.updates.Snooze(installedObsidian, 91),
		"no update":       e.updates.Skip("winget:nope.nope"),
		"not a key":       e.updates.SetMode("local:tool", UpdateIgnore),
		"an unknown mode": e.updates.SetMode(installedObsidian, "weird"),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestUpdatesAuto(t *testing.T) {
	e := newEnv(t)
	// Of no known scope, it could ask for administrator rights.
	if err := e.updates.SetMode(installedObsidian, UpdateAuto); err == nil || !strings.Contains(err.Error(), "安装范围") {
		t.Errorf("unknown scope: %v", err)
	}
	if _, err := e.shelf.SaveApp(catalog.App{Name: "CLI", Source: catalog.Source{Type: catalog.GitHub, Repo: "cli/cli"}, Detect: &catalog.Detect{DisplayName: "^GitHub CLI"}}); err != nil {
		t.Fatal(err)
	}
	if err := e.updates.SetMode("github:cli/cli", UpdateAuto); err == nil || !strings.Contains(err.Error(), "GitHub") {
		t.Errorf("GitHub: %v", err)
	}

	e.installObsidianFor(t.TempDir())
	it := itemOf(t, e.updates.List(), installedObsidian)
	if !it.CanAuto || it.Running {
		t.Fatalf("installed for the user: %+v", it)
	}
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.after[installedObsidian] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()

	// Turned on with an update waiting, it updates at once, on its own.
	if err := e.updates.SetMode(installedObsidian, UpdateAuto); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return len(e.updates.History()) == 1 })
	h := e.updates.History()[0]
	if !h.Auto || h.Result != TaskSucceeded || h.From != "1.8" || h.To != "1.9" || h.Name != "Obsidian" {
		t.Errorf("history: %+v", h)
	}
	e.until(func() bool {
		return slices.ContainsFunc(e.notes(), func(n string) bool { return strings.HasPrefix(n, "已自动更新 1 个软件|Obsidian") })
	})
	if l := e.updates.List(); len(l.Rules) != 1 || l.Rules[0].Mode != UpdateAuto || !l.Rules[0].CanAuto {
		t.Errorf("auto rule: %+v", l.Rules)
	}
	e.winget.mu.Lock()
	actions := slices.Clone(e.winget.actions)
	e.winget.mu.Unlock()
	if len(actions) != 1 || !strings.HasPrefix(actions[0], "upgrade:") {
		t.Errorf("actions: %v", actions)
	}
	if len(e.shelf.Snapshot().Apps) != 1 {
		t.Error("updating on its own added an app to the catalog")
	}
}

func TestUpdatesAutoWaitsWhileRunning(t *testing.T) {
	e := newEnv(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The test binary runs from this directory.
	e.installObsidianFor(filepath.Dir(exe))
	it := itemOf(t, e.updates.List(), installedObsidian)
	if !it.CanAuto || !it.Running {
		t.Fatalf("running: %+v", it)
	}
	updateState(e, availableUpdate())
	if err := e.updates.SetMode(installedObsidian, UpdateAuto); err != nil {
		t.Fatal(err)
	}
	e.check()
	time.Sleep(200 * time.Millisecond)
	e.winget.mu.Lock()
	defer e.winget.mu.Unlock()
	if len(e.winget.actions) != 0 {
		t.Errorf("updated a running program: %v", e.winget.actions)
	}
}

func TestUpdatesNotify(t *testing.T) {
	e := newEnv(t)
	// Starting was a check, which found Obsidian's update.
	e.until(func() bool { return len(e.notes()) == 1 })
	if n := e.notes()[0]; !strings.HasPrefix(n, "有 1 个软件可以更新|Obsidian") {
		t.Errorf("notification: %q", n)
	}
	// A version is told of once.
	e.check()
	time.Sleep(150 * time.Millisecond)
	if len(e.notes()) != 1 {
		t.Errorf("told twice: %v", e.notes())
	}
	e.setUpdate("2.0")
	e.check()
	e.until(func() bool { return len(e.notes()) == 2 })

	// Not while the window is in front, nor when turned off.
	e.mu.Lock()
	e.active = true
	e.mu.Unlock()
	e.setUpdate("2.1")
	e.check()
	e.mu.Lock()
	e.active = false
	e.mu.Unlock()
	s := e.updates.Settings()
	s.Notify = false
	if err := e.updates.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	e.setUpdate("2.2")
	e.check()
	time.Sleep(150 * time.Millisecond)
	if len(e.notes()) != 2 {
		t.Errorf("notified: %v", e.notes())
	}
	// Ignored, an update is not told of.
	s.Notify = true
	e.updates.SetSettings(s)
	e.updates.SetMode(installedObsidian, UpdateIgnore)
	e.setUpdate("2.3")
	e.check()
	time.Sleep(150 * time.Millisecond)
	if len(e.notes()) != 2 {
		t.Errorf("told of an ignored update: %v", e.notes())
	}
}

func TestUpdateChecksAndSettings(t *testing.T) {
	e := newEnv(t)
	s := e.updates.Settings()
	if s.CheckEvery != Check6h || !s.Notify || s.Background || s.CanBackground || s.CanSelfUpdate {
		t.Errorf("defaults: %+v", s)
	}
	l := e.updates.List()
	if l.Checking || l.CheckedAt.IsZero() || !l.NextCheck.Equal(l.CheckedAt.Add(6*time.Hour)) {
		t.Errorf("after starting: %+v", l)
	}
	s.CheckEvery = Check1h
	if err := e.updates.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	l = e.updates.List()
	if !l.NextCheck.Equal(l.CheckedAt.Add(time.Hour)) {
		t.Errorf("every hour: %+v", l)
	}

	// Not due yet, then due.
	e.app.upd.tick()
	if _, at, _ := e.app.upd.times(); !at.Equal(l.CheckedAt) {
		t.Error("checked before it was due")
	}
	later := time.Now().Add(2 * time.Hour)
	e.app.upd.mu.Lock()
	e.app.upd.now = func() time.Time { return later }
	e.app.upd.mu.Unlock()
	e.app.upd.tick()
	e.until(func() bool {
		checking, at, _ := e.app.upd.times()
		return !checking && at.Equal(later)
	})

	s.CheckEvery = CheckOff
	if err := e.updates.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	if l := e.updates.List(); !l.NextCheck.IsZero() {
		t.Errorf("off: next check %v", l.NextCheck)
	}
	data, _ := os.ReadFile(filepath.Join(e.dir, "settings.json"))
	if !strings.Contains(string(data), `"checkEvery": "off"`) {
		t.Errorf("settings.json: %s", data)
	}
	if loadSettings(filepath.Join(e.dir, "settings.json")).Updates.CheckEvery != CheckOff {
		t.Error("the interval was not kept")
	}
	for name, in := range map[string]UpdateSettings{
		"an unknown interval":       {CheckEvery: "5m"},
		"background without a tray": {CheckEvery: Check6h, Background: true},
	} {
		if err := e.updates.SetSettings(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// With the notification area and starting at login.
	var bg []bool
	login := false
	e.app.hooks.SetBackground = func(on bool) { bg = append(bg, on) }
	e.app.hooks.OpenAtLogin = func() bool { return login }
	e.app.hooks.SetOpenAtLogin = func(on bool) error { login = on; return nil }
	if err := e.updates.SetSettings(UpdateSettings{CheckEvery: Check6h, Notify: true, Background: true, OpenAtLogin: true}); err != nil {
		t.Fatal(err)
	}
	if s := e.updates.Settings(); !s.CanBackground || !s.Background || !s.OpenAtLogin || len(bg) != 1 || !bg[0] || !login {
		t.Errorf("background: %+v %v %v", s, bg, login)
	}
	if !LoadBackground(filepath.Join(e.dir, "settings.json")) {
		t.Error("background was not kept")
	}
}

func TestUpdatesStartAndHistory(t *testing.T) {
	e := newEnv(t)
	updateState(e, availableUpdate())
	e.winget.mu.Lock()
	e.winget.after[installedObsidian] = source.Status{State: source.Installed, Version: "1.9"}
	e.winget.mu.Unlock()
	ctx := context.Background()
	for _, keys := range [][]string{{"winget:nope.nope"}, {"local:tool"}, {""}} {
		if _, err := e.updates.Start(ctx, keys); err == nil {
			t.Errorf("started %q", keys)
		}
	}
	// The same package twice, as written another way, is queued once.
	n, err := e.updates.Start(ctx, []string{installedObsidian, " WINGET:Obsidian.Obsidian "})
	if err != nil || n != 1 {
		t.Fatalf("start: %d %v", n, err)
	}
	e.until(func() bool { return len(e.updates.History()) == 1 })
	h := e.updates.History()[0]
	if h.Auto || h.Result != TaskSucceeded || h.Key != installedObsidian || h.Icon != "/w/obsidian.obsidian" || h.At.IsZero() {
		t.Errorf("history: %+v", h)
	}
	if err := e.updates.ClearHistory(); err != nil || len(e.updates.History()) != 0 {
		t.Errorf("cleared: %v %v", err, e.updates.History())
	}
}

func TestInstallDir(t *testing.T) {
	scoopRoot, global := t.TempDir(), t.TempDir()
	t.Setenv("SCOOP", scoopRoot)
	t.Setenv("SCOOP_GLOBAL", global)
	app := t.TempDir()
	cases := []struct {
		name  string
		entry inventory.Entry
		want  string
	}{
		{"location", inventory.Entry{Location: `"` + app + `\"`}, app},
		{"icon", inventory.Entry{Icon: `"` + filepath.Join(app, "app.exe") + `",0`}, app},
		{"icon of an installer", inventory.Entry{Icon: filepath.Join(app, "app.ico")}, ""},
		{"a drive", inventory.Entry{Location: `C:\`}, ""},
		{"Program Files", inventory.Entry{Location: os.Getenv("ProgramFiles")}, ""},
		{"the system", inventory.Entry{Location: filepath.Join(os.Getenv("SystemRoot"), "System32")}, ""},
		{"relative", inventory.Entry{Location: `apps\x`}, ""},
		{"scoop", inventory.Entry{Kind: inventory.Scoop, ScoopName: "7zip", Scope: "User"}, filepath.Join(scoopRoot, "apps", "7zip")},
		{"scoop global", inventory.Entry{Kind: inventory.Scoop, ScoopName: "7zip", Scope: "Machine"}, filepath.Join(global, "apps", "7zip")},
	}
	for _, c := range cases {
		if got := installDir(&pkg{entry: &c.entry}); !strings.EqualFold(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseKey(t *testing.T) {
	for key, want := range map[string]string{
		"winget:Git.Git":       "winget:git.git",
		" scoop:main/7zip ":    "scoop:main/7zip",
		"github:cli/cli":       "github:cli/cli",
		"github:cli":           "",
		"scoop:7zip":           "",
		"local:tool":           "",
		"winget:":              "",
		"winget:a\nb":          "",
		strings.Repeat("x", 9): "",
	} {
		got, err := checkKey(key)
		if (err == nil) != (want != "") || got != want {
			t.Errorf("checkKey(%q) = %q, %v", key, got, err)
		}
	}
}
