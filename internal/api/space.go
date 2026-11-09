package api

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"appshelf/internal/detect"
	"appshelf/internal/inventory"
	"appshelf/internal/leftover"
)

// LocationFrom is how a program's directory is known.
type LocationFrom string

const (
	LocationRegistry LocationFrom = "registry" // the registry's InstallLocation
	LocationGuessed  LocationFrom = "guessed"  // the directory of its icon or uninstaller
	LocationScoop    LocationFrom = "scoop"    // scoop's directory of the app
	LocationStore    LocationFrom = "store"    // the app package's directory
)

// SpaceInfo is the room a program takes, measured: its directory, and the
// directories that may hold its data. For display only.
type SpaceInfo struct {
	Key string `json:"key"`
	// Location is the program's directory, From how it is known; empty
	// when not known. Openable means it is there to open.
	Location string       `json:"location,omitempty"`
	From     LocationFrom `json:"from,omitempty"`
	Openable bool         `json:"openable,omitzero"`
	// Program is the size of the directory, when Measured.
	Program  int64 `json:"program,omitzero"`
	Measured bool  `json:"measured,omitzero"`
	// Note says why the directory was not measured.
	Note string `json:"note,omitempty"`
	// Registered is the registry's estimate of its size.
	Registered int64 `json:"registered,omitzero"`
	// Data are directories named as the program, which may hold its
	// settings and caches, the surer first; DataSize is their sizes.
	Data     []SpaceItem `json:"data"`
	DataSize int64       `json:"dataSize,omitzero"`
	Partial  bool        `json:"partial,omitzero"`
}

// SpaceItem is a directory that may hold a program's data.
type SpaceItem struct {
	// ID names it for OpenLocation.
	ID    string        `json:"id"`
	Path  string        `json:"path"`
	Size  int64         `json:"size,omitzero"`
	Level LeftoverLevel `json:"level"`
	// Reasons are why it is thought the program's.
	Reasons []string `json:"reasons"`
}

// spaceWorkers is how many directories are measured at a time.
const spaceWorkers = 4

// entry returns the program of the installed list with the key.
func (a *App) entry(key string) (inventory.Entry, []inventory.Entry, error) {
	list, _, _ := a.inv.get()
	for _, e := range list.Entries {
		if e.Key == key {
			return e, list.Entries, nil
		}
	}
	return inventory.Entry{}, nil, errors.New("软件已不在当前本机列表中，请重新读取")
}

// locationOf is a program's directory and how it is known; "" when not
// known.
func locationOf(e inventory.Entry) (string, LocationFrom) {
	switch {
	case e.Kind == inventory.Store && e.PackageFullName != "":
		if p := inventory.PackagePath(e.PackageFullName); p != "" {
			return p, LocationStore
		}
	case e.Kind == inventory.Scoop:
		if p := inventory.ScoopDir(e.ScoopName, e.Scope == "Machine"); p != "" {
			return p, LocationScoop
		}
	}
	if e.Location == "" {
		return "", ""
	}
	if e.Reg != nil && e.Reg.LocationGuessed {
		return filepath.Clean(e.Location), LocationGuessed
	}
	return filepath.Clean(e.Location), LocationRegistry
}

// isDir reports a directory that is there.
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Space measures the room a program takes: its directory, and the
// directories associated with its identity or names. Nothing is changed.
func (s *Installed) Space(ctx context.Context, key string) (SpaceInfo, error) {
	a := s.a
	<-a.ready
	e, all, err := a.entry(key)
	if err != nil {
		return SpaceInfo{}, err
	}
	out := SpaceInfo{Key: key, Data: []SpaceItem{}}
	if e.Reg != nil {
		out.Registered = e.Reg.EstimatedSize
	}
	dir, from := locationOf(e)
	out.Location, out.From = dir, from
	own := ""
	switch {
	case dir == "":
		out.Note = "没有登记安装目录"
	case !isDir(dir):
		out.Note = "登记的安装目录已不存在"
	case !detect.OwnDirectory(dir):
		// Measuring Program Files would count every program in it.
		out.Note = "登记的是多个程序共用的目录，不统计"
		out.Openable = true
	default:
		out.Openable, own = true, dir
	}

	cfg := leftover.DataConfig(a.cfg.Protect...)
	if e.Kind == inventory.Scoop {
		cfg.ScoopPersist = nil
		if app := inventory.ScoopDir(e.ScoopName, e.Scope == "Machine"); app != "" {
			cfg.ScoopPersist = []string{filepath.Join(filepath.Dir(filepath.Dir(app)), "persist")}
		}
	}
	prog := leftover.DataProgram{Program: leftover.Program{Name: e.Name, Publisher: e.Publisher, Location: own, ScoopName: e.ScoopName}, Family: e.PackageFamily}
	prog.Aliases = append(prog.Aliases, e.DisplayName)
	if own != "" {
		prog.Aliases = append(prog.Aliases, filepath.Base(own))
	}
	if icon := detect.CommandFile(e.Icon); icon != "" {
		prog.Aliases = append(prog.Aliases, strings.TrimSuffix(filepath.Base(icon), filepath.Ext(icon)))
	}
	var others []leftover.Other
	for _, o := range all {
		if o.Key == e.Key || own != "" && strings.EqualFold(filepath.Clean(o.Location), own) {
			continue
		}
		others = append(others, leftover.Other{Name: o.Name, Publisher: o.Publisher, Location: o.Location})
	}
	dirs := dataDirs(leftover.FindData(ctx, cfg, prog, others), own, cfg.ShortcutDirs)
	if err := ctx.Err(); err != nil {
		return SpaceInfo{}, err
	}

	// The program's directory and the data directories, measured side by
	// side.
	paths := make([]string, 0, len(dirs)+1)
	if own != "" {
		paths = append(paths, own)
	}
	for _, it := range dirs {
		paths = append(paths, it.Path)
	}
	sizes := measure(ctx, paths)
	if err := ctx.Err(); err != nil {
		return SpaceInfo{}, err
	}
	if own != "" {
		out.Program, out.Measured = sizes[0].bytes, true
		out.Partial = sizes[0].partial
		sizes = sizes[1:]
	}
	known := make(map[string]string, len(dirs))
	for i, it := range dirs {
		item := SpaceItem{ID: it.ID, Path: it.Path, Size: sizes[i].bytes, Level: LeftoverLevel(it.Level), Reasons: []string{}}
		for _, r := range it.Reasons {
			if r.Delta > 0 {
				item.Reasons = append(item.Reasons, r.Text)
			}
		}
		out.Data = append(out.Data, item)
		out.DataSize += item.Size
		out.Partial = out.Partial || sizes[i].partial
		known[it.ID] = it.Path
	}
	a.mu.Lock()
	a.spaces[key] = known
	a.mu.Unlock()
	return out, nil
}

// dataDirs keeps the directories of the finds that may hold a program's
// data: not its own directory, nor one holding it, nor the Start menu's
// folders, nor those likelier not its; one inside another counts once.
func dataDirs(items []leftover.Item, own string, shortcutDirs []string) []leftover.Item {
	inside := func(p, dir string) bool {
		return dir != "" && strings.HasPrefix(strings.ToLower(p), strings.ToLower(strings.TrimRight(dir, `\`))+`\`)
	}
	var out []leftover.Item
	for _, it := range items {
		if it.Kind != leftover.Dir || it.Level == leftover.Low {
			continue
		}
		p := filepath.Clean(it.Path)
		if own != "" && (strings.EqualFold(p, own) || inside(p, own) || inside(own, p)) {
			continue
		}
		if slices.ContainsFunc(shortcutDirs, func(d string) bool { return inside(p, d) }) {
			continue
		}
		out = append(out, it)
	}
	// Outer directories first, so that one inside them is left out.
	slices.SortStableFunc(out, func(a, b leftover.Item) int { return len(a.Path) - len(b.Path) })
	var kept []leftover.Item
	for _, it := range out {
		if !slices.ContainsFunc(kept, func(k leftover.Item) bool { return strings.EqualFold(it.Path, k.Path) || inside(it.Path, k.Path) }) {
			kept = append(kept, it)
		}
	}
	// The surer first, then by path.
	rank := map[leftover.Level]int{leftover.High: 0, leftover.Medium: 1}
	slices.SortStableFunc(kept, func(a, b leftover.Item) int {
		if d := rank[a.Level] - rank[b.Level]; d != 0 {
			return d
		}
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	})
	return kept
}

type measuredSize struct {
	bytes   int64
	partial bool
}

// measure sums logical file lengths, not allocated disk clusters. Junctions
// and unreadable files are skipped and reported as a partial measurement.
func measure(ctx context.Context, dirs []string) []measuredSize {
	sizes := make([]measuredSize, len(dirs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(spaceWorkers, len(dirs)) {
		wg.Go(func() {
			for i := range jobs {
				filepath.WalkDir(dirs[i], func(p string, d fs.DirEntry, err error) error {
					if ctx.Err() != nil {
						return fs.SkipAll
					}
					if err != nil {
						sizes[i].partial = true
						return nil
					}
					if d.IsDir() {
						if d.Type() & ^fs.ModeDir != 0 {
							sizes[i].partial = true
							return fs.SkipDir
						}
					} else if d.Type().IsRegular() {
						if info, err := d.Info(); err == nil {
							sizes[i].bytes += info.Size()
						} else {
							sizes[i].partial = true
						}
					} else {
						sizes[i].partial = true
					}
					return nil
				})
			}
		})
	}
	for i := range dirs {
		if ctx.Err() != nil {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return sizes
}

// OpenLocation opens a program's directory in Explorer, or with id one of
// the data directories its last Space found. The page names the program
// and the directory, never a path: Go resolves them.
func (s *Installed) OpenLocation(key, id string) error {
	a := s.a
	<-a.ready
	if id != "" {
		a.mu.Lock()
		path, ok := a.spaces[key][id]
		a.mu.Unlock()
		if !ok {
			return errors.New("这个目录的信息已过期，请重新统计")
		}
		if !isDir(path) {
			return errors.New("目录已不存在")
		}
		return a.openPath(path)
	}
	e, _, err := a.entry(key)
	if err != nil {
		return err
	}
	dir, _ := locationOf(e)
	if dir == "" {
		return errors.New("不知道它装在哪个目录")
	}
	if !isDir(dir) {
		return errors.New("安装目录已不存在")
	}
	return a.openPath(dir)
}
