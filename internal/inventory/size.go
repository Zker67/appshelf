package inventory

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Sizes measures the directories of the programs the registry gives no
// size: the Store's app packages and scoop's apps. A directory of a version
// is measured once and kept while the programs list it; the zero value is
// ready to use.
type Sizes struct {
	mu    sync.Mutex
	known map[string]int64
	// dirSize measures a directory; DirSize when nil.
	dirSize func(dir string) int64
	// packagePath finds an app package's directory; PackagePath when nil.
	packagePath func(fullName string) string
}

// sizeWorkers is how many directories are measured at a time.
const sizeWorkers = 4

// set gives each Store and scoop entry of no size the size of its
// directory, measuring the directories side by side.
func (s *Sizes) set(entries []Entry) {
	dirSize := DirSize
	if s.dirSize != nil {
		dirSize = s.dirSize
	}
	dirs := make([]string, len(entries))
	for i, e := range entries {
		if e.Size == 0 {
			dirs[i] = s.dirOf(e)
		}
	}
	s.mu.Lock()
	old := s.known
	s.known = map[string]int64{}
	todo := map[string]bool{}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if n, ok := old[d]; ok {
			s.known[d] = n
		} else {
			todo[d] = true
		}
	}
	s.mu.Unlock()

	jobs := make(chan string)
	var wg sync.WaitGroup
	for range min(sizeWorkers, len(todo)) {
		wg.Go(func() {
			for d := range jobs {
				n := dirSize(d)
				s.mu.Lock()
				s.known[d] = n
				s.mu.Unlock()
			}
		})
	}
	for d := range todo {
		jobs <- d
	}
	close(jobs)
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range dirs {
		if d != "" {
			entries[i].Size = s.known[d]
		}
	}
}

// dirOf is the directory of an entry Sizes measures, or "".
func (s *Sizes) dirOf(e Entry) string {
	switch {
	case e.Kind == Store && e.PackageFullName != "":
		if s.packagePath != nil {
			return s.packagePath(e.PackageFullName)
		}
		return PackagePath(e.PackageFullName)
	case e.Kind == Scoop:
		return scoopVersionDir(ScoopDir(e.ScoopName, e.Scope == "Machine"), e.Version)
	}
	return ""
}

// scoopVersionDir is the directory of an app's version under the app's
// directory, else the version "current" links to; "" when neither is.
func scoopVersionDir(app, version string) string {
	if app == "" {
		return ""
	}
	if version != "" {
		if d := filepath.Join(app, version); isDir(d) {
			return d
		}
	}
	// "current" is a junction, or a link, which DirSize does not follow.
	d, err := os.Readlink(filepath.Join(app, "current"))
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(d) {
		d = filepath.Join(app, d)
	}
	if !isDir(d) {
		return ""
	}
	return filepath.Clean(d)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ScoopDir is where scoop keeps an app: $env:SCOOP\apps\<name>, or the
// global directory's; "" when not known.
func ScoopDir(name string, global bool) string {
	if name == "" {
		return ""
	}
	root := os.Getenv("SCOOP")
	if global {
		root = cmp.Or(os.Getenv("SCOOP_GLOBAL"), filepath.Join(os.Getenv("ProgramData"), "scoop"))
	} else if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		root = filepath.Join(home, "scoop")
	}
	if !filepath.IsAbs(root) {
		return ""
	}
	return filepath.Join(root, "apps", name)
}

// DirSize is the size in bytes of the files under dir. Links and junctions
// are not followed, so that nothing is counted twice; what cannot be read
// is left out.
func DirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != dir {
				return fs.SkipDir
			}
			return nil
		}
		t := d.Type()
		switch {
		case t.IsDir():
			if t&^fs.ModeDir != 0 && p != dir {
				return fs.SkipDir
			}
		case t.IsRegular():
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
