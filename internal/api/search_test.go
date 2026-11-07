package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
	"appshelf/internal/source/winget"
)

// TestSearchVersions checks that winget's results go out before their
// versions, then again with them, and that a search is kept.
func TestSearchVersions(t *testing.T) {
	e := newEnv(t)
	w := e.wapi
	w.mu.Lock()
	w.results = []source.SearchResult{{ID: "Tool.A", Name: "Tool A"}, {ID: "Tool.B", Name: "Tool B"}}
	w.versions = map[string]string{"tool.a": "2.0"}
	w.vblock = make(chan struct{})
	w.mu.Unlock()

	var mu sync.Mutex
	var batches []SearchBatch
	wingetBatches := func() []SearchBatch {
		mu.Lock()
		defer mu.Unlock()
		var out []SearchBatch
		for _, b := range batches {
			if b.Type == catalog.Winget {
				out = append(out, b)
			}
		}
		return out
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.app.searchAll(context.Background(), "tool", func(b SearchBatch) error {
			mu.Lock()
			batches = append(batches, b)
			mu.Unlock()
			return nil
		})
	}()
	// The results come without waiting for the versions.
	e.until(func() bool { return len(wingetBatches()) == 1 })
	if b := wingetBatches()[0]; len(b.Hits) != 2 || b.Hits[0].Version != "" || b.Hits[1].Version != "" {
		t.Fatalf("first batch = %+v", b)
	}
	close(w.vblock)
	<-done
	got := wingetBatches()
	if len(got) != 2 {
		t.Fatalf("winget batches = %+v", got)
	}
	versionOf := func(hits []SearchHit, id string) string {
		for _, h := range hits {
			if h.ID == id {
				return h.Version
			}
		}
		return "missing"
	}
	if versionOf(got[1].Hits, "Tool.A") != "2.0" || versionOf(got[1].Hits, "Tool.B") != "" {
		t.Errorf("second batch = %+v", got[1])
	}

	// The same query again, in another case, is answered from the cache, in
	// one batch with the versions.
	batches = nil
	if err := e.app.searchAll(context.Background(), " TOOL ", func(b SearchBatch) error { batches = append(batches, b); return nil }); err != nil {
		t.Fatal(err)
	}
	got = wingetBatches()
	w.mu.Lock()
	queries, vqueries := len(w.queries), len(w.vqueries)
	w.mu.Unlock()
	if len(got) != 1 || versionOf(got[0].Hits, "Tool.A") != "2.0" || queries != 1 || vqueries != 1 {
		t.Errorf("cached search: batches %+v, %d searches, %d version reads", got, queries, vqueries)
	}

	// The editor's search waits for the versions.
	hits, err := e.sources.Search(context.Background(), catalog.Winget, "tool b")
	if err != nil || versionOf(hits, "Tool.A") != "2.0" {
		t.Errorf("editor search = %+v, %v", hits, err)
	}

	// Versions that cannot be read leave the results as they are, and are
	// not kept.
	w.mu.Lock()
	w.verr = errors.New("winget failed")
	w.mu.Unlock()
	batches = nil
	if err := e.app.searchAll(context.Background(), "broken", func(b SearchBatch) error { batches = append(batches, b); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := wingetBatches(); len(got) != 1 || got[0].Error != "" || len(got[0].Hits) != 2 {
		t.Errorf("without versions: %+v", got)
	}
	if _, ok := e.app.found.get(keyOf(catalog.Winget, true, "broken")); ok {
		t.Error("failed versions kept")
	}

	// Opening the page readies winget.
	e.discover.Warm()
	e.until(func() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.warmed == 1 })
}

func TestSearchCache(t *testing.T) {
	var c searchCache
	calls := 0
	search := func(context.Context, string) ([]source.SearchResult, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("failed")
		}
		return []source.SearchResult{{ID: "A"}}, nil
	}
	ctx := context.Background()
	if _, err := c.results(ctx, catalog.Scoop, "a", search); err == nil {
		t.Fatal("no error")
	}
	// An error is not kept; results are.
	for range 2 {
		if res, err := c.results(ctx, catalog.Scoop, "a", search); err != nil || len(res) != 1 {
			t.Fatalf("results %v, %v", res, err)
		}
	}
	if calls != 2 {
		t.Errorf("%d calls", calls)
	}
	// Another source's search of the same text is another search.
	if _, ok := c.get(keyOf(catalog.Winget, false, "a")); ok {
		t.Error("winget found scoop's search")
	}

	// Kept results expire, and the oldest go past the limit.
	k := keyOf(catalog.Scoop, false, "a")
	c.mu.Lock()
	e := c.m[k]
	e.at = time.Now().Add(-searchTTL - time.Second)
	c.m[k] = e
	c.mu.Unlock()
	if _, ok := c.get(k); ok {
		t.Error("an expired search kept")
	}
	for i := range searchKeep + 10 {
		c.put(keyOf(catalog.Scoop, false, fmt.Sprint(i)), searchEntry{})
	}
	if len(c.m) != searchKeep {
		t.Errorf("%d searches kept", len(c.m))
	}
	if _, ok := c.get(keyOf(catalog.Scoop, false, fmt.Sprint(searchKeep+9))); !ok {
		t.Error("the newest search dropped")
	}

	// Filling versions copies the results.
	res := []source.SearchResult{{ID: "X.Y"}, {ID: "X.Z", Version: "1"}}
	out, n := withVersions(res, map[string]string{"x.y": "3", "x.z": "2"})
	if n != 1 || out[0].Version != "3" || out[1].Version != "1" || res[0].Version != "" {
		t.Errorf("withVersions = %+v, %d; res = %+v", out, n, res)
	}
}

// TestSearchRealWinget searches this machine's winget through the service,
// as the page does, and times it: the results, their versions, and the
// same search again. It changes nothing; it skips where winget is missing.
func TestSearchRealWinget(t *testing.T) {
	real := winget.New()
	if av := real.Check(context.Background()); !av.OK {
		t.Skipf("winget not available: %s", av.Reason)
	}
	e := newEnv(t)
	e.wapi.mu.Lock()
	e.wapi.real = real
	e.wapi.mu.Unlock()
	if err := real.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"browser", "editor"} {
		start := time.Now()
		var first, last time.Duration
		var batches []SearchBatch
		err := e.app.searchAll(context.Background(), q, func(b SearchBatch) error {
			if b.Type == catalog.Winget {
				if first == 0 {
					first = time.Since(start)
				}
				last = time.Since(start)
				batches = append(batches, b)
			}
			return nil
		})
		if err != nil || len(batches) == 0 || batches[0].Error != "" {
			t.Fatalf("%s: %+v, %v", q, batches, err)
		}
		hits := batches[len(batches)-1].Hits
		n := 0
		for _, h := range hits {
			if h.Version != "" {
				n++
			}
		}
		e.wapi.mu.Lock()
		searched, read := len(e.wapi.queries), len(e.wapi.vqueries)
		e.wapi.mu.Unlock()
		start = time.Now()
		var again []SearchBatch
		e.app.searchAll(context.Background(), q, func(b SearchBatch) error {
			if b.Type == catalog.Winget {
				again = append(again, b)
			}
			return nil
		})
		cached := time.Since(start)
		e.wapi.mu.Lock()
		searched2, read2 := len(e.wapi.queries), len(e.wapi.vqueries)
		e.wapi.mu.Unlock()
		t.Logf("%s: %d results in %v, versions of %d in %v (%d batches); again in %v", q, len(hits), first, n, last, len(batches), cached)
		if len(hits) < 20 || n < len(hits)*8/10 || first > 5*time.Second || last > 10*time.Second {
			t.Errorf("%s: %d results in %v, versions of %d in %v", q, len(hits), first, n, last)
		}
		// The same search again asks winget nothing.
		if len(again) != 1 || searched2 != searched || read2 != read {
			t.Errorf("%s again: %d batches, %d searches and %d version reads more", q, len(again), searched2-searched, read2-read)
		}
	}
}
