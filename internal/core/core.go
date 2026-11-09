// Package core is AppShelf's application layer: it holds the catalog, the
// state of each app on this machine and the install queue, and runs the
// sources' work in the background.
//
// Its methods are called on the UI thread, which alone reads and changes
// what Core holds. PreviewUpdates may read sources off-loop without changing
// the catalog or status maps. Background work hands its results back through post,
// which runs a function on the UI thread (Window.Update in the app).
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/jsonfile"
	"appshelf/internal/queue"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
	"appshelf/internal/state"
)

// Paths are where AppShelf keeps its files.
type Paths struct {
	Catalog string // the catalog, catalog.json
	State   string // the machine's state, state.json
	Logs    string // the directory of the install logs
}

// Config is what Open needs.
type Config struct {
	Paths Paths
	// Sources makes the sources, once the state is open: the GitHub source
	// keeps what it installed there.
	Sources func(store *state.Store) []source.Source
	// Post runs a function on the UI thread and redraws.
	Post func(func())
	// Progress is called, at most every 250 milliseconds, while a task
	// reports progress or writes output; nil when nothing listens.
	Progress func()
	// Uninstaller removes programs; nil when this build cannot.
	Uninstaller Uninstaller
}

// Core is the application.
type Core struct {
	ctx     context.Context
	paths   Paths
	post    func(func())
	cat     *catalog.Catalog
	store   *state.Store
	sources map[catalog.SourceType]source.Source

	uninstaller Uninstaller

	statuses    map[string]source.Status
	refreshing  bool
	again       bool // refresh again when the one running ends
	lastRefresh time.Time
	checking    map[string]bool // apps whose state is being read alone

	availMu sync.Mutex
	avail   map[catalog.SourceType]source.Availability

	// Queue installs and updates the apps.
	Queue *queue.Queue
	// Notes are messages from opening, as a file that could not be read,
	// for the interface to show once.
	Notes []string
}

// Open reads the catalog and the state and returns the application. A file
// that cannot be read is renamed aside, and AppShelf starts without it,
// saying so in Notes.
func Open(ctx context.Context, cfg Config) (*Core, error) {
	c := &Core{
		ctx:      ctx,
		paths:    cfg.Paths,
		post:     cfg.Post,
		sources:  map[catalog.SourceType]source.Source{},
		statuses: map[string]source.Status{},
		checking: map[string]bool{},
		avail:    map[catalog.SourceType]source.Availability{},

		uninstaller: cfg.Uninstaller,
	}
	cat, err := catalog.Load(cfg.Paths.Catalog)
	if err != nil {
		aside, rerr := setAside(cfg.Paths.Catalog)
		if rerr != nil {
			return nil, fmt.Errorf("清单文件无法读取（%v），也无法改名：%w", err, rerr)
		}
		c.Notes = append(c.Notes, fmt.Sprintf("清单文件无法读取：%v。原文件已改名为 %s，现在从空清单开始。", err, filepath.Base(aside)))
		cat = catalog.New()
	}
	c.cat = cat
	store, err := state.Open(cfg.Paths.State)
	if err != nil {
		aside, rerr := setAside(cfg.Paths.State)
		if rerr != nil {
			return nil, fmt.Errorf("本机状态文件无法读取（%v），也无法改名：%w", err, rerr)
		}
		c.Notes = append(c.Notes, fmt.Sprintf("%v。原文件已改名为 %s。", err, filepath.Base(aside)))
		if store, err = state.Open(cfg.Paths.State); err != nil {
			return nil, err
		}
	}
	c.store = store
	if cfg.Sources != nil {
		for _, s := range cfg.Sources(store) {
			c.sources[s.Type()] = s
		}
	}
	c.Queue = queue.New(ctx, executor{c}, func() { c.post(func() {}) })
	c.Queue.SetProgress(cfg.Progress)
	return c, nil
}

// setAside renames a file that cannot be read, keeping it for the user.
func setAside(path string) (string, error) {
	aside := path + ".broken-" + time.Now().Format("20060102-150405")
	return aside, os.Rename(path, aside)
}

// Catalog returns the catalog, which the caller must not change: edits go
// through Core's methods, which save them.
func (c *Core) Catalog() *catalog.Catalog { return c.cat }

// Store returns the state of this machine.
func (c *Core) Store() *state.Store { return c.store }

// Paths returns where AppShelf keeps its files.
func (c *Core) Paths() Paths { return c.paths }

// Status returns what is known of an app's state.
func (c *Core) Status(id string) (source.Status, bool) {
	st, ok := c.statuses[id]
	return st, ok
}

// Checking reports whether the app's state is being read.
func (c *Core) Checking(id string) bool { return c.refreshing || c.checking[id] }

// Refreshing reports whether the states of all apps are being read.
func (c *Core) Refreshing() bool { return c.refreshing }

// LastRefresh returns when the states were last read.
func (c *Core) LastRefresh() time.Time { return c.lastRefresh }

// Availability returns what the last check said of a source.
func (c *Core) Availability(t catalog.SourceType) (source.Availability, bool) {
	c.availMu.Lock()
	defer c.availMu.Unlock()
	av, ok := c.avail[t]
	return av, ok
}

func (c *Core) setAvailability(t catalog.SourceType, av source.Availability) {
	c.availMu.Lock()
	c.avail[t] = av
	c.availMu.Unlock()
}

// HasSource reports whether a source type is set up.
func (c *Core) HasSource(t catalog.SourceType) bool { _, ok := c.sources[t]; return ok }

// Refresh checks the sources and reads the state of every app, as when
// AppShelf starts and on "检查更新".
func (c *Core) Refresh() {
	if c.refreshing {
		c.again = true
		return
	}
	c.refreshing = true
	apps := cloneApps(c.cat.Apps)
	go func() {
		statuses := c.readAll(apps)
		c.post(func() {
			for id, st := range statuses {
				c.statuses[id] = st
			}
			c.prune()
			c.refreshing = false
			c.lastRefresh = time.Now()
			if c.again {
				c.again = false
				c.Refresh()
			}
		})
	}()
}

// readAll runs on a goroutine: the sources side by side, then the
// detection rules.
func (c *Core) readAll(apps []catalog.App) map[string]source.Status {
	return c.readAllContext(c.ctx, apps, true)
}

func (c *Core) readAllContext(ctx context.Context, apps []catalog.App, checkUnused bool) map[string]source.Status {
	groups := map[catalog.SourceType][]catalog.App{}
	for _, a := range apps {
		groups[a.Source.Type] = append(groups[a.Source.Type], a)
	}
	out := map[string]source.Status{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for t, src := range c.sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			group := groups[t]
			if !checkUnused && len(group) == 0 {
				return
			}
			av := src.Check(ctx)
			c.setAvailability(t, av)
			if len(group) == 0 {
				return
			}
			res := map[string]source.Status{}
			if !av.OK {
				for _, a := range group {
					res[a.ID] = source.Status{State: source.Unavailable, Note: av.Reason}
				}
			} else {
				got, err := src.Statuses(ctx, group)
				for _, a := range group {
					switch st, ok := got[a.ID]; {
					case err != nil:
						res[a.ID] = source.UnknownStatus(err.Error())
					case !ok:
						res[a.ID] = source.UnknownStatus("来源没有返回这个软件的状态")
					default:
						res[a.ID] = st
					}
				}
			}
			mu.Lock()
			for id, st := range res {
				out[id] = st
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	for _, a := range apps {
		if _, ok := c.sources[a.Source.Type]; !ok {
			out[a.ID] = source.Status{State: source.Unavailable, Note: "不支持的来源：" + string(a.Source.Type)}
		}
	}
	var entries []detect.Entry
	var scanErr error
	scanned := false
	for _, a := range apps {
		if !usesRule(a) {
			continue
		}
		if !scanned {
			entries, scanErr = detect.Scan()
			scanned = true
		}
		out[a.ID] = applyRule(a, out[a.ID], entries, scanErr)
	}
	return out
}

// usesRule reports whether AppShelf applies the app's detection rule on top
// of its source: GitHub releases apply it themselves.
func usesRule(a catalog.App) bool { return a.Detect != nil && a.Source.Type != catalog.GitHub }

// applyRule lets an app's detection rule find it installed when its
// source does not: installed some other way, it is not to be installed
// again. A rule that cannot be read makes "not installed" unknown.
func applyRule(a catalog.App, st source.Status, entries []detect.Entry, scanErr error) source.Status {
	if st.IsInstalled() || st.State == source.Unknown {
		return st
	}
	if scanErr != nil {
		return source.UnknownStatus("检测规则无法读取注册表：" + scanErr.Error())
	}
	r, err := detect.Match(*a.Detect, entries)
	if err != nil {
		return source.UnknownStatus(err.Error())
	}
	if !r.Installed {
		return st
	}
	return source.Status{State: source.Installed, Version: r.Version, External: true, Note: "已安装（来源外）：" + r.Via}
}

// recheck reads one app's state now, on the calling goroutine, with its
// detection rule: what the queue checks before and after each item.
func (c *Core) recheck(ctx context.Context, a catalog.App) source.Status {
	src, ok := c.sources[a.Source.Type]
	if !ok {
		return source.Status{State: source.Unavailable, Note: "不支持的来源：" + string(a.Source.Type)}
	}
	av, ok := c.Availability(a.Source.Type)
	if !ok || !av.OK {
		// Not checked yet, or unavailable when last checked: check again,
		// the user may have fixed it since.
		av = src.Check(ctx)
		c.setAvailability(a.Source.Type, av)
	}
	var st source.Status
	if !av.OK {
		st = source.Status{State: source.Unavailable, Note: av.Reason}
	} else if s, err := src.Recheck(ctx, a); err != nil {
		st = source.UnknownStatus(err.Error())
	} else {
		st = s
	}
	if usesRule(a) {
		entries, err := detect.Scan()
		st = applyRule(a, st, entries, err)
	}
	return st
}

// checkOne reads one app's state in the background, as after it is added
// or changed.
func (c *Core) checkOne(a catalog.App) {
	if c.checking[a.ID] {
		return
	}
	c.checking[a.ID] = true
	a = a.Clone()
	go func() {
		st := c.recheck(c.ctx, a)
		c.post(func() {
			delete(c.checking, a.ID)
			if _, ok := c.cat.App(a.ID); ok {
				c.statuses[a.ID] = st
			}
		})
	}()
}

// prune forgets the states of apps no longer in the catalog.
func (c *Core) prune() {
	ids := map[string]bool{}
	for _, a := range c.cat.Apps {
		ids[a.ID] = true
	}
	for id := range c.statuses {
		if !ids[id] {
			delete(c.statuses, id)
		}
	}
}

func cloneApps(apps []catalog.App) []catalog.App {
	out := make([]catalog.App, len(apps))
	for i, a := range apps {
		out[i] = a.Clone()
	}
	return out
}

// edit changes a copy of the catalog and keeps it once it is saved.
func (c *Core) edit(fn func(n *catalog.Catalog) error) error {
	n := c.cat.Clone()
	if err := fn(n); err != nil {
		return err
	}
	if err := n.Save(c.paths.Catalog); err != nil {
		var ve *catalog.ValidationError
		if errors.As(err, &ve) {
			return err
		}
		return fmt.Errorf("保存清单失败：%w", err)
	}
	c.cat = n
	return nil
}

// SaveApp adds or changes an app, saves the catalog and reads the app's
// state again. An app changed keeps its note: SetNote changes that.
func (c *Core) SaveApp(a catalog.App) (catalog.App, error) {
	var saved catalog.App
	var old catalog.App
	var existed bool
	if a.ID != "" {
		old, existed = c.cat.App(a.ID)
	}
	if existed {
		a.Note = old.Note
	}
	err := c.edit(func(n *catalog.Catalog) error {
		var err error
		saved, err = n.PutApp(a)
		return err
	})
	if err != nil {
		return catalog.App{}, err
	}
	if existed && old.Source.Key() != saved.Source.Key() {
		// Another source: what AppShelf installed of the old one no longer
		// describes this app.
		c.store.Forget(saved.ID)
	}
	delete(c.statuses, saved.ID)
	c.checkOne(saved)
	return saved, nil
}

// DeleteApp removes an app from the catalog.
func (c *Core) DeleteApp(id string) error { return c.DeleteApps([]string{id}) }

// DeleteApps removes apps from the catalog, saving it once, and forgets
// what AppShelf installed of them.
func (c *Core) DeleteApps(ids []string) error {
	if err := c.edit(func(n *catalog.Catalog) error {
		for _, id := range ids {
			n.DeleteApp(id)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, id := range ids {
		delete(c.statuses, id)
		c.store.Forget(id)
	}
	return nil
}

// AddApps adds apps to the catalog, each on its own: an app that is not
// valid, or whose source the catalog has already, is left out, with its
// error at its index. Apps keep their IDs when they have one, as when a
// deletion is undone. The catalog is saved once; the states of a few apps
// are read one by one, of more all at once.
func (c *Core) AddApps(apps []catalog.App) ([]catalog.App, []error) {
	errs := make([]error, len(apps))
	n := c.cat.Clone()
	var added []catalog.App
	for i, a := range apps {
		if a.ID != "" {
			if _, ok := n.App(a.ID); ok {
				errs[i] = &catalog.ValidationError{Where: "软件「" + a.Name + "」", Field: "id", Message: "ID 已存在"}
				continue
			}
		}
		saved, err := n.PutApp(a)
		if err != nil {
			errs[i] = err
			continue
		}
		added = append(added, saved)
	}
	if len(added) == 0 {
		return nil, errs
	}
	if err := n.Save(c.paths.Catalog); err != nil {
		err = fmt.Errorf("保存清单失败：%w", err)
		for i := range errs {
			if errs[i] == nil {
				errs[i] = err
			}
		}
		return nil, errs
	}
	c.cat = n
	if len(added) <= 3 {
		for _, a := range added {
			c.checkOne(a)
		}
	} else {
		c.Refresh()
	}
	return added, errs
}

// EditApps changes apps of the catalog, as their category or tags, and
// saves it once. fn must not change their IDs or sources.
func (c *Core) EditApps(ids []string, fn func(a *catalog.App)) error {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	return c.edit(func(n *catalog.Catalog) error {
		for i := range n.Apps {
			if want[n.Apps[i].ID] {
				id, src := n.Apps[i].ID, n.Apps[i].Source
				fn(&n.Apps[i])
				n.Apps[i].ID, n.Apps[i].Source = id, src
			}
		}
		return nil
	})
}

// SetNote changes the note of an app and saves the catalog; "" removes it.
// The app's state is not read again.
func (c *Core) SetNote(id, note string) error {
	return c.edit(func(n *catalog.Catalog) error { return n.SetNote(id, note) })
}

// AddCategory adds a category.
func (c *Core) AddCategory(name string) error {
	return c.edit(func(n *catalog.Catalog) error { _, err := n.AddCategory(name); return err })
}

// RenameCategory renames a category.
func (c *Core) RenameCategory(id, name string) error {
	return c.edit(func(n *catalog.Catalog) error { return n.RenameCategory(id, name) })
}

// DeleteCategory removes a category; its apps become uncategorized.
func (c *Core) DeleteCategory(id string) error {
	return c.edit(func(n *catalog.Catalog) error { n.DeleteCategory(id); return nil })
}

// AddTag adds a tag.
func (c *Core) AddTag(name string) error {
	return c.edit(func(n *catalog.Catalog) error { _, err := n.AddTag(name); return err })
}

// RenameTag renames a tag.
func (c *Core) RenameTag(id, name string) error {
	return c.edit(func(n *catalog.Catalog) error { return n.RenameTag(id, name) })
}

// DeleteTag removes a tag from the catalog and its apps.
func (c *Core) DeleteTag(id string) error {
	return c.edit(func(n *catalog.Catalog) error { n.DeleteTag(id); return nil })
}

// Export writes the catalog to path, as its file is.
func (c *Core) Export(path string) error {
	data, err := c.cat.Marshal()
	if err != nil {
		return err
	}
	if err := jsonfile.WriteBytesAtomic(path, data); err != nil {
		return fmt.Errorf("导出失败：%w", err)
	}
	return nil
}

// Import is a catalog read for importing, with what merging it would do.
type Import struct {
	Path     string
	incoming *catalog.Catalog
	Summary  catalog.MergeSummary
}

const maxImportSize = 16 << 20

// ReadImport reads and checks the file at path, and works out what merging
// it would change. Nothing changes until ApplyImport.
func (c *Core) ReadImport(path string) (*Import, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件：%w", err)
	}
	if fi.Size() > maxImportSize {
		return nil, errors.New("文件太大，不是 AppShelf 清单")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件：%w", err)
	}
	in, err := catalog.Parse(data)
	if err != nil {
		return nil, err
	}
	_, sum, err := catalog.Merge(c.cat, in)
	if err != nil {
		return nil, err
	}
	return &Import{Path: path, incoming: in, Summary: sum}, nil
}

// ApplyImport merges an import into the catalog, as it is now, saves it and
// reads the states again.
func (c *Core) ApplyImport(im *Import) (catalog.MergeSummary, error) {
	var sum catalog.MergeSummary
	err := c.edit(func(n *catalog.Catalog) error {
		merged, s, err := catalog.Merge(n, im.incoming)
		if err != nil {
			return err
		}
		*n, sum = *merged, s
		return nil
	})
	if err != nil {
		return catalog.MergeSummary{}, err
	}
	c.Refresh()
	return sum, nil
}

// Search searches a source for packages, in the background, and calls done
// on the UI thread.
func (c *Core) Search(t catalog.SourceType, query string, done func([]source.SearchResult, error)) {
	s, ok := c.sources[t].(source.Searcher)
	if !ok {
		done(nil, errors.New("这个来源不支持搜索"))
		return
	}
	go func() {
		res, err := s.Search(c.ctx, query)
		c.post(func() { done(res, err) })
	}()
}

// fixer is a source that can make itself usable: winget installs its
// PowerShell module, scoop installs scoop.
type fixer interface {
	Fix(ctx context.Context, log io.Writer) error
}

// CanFix reports whether a source can make itself usable.
func (c *Core) CanFix(t catalog.SourceType) bool {
	_, ok := c.sources[t].(fixer)
	return ok
}

// Fix makes a source usable in the background, as by installing winget's
// PowerShell module or scoop, then checks the source again and reads the
// states again.
func (c *Core) Fix(t catalog.SourceType, done func(error)) {
	src := c.sources[t]
	f, ok := src.(fixer)
	if !ok {
		done(errors.New("这个来源无法由 AppShelf 安装"))
		return
	}
	go func() {
		log, _, lerr := c.openLog("setup-" + string(t))
		var w io.Writer = io.Discard
		if lerr == nil {
			w = log
			defer log.Close()
		}
		err := f.Fix(c.ctx, w)
		c.setAvailability(t, src.Check(c.ctx))
		c.post(func() {
			done(err)
			if err == nil {
				c.Refresh()
			}
		})
	}()
}

type describer interface {
	Describe(ctx context.Context, app catalog.App) (github.Release, github.Asset, error)
}

// DescribeGitHub reads the latest release of a GitHub app and the installer
// AppShelf would choose, in the background.
func (c *Core) DescribeGitHub(a catalog.App, done func(github.Release, github.Asset, error)) {
	d, ok := c.sources[catalog.GitHub].(describer)
	if !ok {
		done(github.Release{}, github.Asset{}, errors.New("GitHub 来源不可用"))
		return
	}
	a = a.Clone()
	go func() {
		rel, asset, err := d.Describe(c.ctx, a)
		c.post(func() { done(rel, asset, err) })
	}()
}

type rater interface {
	Rate() github.Rate
	HasToken() bool
}

// GitHubRate returns GitHub's request quota as last reported, and whether
// requests carry a token.
func (c *Core) GitHubRate() (github.Rate, bool) {
	r, ok := c.sources[catalog.GitHub].(rater)
	if !ok {
		return github.Rate{}, false
	}
	return r.Rate(), r.HasToken()
}

// openLog creates a log file named after what it records.
func (c *Core) openLog(what string) (*os.File, string, error) {
	if err := os.MkdirAll(c.paths.Logs, 0o755); err != nil {
		return nil, "", err
	}
	name := time.Now().Format("20060102-150405") + "-" + fileName(what) + ".log"
	path := filepath.Join(c.paths.Logs, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

// fileName makes s fit in a file's name: no characters Windows refuses,
// at most 80 bytes.
func fileName(s string) string {
	b := []rune{}
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '_'
		}
		b = append(b, r)
	}
	out := strings.TrimRight(string(b), ". ")
	for len(out) > 80 {
		r := []rune(out)
		out = string(r[:len(r)-1])
	}
	return out
}

// PreviewItem is what installing or updating an app would do.
type PreviewItem struct {
	Status   source.Status
	App      catalog.App
	Runnable bool
	Plan     string // "安装", or why not
	Detail   string // the exact source, and the installer of a release
}

// Preview says what installing or updating the apps would do, from the
// states known: the queue checks each app again before it runs.
func (c *Core) Preview(ids []string, action queue.Action) []PreviewItem {
	out := make([]PreviewItem, 0, len(ids))
	for _, id := range ids {
		a, ok := c.cat.App(id)
		if !ok {
			continue
		}
		st, known := c.statuses[id]
		out = append(out, c.preview(a, st, known, action))
	}
	return out
}

func (c *Core) preview(a catalog.App, st source.Status, known bool, action queue.Action) PreviewItem {
	item := PreviewItem{App: a, Status: st, Detail: describeSource(a, st)}
	switch {
	case c.Queue.Pending(a.ID) || c.Queue.PendingSource(a.Source):
		item.Plan = "跳过：已在任务中"
	case !known:
		item.Plan = "跳过：还没有读到安装状态"
	case st.State == source.Unavailable:
		item.Plan = "不可用：" + st.Note
	case st.State == source.Unknown:
		item.Plan = "跳过：无法确认安装状态（" + st.Note + "）"
	case action == queue.Install && st.External:
		item.Plan = "跳过：已安装（来源外）"
	case action == queue.Install && st.IsInstalled():
		item.Plan = "跳过：已安装"
	case action == queue.Install:
		item.Runnable, item.Plan = true, "安装"
	case st.State == source.NotInstalled:
		item.Plan = "跳过：未安装"
	case st.External:
		item.Plan = "跳过：不是通过这个来源安装的"
	case st.State != source.UpdateAvailable && st.Note != "":
		// The check may not have been able to look for updates.
		item.Plan = "跳过：没有发现更新（" + st.Note + "）"
	case st.State != source.UpdateAvailable:
		item.Plan = "跳过：已是最新版本"
	default:
		item.Runnable, item.Plan = true, "更新 "+orDash(st.Version)+" → "+st.Latest
	}
	if item.Runnable && st.Note != "" {
		item.Detail += "；" + st.Note
	}
	return item
}

func describeSource(a catalog.App, st source.Status) string {
	switch a.Source.Type {
	case catalog.Winget:
		return "winget · " + a.Source.ID
	case catalog.Scoop:
		return "scoop · " + a.Source.Bucket + "/" + a.Source.Name
	case catalog.GitHub:
		s := "GitHub · " + a.Source.Repo
		if st.Asset != "" {
			s += " · " + st.Asset
			if st.Latest != "" {
				s += "（" + st.Latest + "）"
			}
		}
		return s
	}
	return string(a.Source.Type)
}

func orDash(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// Enqueue queues the runnable items of a preview and returns how many.
func (c *Core) Enqueue(items []PreviewItem, action queue.Action) int {
	var apps []catalog.App
	for _, it := range items {
		if it.Runnable {
			apps = append(apps, it.App)
		}
	}
	if len(apps) == 0 {
		return 0
	}
	return c.Queue.Add(apps, action)
}

// executor does the queue's work, on its goroutine.
type executor struct{ c *Core }

func (e executor) Status(ctx context.Context, it queue.Item) source.Status {
	if it.Action == queue.Uninstall {
		return e.c.presence(ctx, it.Target)
	}
	return e.c.recheck(ctx, it.App)
}

func (e executor) Run(ctx context.Context, it queue.Item, log io.Writer) (source.Result, error) {
	if it.Action == queue.Uninstall {
		return e.c.runUninstall(ctx, it, log)
	}
	a, action := it.App, it.Action
	src, ok := e.c.sources[a.Source.Type]
	if !ok {
		return source.Result{}, errors.New("不支持的来源")
	}
	fmt.Fprintf(log, "AppShelf %s「%s」\n来源：%s %s\n开始：%s\n\n", action, a.Name, a.Source.Type, a.Source.Display(), time.Now().Format(time.DateTime))
	if action == queue.Upgrade {
		return src.Upgrade(ctx, a, log)
	}
	return src.Install(ctx, a, log)
}

func (e executor) OpenLog(it queue.Item) (io.WriteCloser, string, error) {
	verb := "install"
	switch it.Action {
	case queue.Upgrade:
		verb = "upgrade"
	case queue.Uninstall:
		verb = "uninstall"
	}
	f, path, err := e.c.openLog(verb + "-" + it.App.ID)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

func (e executor) Finished(it queue.Item, st source.Status, res source.Result) source.Status {
	c := e.c
	if it.Action == queue.Uninstall {
		c.afterUninstall(it.Target)
		return st
	}
	if res.GitHub != nil && st.IsInstalled() {
		if err := c.store.PutGitHub(it.App.ID, *res.GitHub); err == nil {
			// The record can change what the state is: read it again.
			st = c.recheck(c.ctx, it.App)
		}
	}
	c.post(func() {
		if _, ok := c.cat.App(it.App.ID); ok {
			c.statuses[it.App.ID] = st
		}
	})
	return st
}

// LogsDir returns the directory of the logs, creating it.
func (c *Core) LogsDir() string {
	os.MkdirAll(c.paths.Logs, 0o755)
	return c.paths.Logs
}

// SourceTypes returns the source types set up, in the interface's order.
func (c *Core) SourceTypes() []catalog.SourceType {
	var out []catalog.SourceType
	for _, t := range catalog.SourceTypes {
		if _, ok := c.sources[t]; ok {
			out = append(out, t)
		}
	}
	return out
}
