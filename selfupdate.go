package main

// AppShelf's own updates come from the releases of updateRepo through
// githubFeed, which reads them as MyGo does, made to hold up on the way
// there: a proxy that times out on github.com's downloads (504), caches that
// keep the redirect of the latest release, and the network now and then.
// Taken from Vibe Atlas (Zker67/vibe-atlas updates.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/netproxy"
)

// updateRepo is the repository whose releases hold the updates, which
// updates.github in mygo.json names too.
const updateRepo = "Zker67/appshelf"

// useUpdateFeed makes the updater, and the update window with it, read the
// updates through githubFeed. The archives are checked against the app's
// public key as before.
func useUpdateFeed() {
	mygo.Updater.SetSource(githubFeed{
		site: "https://github.com/" + updateRepo,
		api:  "https://api.github.com/repos/" + updateRepo,
	})
}

// githubFeed reads the updates of a repository's releases as MyGo does,
// with three changes. The manifest of the latest release is asked for with
// a query no cache has seen: GitHub answers with a redirect to the release,
// which a cache on the way kept for a while, so that the release before
// showed for a minute or two after a new one came out. A file that
// github.com does not give, as a proxy that times out on its downloads
// (504) makes it, comes through GitHub's API instead, which leads to the
// same store of files another way; the files of a release never change.
// And a file that neither gives for a reason that may pass, the network or
// a busy server, is asked for again, a few times. AppShelf offers no
// prereleases: the manifest is the latest release's.
type githubFeed struct {
	site string // https://github.com/<repo>
	api  string // https://api.github.com/repos/<repo>
}

func (f githubFeed) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	return retry(ctx, func() (io.ReadCloser, error) {
		name := "update-" + runtime.GOOS + "-" + runtime.GOARCH + ".json"
		rc, err := updateGet(ctx, connections(siteWait), f.site+"/releases/latest/download/"+name+"?t="+fresh(), "")
		if err == nil {
			return rc, nil
		}
		rc, apiErr := f.fromAPI(ctx, "latest", name)
		if apiErr == nil {
			log.Println("the manifest came through GitHub's API:", err)
			return rc, nil
		}
		return nil, errors.Join(err, apiErr)
	})
}

func (f githubFeed) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	return retry(ctx, func() (io.ReadCloser, error) {
		rc, err := updateGet(ctx, connections(siteWait), url, "")
		if err == nil {
			return rc, nil
		}
		// https://github.com/<repo>/releases/download/<tag>/<name>
		tag, name, ok := strings.Cut(strings.TrimPrefix(url, f.site+"/releases/download/"), "/")
		if !ok || !strings.HasPrefix(url, f.site+"/releases/download/") || strings.Contains(name, "/") {
			return nil, err
		}
		rc, apiErr := f.fromAPI(ctx, "tags/"+tag, name)
		if apiErr == nil {
			log.Println("the update came through GitHub's API:", err)
			return rc, nil
		}
		return nil, errors.Join(err, apiErr)
	})
}

// fromAPI downloads the file name of a release, "latest" or "tags/<tag>",
// through GitHub's API, on connections of its own: a proxy may send each
// connection out through another address, and one whose quota is used up
// stays so for the connection.
func (f githubFeed) fromAPI(ctx context.Context, release, name string) (io.ReadCloser, error) {
	rt := connections(0)
	rc, err := updateGet(ctx, rt, f.api+"/releases/"+release+"?t="+fresh(), "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var r struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	err = json.NewDecoder(io.LimitReader(rc, 4<<20)).Decode(&r)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("the release %s: %w", release, err)
	}
	for _, a := range r.Assets {
		if a.Name == name {
			return updateGet(ctx, rt, a.URL, "application/octet-stream")
		}
	}
	return nil, fmt.Errorf("the release %s has no %s", release, name)
}

// siteWait is how long github.com may take to start answering before the
// API is asked instead: a proxy that times out on its downloads answers
// 504 after 12 s or so.
var siteWait = 6 * time.Second // tests shorten it

// A file is asked for this many times, longer and longer apart, before
// it fails: a proxy on the way to GitHub answers 504 now and then, and
// goes out through addresses whose API quota others used up.
var updateRetryWaits = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// retry runs try until it succeeds, fails for a reason that trying again
// does not mend, or has been tried len(updateRetryWaits)+1 times. A file
// that fails partway through is not asked for again here: the update
// window says so, and checking again starts over.
func retry(ctx context.Context, try func() (io.ReadCloser, error)) (io.ReadCloser, error) {
	rc, err := try()
	for _, wait := range updateRetryWaits {
		if err == nil || !transient(err) || ctx.Err() != nil {
			return rc, err
		}
		log.Println("trying again:", err)
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(wait):
		}
		rc, err = try()
	}
	return rc, err
}

// httpStatus finds the status of a failed request in the errors,
// "GET <url>: 504 Gateway Time-out".
var httpStatus = regexp.MustCompile(`GET \S+: (\d{3}) `)

func statusOf(err error) int {
	if m := httpStatus.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// transient reports whether err may go away when tried again: the
// network, or a server or proxy that is busy.
func transient(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	n := statusOf(err)
	return n == 429 || n >= 500
}

// fresh returns a query value no cache has seen.
func fresh() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// updateTransport carries the requests of the updates: through the proxy
// of the system's settings, or of HTTPS_PROXY, as MyGo's own. Tests
// replace it.
var updateTransport = func() http.RoundTripper { return netproxy.Transport() }

// connections returns a transport of updateTransport's with connections of
// its own, which gives up when an answer does not start within wait, when
// not 0; the body may take longer.
func connections(wait time.Duration) http.RoundTripper {
	rt := updateTransport()
	if t, ok := rt.(*http.Transport); ok {
		t = t.Clone()
		t.ResponseHeaderTimeout = wait
		rt = t
	}
	return rt
}

// updateGet fetches a file of the updates over HTTPS through rt, asking
// for accept when not "". Its errors read as MyGo's, "GET <url>: 504
// Gateway Time-out", which statusOf reads.
func updateGet(ctx context.Context, rt http.RoundTripper, rawURL, accept string) (io.ReadCloser, error) {
	if !strings.HasPrefix(rawURL, "https://") {
		return nil, fmt.Errorf("refusing to download %s: updates need HTTPS", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// ASCII: GitHub's store of release files refuses other user agents.
	req.Header.Set("User-Agent", "mygo-updater")
	req.Header.Set("Cache-Control", "no-cache")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := (&http.Client{Transport: rt}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	return resp.Body, nil
}
