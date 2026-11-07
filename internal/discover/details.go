package discover

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"

	"appshelf/internal/catalog"
	"appshelf/internal/jsonfile"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
)

// WingetManifests is where winget's community repository keeps its
// manifests, one directory per package and version.
const WingetManifests = "https://raw.githubusercontent.com/microsoft/winget-pkgs/master/manifests"

// PackageRef names a package of a source: a winget ID, a scoop bucket/name
// or a GitHub owner/name. Version is a winget package's version, when the
// caller knows it.
type PackageRef struct {
	Type    catalog.SourceType `json:"type"`
	ID      string             `json:"id"`
	Version string             `json:"version,omitempty"`
}

// Key identifies the package in the cache, ignoring case.
func (r PackageRef) Key() string {
	k := string(r.Type) + ":" + strings.ToLower(r.ID)
	if r.Type == catalog.Winget && r.Version != "" {
		k += "@" + r.Version
	}
	return k
}

// PackageDetails is what a package's source says of it.
type PackageDetails struct {
	Name         string   `json:"name,omitempty"`
	Publisher    string   `json:"publisher,omitempty"`
	Summary      string   `json:"summary,omitempty"`     // a line
	Description  string   `json:"description,omitempty"` // longer, when there is more than the summary
	Homepage     string   `json:"homepage,omitempty"`
	PublisherURL string   `json:"publisherUrl,omitempty"`
	License      string   `json:"license,omitempty"`
	LicenseURL   string   `json:"licenseUrl,omitempty"`
	ReleaseNotes string   `json:"releaseNotes,omitempty"` // a link
	Copyright    string   `json:"copyright,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Version      string   `json:"version,omitempty"` // the version the source has
	// Of a GitHub repository.
	Stars    int          `json:"stars,omitempty"`
	Archived bool         `json:"archived,omitempty"`
	Release  *ReleaseInfo `json:"release,omitempty"`
}

// ReleaseInfo is the latest release of a repository and the installer
// AppShelf would choose from it.
type ReleaseInfo struct {
	Tag       string    `json:"tag"`
	Published time.Time `json:"published,omitzero"`
	URL       string    `json:"url,omitempty"`
	Asset     string    `json:"asset,omitempty"`
	// AssetError says why no installer could be chosen.
	AssetError string `json:"assetError,omitempty"`
}

// Fetchers read details from the sources. A nil one is a source this
// machine does not have.
type Fetchers struct {
	// WingetLatest returns the version of a package the winget source has,
	// for a reference without one.
	WingetLatest func(ctx context.Context, id string) (string, error)
	ScoopInfo    func(ctx context.Context, bucket, name string) (scoop.Info, error)
	GitHubRepo   func(ctx context.Context, repo string) (github.Repo, error)
	// GitHubRelease returns the latest release and the installer AppShelf
	// would choose for an app of the repository.
	GitHubRelease func(ctx context.Context, app catalog.App) (github.Release, github.Asset, error)
}

// Details reads the details of packages and keeps them on disk.
type Details struct {
	fetch     Fetchers
	manifests string
	client    *http.Client
	dir       string
	now       func() time.Time

	mu  sync.Mutex
	mem map[string]cached
}

type cached struct {
	At      time.Time      `json:"at"`
	Details PackageDetails `json:"details"`
}

// NewDetails returns the details reader, which caches in dir. manifests is
// WingetManifests but in tests.
func NewDetails(fetch Fetchers, dir, manifests string) *Details {
	return &Details{
		fetch:     fetch,
		manifests: strings.TrimRight(cmp.Or(manifests, WingetManifests), "/"),
		client:    &http.Client{Timeout: 15 * time.Second},
		dir:       dir,
		now:       time.Now,
		mem:       map[string]cached{},
	}
}

// ttl is how long details stay fresh: a winget version's manifest does not
// change, a scoop app's manifest does at each version, and a repository's
// latest release may come any time.
func ttl(t catalog.SourceType) time.Duration {
	switch t {
	case catalog.Winget:
		return 7 * 24 * time.Hour
	case catalog.Scoop:
		return 24 * time.Hour
	}
	return time.Hour
}

// Get returns the details of a package, from the cache while they are
// fresh.
func (d *Details) Get(ctx context.Context, ref PackageRef) (PackageDetails, error) {
	ref.ID = strings.TrimSpace(ref.ID)
	if ref.ID == "" {
		return PackageDetails{}, errors.New("缺少软件包 ID")
	}
	if ref.Type == catalog.Winget && ref.Version == "" && d.fetch.WingetLatest != nil {
		v, err := d.fetch.WingetLatest(ctx, ref.ID)
		if err != nil {
			return PackageDetails{}, err
		}
		ref.Version = v
	}
	key := ref.Key()
	if c, ok := d.load(key); ok && d.now().Sub(c.At) < ttl(ref.Type) {
		return c.Details, nil
	}
	var det PackageDetails
	var err error
	keep := true
	switch ref.Type {
	case catalog.Winget:
		det, err = d.winget(ctx, ref.ID, ref.Version)
	case catalog.Scoop:
		det, err = d.scoop(ctx, ref.ID)
	case catalog.GitHub:
		det, keep, err = d.github(ctx, ref.ID)
	default:
		err = errors.New("未知的来源：" + string(ref.Type))
	}
	if err != nil {
		return PackageDetails{}, err
	}
	if keep {
		d.store(key, cached{At: d.now(), Details: det})
	}
	return det, nil
}

func (d *Details) load(key string) (cached, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.mem[key]; ok {
		return c, true
	}
	if d.dir == "" {
		return cached{}, false
	}
	data, err := os.ReadFile(d.file(key))
	if err != nil {
		return cached{}, false
	}
	var c cached
	if json.Unmarshal(data, &c) != nil {
		return cached{}, false
	}
	d.mem[key] = c
	return c, true
}

func (d *Details) store(key string, c cached) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mem[key] = c
	if d.dir != "" {
		// The cache is a convenience: a file that cannot be written is
		// read again next time.
		_ = jsonfile.WriteAtomic(d.file(key), c)
	}
}

func (d *Details) file(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(d.dir, hex.EncodeToString(sum[:12])+".json")
}

// ManifestDir is the directory of a package version in winget's community
// repository: manifests/<first letter>/<ID with dots as slashes>/<version>.
func ManifestDir(id, version string) string {
	parts := []string{url.PathEscape(strings.ToLower(id[:1]))}
	for _, seg := range strings.Split(id, ".") {
		parts = append(parts, url.PathEscape(seg))
	}
	parts = append(parts, url.PathEscape(version))
	return strings.Join(parts, "/")
}

// manifest is the fields of winget's manifests AppShelf shows: a version
// manifest names the default locale, whose manifest has them, and an older
// singleton manifest has all of them in one file.
type manifest struct {
	ManifestType     string   `yaml:"ManifestType"`
	DefaultLocale    string   `yaml:"DefaultLocale"`
	PackageVersion   string   `yaml:"PackageVersion"`
	Publisher        string   `yaml:"Publisher"`
	PublisherURL     string   `yaml:"PublisherUrl"`
	PackageName      string   `yaml:"PackageName"`
	PackageURL       string   `yaml:"PackageUrl"`
	License          string   `yaml:"License"`
	LicenseURL       string   `yaml:"LicenseUrl"`
	Copyright        string   `yaml:"Copyright"`
	ShortDescription string   `yaml:"ShortDescription"`
	Description      string   `yaml:"Description"`
	ReleaseNotesURL  string   `yaml:"ReleaseNotesUrl"`
	Tags             []string `yaml:"Tags"`
}

// ParseManifest reads a winget manifest.
func ParseManifest(data []byte) (manifest, error) {
	var m manifest
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if err := yaml.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("winget 清单无法解析：%w", err)
	}
	return m, nil
}

// errMissing is a manifest the repository does not have.
var errMissing = errors.New("missing")

func (d *Details) getManifest(ctx context.Context, path string) (manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.manifests+"/"+path, nil)
	if err != nil {
		return manifest{}, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return manifest{}, fmt.Errorf("无法读取 winget 清单：%w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return manifest{}, errMissing
	case resp.StatusCode != http.StatusOK:
		return manifest{}, fmt.Errorf("无法读取 winget 清单：服务器返回 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return manifest{}, fmt.Errorf("无法读取 winget 清单：%w", err)
	}
	return ParseManifest(data)
}

// winget reads a package version's manifests from the community
// repository: the version manifest, then the default locale's and the
// Chinese one side by side, the latter for the descriptions.
func (d *Details) winget(ctx context.Context, id, version string) (PackageDetails, error) {
	if version == "" {
		return PackageDetails{}, errors.New("不知道这个软件包的版本")
	}
	dir := ManifestDir(id, version)
	base := dir + "/" + url.PathEscape(id)
	top, err := d.getManifest(ctx, base+".yaml")
	if errors.Is(err, errMissing) {
		return PackageDetails{}, errors.New("winget 仓库里没有这个版本的清单")
	}
	if err != nil {
		return PackageDetails{}, err
	}
	m := top
	if !strings.EqualFold(top.ManifestType, "singleton") {
		locale := cmp.Or(top.DefaultLocale, "en-US")
		var zh manifest
		var zhErr error
		var wg sync.WaitGroup
		if !strings.EqualFold(locale, "zh-CN") {
			wg.Go(func() { zh, zhErr = d.getManifest(ctx, base+".locale.zh-CN.yaml") })
		}
		m, err = d.getManifest(ctx, base+".locale."+url.PathEscape(locale)+".yaml")
		wg.Wait()
		if err != nil {
			if errors.Is(err, errMissing) {
				err = errors.New("winget 仓库里缺少这个版本的语言清单")
			}
			return PackageDetails{}, err
		}
		if zhErr == nil {
			m.ShortDescription = cmp.Or(zh.ShortDescription, m.ShortDescription)
			m.Description = cmp.Or(zh.Description, m.Description)
			if len(zh.Tags) > 0 {
				m.Tags = zh.Tags
			}
		}
	}
	det := PackageDetails{
		Name:         m.PackageName,
		Publisher:    m.Publisher,
		Summary:      strings.TrimSpace(m.ShortDescription),
		Homepage:     m.PackageURL,
		PublisherURL: m.PublisherURL,
		License:      m.License,
		LicenseURL:   m.LicenseURL,
		ReleaseNotes: m.ReleaseNotesURL,
		Copyright:    m.Copyright,
		Tags:         m.Tags,
		Version:      cmp.Or(m.PackageVersion, top.PackageVersion, version),
	}
	if desc := strings.TrimSpace(m.Description); desc != "" && desc != det.Summary {
		det.Description = desc
	}
	return det, nil
}

func (d *Details) scoop(ctx context.Context, ref string) (PackageDetails, error) {
	if d.fetch.ScoopInfo == nil {
		return PackageDetails{}, errors.New("本机没有 scoop")
	}
	bucket, name, ok := strings.Cut(ref, "/")
	if !ok {
		return PackageDetails{}, errors.New("scoop 软件应写成 bucket/name")
	}
	info, err := d.fetch.ScoopInfo(ctx, bucket, name)
	if err != nil {
		return PackageDetails{}, err
	}
	return PackageDetails{
		Name:     info.Name,
		Summary:  strings.TrimSpace(info.Description),
		Homepage: info.Website,
		License:  info.License,
		Version:  info.Version,
	}, nil
}

// github reads the repository and its latest release side by side. A
// repository without a release still has details; the release says why.
// They are kept only when the release could be read: a quota used up or a
// network error does not last.
func (d *Details) github(ctx context.Context, repo string) (PackageDetails, bool, error) {
	if d.fetch.GitHubRepo == nil {
		return PackageDetails{}, false, errors.New("GitHub 来源不可用")
	}
	var (
		rel    github.Release
		asset  github.Asset
		relErr error
		wg     sync.WaitGroup
	)
	if d.fetch.GitHubRelease != nil {
		app := catalog.App{Source: catalog.Source{Type: catalog.GitHub, Repo: repo}}
		wg.Go(func() { rel, asset, relErr = d.fetch.GitHubRelease(ctx, app) })
	}
	r, err := d.fetch.GitHubRepo(ctx, repo)
	wg.Wait()
	if err != nil {
		return PackageDetails{}, false, err
	}
	det := PackageDetails{
		Name:      r.FullName,
		Publisher: r.Owner.Login,
		Summary:   strings.TrimSpace(r.Description),
		Homepage:  cmp.Or(r.Homepage, r.HTMLURL),
		License:   r.LicenseName(),
		Stars:     r.Stars,
		Archived:  r.Archived,
	}
	if r.Homepage != "" {
		det.PublisherURL = r.HTMLURL
	}
	keep := true
	if d.fetch.GitHubRelease != nil {
		info := &ReleaseInfo{}
		if rel.Tag == "" {
			// No release read: none published, or GitHub could not say.
			info.AssetError = "没有读到 release"
			if relErr != nil {
				info.AssetError = relErr.Error()
			}
			keep = false
		} else {
			info.Tag, info.Published, info.URL = rel.Tag, rel.PublishedAt, rel.HTMLURL
			det.Version = rel.Tag
			if relErr != nil {
				info.AssetError = relErr.Error()
			} else {
				info.Asset = asset.Name
			}
		}
		det.Release = info
	}
	return det, keep, nil
}
