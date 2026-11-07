package view

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/source"
	"appshelf/internal/state"
)

// fakeSource reports the states it is given, by source key.
type fakeSource struct {
	typ   catalog.SourceType
	mu    sync.Mutex
	state map[string]source.Status
}

func (f *fakeSource) Type() catalog.SourceType { return f.typ }
func (f *fakeSource) Check(context.Context) source.Availability {
	return source.Availability{OK: true}
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
func (f *fakeSource) Install(context.Context, catalog.App, io.Writer) (source.Result, error) {
	return source.Result{}, nil
}
func (f *fakeSource) Upgrade(context.Context, catalog.App, io.Writer) (source.Result, error) {
	return source.Result{}, nil
}
func (f *fakeSource) Search(_ context.Context, q string) ([]source.SearchResult, error) {
	return []source.SearchResult{{ID: "Found." + q, Name: q + " App", Version: "1.0", Source: "winget"}}, nil
}

type fixture struct {
	t     *testing.T
	v     *View
	c     *core.Core
	src   *fakeSource
	posts chan func()
	tt    *ui.Tester
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{t: t, posts: make(chan func(), 256), src: &fakeSource{typ: catalog.Winget, state: map[string]source.Status{}}}
	post := func(fn func()) { f.posts <- fn }
	c, err := core.Open(context.Background(), core.Config{
		Paths:   core.Paths{Catalog: filepath.Join(dir, "catalog.json"), State: filepath.Join(dir, "state.json"), Logs: filepath.Join(dir, "logs")},
		Sources: func(*state.Store) []source.Source { return []source.Source{f.src} },
		Post:    post,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.c = c
	f.v = New(c, Hooks{Post: post})
	return f
}

// settle runs what background work handed back, until cond holds, and
// draws a frame.
func (f *fixture) settle(cond func() bool) {
	f.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case fn := <-f.posts:
			fn()
		case <-deadline:
			f.t.Fatal("timed out")
		}
	}
	for {
		select {
		case fn := <-f.posts:
			fn()
			continue
		default:
		}
		break
	}
	if f.tt != nil {
		f.tt.Frame()
	}
}

func (f *fixture) start() {
	f.tt = ui.NewTester(f.v.Build, 1180, 900)
}

func (f *fixture) click(text string) {
	f.t.Helper()
	if err := f.tt.Click(text); err != nil {
		f.t.Fatalf("click %q: %v\ntexts: %q", text, err, f.tt.Texts())
	}
}

// catalog adds 开发 and 设计, the tag 常用, and three apps: Alpha in 开发,
// Beta in 设计 with 常用, Gamma in neither.
func (f *fixture) catalog() (alpha, beta, gamma catalog.App) {
	f.t.Helper()
	c := f.c
	c.AddCategory("开发")
	c.AddCategory("设计")
	c.AddTag("常用")
	cat := c.Catalog()
	var err error
	alpha, err = c.SaveApp(catalog.App{Name: "Alpha", CategoryID: cat.Categories[0].ID, Source: catalog.Source{Type: catalog.Winget, ID: "Test.Alpha"}})
	if err != nil {
		f.t.Fatal(err)
	}
	beta, _ = c.SaveApp(catalog.App{Name: "Beta", CategoryID: cat.Categories[1].ID, TagIDs: []string{cat.Tags[0].ID}, Source: catalog.Source{Type: catalog.Winget, ID: "Test.Beta"}})
	gamma, _ = c.SaveApp(catalog.App{Name: "Gamma", Source: catalog.Source{Type: catalog.Winget, ID: "Test.Gamma"}})
	f.settle(func() bool {
		for _, a := range []catalog.App{alpha, beta, gamma} {
			if _, ok := c.Status(a.ID); !ok {
				return false
			}
		}
		return true
	})
	return
}

func TestFilterIsUnion(t *testing.T) {
	f := newFixture(t)
	f.catalog()
	f.start()
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		if !f.tt.HasText(name) {
			t.Fatalf("%s missing before filtering: %q", name, f.tt.Texts())
		}
	}
	f.click("开发（1）")
	if !f.tt.HasText("Alpha") || f.tt.HasText("Beta") || f.tt.HasText("Gamma") {
		t.Errorf("category 开发: %q", f.tt.Texts())
	}
	f.click("常用（1）")
	if !f.tt.HasText("Alpha") || !f.tt.HasText("Beta") || f.tt.HasText("Gamma") {
		t.Errorf("开发 or 常用: %q", f.tt.Texts())
	}
	f.click("全选")
	if !f.tt.HasText("已选 2 项") {
		t.Errorf("select all: %q", f.tt.Texts())
	}
	f.click("未分类（1）")
	if !f.tt.HasText("Gamma") {
		t.Errorf("uncategorized: %q", f.tt.Texts())
	}
	f.click("清除")
	if !f.tt.HasText("Gamma") || !f.tt.HasText("共 3 个软件") {
		t.Errorf("cleared: %q", f.tt.Texts())
	}
}

func TestPreviewSkipsInstalled(t *testing.T) {
	f := newFixture(t)
	alpha, _, _ := f.catalog()
	f.src.mu.Lock()
	f.src.state[alpha.Source.Key()] = source.Status{State: source.Installed, Version: "3.0"}
	f.src.mu.Unlock()
	f.c.Refresh()
	f.settle(func() bool { return !f.c.Refreshing() })
	f.start()
	if !f.tt.HasText("已安装 3.0") {
		t.Fatalf("installed state not shown: %q", f.tt.Texts())
	}
	f.click("全选")
	f.click("安装")
	if !f.tt.HasText("安装预览") || !f.tt.HasText("跳过：已安装") {
		t.Fatalf("preview: %q", f.tt.Texts())
	}
	if !f.tt.HasText("将逐个安装 2 项，执行前会再检查一次安装状态") {
		t.Errorf("runnable count: %q", f.tt.Texts())
	}
	f.click("开始")
	if got := len(f.c.Queue.Items()); got != 2 {
		t.Errorf("queued %d", got)
	}
	f.settle(func() bool {
		for _, it := range f.c.Queue.Items() {
			if !it.State.Done() {
				return false
			}
		}
		return true
	})
	if !f.tt.HasText("Beta") || f.v.tab != tabTasks {
		t.Errorf("tasks page: %q", f.tt.Texts())
	}
}

func TestEditorValidates(t *testing.T) {
	f := newFixture(t)
	f.catalog()
	f.start()

	f.click("添加软件")
	f.click("软件名称")
	f.tt.Type("Tool")
	// winget: no package chosen from a search yet.
	f.click("保存")
	if !f.tt.HasText("请先搜索，并从结果中选择一个 winget 软件包") {
		t.Errorf("no package: %q", f.tt.Texts())
	}

	// GitHub without a detection rule.
	f.click("GitHub")
	f.click("仓库 owner/name")
	f.tt.Type("owner/tool")
	f.click("保存")
	if !f.tt.HasText("软件「Tool」：GitHub 来源必须填写检测规则") && !f.tt.HasText("GitHub 来源必须填写检测规则") {
		t.Errorf("no rule: %q", f.tt.Texts())
	}
	if len(f.c.Catalog().Apps) != 3 {
		t.Errorf("an invalid app was saved: %+v", f.c.Catalog().Apps)
	}
}

func TestSettingsCategories(t *testing.T) {
	f := newFixture(t)
	f.catalog()
	f.start()
	f.click("设置")
	f.click("新分类")
	f.tt.Type("效率")
	f.click("添加")
	if len(f.c.Catalog().Categories) != 3 || !f.tt.HasText("效率") {
		t.Fatalf("category not added: %+v", f.c.Catalog().Categories)
	}
	f.click("新分类")
	f.tt.Type("开发")
	f.click("添加")
	if !f.tt.HasText("分类名称重复：开发") && !strings.Contains(strings.Join(f.tt.Texts(), "\n"), "重复") {
		t.Errorf("a duplicate name gave no error: %q", f.tt.Texts())
	}
	// 开发 holds Alpha: deleting it says so, and Alpha becomes uncategorized.
	if err := f.tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if !f.tt.HasText("1 个软件将变为未分类，软件本身不会删除。") {
		t.Fatalf("delete confirmation: %q", f.tt.Texts())
	}
	// The alert's last button, 删除, is its default: Enter presses it.
	f.tt.Key(0, ui.KeyEnter)
	if len(f.c.Catalog().Categories) != 2 || f.c.Catalog().CategoryUsage("") != 2 {
		t.Errorf("after deleting: %+v", f.c.Catalog().Categories)
	}
}

func TestEditorPicksSearchResult(t *testing.T) {
	f := newFixture(t)
	f.catalog()
	f.start()
	f.click("添加软件")
	f.click("搜索关键词")
	f.tt.Type("Obsidian")
	f.click("搜索")
	f.settle(func() bool { return len(f.v.editor.results) > 0 })
	if !f.tt.HasText("Obsidian App") {
		t.Fatalf("results: %q", f.tt.Texts())
	}
	f.click("选择")
	if !f.tt.HasText("Found.Obsidian") {
		t.Errorf("chosen package not shown: %q", f.tt.Texts())
	}
	f.click("保存")
	var saved *catalog.App
	for _, a := range f.c.Catalog().Apps {
		if a.Source.ID == "Found.Obsidian" {
			saved = &a
		}
	}
	if saved == nil || saved.Name != "Obsidian App" {
		t.Fatalf("saved %+v", f.c.Catalog().Apps)
	}
	if f.v.editor.open {
		t.Error("the editor stayed open")
	}
	f.settle(func() bool { _, ok := f.c.Status(saved.ID); return ok })
	if !f.tt.HasText("Obsidian App") || !strings.Contains(strings.Join(f.tt.Texts(), "\n"), "未安装") {
		t.Errorf("the new app in the table: %q", f.tt.Texts())
	}
}
