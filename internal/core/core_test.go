package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
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
	dir := t.TempDir()
	h := &harness{t: t, posts: make(chan func(), 256), dir: dir}
	c, err := Open(context.Background(), Config{
		Paths:   Paths{Catalog: filepath.Join(dir, "catalog.json"), State: filepath.Join(dir, "state.json"), Logs: filepath.Join(dir, "logs")},
		Sources: func(*state.Store) []source.Source { return sources },
		Post:    func(fn func()) { h.posts <- fn },
	})
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
