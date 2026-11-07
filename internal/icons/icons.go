// Package icons gives software its icon: from UniGetUI's icon database,
// downloaded and cached by AppShelf, from the program installed, or none,
// for the page to draw a letter instead. It serves them on a custom scheme,
// so that the page never loads anything from the Internet itself.
//
// Paths, relative to the scheme's root:
//
//	/w/<winget ID>      a winget package: the program installed, else the database
//	/s/<scoop name>     a scoop app: the database
//	/g/<owner>/<repo>   a GitHub repository: the database, else the owner's avatar
//	/i/<entry key>      an installed program (internal/inventory): as its kind
//
// A path without an icon answers 404.
package icons

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"appshelf/internal/detect"
	"appshelf/internal/inventory"
)

// Options configure the service.
type Options struct {
	// Dir keeps the database and the icons downloaded.
	Dir string
	// DatabaseURL is DatabaseURL but in tests.
	DatabaseURL string
	// AvatarURL is GitHub's avatars, "https://avatars.githubusercontent.com",
	// but in tests.
	AvatarURL string
	// Client downloads; nil means one with timeouts.
	Client *http.Client
	// FileIcon draws the icon of a file or a shell item as a PNG of size
	// pixels (mygo.Shell.FileIcon); nil means no icons of installed
	// programs.
	FileIcon func(path string, size int) ([]byte, error)
	// PackagedApps maps package families to their local PNG logo or shell
	// item for FileIcon. Nil means none.
	PackagedApps func() (map[string]string, error)
}

// localSize is the size of the icons drawn from installed programs: list
// rows show them at 32 to 40 pixels, twice that on high DPI displays.
const localSize = 96

// Service finds and serves icons.
type Service struct {
	opt   Options
	db    *database
	fetch *fetcher

	mu        sync.Mutex
	byKey     map[string]inventory.Entry // the inventory's entries, by key
	byWinget  map[string]inventory.Entry // winget ID, lower case → installed program
	packages  map[string]string          // package family, lower case → shell item
	pkgAt     time.Time
	local     map[string]localImage
	localBusy map[string]*localCall
	pkgBusy   chan struct{}
}

type image struct {
	data  []byte
	ctype string
}

// New returns the service.
func New(opt Options) *Service {
	opt.DatabaseURL = cmp.Or(opt.DatabaseURL, DatabaseURL)
	opt.AvatarURL = strings.TrimRight(cmp.Or(opt.AvatarURL, "https://avatars.githubusercontent.com"), "/")
	if opt.Client == nil {
		opt.Client = &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return checkURL(req.URL)
			},
		}
	}
	return &Service{
		opt:       opt,
		db:        newDatabase(opt.Client, opt.DatabaseURL, opt.Dir),
		fetch:     newFetcher(opt.Client, filepath.Join(opt.Dir, "img")),
		byKey:     map[string]inventory.Entry{},
		byWinget:  map[string]inventory.Entry{},
		local:     map[string]localImage{},
		localBusy: map[string]*localCall{},
	}
}

// SetInventory tells the service what is installed, for the icons of
// installed programs.
func (s *Service) SetInventory(entries []inventory.Entry) {
	byKey := make(map[string]inventory.Entry, len(entries))
	byWinget := map[string]inventory.Entry{}
	for _, e := range entries {
		byKey[e.Key] = e
		if e.Kind == inventory.Winget {
			byWinget[strings.ToLower(e.WingetID)] = e
		}
	}
	s.mu.Lock()
	s.byKey, s.byWinget = byKey, byWinget
	s.mu.Unlock()
}

// Warm starts the shared remote and package lookups before the list needs them.
func (s *Service) Warm() {
	go s.db.icons(context.Background())
	go s.packaged(context.Background(), "__warm__")
}

// ErrNoIcon means there is no icon for the path.
var ErrNoIcon = errors.New("no icon")

// Icon returns the icon of a path of the scheme.
func (s *Service) Icon(ctx context.Context, segments []string) ([]byte, string, error) {
	if len(segments) < 2 {
		return nil, "", ErrNoIcon
	}
	switch kind, rest := segments[0], segments[1:]; {
	case kind == "w" && len(rest) == 1:
		return s.winget(ctx, rest[0])
	case kind == "s" && len(rest) == 1:
		return s.remote(ctx, ScoopIconID(rest[0]), "")
	case kind == "g" && len(rest) == 2:
		return s.remote(ctx, strings.ToLower(rest[0]+"/"+rest[1]), s.opt.AvatarURL+"/"+url.PathEscape(rest[0])+"?size=128")
	case kind == "i" && len(rest) == 1:
		return s.entry(ctx, rest[0])
	}
	return nil, "", ErrNoIcon
}

func (s *Service) winget(ctx context.Context, id string) ([]byte, string, error) {
	s.mu.Lock()
	e := s.byWinget[strings.ToLower(id)]
	s.mu.Unlock()
	if data, ctype, err := s.installedFile(e); err == nil {
		return data, ctype, nil
	}
	if e.Name != "" {
		if data, ctype, err := s.packaged(ctx, ApplicationIconKey(e.Name)); err == nil {
			return data, ctype, nil
		}
	}
	return s.remote(ctx, WingetIconID(id), "")
}

func (s *Service) entry(ctx context.Context, key string) ([]byte, string, error) {
	s.mu.Lock()
	e, ok := s.byKey[key]
	s.mu.Unlock()
	if !ok {
		return nil, "", ErrNoIcon
	}
	if data, ctype, err := s.installedFile(e); err == nil {
		return data, ctype, nil
	}
	if e.Kind != inventory.Store && e.Name != "" {
		if data, ctype, err := s.packaged(ctx, ApplicationIconKey(e.Name)); err == nil {
			return data, ctype, nil
		}
	}
	switch e.Kind {
	case inventory.Winget:
		return s.remote(ctx, WingetIconID(e.WingetID), "")
	case inventory.Scoop:
		return s.remote(ctx, ScoopIconID(e.ScoopName), "")
	case inventory.Store:
		return s.packaged(ctx, e.PackageFamily)
	case inventory.Local:
		if id := registryWingetID(e.Key); id != "" {
			return s.remote(ctx, WingetIconID(id), "")
		}
	}
	return nil, "", ErrNoIcon
}

// remote downloads the database's icon of key, else the fallback URL.
func (s *Service) remote(ctx context.Context, key, fallback string) ([]byte, string, error) {
	icons, err := s.db.icons(ctx)
	if err != nil && fallback == "" {
		return nil, "", err
	}
	if u := icons[key]; u != "" {
		if data, ctype, err := s.fetch.get(ctx, u); err == nil {
			return data, ctype, nil
		}
	}
	if fallback != "" {
		return s.fetch.get(ctx, fallback)
	}
	return nil, "", ErrNoIcon
}

// IconFile returns the file of a registry DisplayIcon, which may be quoted
// and end with ",<index>", with its environment variables expanded; or ""
// for none worth showing, as the Windows Installer's own.
func IconFile(displayIcon string) string {
	p := strings.TrimSpace(displayIcon)
	if strings.HasPrefix(p, `"`) {
		if end := strings.Index(p[1:], `"`); end >= 0 {
			p = p[1 : end+1]
		}
	} else if i := strings.LastIndexByte(p, ','); i > 0 && isIndex(p[i+1:]) {
		p = strings.TrimSpace(p[:i])
	}
	p = strings.Trim(detect.ExpandEnv(p), `"`)
	if p == "" || !filepath.IsAbs(p) {
		return ""
	}
	switch strings.ToLower(filepath.Base(p)) {
	case "msiexec.exe", "rundll32.exe", "explorer.exe":
		return ""
	}
	return p
}

func isIndex(s string) bool {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "-"))
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ServeHTTP serves the icons on the custom scheme.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var segments []string
	for _, seg := range strings.Split(strings.Trim(r.URL.EscapedPath(), "/"), "/") {
		v, err := url.PathUnescape(seg)
		if err != nil || v == "" {
			http.NotFound(w, r)
			return
		}
		segments = append(segments, v)
	}
	data, ctype, err := s.Icon(r.Context(), segments)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Cache-Control", "max-age=86400")
	h.Set("X-Content-Type-Options", "nosniff")
	if ctype == "image/svg+xml" {
		// An SVG may hold scripts: none of them runs.
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	if r.Method == http.MethodHead {
		return
	}
	w.Write(data)
}
