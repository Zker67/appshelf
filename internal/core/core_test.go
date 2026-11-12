package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/procs"
	"appshelf/internal/queue"
	"appshelf/internal/source"
	"appshelf/internal/state"
)

// fakeSource reports states from a map and records what it installs.
type fakeSource struct {
	typ   catalog.SourceType
	avail source.Availability
	mu    sync.Mutex
	state map[string]source.Status // by source key
	after map[string]source.Status // after installing
	res   source.Result
	ran   []string
}

func newFake(t catalog.SourceType) *fakeSource {
	return &fakeSource{typ: t, avail: source.Availability{OK: true}, state: map[string]source.Status{}, after: map[string]source.Status{}}
}

func (f *fakeSource) Type() catalog.SourceType { return f.typ }
func (f *fakeSource) Check(context.Context) source.Availability {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.avail
}
func (f *fakeSource) get(a catalog.App) source.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.state[a.Source.Key()]; ok {
		return st
	}
	return source.Status{State: source.NotInstalled}
}
func (f *fakeSource) Statuses(_ context.Context, apps []catalog.App) (map[string]source.Status, error) {
	out := map[string]source.Status{}
	for _, a := range apps {
		out[a.ID] = f.get(a)
	}
	return out, nil
}
func (f *fakeSource) Recheck(_ context.Context, a catalog.App) (source.Status, error) {
	return f.get(a), nil
}
func (f *fakeSource) Install(_ context.Context, a catalog.App, _ io.Writer) (source.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran = append(f.ran, a.ID)
	if st, ok := f.after[a.Source.Key()]; ok {
		f.state[a.Source.Key()] = st
	}
	return f.res, nil
}
func (f *fakeSource) Upgrade(ctx context.Context, a catalog.App, log io.Writer) (source.Result, error) {
	return f.Install(ctx, a, log)
}
func (f *fakeSource) Search(_ context.Context, q string) ([]source.SearchResult, error) {
	return []source.SearchResult{{ID: "Found." + q, Name: q}}, nil
}

// harness runs a Core whose posts the test runs on its own goroutine, as
// the UI thread would.
type harness struct {
	t     *testing.T
	c     *Core
	posts chan func()
	dir   string
}

func newHarness(t *testing.T, sources ...source.Source) *harness {
	t.Helper()
	return newHarnessWith(t, nil, sources...)
}

// newHarnessWith is newHarness with what change changes of the config.
func newHarnessWith(t *testing.T, change func(*Config), sources ...source.Source) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, posts: make(chan func(), 256), dir: dir}
	cfg := Config{
		Paths:   Paths{Catalog: filepath.Join(dir, "catalog.json"), State: filepath.Join(dir, "state.json"), Logs: filepath.Join(dir, "logs")},
		Sources: func(*state.Store) []source.Source { return sources },
		Post:    func(fn func()) { h.posts <- fn },
	}
	if change != nil {
		change(&cfg)
	}
	c, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.c = c
	return h
}

// until runs posted functions until cond holds.
func (h *harness) until(cond func() bool) {
	h.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case fn := <-h.posts:
			fn()
		case <-deadline:
			h.t.Fatal("timed out")
		}
	}
}

func wingetApp(name, id string) catalog.App {
	return catalog.App{Name: name, Source: catalog.Source{Type: catalog.Winget, ID: id}}
}

func TestOpenSetsBrokenFilesAside(t *testing.T) {
	dir := t.TempDir()
	cat := filepath.Join(dir, "catalog.json")
	st := filepath.Join(dir, "state.json")
	os.WriteFile(cat, []byte("{broken"), 0o644)
	os.WriteFile(st, []byte("{broken"), 0o644)
	c, err := Open(context.Background(), Config{Paths: Paths{Catalog: cat, State: st, Logs: dir}, Post: func(func()) {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Notes) != 2 || len(c.Catalog().Apps) != 0 {
		t.Errorf("notes %q", c.Notes)
	}
	entries, _ := os.ReadDir(dir)
	broken := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), ".broken-") {
			broken++
		}
	}
	if broken != 2 {
		t.Errorf("files set aside: %d (%v)", broken, entries)
	}
}

func TestSaveRefreshAndPreview(t *testing.T) {
	w := newFake(catalog.Winget)
	s := newFake(catalog.Scoop)
	s.avail = source.Availability{Reason: "未安装 scoop"}
	h := newHarness(t, w, s)
	c := h.c

	code, err := c.SaveApp(wingetApp("VS Code", "Microsoft.VisualStudioCode"))
	if err != nil {
		t.Fatal(err)
	}
	w.state[code.Source.Key()] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	obs, _ := c.SaveApp(wingetApp("Obsidian", "Obsidian.Obsidian"))
	zip, _ := c.SaveApp(catalog.App{Name: "7-Zip", Source: catalog.Source{Type: catalog.Scoop, Bucket: "main", Name: "7zip"}})
	if _, err := c.SaveApp(wingetApp("VS Code again", "microsoft.visualstudiocode")); err == nil {
		t.Error("a second app of one source was saved")
	}
	// Saved to disk.
	back, err := catalog.Load(c.Paths().Catalog)
	if err != nil || len(back.Apps) != 3 {
		t.Fatalf("saved %d apps, %v", len(back.Apps), err)
	}

	c.Refresh()
	h.until(func() bool { return !c.Refreshing() && len(c.checking) == 0 })
	if st, _ := c.Status(code.ID); st.State != source.UpdateAvailable {
		t.Errorf("code: %+v", st)
	}
	if st, _ := c.Status(zip.ID); st.State != source.Unavailable || st.Note != "未安装 scoop" {
		t.Errorf("7-zip: %+v", st)
	}

	ids := []string{code.ID, obs.ID, zip.ID}
	inst := c.Preview(ids, queue.Install)
	if inst[0].Runnable || !inst[1].Runnable || inst[2].Runnable {
		t.Errorf("install preview %+v", inst)
	}
	if !strings.Contains(inst[1].Detail, "Obsidian.Obsidian") || !strings.Contains(inst[2].Plan, "未安装 scoop") {
		t.Errorf("install preview %+v", inst)
	}
	up := c.Preview(ids, queue.Upgrade)
	if !up[0].Runnable || up[1].Runnable || !strings.Contains(up[0].Plan, "1.0 → 2.0") {
		t.Errorf("upgrade preview %+v", up)
	}

	// An installed app whose updates could not be looked for says why
	// rather than that it is up to date.
	git, _ := c.SaveApp(wingetApp("Git", "Git.Git"))
	w.mu.Lock()
	w.state[git.Source.Key()] = source.Status{State: source.Installed, Version: "2.0", Note: "无法检查更新：GitHub 请求额度已用完"}
	w.mu.Unlock()
	c.Refresh()
	h.until(func() bool { return !c.Refreshing() && len(c.checking) == 0 })
	if p := c.Preview([]string{git.ID}, queue.Upgrade); p[0].Runnable || p[0].Plan != "跳过：没有发现更新（无法检查更新：GitHub 请求额度已用完）" {
		t.Errorf("upgrade preview of an unchecked app %+v", p)
	}
}

func TestQueueThroughCore(t *testing.T) {
	g := newFake(catalog.GitHub)
	h := newHarness(t, g)
	c := h.c
	a, err := c.SaveApp(catalog.App{Name: "Tool", Source: catalog.Source{Type: catalog.GitHub, Repo: "o/tool"}, Detect: &catalog.Detect{DisplayName: "^Tool$"}})
	if err != nil {
		t.Fatal(err)
	}
	h.until(func() bool { _, ok := c.Status(a.ID); return ok })
	g.after[a.Source.Key()] = source.Status{State: source.Installed, Version: "1.0"}
	g.res = source.Result{GitHub: &state.GitHubRecord{Tag: "v1.0", AssetName: "tool.msi"}}

	items := c.Preview([]string{a.ID}, queue.Install)
	if n := c.Enqueue(items, queue.Install); n != 1 {
		t.Fatalf("queued %d: %+v", n, items)
	}
	h.until(func() bool {
		its := c.Queue.Items()
		return len(its) == 1 && its[0].State.Done()
	})
	it := c.Queue.Items()[0]
	if it.State != queue.Succeeded || it.LogPath == "" {
		t.Fatalf("item %+v", it)
	}
	if rec, ok := c.Store().GitHub(a.ID); !ok || rec.Tag != "v1.0" {
		t.Errorf("record %+v %v", rec, ok)
	}
	h.until(func() bool { st, _ := c.Status(a.ID); return st.IsInstalled() })
	log, _ := os.ReadFile(it.LogPath)
	if !strings.Contains(string(log), "Tool") {
		t.Errorf("log %q", log)
	}

	// Installing it again is skipped: it is installed.
	if items := c.Preview([]string{a.ID}, queue.Install); items[0].Runnable {
		t.Errorf("an installed app is runnable: %+v", items[0])
	}
}

// silentSource is a source that can update with no installer window;
// ran is called as an update runs.
type silentSource struct {
	*fakeSource
	silent []string
	ran    func()
}

func (s *silentSource) Upgrade(ctx context.Context, a catalog.App, log io.Writer) (source.Result, error) {
	s.ran()
	return s.Install(ctx, a, log)
}

func (s *silentSource) UpgradeSilent(ctx context.Context, a catalog.App, log io.Writer) (source.Result, error) {
	s.mu.Lock()
	s.silent = append(s.silent, a.ID)
	s.mu.Unlock()
	s.ran()
	return s.Install(ctx, a, log)
}

// An update AppShelf starts itself runs silently; the check after any
// update waits for the setup its installer left running, which does the
// work.
func TestUpgradesSilentlyAndWaitsForInstallers(t *testing.T) {
	w := &silentSource{fakeSource: newFake(catalog.Winget)}
	dir := t.TempDir()
	posts := make(chan func(), 256)
	h := &harness{t: t, posts: posts, dir: dir}
	var (
		mu       sync.Mutex
		upgrades int // the updates run, whose setups are left
		polls    int // the snapshots since the last update ran
		watches  int
	)
	w.ran = func() {
		mu.Lock()
		upgrades++
		mu.Unlock()
	}
	app := wingetApp("Tool", "Tool.Tool")
	key := app.Source.Key()
	sys := procs.System{
		Snapshot: func() ([]procs.Proc, error) {
			mu.Lock()
			defer mu.Unlock()
			if upgrades == 0 {
				return nil, nil
			}
			if polls++; polls <= 3 {
				return []procs.Proc{{PID: 7, Name: "setup.tmp"}}, nil
			}
			// The setup ended, having updated the program.
			w.mu.Lock()
			w.state[key] = source.Status{State: source.Installed, Version: "2.0"}
			w.mu.Unlock()
			return nil, nil
		},
		Path:  func(uint32) (string, bool) { return `C:\Temp\is-1.tmp\setup.tmp`, true },
		Usage: func(uint32) (procs.Usage, bool) { return procs.Usage{}, false },
	}
	c, err := Open(context.Background(), Config{
		Paths:   Paths{Catalog: filepath.Join(dir, "catalog.json"), State: filepath.Join(dir, "state.json"), Logs: filepath.Join(dir, "logs")},
		Sources: func(*state.Store) []source.Source { return []source.Source{w} },
		Post:    func(fn func()) { posts <- fn },
		Installers: func() *procs.Installers {
			mu.Lock()
			watches++
			mu.Unlock()
			in := procs.WatchInstallers(sys, []string{`C:\Temp`})
			in.Times = procs.WaitTimes{Poll: time.Millisecond, Grace: 5 * time.Millisecond, Idle: time.Hour, Max: time.Hour}
			return in
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.c = c
	a, err := c.SaveApp(app)
	if err != nil {
		t.Fatal(err)
	}
	h.until(func() bool { _, ok := c.Status(a.ID); return ok })

	run := func(auto bool) queue.Item {
		t.Helper()
		w.mu.Lock()
		w.state[key] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
		w.mu.Unlock()
		mu.Lock()
		upgrades, polls = 0, 0
		mu.Unlock()
		c.Queue.ClearDone()
		c.Refresh()
		h.until(func() bool { st, _ := c.Status(a.ID); return !c.Refreshing() && st.State == source.UpdateAvailable })
		items := c.Preview([]string{a.ID}, queue.Upgrade)
		// The upgrade itself changes nothing: its setup does, once it ends.
		var n int
		if auto {
			n = c.EnqueueAuto(items)
		} else {
			n = c.Enqueue(items, queue.Upgrade)
		}
		if n != 1 {
			t.Fatalf("queued %d: %+v", n, items)
		}
		h.until(func() bool {
			its := c.Queue.Items()
			return len(its) == 1 && its[0].State.Done()
		})
		return c.Queue.Items()[0]
	}

	it := run(true)
	if it.State != queue.Succeeded || it.To != "2.0" {
		t.Fatalf("auto update: %+v", it)
	}
	log, _ := os.ReadFile(it.LogPath)
	if !strings.Contains(string(log), "等待安装程序结束：setup.tmp") {
		t.Errorf("log %q", log)
	}
	it = run(false)
	if it.State != queue.Succeeded {
		t.Fatalf("update: %+v", it)
	}
	w.mu.Lock()
	silent := slices.Clone(w.silent)
	w.mu.Unlock()
	if !slices.Equal(silent, []string{a.ID}) {
		t.Errorf("silent updates %v: only the one AppShelf started", silent)
	}
	mu.Lock()
	defer mu.Unlock()
	if watches != 2 {
		t.Errorf("watched %d times", watches)
	}
}

// scripted is a source whose updates return errs in turn, then succeed,
// bringing after; modes are the modes they ran in.
type scripted struct {
	*fakeSource
	errs  []error
	modes []string
}

func (s *scripted) up(mode string, a catalog.App) (source.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modes = append(s.modes, mode)
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		return source.Result{}, err
	}
	if st, ok := s.after[a.Source.Key()]; ok {
		s.state[a.Source.Key()] = st
	}
	return source.Result{}, nil
}

func (s *scripted) Upgrade(_ context.Context, a catalog.App, _ io.Writer) (source.Result, error) {
	return s.up("default", a)
}

func (s *scripted) UpgradeInteractive(_ context.Context, a catalog.App, _ io.Writer) (source.Result, error) {
	return s.up("interactive", a)
}

// An update waits while Windows Installer is busy, runs again once when it
// found it busy, runs what is set to run before it, and is retried with
// its installer's window.
func TestUpgradeWaitsForTheInstallerAndRetriesWithAWindow(t *testing.T) {
	src := &scripted{fakeSource: newFake(catalog.Winget)}
	var mu sync.Mutex
	busy := 0 // how many more times Windows Installer is busy
	h := newHarnessWith(t, func(c *Config) {
		c.InstallerBusy = func() bool {
			mu.Lock()
			defer mu.Unlock()
			if busy > 0 {
				busy--
				return true
			}
			return false
		}
	}, src)
	c := h.c
	c.busyPoll = time.Millisecond
	a, err := c.SaveApp(wingetApp("Tool", "Tool.Tool"))
	if err != nil {
		t.Fatal(err)
	}
	key := a.Source.Key()
	h.until(func() bool { _, ok := c.Status(a.ID); return ok })
	var before []string
	var beforeErr error
	c.SetBeforeUpgrade(func(_ context.Context, it queue.Item, log io.Writer) error {
		mu.Lock()
		defer mu.Unlock()
		before = append(before, it.App.ID)
		return beforeErr
	})

	run := func(errs ...error) queue.Item {
		t.Helper()
		src.mu.Lock()
		src.state[key] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
		src.after[key] = source.Status{State: source.Installed, Version: "2.0"}
		src.errs, src.modes = errs, nil
		src.mu.Unlock()
		c.Queue.ClearDone()
		c.Refresh()
		h.until(func() bool { st, _ := c.Status(a.ID); return !c.Refreshing() && st.State == source.UpdateAvailable })
		if n := c.Enqueue(c.Preview([]string{a.ID}, queue.Upgrade), queue.Upgrade); n != 1 {
			t.Fatalf("queued %d", n)
		}
		return h.done()
	}

	mu.Lock()
	busy = 3
	mu.Unlock()
	it := run(fmt.Errorf("%w（安装程序退出码 1618）", source.ErrInstallerBusy))
	log, _ := os.ReadFile(it.LogPath)
	if it.State != queue.Succeeded || !slices.Equal(src.modes, []string{"default", "default"}) {
		t.Errorf("busy: %+v, modes %v", it, src.modes)
	}
	for _, want := range []string{"Windows Installer 正在进行另一个安装或卸载", "没有开始；等它结束后重新开始"} {
		if !strings.Contains(string(log), want) {
			t.Errorf("log lacks %q: %s", want, log)
		}
	}
	// Busy twice in a row, it gives up.
	it = run(source.ErrInstallerBusy, source.ErrInstallerBusy)
	if it.State != queue.Failed || len(src.modes) != 2 {
		t.Errorf("busy twice: %+v, modes %v", it, src.modes)
	}
	// Failed quietly, it is retried with its installer's window.
	it = run(errors.New("安装程序运行失败"))
	if it.State != queue.Failed || !c.CanUpgradeInteractive(catalog.Winget) || c.CanUpgradeInteractive(catalog.Scoop) {
		t.Fatalf("failed: %+v", it)
	}
	if !c.Queue.RetryInteractive(it.ID) {
		t.Fatal("not retried")
	}
	if it = h.done(); it.State != queue.Succeeded || !slices.Equal(src.modes, []string{"default", "interactive"}) {
		t.Errorf("interactive: %+v, modes %v", it, src.modes)
	}
	// What runs before it can keep it from running.
	mu.Lock()
	beforeErr = errors.New("「Tool」还开着")
	mu.Unlock()
	it = run()
	if it.State != queue.Failed || it.Message != "「Tool」还开着" || len(src.modes) != 0 {
		t.Errorf("before: %+v, modes %v", it, src.modes)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(before) != 5 {
		t.Errorf("before ran %d times: %v", len(before), before)
	}
}

// An update waiting for Windows Installer can be canceled: nothing ran.
func TestCancelWhileWaitingForTheInstaller(t *testing.T) {
	src := &scripted{fakeSource: newFake(catalog.Winget)}
	h := newHarnessWith(t, func(c *Config) { c.InstallerBusy = func() bool { return true } }, src)
	c := h.c
	c.busyPoll = time.Millisecond
	a, err := c.SaveApp(wingetApp("Tool", "Tool.Tool"))
	if err != nil {
		t.Fatal(err)
	}
	src.mu.Lock()
	src.state[a.Source.Key()] = source.Status{State: source.UpdateAvailable, Version: "1.0", Latest: "2.0"}
	src.mu.Unlock()
	h.until(func() bool { st, _ := c.Status(a.ID); return st.State == source.UpdateAvailable })
	if n := c.Enqueue(c.Preview([]string{a.ID}, queue.Upgrade), queue.Upgrade); n != 1 {
		t.Fatalf("queued %d", n)
	}
	// Progress posts nothing: look at the queue, which needs no loop.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		its := c.Queue.Items()
		if len(its) == 1 && its[0].State == queue.Running && its[0].CanCancel() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not cancelable: %+v", its)
		}
	}
	if !c.Queue.Cancel(c.Queue.Items()[0].ID) {
		t.Fatal("not canceled")
	}
	it := h.done()
	src.mu.Lock()
	defer src.mu.Unlock()
	if it.State != queue.Failed || !it.Canceled || len(src.modes) != 0 {
		t.Errorf("%+v, modes %v", it, src.modes)
	}
}

// done waits for the only task to end and returns it.
func (h *harness) done() queue.Item {
	h.t.Helper()
	h.until(func() bool {
		its := h.c.Queue.Items()
		return len(its) == 1 && its[0].State.Done()
	})
	return h.c.Queue.Items()[0]
}

func TestApplyRule(t *testing.T) {
	a := catalog.App{ID: "a_1", Source: catalog.Source{Type: catalog.Scoop, Bucket: "main", Name: "git"}, Detect: &catalog.Detect{DisplayName: "^Git$"}}
	entries := []detect.Entry{{DisplayName: "Git", DisplayVersion: "2.55.0"}}
	st := applyRule(a, source.Status{State: source.NotInstalled}, entries, nil)
	if st.State != source.Installed || !st.External || st.Version != "2.55.0" {
		t.Errorf("found outside its source: %+v", st)
	}
	st = applyRule(a, source.Status{State: source.Unavailable, Note: "未安装 scoop"}, entries, nil)
	if !st.External {
		t.Errorf("unavailable source, installed otherwise: %+v", st)
	}
	st = applyRule(a, source.Status{State: source.NotInstalled}, nil, errors.New("拒绝访问"))
	if st.State != source.Unknown {
		t.Errorf("a registry that cannot be read: %+v", st)
	}
	st = applyRule(a, source.Status{State: source.NotInstalled}, []detect.Entry{{DisplayName: "GitHub Desktop"}}, nil)
	if st.State != source.NotInstalled {
		t.Errorf("no match: %+v", st)
	}
}

func TestImportExport(t *testing.T) {
	h := newHarness(t, newFake(catalog.Winget))
	c := h.c
	if err := c.AddCategory("开发"); err != nil {
		t.Fatal(err)
	}
	if err := c.AddTag("常用"); err != nil {
		t.Fatal(err)
	}
	a := wingetApp("Git", "Git.Git")
	a.CategoryID = c.Catalog().Categories[0].ID
	a.TagIDs = []string{c.Catalog().Tags[0].ID}
	git, err := c.SaveApp(a)
	if err != nil {
		t.Fatal(err)
	}
	c.SaveApp(wingetApp("Obsidian", "Obsidian.Obsidian"))
	out := filepath.Join(h.dir, "export", "my-apps.json")
	if err := c.Export(out); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteApp(git.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteTag(c.Catalog().Tags[0].ID); err != nil {
		t.Fatal(err)
	}
	im, err := c.ReadImport(out)
	if err != nil {
		t.Fatal(err)
	}
	if im.Summary.AddedApps != 1 || im.Summary.MergedApps != 1 || im.Summary.AddedTags != 1 {
		t.Errorf("summary %+v", im.Summary)
	}
	if len(c.Catalog().Apps) != 1 {
		t.Error("reading an import changed the catalog")
	}
	if _, err := c.ApplyImport(im); err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog().Apps) != 2 || len(c.Catalog().Tags) != 1 {
		t.Errorf("after import: %d apps, %d tags", len(c.Catalog().Apps), len(c.Catalog().Tags))
	}
	restored := c.Catalog().Apps[1]
	if restored.Name != "Git" || len(restored.TagIDs) != 1 || restored.CategoryID == "" {
		t.Errorf("restored %+v", restored)
	}

	bad := filepath.Join(h.dir, "bad.json")
	os.WriteFile(bad, []byte(`{"schemaVersion":1,"apps":[{"id":"a_1","name":"x","source":{"type":"winget","id":"bad id"}}]}`), 0o644)
	if _, err := c.ReadImport(bad); err == nil {
		t.Error("a bad file was read for import")
	}
	h.until(func() bool { return !c.Refreshing() })
}

func TestSearchAndCategoryErrors(t *testing.T) {
	h := newHarness(t, newFake(catalog.Winget))
	c := h.c
	var got []source.SearchResult
	done := false
	c.Search(catalog.Winget, "git", func(r []source.SearchResult, err error) { got, done = r, true })
	h.until(func() bool { return done })
	if len(got) != 1 || got[0].ID != "Found.git" {
		t.Errorf("search %+v", got)
	}
	c.Search(catalog.Scoop, "git", func(r []source.SearchResult, err error) {
		if err == nil {
			t.Error("a missing source searched")
		}
	})
	if err := c.AddCategory(" "); err == nil {
		t.Error("an empty category was added")
	}
	c.AddCategory("A")
	if err := c.AddCategory("a"); err == nil {
		t.Error("a duplicate category was added")
	}
}

func TestAddApps(t *testing.T) {
	w := newFake(catalog.Winget)
	w.state["winget:git.git"] = source.Status{State: source.Installed, Version: "2.0"}
	h := newHarness(t, w)
	c := h.c
	if _, err := c.SaveApp(wingetApp("Existing", "Some.App")); err != nil {
		t.Fatal(err)
	}
	h.until(func() bool { return !c.Checking(c.Catalog().Apps[0].ID) })
	existing := c.Catalog().Apps[0]

	added, errs := c.AddApps([]catalog.App{
		wingetApp("Git", "Git.Git"),
		wingetApp("", "No.Name"),      // not valid
		wingetApp("Same", "some.app"), // the catalog has its source
		{ID: existing.ID, Name: "Clash", Source: catalog.Source{Type: catalog.Winget, ID: "Other.Id"}},
		{ID: "a_kept", Name: "Kept", Source: catalog.Source{Type: catalog.Winget, ID: "Kept.Id"}},
	})
	if len(added) != 2 || added[0].Name != "Git" || added[1].ID != "a_kept" {
		t.Fatalf("added = %+v", added)
	}
	for i, want := range []bool{false, true, true, true, false} {
		if (errs[i] != nil) != want {
			t.Errorf("errs[%d] = %v", i, errs[i])
		}
	}
	// Saved once with both, and their states read.
	saved, err := catalog.Load(filepath.Join(h.dir, "catalog.json"))
	if err != nil || len(saved.Apps) != 3 {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	h.until(func() bool {
		st, ok := c.Status(added[0].ID)
		return ok && st.State == source.Installed
	})

	// Nothing valid: nothing saved.
	if added, errs := c.AddApps([]catalog.App{wingetApp("", "x.y")}); added != nil || errs[0] == nil {
		t.Errorf("invalid only: %v %v", added, errs)
	}
}

func TestDeleteAndEditApps(t *testing.T) {
	h := newHarness(t, newFake(catalog.Winget))
	c := h.c
	if err := c.AddCategory("工具"); err != nil {
		t.Fatal(err)
	}
	if err := c.AddTag("常用"); err != nil {
		t.Fatal(err)
	}
	cat, tag := c.Catalog().Categories[0], c.Catalog().Tags[0]
	added, _ := c.AddApps([]catalog.App{wingetApp("A", "A.A"), wingetApp("B", "B.B"), wingetApp("C", "C.C")})
	ids := []string{added[0].ID, added[1].ID}
	c.Store().PutGitHub(added[0].ID, state.GitHubRecord{Tag: "v1"})

	if err := c.EditApps(ids, func(a *catalog.App) {
		a.CategoryID = cat.ID
		a.TagIDs = append(a.TagIDs, tag.ID)
		a.ID, a.Source = "changed", catalog.Source{} // ignored
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		a, ok := c.Catalog().App(id)
		if !ok || a.CategoryID != cat.ID || len(a.TagIDs) != 1 || a.Source.ID == "" {
			t.Errorf("edited app = %+v, %v", a, ok)
		}
	}
	if err := c.EditApps(ids, func(a *catalog.App) { a.TagIDs = []string{"t_missing"} }); err == nil {
		t.Error("an unknown tag saved")
	}

	if err := c.DeleteApps(ids); err != nil {
		t.Fatal(err)
	}
	if len(c.Catalog().Apps) != 1 || c.Catalog().Apps[0].Name != "C" {
		t.Errorf("apps left = %+v", c.Catalog().Apps)
	}
	if _, ok := c.Store().GitHub(added[0].ID); ok {
		t.Error("record of a deleted app kept")
	}
	saved, _ := catalog.Load(filepath.Join(h.dir, "catalog.json"))
	if len(saved.Apps) != 1 {
		t.Errorf("saved %d apps", len(saved.Apps))
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		`uninstall-uninstall:arp:machine\x64\{AB}`: `uninstall-uninstall_arp_machine_x64_{AB}`,
		"install-a_1":        "install-a_1",
		"a<b>c\"d|e?f*g/h. ": "a_b_c_d_e_f_g_h",
	} {
		if got := fileName(in); got != want {
			t.Errorf("fileName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := fileName(strings.Repeat("长", 100)); len(got) > 80 {
		t.Errorf("long name kept %d bytes", len(got))
	}
}
