package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
)

const (
	// searchTTL is how long what a source found is kept: its packages change
	// when its index updates, a few times a day.
	searchTTL = 10 * time.Minute
	// searchKeep is how many searches are kept.
	searchKeep = 64
)

// searchCache keeps what the sources found, so that a query typed again,
// or reached again by deleting letters, is answered at once. It keeps the
// sources' results only: what is installed or in the catalog is marked when
// they are sent.
type searchCache struct {
	mu sync.Mutex
	m  map[searchKey]searchEntry
}

type searchKey struct {
	t        catalog.SourceType
	versions bool
	query    string
}

type searchEntry struct {
	res      []source.SearchResult
	versions map[string]string
	at       time.Time
}

func keyOf(t catalog.SourceType, versions bool, query string) searchKey {
	return searchKey{t, versions, strings.ToLower(strings.TrimSpace(query))}
}

func (c *searchCache) get(k searchKey) (searchEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok || time.Since(e.at) > searchTTL {
		return searchEntry{}, false
	}
	return e, true
}

func (c *searchCache) put(k searchKey, e searchEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[searchKey]searchEntry{}
	}
	e.at = time.Now()
	c.m[k] = e
	for len(c.m) > searchKeep {
		var oldest searchKey
		first := true
		for k, e := range c.m {
			if first || e.at.Before(c.m[oldest].at) {
				oldest, first = k, false
			}
		}
		delete(c.m, oldest)
	}
}

// results searches a source, or answers from the cache. Errors are not
// kept.
func (c *searchCache) results(ctx context.Context, t catalog.SourceType, query string, search func(context.Context, string) ([]source.SearchResult, error)) ([]source.SearchResult, error) {
	k := keyOf(t, false, query)
	if e, ok := c.get(k); ok {
		return e.res, nil
	}
	res, err := search(ctx, query)
	if err != nil {
		return nil, err
	}
	c.put(k, searchEntry{res: res})
	return res, nil
}

// versions reads the versions of a search in the background: the channel
// gets them, or nil when they could not be read, which only leaves the
// versions out.
func (c *searchCache) versions(ctx context.Context, t catalog.SourceType, query string, read func(context.Context, string) (map[string]string, error)) <-chan map[string]string {
	out := make(chan map[string]string, 1)
	k := keyOf(t, true, query)
	if e, ok := c.get(k); ok {
		out <- e.versions
		return out
	}
	go func() {
		v, err := read(ctx, query)
		if err != nil {
			out <- nil
			return
		}
		c.put(k, searchEntry{versions: v})
		out <- v
	}()
	return out
}

// withVersions returns a copy of res with the versions it lacks filled in
// from v, by lowercase ID, and how many it filled.
func withVersions(res []source.SearchResult, v map[string]string) ([]source.SearchResult, int) {
	out := make([]source.SearchResult, len(res))
	copy(out, res)
	n := 0
	for i := range out {
		if out[i].Version == "" {
			if ver := v[strings.ToLower(out[i].ID)]; ver != "" {
				out[i].Version = ver
				n++
			}
		}
	}
	return out, n
}

// lacksVersion reports whether a result has no version.
func lacksVersion(res []source.SearchResult) bool {
	for _, r := range res {
		if r.Version == "" {
			return true
		}
	}
	return false
}
