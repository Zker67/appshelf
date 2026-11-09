// Package api is what AppShelf's web page calls: the services main binds
// with mygo.Bind, and the event that tells the page what changed.
//
// internal/core reads and changes the catalog and the states on one
// goroutine, as a UI thread would. Here that is the loop of App: it runs
// what the core posts from its background work and what the services ask
// of the core, one at a time and in order. The services' methods run on
// goroutines of their own and wait for the loop.
//
// The page only shows: what is installed, and what installing does, is
// decided in Go by the sources and the detection rules.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/detect"
	"appshelf/internal/discover"
	"appshelf/internal/icons"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
	"appshelf/internal/state"
)

// WingetAPI is what the services use of the winget source, beyond the
// core's.
type WingetAPI interface {
	Check(ctx context.Context) source.Availability
	Search(ctx context.Context, query string) ([]source.SearchResult, error)
	// SearchVersions reads the versions of what Search finds, by lowercase
	// ID, for display.
	SearchVersions(ctx context.Context, query string) (map[string]string, error)
	// Warm readies the searches, for the first one to be quick.
	Warm(ctx context.Context) error
	Installed(ctx context.Context, maxAge time.Duration) ([]winget.Package, time.Time, error)
	LatestVersion(ctx context.Context, id string) (string, error)
}

// ScoopAPI is what the services use of the scoop source.
type ScoopAPI interface {
	Check(ctx context.Context) source.Availability
	Search(ctx context.Context, query string) ([]source.SearchResult, error)
	Installed(ctx context.Context) ([]scoop.App, error)
	Info(ctx context.Context, bucket, name string) (scoop.Info, error)
}

// GitHubAPI is what the services use of the GitHub source.
type GitHubAPI interface {
	SearchRepos(ctx context.Context, query string, n int) ([]github.Repo, error)
	Repository(ctx context.Context, repo string) (github.Repo, error)
	Describe(ctx context.Context, app catalog.App) (github.Release, github.Asset, error)
	SearchRate() github.Rate
	// UseToken checks a token with GitHub and, accepted, carries it;
	// SetToken carries one without asking.
	UseToken(ctx context.Context, token string) error
	SetToken(token string)
}

// SourceAPIs are the sources the core uses, for what the services do beside
// it. A nil one is a source the build lacks.
type SourceAPIs struct {
	Winget WingetAPI
	Scoop  ScoopAPI
	GitHub GitHubAPI
}

// Hooks are what the services need of the app around them.
type Hooks struct {
	// SaveFile and OpenFile ask the user for a catalog file, over the
	// window of the call; "" means the user canceled.
	SaveFile func(ctx context.Context, title, name string) (string, error)
	OpenFile func(ctx context.Context, title string) (string, error)
	// OpenPath opens a file or directory; OpenURL a web page.
	OpenPath func(path string) error
	OpenURL  func(url string) error
	// CanUpdate reports whether this copy can update itself: not a
	// development build, and installed where it can be replaced. Nil means
	// it cannot.
	CanUpdate func() bool
	// CheckUpdates checks for a newer AppShelf, showing the update window.
	CheckUpdates func()
	// SetTheme applies an appearance.
	SetTheme func(ThemeSource)
	// Emit sends a change to the pages; nil means Changed.Broadcast.
	Emit func(Change)

	// Notify shows a system notification, which shows the updates when
	// clicked; nil shows none. Active reports whether AppShelf's window is
	// in front, when notifications are not shown.
	Notify func(title, body string)
	Active func() bool
	// SetBackground keeps AppShelf in the notification area when its
	// window closes, or stops. OpenAtLogin and SetOpenAtLogin read and
	// change whether it starts when the user logs in. All three or none.
	SetBackground  func(on bool)
	OpenAtLogin    func() bool
	SetOpenAtLogin func(on bool) error
	// SelfAvailable is the newer AppShelf the update window found, or "".
	SelfAvailable func() string
	// SelfChecks and SetSelfChecks read and change whether the update
	// window checks every day, and installs what it finds without asking.
	SelfChecks    func() (checks, downloads bool)
	SetSelfChecks func(checks, downloads bool)
}

// Config is what Start needs.
type Config struct {
	Core    *core.Core
	Sources SourceAPIs
	Icons   *icons.Service
	Details *discover.Details
	// Scan reads the registry's programs (detect.Scan).
	Scan func() ([]detect.Entry, error)
	// Version is AppShelf's.
	Version  string
	DataDir  string
	Settings string // the settings file
	IconBase string
	// TokenFile keeps the GitHub token entered in the settings, encrypted;
	// EnvToken is the one of GH_TOKEN or GITHUB_TOKEN, used without it.
	TokenFile string
	EnvToken  string
}

// App connects the services to the core.
type App struct {
	hooks Hooks
	ready chan struct{}
	start sync.Once
	loop  *loop
	note  *notifier

	cfg  Config
	core *core.Core
	inv  *inventoryState
	upd  *checker

	// Owned by the loop: the core's work before the last function, to
	// see it end.
	wasRefreshing, wasRunning bool

	mu       sync.Mutex
	imports  map[string]*core.Import
	deleted  map[string]deletion
	order    []string // tokens of imports and deletions, oldest first
	searchMu sync.Mutex
	search   context.CancelFunc
	found    searchCache
	settings settings
	// tokenFrom is where the GitHub token in use comes from; token is it.
	tokenFrom TokenSource
	token     string
}

type deletion struct {
	apps    []catalog.App
	records map[string]state.GitHubRecord
}

// New returns the app, whose services can be bound before the core opens.
func New(hooks Hooks) *App {
	a := &App{
		hooks:   hooks,
		ready:   make(chan struct{}),
		loop:    newLoop(),
		imports: map[string]*core.Import{},
		deleted: map[string]deletion{},
	}
	a.note = newNotifier(80*time.Millisecond, func(c Change) {
		if a.hooks.Emit != nil {
			a.hooks.Emit(c)
			return
		}
		Changed.Broadcast(c)
	})
	a.inv = &inventoryState{app: a}
	a.upd = newChecker(a)
	return a
}

// Services returns the services to bind.
func (a *App) Services() []any {
	return []any{&Shelf{a}, &Tasks{a}, &Sources{a}, &Discover{a}, &Installed{a}, &Updates{a}, &About{a}}
}

// Post runs fn on the loop: the core's Config.Post. What the core posts
// may change the states, the tasks and the sources.
func (a *App) Post(fn func()) {
	a.loop.post(func() {
		defer func() {
			if p := recover(); p != nil {
				log.Printf("appshelf: panic on the loop: %v\n%s", p, debug.Stack())
			}
		}()
		fn()
		a.note.mark(ChangedShelf, ChangedTasks, ChangedSources)
	})
}

// Start runs the loop with the core, reads the states and lists what is
// installed. ctx ends the loop.
func (a *App) Start(ctx context.Context, cfg Config) {
	a.start.Do(func() {
		a.cfg, a.core = cfg, cfg.Core
		a.settings = loadSettings(cfg.Settings)
		a.loadToken()
		a.upd.ctx = ctx
		go a.loop.run(ctx, a.afterLoop)
		close(a.ready)
		// Reading everything as AppShelf starts is a check for updates.
		a.upd.begin()
		go a.upd.run(ctx)
		a.write([]ChangeKind{ChangedShelf}, func() { a.core.Refresh() })
		a.inv.refresh(0)
		if cfg.Icons != nil {
			cfg.Icons.Warm()
		}
	})
}

// read runs fn on the loop and waits for it.
func (a *App) read(fn func()) { a.write(nil, fn) }

// write runs fn on the loop, waits for it, and tells the pages the kinds
// changed. A panic of fn is the caller's, as for a method called directly.
func (a *App) write(kinds []ChangeKind, fn func()) {
	<-a.ready
	done := make(chan any, 1)
	a.loop.post(func() {
		defer func() { done <- recover() }()
		fn()
	})
	if p := <-done; p != nil {
		panic(p)
	}
	a.note.mark(kinds...)
}

// afterLoop runs on the loop after the functions it ran: once the states
// are read, or the tasks done, what is installed is read again.
func (a *App) afterLoop() {
	if a.core == nil {
		return
	}
	refreshing := a.core.Refreshing()
	running, _ := a.core.Queue.Running()
	if a.wasRefreshing && !refreshing {
		// The list read when AppShelf started is still current.
		a.inv.refresh(time.Minute)
	}
	if a.wasRunning && !running {
		a.inv.refresh(0)
	}
	a.wasRefreshing, a.wasRunning = refreshing, running
	a.upd.afterLoop()
}

// remember keeps an import or a deletion under a new token, forgetting the
// oldest beyond twenty.
func (a *App) remember(fn func(token string)) string {
	b := make([]byte, 12)
	rand.Read(b)
	token := hex.EncodeToString(b)
	a.mu.Lock()
	defer a.mu.Unlock()
	fn(token)
	a.order = append(a.order, token)
	for len(a.order) > 20 {
		delete(a.imports, a.order[0])
		delete(a.deleted, a.order[0])
		a.order = a.order[1:]
	}
	return token
}

// iconOf returns the path of a source's icon on the icon scheme.
func iconOf(s catalog.Source) string {
	switch s.Type {
	case catalog.Winget:
		return "/w/" + url.PathEscape(s.ID)
	case catalog.Scoop:
		return "/s/" + url.PathEscape(s.Name)
	case catalog.GitHub:
		owner, repo, _ := strings.Cut(s.Repo, "/")
		return "/g/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
	}
	return ""
}

// availability returns whether a source can be used, checking it when the
// core has not yet.
func (a *App) availability(ctx context.Context, t catalog.SourceType, check func(context.Context) source.Availability) source.Availability {
	if a.core != nil {
		if av, ok := a.core.Availability(t); ok {
			return av
		}
	}
	if check == nil {
		return source.Availability{Reason: "不支持的来源"}
	}
	return check(ctx)
}

// loop runs functions one at a time, in the order posted. Posting never
// waits: the core posts from the loop itself, as when queueing tasks.
type loop struct {
	mu   sync.Mutex
	fns  []func()
	wake chan struct{}
}

func newLoop() *loop { return &loop{wake: make(chan struct{}, 1)} }

func (l *loop) post(fn func()) {
	l.mu.Lock()
	l.fns = append(l.fns, fn)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// run runs the functions posted until ctx ends, and after after those it
// found together.
func (l *loop) run(ctx context.Context, after func()) {
	for {
		select {
		case <-l.wake:
		case <-ctx.Done():
			return
		}
		for {
			l.mu.Lock()
			fns := l.fns
			l.fns = nil
			l.mu.Unlock()
			if len(fns) == 0 {
				break
			}
			for _, fn := range fns {
				fn()
			}
			after()
		}
	}
}

// notifier gathers the kinds changed within delay into one Change.
type notifier struct {
	delay time.Duration
	emit  func(Change)

	mu    sync.Mutex
	kinds map[ChangeKind]bool
	timer *time.Timer
	rev   int64
}

func newNotifier(delay time.Duration, emit func(Change)) *notifier {
	return &notifier{delay: delay, emit: emit, kinds: map[ChangeKind]bool{}}
}

func (n *notifier) mark(kinds ...ChangeKind) {
	if len(kinds) == 0 {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, k := range kinds {
		n.kinds[k] = true
	}
	if n.timer == nil {
		n.timer = time.AfterFunc(n.delay, n.flush)
	}
}

func (n *notifier) flush() {
	n.mu.Lock()
	kinds := make([]ChangeKind, 0, len(n.kinds))
	for k := range n.kinds {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	n.kinds = map[ChangeKind]bool{}
	n.timer = nil
	n.rev++
	c := Change{Kinds: kinds, Revision: n.rev}
	n.mu.Unlock()
	n.emit(c)
}
