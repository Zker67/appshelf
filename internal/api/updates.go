package api

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/inventory"
	"appshelf/internal/procs"
	"appshelf/internal/queue"
	"appshelf/internal/source"
	"appshelf/internal/state"
	"appshelf/internal/version"
)

// Navigate asks the page to show one of its pages, as "updates" after a
// click on a notification.
var Navigate = mygo.NewEvent[string]("navigate")

// Updates is the updates of what AppShelf can update, one entry per
// package: the catalog's apps and the installed winget and scoop
// packages, how each is to be updated, the checks and their settings.
//
// The page names packages by their source keys only: Go finds their
// sources again, as Installed does, and the queue checks each again
// before and after it runs.
type Updates struct{ a *App }

// pkg is a package that can be updated, as the catalog and the installed
// programs know it.
type pkg struct {
	key      string
	name     string
	icon     string
	src      catalog.Source
	version  string
	latest   string
	update   bool // its source has a newer version
	external bool // installed some other way than its source
	note     string
	pending  bool
	// appID is the catalog's app; known that its state was read, and so
	// decides over the installed list's.
	appID string
	known bool
	// entry is the installed program, of winget or scoop.
	entry *inventory.Entry
}

// packages are what can be updated, by source key, in the order found.
type packages struct {
	byKey  map[string]*pkg
	order  []string
	loaded bool
}

func (a *App) packages() packages {
	<-a.ready
	list, loaded, _ := a.inv.get()
	out := packages{byKey: map[string]*pkg{}}
	add := func(p *pkg) {
		out.byKey[p.key] = p
		out.order = append(out.order, p.key)
	}
	refreshed := false
	a.read(func() {
		c := a.core
		refreshed = !c.LastRefresh().IsZero()
		for _, app := range c.Catalog().Apps {
			key := app.Source.Key()
			if _, dup := out.byKey[key]; dup {
				continue
			}
			p := &pkg{key: key, name: app.Name, icon: iconOf(app.Source), src: app.Source, appID: app.ID,
				pending: c.Queue.Pending(app.ID) || c.Queue.PendingSource(app.Source)}
			if st, ok := c.Status(app.ID); ok && !st.Blocked() {
				p.known = true
				p.version, p.external, p.note = st.Version, st.External, st.Note
				if st.State == source.UpdateAvailable && !st.External {
					p.update, p.latest = true, st.Latest
				}
			}
			add(p)
		}
	})
	q := a.core.Queue
	for i := range list.Entries {
		e := &list.Entries[i]
		src, ok := entrySource(*e)
		if !ok {
			continue
		}
		key := src.Key()
		p, ok := out.byKey[key]
		if !ok {
			p = &pkg{key: key, name: e.Name, icon: "/i/" + url.PathEscape(e.Key), src: src, pending: q.PendingSource(src)}
			add(p)
		}
		if p.entry == nil {
			p.entry = e
			if !p.known {
				p.version = knownVersion(e.Version)
				if e.Update != "" {
					p.update, p.latest = true, e.Update
				}
			}
		} else if !strings.EqualFold(p.entry.Scope, e.Scope) {
			// Installed for the machine and for the user: which one an
			// update changes is not known.
			cp := *p.entry
			cp.Scope = ""
			p.entry = &cp
		}
	}
	out.loaded = loaded && refreshed
	return out
}

// entrySource is the source of an installed winget or scoop package.
func entrySource(e inventory.Entry) (catalog.Source, bool) {
	switch e.Kind {
	case inventory.Winget:
		return catalog.Source{Type: catalog.Winget, ID: e.WingetID}, e.WingetID != ""
	case inventory.Scoop:
		return catalog.Source{Type: catalog.Scoop, Bucket: e.Bucket, Name: e.ScoopName}, e.ScoopName != ""
	}
	return catalog.Source{}, false
}

func knownVersion(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "unknown") {
		return ""
	}
	return strings.TrimSpace(v)
}

// autoBlock says why AppShelf cannot update a package on its own, without
// a window or an administrator prompt, or "" when it can.
func autoBlock(p *pkg) string {
	switch {
	case p.src.Type == catalog.GitHub:
		return "GitHub 软件的安装程序可能弹出窗口或要求管理员权限，只提醒，不自动更新"
	case p.src.Type != catalog.Winget && p.src.Type != catalog.Scoop:
		return "这个来源不支持自动更新"
	case p.external:
		return "不是通过这个来源安装的"
	case p.entry == nil:
		return "本机列表里没有找到它，无法确认安装方式"
	case p.entry.Scope == "Machine" && p.src.Type == catalog.Scoop:
		return "为所有用户安装（scoop --global），更新需要管理员权限"
	case p.entry.Scope == "Machine":
		return "为所有用户安装，更新会弹出管理员确认"
	case p.entry.Scope == "":
		return "无法确认安装范围，更新可能需要管理员确认"
	case p.version == "":
		return "读不到当前版本"
	case installDir(p) == "":
		return "找不到安装目录，无法确认它是否在运行"
	}
	return ""
}

// installDir is where a package's programs are, or "" when not known well
// enough to tell whether they run.
func installDir(p *pkg) string {
	e := p.entry
	if e == nil {
		return ""
	}
	if e.Kind == inventory.Scoop {
		return inventory.ScoopDir(e.ScoopName, e.Scope == "Machine")
	}
	if dir := ownDir(e.Location); dir != "" {
		return dir
	}
	icon := strings.Trim(strings.TrimSpace(e.Icon), `"`)
	if i := strings.LastIndexByte(icon, ','); i > 0 {
		icon = strings.Trim(strings.TrimSpace(icon[:i]), `"`)
	}
	if !strings.EqualFold(filepath.Ext(icon), ".exe") {
		return ""
	}
	return ownDir(filepath.Dir(icon))
}

// ownDir returns dir when it is a program's own directory: absolute, and
// not a directory programs share, as Program Files or the system's.
func ownDir(dir string) string {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	dir = filepath.Clean(dir)
	if filepath.Dir(dir) == dir {
		return "" // a drive's root
	}
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData", "LOCALAPPDATA", "APPDATA", "USERPROFILE", "PUBLIC"} {
		shared := os.Getenv(v)
		if shared == "" {
			continue
		}
		shared = filepath.Clean(shared)
		if strings.EqualFold(dir, shared) || strings.EqualFold(dir, filepath.Join(shared, "Programs")) {
			return ""
		}
	}
	if sys := os.Getenv("SystemRoot"); sys != "" {
		sys = filepath.Clean(sys)
		if strings.EqualFold(dir, sys) || strings.HasPrefix(strings.ToLower(dir), strings.ToLower(sys)+`\`) {
			return ""
		}
	}
	if users := filepath.Dir(filepath.Clean(os.Getenv("USERPROFILE"))); os.Getenv("USERPROFILE") != "" && strings.EqualFold(filepath.Dir(dir), users) {
		return "" // a user's home
	}
	return dir
}

// running reports whether a program of the package runs.
func running(p *pkg, paths []string) bool {
	dir := installDir(p)
	return dir != "" && procs.Under(paths, dir)
}

func holdOf(h state.Hold) UpdateHold {
	switch h {
	case state.HoldIgnore:
		return HoldIgnored
	case state.HoldSkip:
		return HoldSkipped
	case state.HoldSnooze:
		return HoldSnoozed
	}
	return HoldNone
}

func stateMode(m UpdateMode) state.UpdateMode {
	switch m {
	case UpdateAuto:
		return state.ModeAuto
	case UpdateIgnore:
		return state.ModeIgnore
	}
	return state.ModeNotify
}

// parseKey reads a source key back: "winget:<id>", "scoop:<bucket>/<name>"
// or "github:<owner>/<repo>", in lower case.
func parseKey(key string) (catalog.Source, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || len(key) > 300 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return catalog.Source{}, false
	}
	t, rest, ok := strings.Cut(key, ":")
	if !ok || rest == "" {
		return catalog.Source{}, false
	}
	switch catalog.SourceType(t) {
	case catalog.Winget:
		return catalog.Source{Type: catalog.Winget, ID: rest}, true
	case catalog.Scoop:
		bucket, name, ok := strings.Cut(rest, "/")
		if !ok || name == "" {
			return catalog.Source{}, false
		}
		return catalog.Source{Type: catalog.Scoop, Bucket: bucket, Name: name}, true
	case catalog.GitHub:
		owner, repo, ok := strings.Cut(rest, "/")
		if !ok || owner == "" || repo == "" {
			return catalog.Source{}, false
		}
		return catalog.Source{Type: catalog.GitHub, Repo: rest}, true
	}
	return catalog.Source{}, false
}

func checkKey(key string) (string, error) {
	src, ok := parseKey(key)
	if !ok {
		return "", errors.New("不是可以更新的软件来源")
	}
	return src.Key(), nil
}

// future returns t when it is after now, else the zero time.
func future(t, now time.Time) time.Time {
	if t.After(now) {
		return t
	}
	return time.Time{}
}

func foldLess(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }

// updateList builds what List returns.
func (a *App) updateList() UpdateList {
	ps := a.packages()
	pol := a.core.Store().Policies()
	store := a.core.Store()
	hist := store.History()
	now := time.Now()
	a.mu.Lock()
	delay := time.Duration(a.settings.Updates.AutoDelay) * 24 * time.Hour
	a.mu.Unlock()
	paths := a.upd.processes()
	out := UpdateList{Items: []UpdateItem{}, Rules: []UpdateRule{}, Loaded: ps.loaded}
	for _, key := range ps.order {
		p := ps.byKey[key]
		if !p.update {
			continue
		}
		policy := pol[key]
		block := autoNote(p, policy, hist)
		it := UpdateItem{
			Key: key, Name: p.name, Icon: p.icon, Source: p.src, Version: p.version, Latest: p.latest,
			Major: version.MajorUpgrade(p.version, p.latest), Mode: modeOf(policy.Mode),
			Hold: holdOf(policy.Hold(p.latest, now)), Skip: policy.Skip, Until: future(policy.Until, now),
			CanAuto: block == "", AutoNote: block, Running: running(p, paths), Pending: p.pending,
			AppID: p.appID, Note: p.note,
		}
		if it.Mode == UpdateAuto && it.CanAuto && delay > 0 {
			// Not found by a check yet, it waits from now.
			since, ok := store.Offered(key, p.latest)
			if !ok {
				since = now
			}
			if at := since.Add(delay); at.After(now) {
				it.AutoAt = at
			}
		}
		out.Items = append(out.Items, it)
		if it.Hold == HoldNone {
			out.Count++
		}
	}
	slices.SortStableFunc(out.Items, func(x, y UpdateItem) int { return foldLess(x.Name, y.Name) })
	for key, policy := range pol {
		p := ps.byKey[key]
		r := UpdateRule{Key: key, Name: policy.Name, Mode: modeOf(policy.Mode), Skip: policy.Skip, Until: future(policy.Until, now)}
		if p != nil {
			block := autoNote(p, policy, hist)
			r.Name, r.Icon, r.Source, r.Installed, r.CanAuto, r.AutoNote = p.name, p.icon, p.src, true, block == "", block
		} else {
			r.Source, _ = parseKey(key)
			r.Icon, r.AutoNote = iconOf(r.Source), "本机没有找到这个软件"
		}
		// A version skipped that is no longer the newest, or a time
		// passed, holds nothing.
		skipping := r.Skip != "" && p != nil && p.update && strings.EqualFold(p.latest, r.Skip)
		if r.Mode == UpdateNotify && r.Until.IsZero() && !skipping {
			continue
		}
		if !skipping {
			r.Skip = ""
		}
		r.Name = cmp.Or(r.Name, r.Source.Display())
		out.Rules = append(out.Rules, r)
	}
	slices.SortStableFunc(out.Rules, func(x, y UpdateRule) int { return foldLess(x.Name, y.Name) })
	out.Checking, out.CheckedAt, out.NextCheck = a.upd.times()
	self := a.selfState()
	out.Self = SelfUpdate{CanUpdate: self.Enabled, Version: self.Version}
	if self.Phase.offers() {
		out.Self.Available = self.Available
	}
	return out
}

// List returns the updates, those held back included, and the packages
// whose updates are not handled the default way.
func (s *Updates) List() UpdateList { return s.a.updateList() }

// Check checks for updates now: the catalog's states and the installed
// programs are read again.
func (s *Updates) Check() {
	<-s.a.ready
	s.a.checkNow()
}

// resolveUpdates finds the packages of keys: the catalog's apps whose
// state was read by ID, the others by the installed list's keys.
func (a *App) resolveUpdates(keys []string) (ids, installed []string, err error) {
	ps := a.packages()
	seen := map[string]bool{}
	for _, key := range keys {
		k, err := checkKey(key)
		if err != nil {
			return nil, nil, err
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		p := ps.byKey[k]
		switch {
		case p == nil:
			return nil, nil, errors.New("软件已不在可更新的列表中，请重新检查")
		case p.appID != "" && p.known:
			ids = append(ids, p.appID)
		case p.entry != nil:
			installed = append(installed, p.entry.Key)
		default:
			return nil, nil, fmt.Errorf("还没有读到「%s」的安装状态，请稍后重试", p.name)
		}
	}
	return ids, installed, nil
}

// Start queues the updates of the packages that can run, after previewing
// them again, and returns how many.
func (s *Updates) Start(ctx context.Context, keys []string) (int, error) {
	return s.a.startUpdates(ctx, keys, nil)
}

// startUpdates queues updates; with auto, they are AppShelf's own, and
// the keys queued are added to it.
func (a *App) startUpdates(ctx context.Context, keys []string, auto map[string]bool) (int, error) {
	ids, installed, err := a.resolveUpdates(keys)
	if err != nil {
		return 0, err
	}
	n := 0
	enqueue := func(items []core.PreviewItem) {
		if auto == nil {
			n += a.core.Enqueue(items, queue.Upgrade)
			return
		}
		n += a.core.EnqueueAuto(items)
		for _, it := range items {
			if it.Runnable {
				key := it.App.Source.Key()
				auto[key] = true
				a.upd.markAuto(key)
			}
		}
	}
	if len(ids) > 0 {
		a.write([]ChangeKind{ChangedTasks, ChangedShelf, ChangedUpdates}, func() {
			enqueue(a.core.Preview(ids, queue.Upgrade))
		})
	}
	if len(installed) > 0 {
		apps, err := (&Installed{a}).updateApps(installed)
		if err != nil {
			if n == 0 {
				return 0, err
			}
		} else {
			items := a.core.PreviewUpdates(ctx, apps)
			if err := ctx.Err(); err != nil {
				return n, err
			}
			a.write([]ChangeKind{ChangedTasks, ChangedShelf, ChangedUpdates}, func() { enqueue(items) })
		}
	}
	if n == 0 {
		return 0, errors.New("没有可以执行的更新，请重新检查")
	}
	return n, nil
}

// setPolicy changes the policy of a package and tells the pages.
func (a *App) setPolicy(key string, fn func(p *state.UpdatePolicy)) error {
	if err := a.core.Store().SetPolicy(key, fn); err != nil {
		return fmt.Errorf("保存更新设置失败：%w", err)
	}
	a.note.mark(ChangedUpdates, ChangedInstalled, ChangedShelf)
	return nil
}

// SetMode sets how the updates of a package are handled. A package is
// updated on its own only when AppShelf can do it without a window or an
// administrator prompt; turning it on updates it now if it has an update.
func (s *Updates) SetMode(key string, mode UpdateMode) error {
	a := s.a
	<-a.ready
	k, err := checkKey(key)
	if err != nil {
		return err
	}
	p := a.packages().byKey[k]
	switch mode {
	case UpdateAuto:
		if p == nil {
			return errors.New("本机没有找到这个软件，无法自动更新")
		}
		if block := autoBlock(p); block != "" {
			return errors.New("无法自动更新：" + block)
		}
	case UpdateNotify, UpdateIgnore:
	default:
		return errors.New("未知的更新方式")
	}
	err = a.setPolicy(k, func(pp *state.UpdatePolicy) {
		pp.Mode = stateMode(mode)
		if p != nil {
			pp.Name = p.name
		}
		if mode != UpdateNotify {
			pp.Skip, pp.Until = "", time.Time{}
		}
	})
	if err == nil && mode == UpdateAuto && p.update {
		go a.upd.runAuto()
	}
	return err
}

// Skip stops offering the package's current update: its next version is
// offered again.
func (s *Updates) Skip(key string) error {
	a := s.a
	<-a.ready
	k, err := checkKey(key)
	if err != nil {
		return err
	}
	p := a.packages().byKey[k]
	if p == nil || !p.update || p.latest == "" {
		return errors.New("这个软件现在没有可用的更新")
	}
	return a.setPolicy(k, func(pp *state.UpdatePolicy) {
		pp.Skip, pp.Until, pp.Name = p.latest, time.Time{}, p.name
		if pp.Mode == state.ModeIgnore {
			pp.Mode = state.ModeNotify
		}
	})
}

// Snooze puts off offering the package's updates for days, from 1 to 90.
func (s *Updates) Snooze(key string, days int) error {
	a := s.a
	<-a.ready
	k, err := checkKey(key)
	if err != nil {
		return err
	}
	if days < 1 || days > 90 {
		return errors.New("暂缓的天数应在 1 到 90 之间")
	}
	p := a.packages().byKey[k]
	if p == nil {
		return errors.New("本机没有找到这个软件")
	}
	return a.setPolicy(k, func(pp *state.UpdatePolicy) {
		pp.Until, pp.Name = time.Now().Add(time.Duration(days)*24*time.Hour), p.name
		if pp.Mode == state.ModeIgnore {
			pp.Mode = state.ModeNotify
		}
	})
}

// Restore offers the package's updates again: no version skipped, nothing
// put off, not ignored. Updating on its own stays as it was.
func (s *Updates) Restore(key string) error {
	a := s.a
	<-a.ready
	k, err := checkKey(key)
	if err != nil {
		return err
	}
	return a.setPolicy(k, func(pp *state.UpdatePolicy) {
		pp.Skip, pp.Until = "", time.Time{}
		if pp.Mode == state.ModeIgnore {
			pp.Mode = state.ModeNotify
		}
	})
}

// History returns the updates done, newest first.
func (s *Updates) History() []UpdateRecord {
	a := s.a
	<-a.ready
	recs := a.core.Store().History()
	out := make([]UpdateRecord, 0, len(recs))
	for _, r := range recs {
		src, _ := parseKey(r.Key)
		out = append(out, UpdateRecord{
			Key: r.Key, Name: r.Name, Icon: iconOf(src), From: r.From, To: r.To,
			Result: TaskState(r.Result), Message: r.Message, Auto: r.Auto, At: r.At,
		})
	}
	return out
}

// ClearHistory forgets the updates done.
func (s *Updates) ClearHistory() error {
	a := s.a
	<-a.ready
	if err := a.core.Store().ClearHistory(); err != nil {
		return fmt.Errorf("清除记录失败：%w", err)
	}
	a.note.mark(ChangedUpdates)
	return nil
}

// Settings returns the user's choices for updates.
func (s *Updates) Settings() UpdateSettings {
	a := s.a
	<-a.ready
	a.mu.Lock()
	c := a.settings.Updates
	a.mu.Unlock()
	h := a.hooks
	out := UpdateSettings{CheckEvery: c.CheckEvery, AutoDelay: c.AutoDelay, Notify: !c.Quiet,
		CanBackground: h.SetBackground != nil && h.SetOpenAtLogin != nil && h.OpenAtLogin != nil}
	if out.CanBackground {
		out.Background = !c.ExitOnClose
		out.OpenAtLogin = h.OpenAtLogin()
	}
	out.CanSelfUpdate = a.canUpdate()
	out.SelfCheck, out.SelfEvery = c.selfCheck(), c.selfEvery()
	return out
}

// SetSettings changes the user's choices for updates and keeps them.
func (s *Updates) SetSettings(choices UpdateSettings) error {
	a := s.a
	<-a.ready
	if _, ok := checkIntervals[choices.CheckEvery]; !ok {
		return errors.New("未知的检查间隔")
	}
	cur := s.Settings()
	// Left out, AppShelf's own checks stay as they are.
	choices.SelfCheck = cmp.Or(choices.SelfCheck, cur.SelfCheck)
	choices.SelfEvery = cmp.Or(choices.SelfEvery, cur.SelfEvery)
	switch choices.SelfCheck {
	case SelfCheckOff, SelfCheckLaunch, SelfCheckInterval:
	default:
		return errors.New("未知的检查方式：" + string(choices.SelfCheck))
	}
	if !slices.Contains(selfIntervals, choices.SelfEvery) {
		return errors.New("检查间隔只能是 1、6 或 24 小时")
	}
	if !slices.Contains(autoDelays, choices.AutoDelay) {
		return errors.New("等待天数只能是 0、1、3、7 或 14 天")
	}
	if !cur.CanBackground && (choices.Background || choices.OpenAtLogin) {
		return errors.New("这个版本不支持后台运行和开机启动")
	}
	a.mu.Lock()
	// Without the notification area, closing the window quits whatever was
	// kept.
	exit := a.settings.Updates.ExitOnClose
	if cur.CanBackground {
		exit = !choices.Background
	}
	a.settings.Updates = updatesChoices{
		CheckEvery: choices.CheckEvery, Quiet: !choices.Notify, ExitOnClose: exit,
		SelfCheck: choices.SelfCheck, SelfEvery: choices.SelfEvery, AutoDelay: choices.AutoDelay,
	}
	st := a.settings
	a.mu.Unlock()
	if err := saveSettings(a.cfg.Settings, st); err != nil {
		return fmt.Errorf("保存设置失败：%w", err)
	}
	h := a.hooks
	if cur.CanBackground && choices.Background != cur.Background {
		h.SetBackground(choices.Background)
	}
	var errs []error
	if cur.CanBackground && choices.OpenAtLogin != cur.OpenAtLogin {
		if err := h.SetOpenAtLogin(choices.OpenAtLogin); err != nil {
			errs = append(errs, fmt.Errorf("无法设置开机启动：%w", err))
		}
	}
	if choices.SelfCheck != cur.SelfCheck || choices.SelfEvery != cur.SelfEvery {
		a.rescheduleSelf()
	}
	a.upd.reschedule()
	a.note.mark(ChangedUpdates)
	if choices.AutoDelay < cur.AutoDelay {
		// Updates that waited may be due now.
		go a.upd.runAuto()
	}
	return errors.Join(errs...)
}

// CheckUpdates checks for updates now, as the notification area's menu
// asks.
func (a *App) CheckUpdates() {
	<-a.ready
	a.checkNow()
}

// ShowPage asks the page to show one of its pages, as "updates".
func (a *App) ShowPage(page string) { Navigate.Broadcast(page) }

// autoDelays are the days UpdateSettings.AutoDelay can be.
var autoDelays = []int{0, 1, 3, 7, 14}
