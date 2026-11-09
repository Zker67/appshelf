package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// updateRepo is the repository of mygo.json, whose public key the builds
// check the updates with.
func TestUpdateRepoIsMygoJSONs(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Updates struct {
			GitHub string `json:"github"`
		} `json:"updates"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Updates.GitHub != updateRepo {
		t.Fatalf("mygo.json updates.github = %q, updateRepo = %q", cfg.Updates.GitHub, updateRepo)
	}
}

func quickRetries(t *testing.T) {
	saved := updateRetryWaits
	updateRetryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { updateRetryWaits = saved })
}

func TestUpdateRetries(t *testing.T) {
	busy := errors.New("mygo: checking for updates: GET https://github.com/Zker67/appshelf/releases/latest/download/update-windows-amd64.json: 504 Gateway Time-out")
	missing := errors.New("GET https://example.com/update.json: 404 Not Found")
	unsigned := errors.New("mygo: installing the update: the update is not signed with the app's key")
	if !transient(busy) || transient(missing) || transient(unsigned) || !transient(fmt.Errorf("x: %w", context.DeadlineExceeded)) {
		t.Fatal("transient misjudged an error")
	}
	if statusOf(busy) != 504 || statusOf(missing) != 404 || statusOf(unsigned) != 0 {
		t.Fatal("statusOf misread an error")
	}

	quickRetries(t)
	ctx := context.Background()
	body := io.NopCloser(strings.NewReader("ok"))
	tries := 0
	if rc, err := retry(ctx, func() (io.ReadCloser, error) {
		tries++
		if tries < 3 {
			return nil, busy
		}
		return body, nil
	}); err != nil || rc != body || tries != 3 {
		t.Fatalf("retry after 504: %v, %d tries", err, tries)
	}
	tries = 0
	if _, err := retry(ctx, func() (io.ReadCloser, error) { tries++; return nil, busy }); err != busy || tries != 5 {
		t.Fatalf("retry gave up with %v after %d tries", err, tries)
	}
	tries = 0
	if _, err := retry(ctx, func() (io.ReadCloser, error) { tries++; return nil, missing }); err != missing || tries != 1 {
		t.Fatalf("a 404 was tried %d times", tries)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	tries = 0
	if _, err := retry(canceled, func() (io.ReadCloser, error) { tries++; return nil, busy }); err == nil || tries != 1 {
		t.Fatalf("a canceled check was tried %d times", tries)
	}
}

// Each check asks for the manifest with a query no cache has seen, as
// mygo-updater. When github.com answers 504, as a proxy on the way does,
// the manifest and the archive come through the API; when both do now and
// then, they are asked for again; a 504 of both each time reads as one.
func TestGithubFeed(t *testing.T) {
	manifest := "update-" + runtime.GOOS + "-" + runtime.GOARCH + ".json"
	var mu sync.Mutex
	var queries []string
	siteDown, siteSlow, allDown := false, false, 0
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "mygo-updater" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		mu.Lock()
		down, slow := siteDown, siteSlow
		outage := allDown > 0
		if outage {
			allDown--
		}
		mu.Unlock()
		p := r.URL.Path
		if strings.HasPrefix(p, "/site/") && slow {
			time.Sleep(300 * time.Millisecond)
		}
		switch {
		case outage || (strings.HasPrefix(p, "/site/") && down):
			w.WriteHeader(http.StatusGatewayTimeout)
		case p == "/site/releases/latest/download/"+manifest:
			mu.Lock()
			queries = append(queries, r.URL.Query().Get("t"))
			mu.Unlock()
			io.WriteString(w, `{"version":"9.9.9"}`)
		case p == "/site/releases/download/v9.9.9/archive.tar.gz":
			io.WriteString(w, "archive")
		case p == "/api/releases/latest" || p == "/api/releases/tags/v9.9.9":
			if r.URL.Query().Get("t") == "" {
				t.Errorf("%s without a fresh query", p)
			}
			fmt.Fprintf(w, `{"assets":[{"name":%q,"url":%q},{"name":"archive.tar.gz","url":%q}]}`,
				manifest, srv.URL+"/api/assets/1", srv.URL+"/api/assets/2")
		case p == "/api/assets/1" && r.Header.Get("Accept") == "application/octet-stream":
			io.WriteString(w, `{"version":"9.9.9","via":"api"}`)
		case p == "/api/assets/2" && r.Header.Get("Accept") == "application/octet-stream":
			io.WriteString(w, "archive via api")
		default:
			w.WriteHeader(http.StatusGatewayTimeout)
		}
	}))
	defer srv.Close()
	saved := updateTransport
	updateTransport = func() http.RoundTripper { return srv.Client().Transport }
	t.Cleanup(func() { updateTransport = saved })
	quickRetries(t)
	set := func(fn func()) {
		mu.Lock()
		defer mu.Unlock()
		fn()
	}

	ctx := context.Background()
	feed := githubFeed{site: srv.URL + "/site", api: srv.URL + "/api"}
	read := func(rc io.ReadCloser, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		b, _ := io.ReadAll(rc)
		return string(b)
	}
	for range 2 {
		if got := read(feed.Manifest(ctx, false)); got != `{"version":"9.9.9"}` {
			t.Fatalf("manifest = %q", got)
		}
	}
	if len(queries) != 2 || queries[0] == "" || queries[0] == queries[1] {
		t.Fatalf("queries = %q, want two different ones", queries)
	}
	archive := srv.URL + "/site/releases/download/v9.9.9/archive.tar.gz"
	if got := read(feed.Open(ctx, archive)); got != "archive" {
		t.Fatalf("archive = %q", got)
	}

	// Both github.com and the API answer 504 for the first tries (the site
	// and the API's release each count one a try).
	set(func() { allDown = 4 })
	if got := read(feed.Manifest(ctx, false)); got != `{"version":"9.9.9"}` {
		t.Fatalf("manifest after an outage = %q", got)
	}
	set(func() { allDown = 4 })
	if got := read(feed.Open(ctx, archive)); got != "archive" {
		t.Fatalf("archive after an outage = %q", got)
	}

	savedWait := siteWait
	siteWait = 50 * time.Millisecond
	t.Cleanup(func() { siteWait = savedWait })
	set(func() { siteSlow = true })
	if got := read(feed.Manifest(ctx, false)); !strings.Contains(got, `"via":"api"`) {
		t.Fatalf("manifest with github.com slow = %q", got)
	}
	set(func() { siteSlow, siteDown = false, true })
	if got := read(feed.Manifest(ctx, false)); !strings.Contains(got, `"via":"api"`) {
		t.Fatalf("manifest with github.com down = %q", got)
	}
	if got := read(feed.Open(ctx, archive)); got != "archive via api" {
		t.Fatalf("archive with github.com down = %q", got)
	}
	_, err := feed.Open(ctx, srv.URL+"/site/releases/download/v9.9.9/missing.tar.gz")
	if err == nil || statusOf(err) != 504 || !transient(err) {
		t.Fatalf("a 504 reads as %v", err)
	}
	if _, err := feed.Open(ctx, "http://example.com/archive.tar.gz"); err == nil {
		t.Fatal("a download without HTTPS was made")
	}
}

// The manifest of the latest release on GitHub, read as AppShelf reads it,
// and through GitHub's API as when github.com fails: read-only, through the
// network and the system's proxy. It is skipped with -short, and when the
// network does not reach GitHub at all or the API's quota is used up.
func TestGithubFeedLive(t *testing.T) {
	if testing.Short() {
		t.Skip("reads GitHub")
	}
	feed := githubFeed{site: "https://github.com/" + updateRepo, api: "https://api.github.com/repos/" + updateRepo}
	name := "update-" + runtime.GOOS + "-" + runtime.GOARCH + ".json"
	ctx := context.Background()
	for _, way := range []struct {
		name string
		get  func() (io.ReadCloser, error)
	}{
		{"github.com", func() (io.ReadCloser, error) { return feed.Manifest(ctx, false) }},
		{"the API", func() (io.ReadCloser, error) { return feed.fromAPI(ctx, "latest", name) }},
	} {
		rc, err := way.get()
		if err != nil {
			if transient(err) || statusOf(err) == 403 {
				t.Skipf("GitHub cannot be reached through %s: %v", way.name, err)
			}
			t.Fatalf("%s: %v", way.name, err)
		}
		var m struct {
			Version   string `json:"version"`
			Signature string `json:"signature"`
		}
		err = json.NewDecoder(rc).Decode(&m)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", way.name, err)
		}
		t.Logf("latest release through %s: %s", way.name, m.Version)
		if m.Version == "" || m.Signature == "" {
			t.Fatalf("%s: not a manifest: %+v", way.name, m)
		}
	}
}
