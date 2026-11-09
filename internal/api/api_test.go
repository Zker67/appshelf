package api

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/detect"
	"appshelf/internal/discover"
	"appshelf/internal/jsonfile"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
	"appshelf/internal/state"
)

// coreSource is a source for the core: states by source key, installing
// sets the state after.
type coreSource struct {
	typ           catalog.SourceType
	avail         source.Availability
	mu            sync.Mutex
	state         map[string]source.Status
	after         map[string]source.Status
	actions       []string
	upgradeErrors int
	// fixErr is what Fix returns; Fix without an error makes the source
	// available.
	fixErr error
	fixes  int
}

func newCoreSource(t catalog.SourceType) *coreSource {
	return &coreSource{typ: t, avail: source.Availability{OK: true}, state: map[string]source.Status{}, after: map[string]source.Status{}}
}

func (f *coreSource) Type() catalog.SourceType { return f.typ }
func (f *coreSource) Check(context.Context) source.Availability {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.avail
}
func (f *coreSource) Fix(context.Context, io.Writer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fixes++
	if f.fixErr != nil {
		return f.fixErr
	}
	f.avail = source.Availability{OK: true}
	return nil
}
func (f *coreSource) get(a catalog.App) source.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.state[a.Source.Key()]; ok {
		return st
	}
	return source.Status{State: source.NotInstalled}
}
func (f *coreSource) Statuses(_ context.Context, apps []catalog.App) (map[string]source.Status, error) {
	out := map[string]source.Status{}
	for _, a := range apps {
		out[a.ID] = f.get(a)
	}
	return out, nil
}
func (f *coreSource) Recheck(_ context.Context, a catalog.App) (source.Status, error) {
	return f.get(a), nil
}
func (f *coreSource) Install(_ context.Context, a catalog.App, _ io.Writer) (source.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, "install:"+a.ID)
	if st, ok := f.after[a.Source.Key()]; ok {
		f.state[a.Source.Key()] = st
	}
	return source.Result{}, nil
}
func (f *coreSource) Upgrade(ctx context.Context, a catalog.App, log io.Writer) (source.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, "upgrade:"+a.ID)
	if f.upgradeErrors > 0 {
		f.upgradeErrors--
		return source.Result{}, errors.New("temporary update failure")
	}
	if st, ok := f.after[a.Source.Key()]; ok {
		f.state[a.Source.Key()] = st
	}
	return source.Result{}, nil
}

// fakeWinget is what the services use of winget.
type fakeWinget struct {
	results []source.SearchResult
	block   chan struct{} // a search waits for it, or its context
	pkgs    []winget.Package
	mu      sync.Mutex
	queries []string
	// listBlock, when set, makes reading what is installed wait for it;
	// lists counts the readings.
	listBlock chan struct{}
	lists     int

	versions map[string]string
	verr     error
	vblock   chan struct{} // reading versions waits for it, or its context
	vqueries []string
	warmed   int
	// real, when set, answers the searches.
	real *winget.Source
}

func (w *fakeWinget) SearchVersions(ctx context.Context, q string) (map[string]string, error) {
	w.mu.Lock()
	w.vqueries = append(w.vqueries, q)
	block, v, err, real := w.vblock, w.versions, w.verr, w.real
	w.mu.Unlock()
	if real != nil {
		return real.SearchVersions(ctx, q)
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return v, err
}
func (w *fakeWinget) Warm(ctx context.Context) error {
	w.mu.Lock()
	w.warmed++
	real := w.real
	w.mu.Unlock()
	if real != nil {
		return real.Warm(ctx)
	}
	return nil
}

func (w *fakeWinget) Check(context.Context) source.Availability { return source.Availability{OK: true} }
func (w *fakeWinget) Search(ctx context.Context, q string) ([]source.SearchResult, error) {
	w.mu.Lock()
	w.queries = append(w.queries, q)
	block, real := w.block, w.real
	w.mu.Unlock()
	if real != nil {
		return real.Search(ctx, q)
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return w.results, nil
}
func (w *fakeWinget) Installed(context.Context, time.Duration) ([]winget.Package, time.Time, error) {
	w.mu.Lock()
	block := w.listBlock
	w.lists++
	w.mu.Unlock()
	if block != nil {
		<-block
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.pkgs), time.Now(), nil
}
func (w *fakeWinget) LatestVersion(context.Context, string) (string, error) { return "1.0", nil }

type fakeScoop struct{}

func (fakeScoop) Check(context.Context) source.Availability {
	return source.Availability{Reason: "未安装 scoop"}
}
func (fakeScoop) Search(context.Context, string) ([]source.SearchResult, error) { return nil, nil }
func (fakeScoop) Installed(context.Context) ([]scoop.App, error)                { return nil, nil }
func (fakeScoop) Info(context.Context, string, string) (scoop.Info, error)      { return scoop.Info{}, nil }

type fakeGitHub struct {
	repos []github.Repo
	mu    sync.Mutex
	token string // the token carried
}

// UseToken accepts tokens starting with ghp_good.
func (g *fakeGitHub) UseToken(_ context.Context, token string) error {
	if !strings.HasPrefix(token, "ghp_good") {
		return github.ErrBadToken
	}
	g.SetToken(token)
	return nil
}
func (g *fakeGitHub) SetToken(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = token
}
func (g *fakeGitHub) carried() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.token
}

func (g *fakeGitHub) SearchRepos(context.Context, string, int) ([]github.Repo, error) {
	return g.repos, nil
}
func (g *fakeGitHub) Repository(context.Context, string) (github.Repo, error) {
	return github.Repo{}, nil
}
func (g *fakeGitHub) Describe(_ context.Context, app catalog.App) (github.Release, github.Asset, error) {
	if app.Source.Repo == "o/none" {
		return github.Release{}, github.Asset{}, errors.New("没有正式版 release")
	}
	return github.Release{Tag: "v2"}, github.Asset{Name: "setup.exe"}, nil
}
func (g *fakeGitHub) SearchRate() github.Rate {
	return github.Rate{Known: true, Limit: 10, Remaining: 9}
}

// env is an App on a core with fake sources.
type env struct {
	t       *testing.T
	app     *App
	core    *core.Core
	winget  *coreSource
	wapi    *fakeWinget
	gh      *fakeGitHub
	dir     string
	mu      sync.Mutex
	changes []Change
	opened  []string
	file    string // what OpenFile and SaveFile answer
	theme   ThemeSource

	shelf     *Shelf
	tasks     *Tasks
	sources   *Sources
	discover  *Discover
	installed *Installed
	updates   *Updates
	about     *About

	// notified are the notifications shown, as "title|body"; active is
	// whether the window is in front.
	notified []string
	active   bool
	// registry are entries the registry has beside Tool's.
	registry []detect.Entry
	// uninst plays the removal of programs.
	uninst *fakeUninstaller
}

// newEnv starts AppShelf on fakes; opts change its Config first.
func newEnv(t *testing.T, opts ...func(*Config)) *env {
	t.Helper()
	e := &env{t: t, dir: t.TempDir()}
	e.app = New(Hooks{
		Emit: func(c Change) {
			e.mu.Lock()
			e.changes = append(e.changes, c)
			e.mu.Unlock()
		},
		OpenPath: func(p string) error { e.opened = append(e.opened, p); return nil },
		OpenURL:  func(u string) error { e.opened = append(e.opened, u); return nil },
		SaveFile: func(context.Context, string, string) (string, error) { return e.file, nil },
		OpenFile: func(context.Context, string) (string, error) { return e.file, nil },
		SetTheme: func(t ThemeSource) { e.theme = t },
		Notify: func(title, body string) {
			e.mu.Lock()
			e.notified = append(e.notified, title+"|"+body)
			e.mu.Unlock()
		},
		Active: func() bool {
			e.mu.Lock()
			defer e.mu.Unlock()
			return e.active
		},
	})
	for _, s := range e.app.Services() {
		switch s := s.(type) {
		case *Updates:
			e.updates = s
		case *Shelf:
			e.shelf = s
		case *Tasks:
			e.tasks = s
		case *Sources:
			e.sources = s
		case *Discover:
			e.discover = s
		case *Installed:
			e.installed = s
		case *About:
			e.about = s
		}
	}
	e.winget = newCoreSource(catalog.Winget)
	e.uninst = &fakeUninstaller{gone: map[string]bool{}, keep: map[string]bool{}}
	c, err := core.Open(context.Background(), core.Config{
		Paths:       core.Paths{Catalog: filepath.Join(e.dir, "catalog.json"), State: filepath.Join(e.dir, "state.json"), Logs: filepath.Join(e.dir, "logs")},
		Sources:     func(*state.Store) []source.Source { return []source.Source{e.winget, newCoreSource(catalog.GitHub)} },
		Post:        e.app.Post,
		Uninstaller: e.uninst,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.core = c
	e.gh = &fakeGitHub{repos: []github.Repo{{FullName: "localsend/localsend", Description: "Share files", Stars: 50000}}}
	e.wapi = &fakeWinget{
		results: []source.SearchResult{{ID: "Obsidian.Clipper", Name: "Obsidian Clipper", Version: "1"}, {ID: "Obsidian.Obsidian", Name: "Obsidian", Version: "1.9"}},
		pkgs:    []winget.Package{{ID: "Obsidian.Obsidian", Name: "Obsidian", Version: "1.8", Source: "winget", Update: "1.9"}, {ID: `ARP\Machine\X64\Tool`, Name: "Tool 2.1 (x64)", Version: "2.1"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := Config{
		Core:    c,
		Sources: SourceAPIs{Winget: e.wapi, Scoop: fakeScoop{}, GitHub: e.gh},
		Details: discover.NewDetails(discover.Fetchers{}, "", ""),
		Scan: func() ([]detect.Entry, error) {
			entries := []detect.Entry{{DisplayName: "Tool 2.1 (x64)", DisplayVersion: "2.1", Publisher: "Tool Co", Name: "Tool", Scope: "Machine", Arch: "X64", Key: `HKLM\Tool`}}
			e.mu.Lock()
			defer e.mu.Unlock()
			return append(entries, e.registry...), nil
		},
		Version:  "0.2.0",
		DataDir:  e.dir,
		Settings: filepath.Join(e.dir, "settings.json"),
		IconBase: "http://icon.localhost",

		TokenFile: filepath.Join(e.dir, "github-token.bin"),
		EnvToken:  "ghp_fromenv",
	}
	for _, o := range opts {
		o(&cfg)
	}
	e.app.Start(ctx, cfg)
	e.until(func() bool { return !e.shelf.Snapshot().Refreshing })
	e.until(func() bool { l := e.installed.List(); return l.Loaded && !l.Partial })
	return e
}

// until waits for cond.
func (e *env) until(cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			e.t.Fatal("timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// kindsSince returns the kinds of the changes emitted after n changes.
func (e *env) changesSince(n int) []Change {
	time.Sleep(150 * time.Millisecond)
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.changes[n:])
}

func (e *env) changeCount() int {
	time.Sleep(150 * time.Millisecond) // let pending changes out
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.changes)
}

func TestShelfAndChanges(t *testing.T) {
	e := newEnv(t)
	n := e.changeCount()
	saved, err := e.shelf.SaveApp(catalog.App{Name: "Git", Source: catalog.Source{Type: catalog.Winget, ID: "Git.Git"}})
	if err != nil {
		t.Fatal(err)
	}
	changes := e.changesSince(n)
	if len(changes) == 0 || !slices.Contains(changes[0].Kinds, ChangedShelf) {
		t.Fatalf("changes after saving = %+v", changes)
	}
	// Changes close together come as one.
	n = e.changeCount()
	for i := range 5 {
		if _, err := e.shelf.AddTag("标签" + string(rune('A'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if changes := e.changesSince(n); len(changes) == 0 || len(changes) >= 5 {
		t.Errorf("%d changes for five quick edits: %+v", len(changes), changes)
	}

	e.until(func() bool {
		s := e.shelf.Snapshot()
		return len(s.Apps) == 1 && s.Apps[0].Status != nil && !s.Apps[0].Checking
	})
	snap := e.shelf.Snapshot()
	app := snap.Apps[0]
	if app.App.ID != saved.ID || app.Icon != "/w/Git.Git" || app.Status.State != StateNotInstalled || snap.Uncategorized != 1 || len(snap.Tags) != 5 {
		t.Errorf("snapshot = %+v", snap)
	}
	if _, err := e.shelf.SaveApp(catalog.App{Name: "", Source: catalog.Source{Type: catalog.Winget, ID: "X.Y"}}); err == nil {
		t.Error("an app without a name saved")
	}

	cat, err := e.shelf.AddCategory("开发")
	if err != nil || cat.ID == "" || cat.Name != "开发" {
		t.Fatalf("category = %+v, %v", cat, err)
	}
	if err := e.shelf.SetCategory([]string{saved.ID}, cat.ID); err != nil {
		t.Fatal(err)
	}
	tag := e.shelf.Snapshot().Tags[0]
	if err := e.shelf.SetTag([]string{saved.ID}, tag.ID, true); err != nil {
		t.Fatal(err)
	}
	snap = e.shelf.Snapshot()
	if snap.Apps[0].App.CategoryID != cat.ID || !slices.Equal(snap.Apps[0].App.TagIDs, []string{tag.ID}) || snap.Categories[0].Count != 1 || snap.Tags[0].Count != 1 {
		t.Errorf("after category and tag: %+v", snap)
	}
	if err := e.shelf.SetTag([]string{saved.ID}, tag.ID, false); err != nil || len(e.shelf.Snapshot().Apps[0].App.TagIDs) != 0 {
		t.Errorf("tag not removed: %v", err)
	}
	if err := e.shelf.RenameCategory(cat.ID, "开发工具"); err != nil || e.shelf.Snapshot().Categories[0].Name != "开发工具" {
		t.Errorf("rename: %v", err)
	}
	if err := e.shelf.DeleteCategory(cat.ID); err != nil || e.shelf.Snapshot().Uncategorized != 1 {
		t.Errorf("delete category: %v", err)
	}
}

func TestAddPackagesAndUndoDelete(t *testing.T) {
	e := newEnv(t)
	res, err := e.shelf.AddPackages(AddRequest{
		Items: []AddItem{
			{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}},
			{Name: "", Source: catalog.Source{Type: catalog.Winget, ID: "No.Name"}},
			{Name: "LocalSend", Source: catalog.Source{Type: catalog.GitHub, Repo: "localsend/localsend"}, Detect: &catalog.Detect{DisplayName: "^LocalSend"}},
		},
		NewCategory: "效率",
		NewTags:     []string{"常用", "常用", " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Added) != 2 || len(res.Failed) != 1 || res.Failed[0].Name != "" {
		t.Fatalf("result = %+v", res)
	}
	snap := e.shelf.Snapshot()
	if len(snap.Categories) != 1 || len(snap.Tags) != 1 || snap.Categories[0].Count != 2 || snap.Tags[0].Count != 2 {
		t.Fatalf("snapshot = %+v", snap)
	}
	// The same category by name again: no second one.
	if _, err := e.shelf.AddPackages(AddRequest{Items: []AddItem{{Name: "Git", Source: catalog.Source{Type: catalog.Winget, ID: "Git.Git"}}}, NewCategory: "效率"}); err != nil {
		t.Fatal(err)
	}
	if n := len(e.shelf.Snapshot().Categories); n != 1 {
		t.Errorf("%d categories", n)
	}
	if _, err := e.shelf.AddPackages(AddRequest{}); err == nil {
		t.Error("nothing to add accepted")
	}

	e.core.Store().PutGitHub(res.Added[1], state.GitHubRecord{Tag: "v1", AssetName: "a.exe"})
	token, err := e.shelf.DeleteApps(res.Added)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(e.shelf.Snapshot().Apps); n != 1 {
		t.Fatalf("%d apps after deleting", n)
	}
	catID := e.shelf.Snapshot().Categories[0].ID
	if err := e.shelf.DeleteCategory(catID); err != nil {
		t.Fatal(err)
	}
	n, err := e.shelf.UndoDelete(token)
	if err != nil || n != 2 {
		t.Fatalf("undo = %d, %v", n, err)
	}
	snap = e.shelf.Snapshot()
	var ids []string
	for _, a := range snap.Apps {
		ids = append(ids, a.App.ID)
		if a.App.CategoryID != "" {
			t.Errorf("restored into a deleted category: %+v", a.App)
		}
	}
	for _, id := range res.Added {
		if !slices.Contains(ids, id) {
			t.Errorf("app %s not restored", id)
		}
	}
	if r, ok := e.core.Store().GitHub(res.Added[1]); !ok || r.Tag != "v1" {
		t.Error("GitHub record not restored")
	}
	if _, err := e.shelf.UndoDelete(token); err == nil {
		t.Error("undone twice")
	}
}

func TestTasks(t *testing.T) {
	e := newEnv(t)
	e.winget.state["winget:has.it"] = source.Status{State: source.Installed, Version: "1"}
	e.winget.after["winget:new.app"] = source.Status{State: source.Installed, Version: "2"}
	res, err := e.shelf.AddPackages(AddRequest{Items: []AddItem{
		{Name: "New", Source: catalog.Source{Type: catalog.Winget, ID: "New.App"}},
		{Name: "Has", Source: catalog.Source{Type: catalog.Winget, ID: "Has.It"}},
	}})
	if err != nil || len(res.Added) != 2 {
		t.Fatal(res, err)
	}
	// Started at once, while the states of the apps just added are read:
	// Start waits for them, installs New and skips Has, installed already.
	n, err := e.tasks.Start(context.Background(), res.Added, ActionInstall)
	if err != nil || n != 1 {
		t.Fatalf("start = %d, %v", n, err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded && !l.Running
	})
	item := e.tasks.List().Items[0]
	if item.Name != "New" || item.Icon != "/w/New.App" || item.Action != ActionInstall || !item.HasLog || item.Started.IsZero() {
		t.Errorf("task = %+v", item)
	}
	if err := e.tasks.OpenLog(item.ID); err != nil || len(e.opened) != 1 {
		t.Errorf("open log: %v %v", err, e.opened)
	}
	lg, err := e.tasks.Log(item.ID, 0)
	if err != nil || !lg.Done || lg.Start != 0 || lg.Next != len(lg.Lines) || len(lg.Lines) < 3 || !strings.HasPrefix(lg.Lines[0], "AppShelf 安装「New」") {
		t.Errorf("log = %+v, %v", lg, err)
	}
	if more, err := e.tasks.Log(item.ID, lg.Next); err != nil || len(more.Lines) != 0 || more.Next != lg.Next {
		t.Errorf("log from its end = %+v, %v", more, err)
	}
	if err := e.tasks.Retry(item.ID); err == nil {
		t.Error("a task that succeeded retried")
	}
	e.tasks.ClearDone()
	if len(e.tasks.List().Items) != 0 {
		t.Error("done tasks not cleared")
	}
	if _, err := e.tasks.Start(context.Background(), res.Added[1:], ActionInstall); err == nil {
		t.Error("an installed app queued")
	}
	e.until(func() bool {
		a := e.shelf.Snapshot().Apps[0]
		return a.Status != nil && a.Status.State == StateInstalled
	})
}

func TestSearch(t *testing.T) {
	e := newEnv(t)
	if _, err := e.shelf.SaveApp(catalog.App{Name: "Clipper", Source: catalog.Source{Type: catalog.Winget, ID: "obsidian.clipper"}}); err != nil {
		t.Fatal(err)
	}
	var batches []SearchBatch
	if err := e.app.searchAll(context.Background(), "obsidian", func(b SearchBatch) error { batches = append(batches, b); return nil }); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(batches, func(a, b SearchBatch) int { return strings.Compare(string(a.Type), string(b.Type)) })
	if len(batches) != 2 || batches[0].Type != catalog.Scoop || !batches[0].Unavailable || batches[1].Type != catalog.Winget {
		t.Fatalf("batches = %+v", batches)
	}
	hits := batches[1].Hits
	if len(hits) != 2 || hits[0].ID != "Obsidian.Obsidian" || !hits[0].Installed || hits[0].InShelf != "" || hits[1].InShelf == "" || hits[0].Icon != "/w/Obsidian.Obsidian" {
		t.Errorf("hits = %+v", hits)
	}

	// A new search cancels the one under way, which sends nothing.
	e.wapi.mu.Lock()
	e.wapi.block = make(chan struct{})
	e.wapi.mu.Unlock()
	first := make(chan []SearchBatch, 1)
	go func() {
		var got []SearchBatch
		e.app.searchAll(context.Background(), "obs", func(b SearchBatch) error { got = append(got, b); return nil })
		first <- got
	}()
	e.until(func() bool { e.wapi.mu.Lock(); defer e.wapi.mu.Unlock(); return len(e.wapi.queries) == 2 })
	e.wapi.mu.Lock()
	e.wapi.block = nil
	e.wapi.mu.Unlock()
	var second []SearchBatch
	e.app.searchAll(context.Background(), "obsidian", func(b SearchBatch) error { second = append(second, b); return nil })
	got := <-first
	for _, b := range got {
		if b.Type == catalog.Winget {
			t.Errorf("canceled search sent %+v", b)
		}
	}
	if len(second) != 2 {
		t.Errorf("second search sent %d batches", len(second))
	}

	gh, err := e.discover.SearchGitHub(context.Background(), "localsend")
	if err != nil || len(gh.Hits) != 1 || gh.Hits[0].Name != "localsend" || gh.Hits[0].Stars != 50000 || gh.Rate.Remaining != 9 || gh.Hits[0].Icon != "/g/localsend/localsend" {
		t.Errorf("github = %+v, %v", gh, err)
	}
	rules := e.discover.SuggestRules("someone/tool")
	if len(rules) != 2 || rules[0].From != "Tool 2.1 (x64)" || rules[1].Rule != "^tool" {
		t.Errorf("rules = %+v", rules)
	}
	hitsEd, err := e.sources.Search(context.Background(), catalog.Winget, "obsidian")
	if err != nil || len(hitsEd) != 2 {
		t.Errorf("editor search = %+v, %v", hitsEd, err)
	}
	if _, err := e.sources.Search(context.Background(), catalog.Scoop, "x"); err == nil || !strings.Contains(err.Error(), "scoop") {
		t.Errorf("scoop search: %v", err)
	}
	chk, err := e.sources.CheckGitHub(context.Background(), "o/app", "")
	if err != nil || chk.Tag != "v2" || chk.Asset != "setup.exe" {
		t.Errorf("check = %+v, %v", chk, err)
	}
	if _, err := e.sources.CheckGitHub(context.Background(), "not a repo", ""); err == nil {
		t.Error("bad repository checked")
	}
}

func TestInstalledList(t *testing.T) {
	e := newEnv(t)
	if _, err := e.shelf.SaveApp(catalog.App{Name: "Obsidian", Source: catalog.Source{Type: catalog.Winget, ID: "Obsidian.Obsidian"}}); err != nil {
		t.Fatal(err)
	}
	list := e.installed.List()
	if !list.Loaded || len(list.Entries) != 2 {
		t.Fatalf("list = %+v", list)
	}
	var obs, tool InstalledEntry
	for _, en := range list.Entries {
		switch en.Kind {
		case InstalledWinget:
			obs = en
		case InstalledLocal:
			tool = en
		}
	}
	if obs.Source == nil || obs.Source.ID != "Obsidian.Obsidian" || len(obs.InShelf) != 1 || obs.Update != "1.9" || obs.Rule != "" {
		t.Errorf("winget entry = %+v", obs)
	}
	if tool.Source != nil || tool.Publisher != "Tool Co" || tool.Rule != `^Tool v?\d[\w.+-]* \(x64\)$` || !strings.HasPrefix(tool.Icon, "/i/") {
		t.Errorf("local entry = %+v", tool)
	}
	// Adding it with the rule matches it.
	if _, err := e.shelf.AddPackages(AddRequest{Items: []AddItem{{Name: "Tool", Source: catalog.Source{Type: catalog.Winget, ID: "Tool.Tool"}, Detect: &catalog.Detect{DisplayName: tool.Rule}}}}); err != nil {
		t.Fatal(err)
	}
	for _, en := range e.installed.List().Entries {
		if en.Kind == InstalledLocal && len(en.InShelf) != 1 {
			t.Errorf("local entry not matched: %+v", en)
		}
	}
	n := e.changeCount()
	e.installed.Refresh()
	e.until(func() bool { return !e.installed.List().Loading })
	found := false
	for _, c := range e.changesSince(n) {
		found = found || slices.Contains(c.Kinds, ChangedInstalled)
	}
	if !found {
		t.Error("no change for the installed list")
	}
}

// TestInstalledShowsTheRegistryFirst lists the registry's programs before
// winget answers, with the keys they keep once it does.
func TestInstalledShowsTheRegistryFirst(t *testing.T) {
	e := newEnv(t)
	block := make(chan struct{})
	e.wapi.mu.Lock()
	e.wapi.listBlock = block
	e.wapi.mu.Unlock()
	e.app.inv = &inventoryState{app: e.app}

	e.until(func() bool { return e.installed.List().Partial })
	list := e.installed.List()
	if !list.Loaded || !list.Loading || len(list.Entries) != 1 {
		t.Fatalf("partial list = %+v", list)
	}
	tool := list.Entries[0]
	if tool.Key != `arp:machine\x64\tool` || tool.Kind != InstalledLocal || tool.Publisher != "Tool Co" {
		t.Fatalf("registry entry = %+v", tool)
	}
	close(block)
	e.until(func() bool { l := e.installed.List(); return !l.Partial && !l.Loading })
	keys := map[string]bool{}
	for _, en := range e.installed.List().Entries {
		keys[en.Key] = true
	}
	if !keys[tool.Key] || !keys["winget:obsidian.obsidian"] {
		t.Fatalf("full list keys = %v", keys)
	}
}

// TestInstalledReadsAgainAfterOutsideChanges reads the list once changes
// outside AppShelf stop coming.
func TestInstalledReadsAgainAfterOutsideChanges(t *testing.T) {
	old := settleDelay
	settleDelay = 50 * time.Millisecond
	t.Cleanup(func() { settleDelay = old })
	e := newEnv(t)
	e.wapi.mu.Lock()
	before := e.wapi.lists
	e.wapi.mu.Unlock()
	for range 5 {
		e.app.inv.outside()
		time.Sleep(10 * time.Millisecond)
	}
	e.until(func() bool {
		e.wapi.mu.Lock()
		defer e.wapi.mu.Unlock()
		return e.wapi.lists > before
	})
	time.Sleep(150 * time.Millisecond)
	e.wapi.mu.Lock()
	n := e.wapi.lists - before
	e.wapi.mu.Unlock()
	if n != 1 {
		t.Fatalf("read %d times after a burst of changes, want once", n)
	}
}

func TestImportExport(t *testing.T) {
	e := newEnv(t)
	if _, err := e.shelf.SaveApp(catalog.App{Name: "Git", Source: catalog.Source{Type: catalog.Winget, ID: "Git.Git"}}); err != nil {
		t.Fatal(err)
	}
	e.file = filepath.Join(e.dir, "export")
	path, err := e.shelf.Export(context.Background())
	if err != nil || path != e.file+".json" {
		t.Fatalf("export = %q, %v", path, err)
	}
	if _, err := catalog.Load(path); err != nil {
		t.Fatal(err)
	}
	other := catalog.New()
	other.Apps = append(other.Apps, catalog.App{ID: "a_x", Name: "Git again", Source: catalog.Source{Type: catalog.Winget, ID: "git.git"}}, catalog.App{ID: "a_y", Name: "7-Zip", Source: catalog.Source{Type: catalog.Winget, ID: "7zip.7zip"}})
	data, _ := other.Marshal()
	e.file = filepath.Join(e.dir, "in.json")
	os.WriteFile(e.file, data, 0o644)
	prev, err := e.shelf.ReadImport(context.Background())
	if err != nil || prev == nil || prev.File != "in.json" || prev.Counts.AddedApps != 1 || prev.Counts.MergedApps != 1 {
		t.Fatalf("preview = %+v, %v", prev, err)
	}
	counts, err := e.shelf.ApplyImport(prev.Token)
	if err != nil || counts.AddedApps != 1 || len(e.shelf.Snapshot().Apps) != 2 {
		t.Fatalf("import = %+v, %v", counts, err)
	}
	if _, err := e.shelf.ApplyImport(prev.Token); err == nil {
		t.Error("imported twice")
	}
	e.file = ""
	if prev, err := e.shelf.ReadImport(context.Background()); prev != nil || err != nil {
		t.Errorf("canceled import = %+v, %v", prev, err)
	}
}

func TestAbout(t *testing.T) {
	e := newEnv(t)
	info := e.about.Info()
	if info.Version != "0.2.0" || info.CanUpdate || info.Theme != ThemeSystem || info.IconBase != "http://icon.localhost" || info.LogsDir == "" {
		t.Errorf("info = %+v", info)
	}
	if err := e.about.CheckForUpdates(); err == nil {
		t.Error("a copy that cannot update itself checked")
	}
	for _, bad := range []string{"file:///C:/Windows", "javascript:alert(1)", "ms-settings:", "https://"} {
		if err := e.about.OpenURL(bad); err == nil {
			t.Errorf("opened %q", bad)
		}
	}
	if err := e.about.OpenURL("https://example.com/x"); err != nil || e.opened[len(e.opened)-1] != "https://example.com/x" {
		t.Errorf("open url: %v %v", err, e.opened)
	}
	if err := e.about.SetTheme(ThemeDark); err != nil || e.theme != ThemeDark {
		t.Fatal(err)
	}
	if LoadTheme(filepath.Join(e.dir, "settings.json")) != ThemeDark || e.about.Info().Theme != ThemeDark {
		t.Error("theme not kept")
	}
	if err := e.about.SetTheme("purple"); err == nil {
		t.Error("unknown theme set")
	}
	ov := e.sources.Overview()
	if len(ov.Sources) != 2 || !ov.Sources[0].Checked || !ov.Sources[0].OK || ov.GitHubSearchRate.Remaining != 9 {
		t.Errorf("overview = %+v", ov)
	}
}

func TestNotifier(t *testing.T) {
	var got []Change
	var mu sync.Mutex
	n := newNotifier(30*time.Millisecond, func(c Change) { mu.Lock(); got = append(got, c); mu.Unlock() })
	n.mark(ChangedTasks)
	n.mark(ChangedShelf, ChangedTasks)
	n.mark()
	time.Sleep(100 * time.Millisecond)
	n.mark(ChangedInstalled)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || !slices.Equal(got[0].Kinds, []ChangeKind{ChangedShelf, ChangedTasks}) || got[1].Revision != 2 {
		t.Errorf("changes = %+v", got)
	}
}

func TestLoopPanics(t *testing.T) {
	a := New(Hooks{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.loop.run(ctx, func() {})
	close(a.ready)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic lost")
			}
		}()
		a.read(func() { panic("boom") })
	}()
	// The loop goes on.
	ran := false
	a.read(func() { ran = true })
	if !ran {
		t.Error("loop stopped")
	}
}

// TestInstalledCache keeps the list read for the next run, which shows it
// at once while it reads the list again; a removal waits for that reading.
func TestInstalledCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	e := newEnv(t, func(c *Config) { c.InstalledCache = path })
	var kept inventoryCache
	if err := jsonfile.Read(path, &kept); err != nil || kept.Version != cacheVersion || len(kept.List.Entries) != len(e.installed.List().Entries) {
		t.Fatalf("kept list: %+v %v", kept, err)
	}

	next := New(Hooks{})
	next.cfg = Config{InstalledCache: path}
	next.inv.restore()
	list, loaded, _ := next.inv.get()
	if !loaded || len(list.Entries) != len(kept.List.Entries) || list.Entries[0].Reg == nil && kept.List.Entries[0].Reg != nil {
		t.Fatalf("restored: loaded %v, %+v", loaded, list)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := next.inv.fresh(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a removal used the earlier run's list: %v", err)
	}

	// A file of another version, or none, shows nothing.
	if err := jsonfile.WriteAtomic(path, inventoryCache{Version: cacheVersion + 1, List: kept.List}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, filepath.Join(filepath.Dir(path), "missing.json")} {
		other := New(Hooks{})
		other.cfg = Config{InstalledCache: p}
		other.inv.restore()
		if other.inv.loaded {
			t.Errorf("restored %s", filepath.Base(p))
		}
	}
}
