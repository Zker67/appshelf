package icons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"appshelf/internal/jsonfile"
)

// DatabaseURL is UniGetUI's database of icons, MIT licensed; the icons are
// their owners'. AppShelf downloads it at run time and keeps it in its
// cache, it does not ship it.
const DatabaseURL = "https://raw.githubusercontent.com/Devolutions/UniGetUI/main/WebBasedData/screenshot-database-v2.json"

const (
	// dbMaxAge is how long the database is used before asking for a newer
	// one.
	dbMaxAge = 7 * 24 * time.Hour
	// dbRetry is how long a failed download waits before the next.
	dbRetry = 10 * time.Minute
	// dbMaxSize bounds the download: it was 1.5 MB in 2026.
	dbMaxSize = 32 << 20
)

var replacer = strings.NewReplacer(" ", "-", "_", "-", ".", "-", "/", "-", ",", "-")

// iconID normalizes a name as UniGetUI's GenerateIconId does: lower case,
// with spaces, underscores, dots, slashes and commas as hyphens.
func iconID(s string) string { return replacer.Replace(strings.ToLower(strings.TrimSpace(s))) }

// WingetIconID is the database's key of a winget package: its ID without
// the publisher, "Obsidian.Obsidian" → "obsidian".
func WingetIconID(id string) string {
	if _, rest, ok := strings.Cut(id, "."); ok && rest != "" {
		id = rest
	}
	return iconID(id)
}

// ScoopIconID is the database's key of a scoop app: its name without
// ".app".
func ScoopIconID(name string) string {
	return iconID(strings.ReplaceAll(strings.ToLower(name), ".app", ""))
}

// database is the icon database: its file in the cache, read once, and
// downloaded again after dbMaxAge.
type database struct {
	client *http.Client
	url    string
	dir    string

	mu       sync.Mutex
	loaded   map[string]string // key, lower case → icon URL
	loadedAt time.Time         // when the file was downloaded or found current
	failedAt time.Time
	busy     chan struct{} // closed when the running download ends
}

type dbMeta struct {
	ETag      string    `json:"etag"`
	CheckedAt time.Time `json:"checkedAt"`
}

func newDatabase(client *http.Client, url, dir string) *database {
	return &database{client: client, url: url, dir: dir}
}

func (d *database) file() string     { return filepath.Join(d.dir, "database.json") }
func (d *database) metaFile() string { return filepath.Join(d.dir, "database.meta.json") }

// icons returns the database. The first call reads the file of the cache,
// or downloads it; a database older than dbMaxAge is used while a newer one
// downloads.
func (d *database) icons(ctx context.Context) (map[string]string, error) {
	d.mu.Lock()
	if d.loaded == nil && d.busy == nil {
		// Read the cache once.
		if m, at, err := d.readCache(); err == nil {
			d.loaded, d.loadedAt = m, at
		}
	}
	icons := d.loaded
	stale := icons == nil || time.Since(d.loadedAt) > dbMaxAge
	retry := d.failedAt.IsZero() || time.Since(d.failedAt) > dbRetry
	var wait chan struct{}
	if stale && retry {
		if d.busy == nil {
			d.busy = make(chan struct{})
			go d.download()
		}
		wait = d.busy
	}
	d.mu.Unlock()
	if icons != nil {
		return icons, nil
	}
	if wait == nil {
		return nil, errors.New("图标库暂时无法下载")
	}
	select {
	case <-wait:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.loaded == nil {
		return nil, errors.New("图标库暂时无法下载")
	}
	return d.loaded, nil
}

func (d *database) readCache() (map[string]string, time.Time, error) {
	data, err := os.ReadFile(d.file())
	if err != nil {
		return nil, time.Time{}, err
	}
	m, err := parseDatabase(data)
	if err != nil {
		return nil, time.Time{}, err
	}
	var meta dbMeta
	_ = jsonfile.Read(d.metaFile(), &meta)
	return m, meta.CheckedAt, nil
}

// download asks for the database, with the ETag of the file kept, and
// keeps a newer one.
func (d *database) download() {
	m, err := d.fetchDatabase()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		d.failedAt = time.Now()
	} else {
		if m != nil {
			d.loaded = m
		}
		d.loadedAt, d.failedAt = time.Now(), time.Time{}
	}
	close(d.busy)
	d.busy = nil
}

// fetchDatabase returns the database downloaded, or nil when the file kept
// is current.
func (d *database) fetchDatabase() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return nil, err
	}
	var meta dbMeta
	_ = jsonfile.Read(d.metaFile(), &meta)
	d.mu.Lock()
	haveFile := d.loaded != nil
	d.mu.Unlock()
	if meta.ETag != "" && haveFile {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	// The client's timeout is for icons: the database is bigger.
	client := *d.client
	client.Timeout = 10 * time.Second
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		meta.CheckedAt = time.Now()
		_ = jsonfile.WriteAtomic(d.metaFile(), meta)
		return nil, nil
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("图标库下载失败：服务器返回 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, dbMaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > dbMaxSize {
		return nil, errors.New("图标库太大")
	}
	m, err := parseDatabase(data)
	if err != nil {
		return nil, err
	}
	if err := jsonfile.WriteBytesAtomic(d.file(), data); err == nil {
		_ = jsonfile.WriteAtomic(d.metaFile(), dbMeta{ETag: resp.Header.Get("ETag"), CheckedAt: time.Now()})
	}
	return m, nil
}

// parseDatabase reads the icons of the database by key, in lower case.
func parseDatabase(data []byte) (map[string]string, error) {
	var raw struct {
		Icons map[string]struct {
			Icon string `json:"icon"`
		} `json:"icons_and_screenshots"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("图标库无法解析：%w", err)
	}
	if raw.Icons == nil {
		return nil, errors.New("图标库没有内容")
	}
	m := make(map[string]string, len(raw.Icons))
	for k, v := range raw.Icons {
		if v.Icon == "" {
			continue
		}
		k = strings.ToLower(k)
		if _, ok := m[k]; !ok {
			m[k] = v.Icon
		}
	}
	return m, nil
}
