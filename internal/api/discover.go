package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"

	"appshelf/internal/catalog"
	"appshelf/internal/discover"
	"appshelf/internal/source"
)

// Changed tells the pages what changed, for them to read it again.
var Changed = mygo.NewEvent[Change]("changed")

// Discover finds software to add: one search over winget and scoop, GitHub
// on request, the details of a package, and detection rules for GitHub
// apps.
type Discover struct{ a *App }

// Search searches winget and scoop side by side and sends each source's
// results as they come. A new search cancels the one under way. winget's
// results come first without versions, and again once the versions are
// read.
func (d *Discover) Search(ctx context.Context, query string, results *mygo.Channel[SearchBatch]) error {
	return d.a.searchAll(ctx, query, results.Send)
}

// searchAll is Search, sending with send.
func (a *App) searchAll(ctx context.Context, query string, send func(SearchBatch) error) error {
	<-a.ready
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.searchMu.Lock()
	if a.search != nil {
		a.search()
	}
	a.search = cancel
	a.searchMu.Unlock()

	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(discover.Fold(query)) == 0 {
		return nil
	}
	var mu sync.Mutex // send one batch at a time
	sendOne := func(batch SearchBatch) {
		if ctx.Err() != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if err := send(batch); err != nil {
			cancel()
		}
	}
	var wg sync.WaitGroup
	for _, s := range a.searchers() {
		wg.Go(func() {
			batch := SearchBatch{Type: s.t, Hits: []SearchHit{}}
			if av := a.availability(ctx, s.t, s.check); !av.OK {
				batch.Unavailable, batch.Error = true, av.Reason
				sendOne(batch)
				return
			}
			var versions <-chan map[string]string
			if s.versions != nil {
				versions = a.found.versions(ctx, s.t, query, s.versions)
			}
			res, err := a.found.results(ctx, s.t, query, s.search)
			if err != nil {
				batch.Error = err.Error()
				sendOne(batch)
				return
			}
			// Versions already read go out with the results; the others
			// follow.
			got := false
			select {
			case v := <-versions:
				res, _ = withVersions(res, v)
				got = true
			default:
			}
			batch.Hits = a.hits(s.t, query, res)
			sendOne(batch)
			if got || versions == nil || !lacksVersion(res) {
				return
			}
			select {
			case v := <-versions:
				if res, n := withVersions(res, v); n > 0 {
					sendOne(SearchBatch{Type: s.t, Hits: a.hits(s.t, query, res)})
				}
			case <-ctx.Done():
			}
		})
	}
	wg.Wait()
	return nil
}

// searcher is a source that searches.
type searcher struct {
	t      catalog.SourceType
	search func(context.Context, string) ([]source.SearchResult, error)
	check  func(context.Context) source.Availability
	// versions reads the versions search leaves out; nil when it leaves
	// none out.
	versions func(context.Context, string) (map[string]string, error)
}

// searchers returns winget and scoop, those the build has.
func (a *App) searchers() []searcher {
	var all []searcher
	if w := a.cfg.Sources.Winget; w != nil {
		all = append(all, searcher{catalog.Winget, w.Search, w.Check, w.SearchVersions})
	}
	if sc := a.cfg.Sources.Scoop; sc != nil {
		all = append(all, searcher{catalog.Scoop, sc.Search, sc.Check, nil})
	}
	return all
}

// Warm readies the searches as the page opens, for the first one to be
// quick. It returns at once.
func (d *Discover) Warm() {
	a := d.a
	go func() {
		<-a.ready
		w := a.cfg.Sources.Winget
		if w == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if av := a.availability(ctx, catalog.Winget, w.Check); av.OK {
			w.Warm(ctx)
		}
	}()
}

// hits annotates and ranks a source's results.
func (a *App) hits(t catalog.SourceType, query string, res []source.SearchResult) []SearchHit {
	keys := a.shelfKeys()
	idx := a.installedIndex()
	out := make([]SearchHit, 0, len(res))
	for _, r := range res {
		h := SearchHit{Type: t, ID: r.ID, Name: r.Name, Version: r.Version}
		var src catalog.Source
		switch t {
		case catalog.Winget:
			src = catalog.Source{Type: catalog.Winget, ID: r.ID}
			h.Installed = idx.winget[strings.ToLower(r.ID)]
		case catalog.Scoop:
			src = catalog.Source{Type: catalog.Scoop, Bucket: r.Source, Name: r.Name}
			h.ID = r.Source + "/" + r.Name
			h.Installed = idx.scoop[strings.ToLower(r.Name)]
		}
		h.Icon = iconOf(src)
		h.InShelf = keys[src.Key()]
		out = append(out, h)
	}
	return discover.Rank(query, out, func(h SearchHit) []string {
		return []string{h.Name, h.ID, discover.LastPart(h.ID)}
	})
}

// shelfKeys maps the catalog's sources, by Source.Key, to their apps.
func (a *App) shelfKeys() map[string]string {
	keys := map[string]string{}
	var apps []catalog.App
	a.read(func() { apps = a.core.Catalog().Apps })
	for _, app := range apps {
		keys[app.Source.Key()] = app.ID
	}
	return keys
}

// SearchGitHub searches GitHub's repositories: ten searches a minute
// without a token, so only when asked.
func (d *Discover) SearchGitHub(ctx context.Context, query string) (GitHubSearch, error) {
	a := d.a
	<-a.ready
	g := a.cfg.Sources.GitHub
	if g == nil {
		return GitHubSearch{}, errors.New("GitHub 来源不可用")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return GitHubSearch{Hits: []SearchHit{}, Rate: rateOf(g.SearchRate())}, nil
	}
	repos, err := g.SearchRepos(ctx, query, 10)
	a.note.mark(ChangedSources)
	if err != nil {
		return GitHubSearch{}, err
	}
	keys := a.shelfKeys()
	out := GitHubSearch{Hits: make([]SearchHit, 0, len(repos)), Rate: rateOf(g.SearchRate())}
	for _, r := range repos {
		src := catalog.Source{Type: catalog.GitHub, Repo: r.FullName}
		out.Hits = append(out.Hits, SearchHit{
			Type: catalog.GitHub, ID: r.FullName, Name: r.FullName[strings.IndexByte(r.FullName, '/')+1:],
			Summary: r.Description, Stars: r.Stars, Archived: r.Archived,
			Icon: iconOf(src), InShelf: keys[src.Key()],
		})
	}
	return out, nil
}

// Details returns what a package's source says of it. It is for display:
// failing to read it changes nothing else.
func (d *Discover) Details(ctx context.Context, ref discover.PackageRef) (discover.PackageDetails, error) {
	a := d.a
	<-a.ready
	if a.cfg.Details == nil {
		return discover.PackageDetails{}, errors.New("详情暂时无法读取")
	}
	det, err := a.cfg.Details.Get(ctx, ref)
	if ref.Type == catalog.GitHub {
		a.note.mark(ChangedSources)
	}
	return det, err
}

// SuggestRules proposes detection rules for an app of a GitHub repository,
// from the programs installed and from the repository's name.
func (d *Discover) SuggestRules(repo string) []discover.RuleSuggestion {
	a := d.a
	<-a.ready
	list, _, _ := a.inv.get()
	return discover.SuggestRules(strings.Trim(strings.TrimSpace(repo), "/"), list.Entries)
}
