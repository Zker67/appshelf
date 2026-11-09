package api

import (
	"context"
	"errors"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/inventory"
	"appshelf/internal/jsonfile"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
)

// inventoryState keeps the list of installed programs, read in the
// background, one reading at a time. The last list read is kept in a file
// (Config.InstalledCache): shown as AppShelf starts, while it is read
// again.
type inventoryState struct {
	app *App

	mu      sync.Mutex
	list    inventory.List
	loaded  bool
	loading bool
	cached  bool          // list is the file's, of an earlier run
	again   bool          // read again after the reading under way
	maxAge  time.Duration // of the reading asked again

	// settle is when a change seen outside AppShelf is read, once changes
	// stop coming.
	settle *time.Timer

	// gone are the programs AppShelf removed, by key, with when: they stay
	// off the list until a reading begun after their removal.
	gone map[string]time.Time

	// sizes measures the directories of the Store's apps and scoop's, which
	// the registry gives no size, keeping what it measured between readings.
	sizes inventory.Sizes
}

// settleDelay is how long the changes of an install or removal outside
// AppShelf must stop before the list is read again: installers write many
// keys one after the other.
var settleDelay = 1500 * time.Millisecond

// outside notes that what is installed may have changed outside AppShelf.
// The list is read again once the changes stop; while AppShelf's own tasks
// run it is not, as they read it again when they end.
func (s *inventoryState) outside() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settle != nil {
		s.settle.Reset(settleDelay)
		return
	}
	s.settle = time.AfterFunc(settleDelay, func() {
		s.mu.Lock()
		s.settle = nil
		s.mu.Unlock()
		if running, _ := s.app.core.Queue.Running(); running {
			return
		}
		s.refresh(0)
	})
}

// refresh reads the list again in the background, letting winget's list be
// up to maxAge old.
func (s *inventoryState) refresh(maxAge time.Duration) {
	s.mu.Lock()
	if s.loading {
		if !s.again || maxAge < s.maxAge {
			s.maxAge = maxAge
		}
		s.again = true
		s.mu.Unlock()
		return
	}
	s.loading = true
	s.mu.Unlock()
	s.app.note.mark(ChangedInstalled)
	go s.run(maxAge)
}

func (s *inventoryState) run(maxAge time.Duration) {
	s.quick()
	for {
		begun := time.Now()
		list := inventory.Load(context.Background(), s.app.inventorySources(maxAge))
		s.mu.Lock()
		list.Entries = s.withoutGoneLocked(list.Entries, begun)
		s.mu.Unlock()
		if s.app.cfg.Icons != nil {
			s.app.cfg.Icons.SetInventory(list.Entries)
		}
		s.save(list)
		s.mu.Lock()
		s.list, s.loaded, s.cached = list, true, false
		if !s.again {
			s.loading = false
			s.mu.Unlock()
			break
		}
		s.again, maxAge = false, s.maxAge
		s.mu.Unlock()
	}
	s.app.note.mark(ChangedInstalled)
	// The loop sees the reading end, as a check waits for it.
	s.app.loop.post(func() {})
}

// cacheVersion is the version of the cache file's content: a file of
// another is ignored.
const cacheVersion = 1

type inventoryCache struct {
	Version int            `json:"version"`
	List    inventory.List `json:"list"`
}

// restore shows the list kept by an earlier run, before anything is read.
// A missing or unreadable file shows nothing.
func (s *inventoryState) restore() {
	path := s.app.cfg.InstalledCache
	if path == "" {
		return
	}
	var c inventoryCache
	if err := jsonfile.Read(path, &c); err != nil || c.Version != cacheVersion || len(c.List.Entries) == 0 {
		return
	}
	c.List.Partial = false
	s.mu.Lock()
	if s.loaded {
		s.mu.Unlock()
		return
	}
	s.list, s.loaded, s.cached = c.List, true, true
	s.mu.Unlock()
	if s.app.cfg.Icons != nil {
		s.app.cfg.Icons.SetInventory(c.List.Entries)
	}
}

// save keeps a whole list for the next run.
func (s *inventoryState) save(list inventory.List) {
	path := s.app.cfg.InstalledCache
	if path == "" || list.Partial {
		return
	}
	if err := jsonfile.WriteAtomic(path, inventoryCache{Version: cacheVersion, List: list}); err != nil {
		log.Printf("appshelf: keeping the installed list: %v", err)
	}
}

// freshWait is how long fresh waits for this run's reading.
var freshWait = 2 * time.Minute

// fresh returns the list read in this run, waiting while only the earlier
// run's is there: what a removal runs must not be stale.
func (s *inventoryState) fresh(ctx context.Context) (inventory.List, error) {
	deadline := time.Now().Add(freshWait)
	for {
		list, loaded, _ := s.get()
		s.mu.Lock()
		cached := s.cached
		s.mu.Unlock()
		if loaded && !cached {
			return list, nil
		}
		if time.Now().After(deadline) {
			return inventory.List{}, errors.New("本机列表正在读取，请稍后重试")
		}
		select {
		case <-ctx.Done():
			return inventory.List{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// quick shows the registry's programs, which take milliseconds to read,
// while winget takes seconds: only before a whole list was read, later
// readings keep the last list until they end; nor when the earlier run's
// list is shown.
func (s *inventoryState) quick() {
	s.mu.Lock()
	loaded := s.loaded
	s.mu.Unlock()
	if loaded || s.app.cfg.Scan == nil {
		return
	}
	reg, err := s.app.cfg.Scan()
	if err != nil {
		return
	}
	list := inventory.List{Entries: inventory.Build(nil, false, nil, reg), At: time.Now(), Partial: true}
	s.mu.Lock()
	if s.loaded {
		s.mu.Unlock()
		return
	}
	s.list, s.loaded = list, true
	s.mu.Unlock()
	if s.app.cfg.Icons != nil {
		s.app.cfg.Icons.SetInventory(list.Entries)
	}
	s.app.note.mark(ChangedInstalled)
}

// drop takes programs AppShelf removed off the list at once, without
// waiting for the list to be read again.
func (s *inventoryState) drop(keys []string) {
	if len(keys) == 0 {
		return
	}
	now := time.Now()
	s.mu.Lock()
	if s.gone == nil {
		s.gone = map[string]time.Time{}
	}
	for _, k := range keys {
		s.gone[k] = now
	}
	s.list.Entries = s.withoutGoneLocked(s.list.Entries, time.Time{})
	s.mu.Unlock()
	s.app.note.mark(ChangedInstalled)
}

// withoutGoneLocked returns entries without the programs removed, first
// forgetting those removed before begun: a reading begun since lists what
// is left. entries is not changed.
func (s *inventoryState) withoutGoneLocked(entries []inventory.Entry, begun time.Time) []inventory.Entry {
	for k, at := range s.gone {
		if at.Before(begun) {
			delete(s.gone, k)
		}
	}
	if len(s.gone) == 0 {
		return entries
	}
	out := make([]inventory.Entry, 0, len(entries))
	for _, e := range entries {
		if _, ok := s.gone[e.Key]; !ok {
			out = append(out, e)
		}
	}
	return out
}

// busy reports whether the list is being read.
func (s *inventoryState) busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loading
}

// get returns the list, and whether it was read and is being read. A list
// never read is read now, in the background.
func (s *inventoryState) get() (inventory.List, bool, bool) {
	s.mu.Lock()
	list, loaded, loading := s.list, s.loaded, s.loading
	s.mu.Unlock()
	if !loaded && !loading {
		s.refresh(10 * time.Minute)
		loading = true
	}
	return list, loaded, loading
}

// inventorySources are the sources usable here.
func (a *App) inventorySources(maxAge time.Duration) inventory.Sources {
	ctx := context.Background()
	src := inventory.Sources{Scan: a.cfg.Scan, Usage: a.cfg.Usage, Sizes: &a.inv.sizes}
	if w := a.cfg.Sources.Winget; w != nil {
		av := a.availability(ctx, catalog.Winget, w.Check)
		if av.OK {
			src.Winget = func(ctx context.Context) ([]winget.Package, error) {
				pkgs, _, err := w.Installed(ctx, maxAge)
				return pkgs, err
			}
		} else {
			src.Notes = append(src.Notes, "winget 未参与读取："+av.Reason+"；当前仅显示注册表程序，无法判断其可用更新")
		}
	}
	if sc := a.cfg.Sources.Scoop; sc != nil && a.availability(ctx, catalog.Scoop, sc.Check).OK {
		src.Scoop = func(ctx context.Context) ([]scoop.App, error) { return sc.Installed(ctx) }
	}
	return src
}

// Installed lists the programs installed on this machine, and which of
// them are in the catalog.
type Installed struct{ a *App }

// List returns what is installed. The first call starts reading it.
func (s *Installed) List() InstalledList {
	a := s.a
	<-a.ready
	list, loaded, loading := a.inv.get()
	var apps []catalog.App
	a.read(func() { apps = a.core.Catalog().Apps })
	match := inventory.Match(list.Entries, apps)
	pol := a.core.Store().Policies()
	now := time.Now()
	out := InstalledList{Entries: make([]InstalledEntry, 0, len(list.Entries)), Loaded: loaded, Loading: loading, Partial: list.Partial, At: list.At, Notes: list.Notes, UsageTracked: list.UsageTracked}
	for _, e := range list.Entries {
		entry := entryOf(e, match[e.Key])
		if entry.Source != nil {
			p := pol[entry.Source.Key()]
			entry.Mode = modeOf(p.Mode)
			if entry.Update != "" {
				entry.Hold = holdOf(p.Hold(entry.Update, now))
			}
		}
		// Refresh also retries icons which were unavailable on the previous scan.
		entry.Icon += "?v=" + url.QueryEscape(list.At.Format(time.RFC3339Nano))
		out.Entries = append(out.Entries, entry)
	}
	return out
}

// Refresh reads what is installed again.
func (s *Installed) Refresh() {
	<-s.a.ready
	s.a.inv.refresh(0)
}

func entryOf(e inventory.Entry, inShelf []string) InstalledEntry {
	out := InstalledEntry{
		Key: e.Key, Name: e.Name, Version: e.Version, Publisher: e.Publisher,
		Kind: InstalledKind(e.Kind), Update: e.Update, InShelf: inShelf,
		Icon: "/i/" + url.PathEscape(e.Key),
		Size: e.Size, Installed: e.InstallDate, Location: e.Location, LastUsed: e.LastUsed,
	}
	if out.InShelf == nil {
		out.InShelf = []string{}
	}
	switch e.Kind {
	case inventory.Winget:
		out.Source = &catalog.Source{Type: catalog.Winget, ID: e.WingetID}
	case inventory.Scoop:
		out.Source = &catalog.Source{Type: catalog.Scoop, Bucket: e.Bucket, Name: e.ScoopName}
	default:
		out.Rule = detect.SuggestRule(e.RuleName())
	}
	return out
}

// installedIndex tells which packages are installed, by winget ID and
// scoop name in lower case, for search results.
type installedIndex struct {
	winget, scoop map[string]bool
}

func (a *App) installedIndex() installedIndex {
	list, _, _ := a.inv.get()
	idx := installedIndex{winget: map[string]bool{}, scoop: map[string]bool{}}
	for _, e := range list.Entries {
		switch e.Kind {
		case inventory.Winget:
			idx.winget[strings.ToLower(e.WingetID)] = true
		case inventory.Scoop:
			idx.scoop[strings.ToLower(e.ScoopName)] = true
		}
	}
	return idx
}
