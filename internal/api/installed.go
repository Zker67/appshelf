package api

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/inventory"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
)

// inventoryState keeps the list of installed programs, read in the
// background, one reading at a time.
type inventoryState struct {
	app *App

	mu      sync.Mutex
	list    inventory.List
	loaded  bool
	loading bool
	again   bool          // read again after the reading under way
	maxAge  time.Duration // of the reading asked again
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
	for {
		list := inventory.Load(context.Background(), s.app.inventorySources(maxAge))
		if s.app.cfg.Icons != nil {
			s.app.cfg.Icons.SetInventory(list.Entries)
		}
		s.mu.Lock()
		s.list, s.loaded = list, true
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
	src := inventory.Sources{Scan: a.cfg.Scan}
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
	out := InstalledList{Entries: make([]InstalledEntry, 0, len(list.Entries)), Loaded: loaded, Loading: loading, At: list.At, Notes: list.Notes}
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
