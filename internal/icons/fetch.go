package icons

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"appshelf/internal/jsonfile"
)

const (
	// maxIconSize bounds an icon downloaded or read.
	maxIconSize = 2 << 20
	// failTTL is how long a URL that failed is not asked again.
	failTTL = 2 * time.Minute
	// maxDownloads is how many icons download at once.
	maxDownloads = 6
)

// types are the image types served, by the extension of their cache file.
var types = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",
}

// extOf returns the extension of an image type, or "" for one not served.
func extOf(ctype string) string {
	switch ctype {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg", "image/pjpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "image/x-icon", "image/vnd.microsoft.icon", "image/ico":
		return ".ico"
	}
	return ""
}

// fetcher downloads icons into a cache directory, one file per URL, and
// briefly remembers URLs that failed, without pinning temporary outages all day.
type fetcher struct {
	client *http.Client
	dir    string
	sem    chan struct{}

	mu       sync.Mutex
	inflight map[string]*call
}

type call struct {
	done  chan struct{}
	data  []byte
	ctype string
	err   error
}

func newFetcher(client *http.Client, dir string) *fetcher {
	return &fetcher{client: client, dir: dir, sem: make(chan struct{}, maxDownloads), inflight: map[string]*call{}}
}

func (f *fetcher) base(u string) string {
	sum := sha256.Sum256([]byte(u))
	return filepath.Join(f.dir, hex.EncodeToString(sum[:16]))
}

// get returns the icon at u, from the cache, else downloaded once however
// many ask for it at the same time.
func (f *fetcher) get(ctx context.Context, u string) ([]byte, string, error) {
	base := f.base(u)
	for ext, ctype := range types {
		if data, err := os.ReadFile(base + ext); err == nil {
			return data, ctype, nil
		}
	}
	if fi, err := os.Stat(base + ".fail"); err == nil && time.Since(fi.ModTime()) < failTTL {
		return nil, "", ErrNoIcon
	}

	f.mu.Lock()
	c, ok := f.inflight[u]
	if !ok {
		c = &call{done: make(chan struct{})}
		f.inflight[u] = c
		go func() {
			c.data, c.ctype, c.err = f.download(u)
			if c.err != nil {
				_ = jsonfile.WriteBytesAtomic(base+".fail", nil)
			} else {
				_ = jsonfile.WriteBytesAtomic(base+extOf(c.ctype), c.data)
			}
			f.mu.Lock()
			delete(f.inflight, u)
			f.mu.Unlock()
			close(c.done)
		}()
	}
	f.mu.Unlock()
	select {
	case <-c.done:
		return c.data, c.ctype, c.err
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
}

// download fetches an icon: http(s) only, of a public host, at most
// maxIconSize, of an image type served.
func (f *fetcher) download(raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	if err := checkURL(u); err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	select {
	case f.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer func() { <-f.sem }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "AppShelf")
	req.Header.Set("Accept", "image/*")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("server returned %d", resp.StatusCode)
	}
	if resp.ContentLength > maxIconSize {
		return nil, "", errors.New("icon too big")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxIconSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxIconSize {
		return nil, "", errors.New("icon too big")
	}
	ctype := imageType(resp.Header.Get("Content-Type"), data)
	if ctype == "" {
		return nil, "", errors.New("not an image")
	}
	return data, ctype, nil
}

// imageType returns the type of an image served, from its header or, when
// the server says nothing useful, its content; "" when it is no image.
func imageType(header string, data []byte) string {
	mt, _, _ := mime.ParseMediaType(header)
	if ext := extOf(strings.ToLower(mt)); ext != "" {
		return types[ext]
	}
	sniffed, _, _ := mime.ParseMediaType(http.DetectContentType(data))
	if ext := extOf(sniffed); ext != "" {
		return types[ext]
	}
	head := bytes.ToLower(data[:min(len(data), 1024)])
	if bytes.Contains(head, []byte("<svg")) {
		return "image/svg+xml"
	}
	return ""
}

// checkURL is publicURL but in tests, whose servers are local.
var checkURL = publicURL

// publicURL lets icons come from http(s) on public hosts only: the database
// is someone else's, and its URLs must not reach this machine or its
// network.
func publicURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("not an http(s) URL")
	}
	host := u.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return errors.New("local host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return errors.New("private address")
		}
	}
	return nil
}
