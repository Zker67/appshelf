package discover

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/source/github"
	"appshelf/internal/version"
)

// ReleaseNotes are what changed in the versions of a package after the one
// installed, newest first. They are for display only.
type ReleaseNotes struct {
	Notes []ReleaseNote `json:"notes"`
	// From is where the notes come from: "github" or "winget"; "" when
	// there are none.
	From string `json:"from,omitempty"`
	// Repo is the GitHub repository read, as "owner/name".
	Repo string `json:"repo,omitempty"`
	// Link is a page of the release notes, when the source names one.
	Link string `json:"link,omitempty"`
	// More means there are versions between the one installed and those
	// listed that are left out.
	More bool `json:"more,omitzero"`
	// Reason says why there are none, when there are none.
	Reason string `json:"reason,omitempty"`

	// failed means reading failed on the way, as GitHub's quota used up:
	// the notes are not kept.
	failed bool
}

// ReleaseNote is what a version changed.
type ReleaseNote struct {
	Version   string    `json:"version"`
	Title     string    `json:"title,omitempty"`
	Published time.Time `json:"published,omitzero"`
	// Body is the notes: Markdown of a GitHub release, plain text of a
	// winget manifest.
	Body       string `json:"body,omitempty"`
	URL        string `json:"url,omitempty"`
	Prerelease bool   `json:"prerelease,omitzero"`
}

type cachedNotes struct {
	at    time.Time
	notes ReleaseNotes
}

// maxNotes is how many versions are listed at most.
const maxNotes = 10

// notesTTL is how long release notes read are kept, in memory: GitHub's
// quota without a token is 60 requests an hour.
const notesTTL = time.Hour

// ReleaseNotes returns what changed in the versions of a package after
// from, the version installed, up to ref.Version, the version to update
// to (the latest when empty). A GitHub repository's releases are listed
// version by version; a winget package's come from the repository its
// links or installers name, else from its manifest; a scoop app's from the
// repository its homepage is. Versions that do not match the repository's
// tags are not guessed: the notes are left empty and the link stays.
func (d *Details) ReleaseNotes(ctx context.Context, ref PackageRef, from string) (ReleaseNotes, error) {
	ref.ID = strings.TrimSpace(ref.ID)
	if ref.ID == "" {
		return ReleaseNotes{}, errors.New("缺少软件包 ID")
	}
	key := ref.Key() + "#" + strings.ToLower(ref.Version) + "#" + strings.ToLower(from)
	d.mu.Lock()
	c, ok := d.notes[key]
	d.mu.Unlock()
	if ok && d.now().Sub(c.at) < notesTTL {
		return c.notes, nil
	}
	var out ReleaseNotes
	var err error
	switch ref.Type {
	case catalog.GitHub:
		out, err = d.githubNotes(ctx, ref.ID, from, ref.Version)
	case catalog.Winget:
		out, err = d.wingetNotes(ctx, ref.ID, from, ref.Version)
	case catalog.Scoop:
		out, err = d.scoopNotes(ctx, ref.ID, from, ref.Version)
	default:
		err = errors.New("未知的来源：" + string(ref.Type))
	}
	if err != nil {
		return ReleaseNotes{}, err
	}
	if out.Notes == nil {
		out.Notes = []ReleaseNote{}
	}
	if !out.failed {
		d.mu.Lock()
		d.notes[key] = cachedNotes{at: d.now(), notes: out}
		d.mu.Unlock()
	}
	return out, nil
}

func (d *Details) githubNotes(ctx context.Context, repo, from, to string) (ReleaseNotes, error) {
	if d.fetch.GitHubReleases == nil {
		return ReleaseNotes{}, errors.New("GitHub 来源不可用")
	}
	rels, err := d.fetch.GitHubReleases(ctx, repo)
	if err != nil {
		return ReleaseNotes{}, err
	}
	out := ReleaseNotes{From: "github", Repo: repo, Link: "https://github.com/" + repo + "/releases"}
	picked, more, ok := pickReleases(rels, from, to)
	if !ok {
		out.From = ""
		out.Reason = "仓库的 release 和这个版本号对不上"
		if len(rels) == 0 {
			out.Reason = "仓库没有发布 release"
		}
		return out, nil
	}
	out.More = more
	for _, r := range picked {
		out.Notes = append(out.Notes, ReleaseNote{
			Version: r.Tag, Title: strings.TrimSpace(r.Name), Published: r.PublishedAt,
			Body: strings.TrimSpace(r.Body), URL: r.HTMLURL, Prerelease: r.Prerelease,
		})
	}
	return out, nil
}

// pickReleases chooses the releases after from up to to, newest first, at
// most maxNotes; more means some were left out, or may be older than those
// read. ok is false when no release is to: the tags do not follow the
// versions. With to empty the latest release is to; with from unknown, or
// not comparable, only to is listed. Tags are matched as written first,
// else with the words between their numbers left out, as "v2.56.0.windows.4"
// for "2.56.0.4".
func pickReleases(rels []github.Release, from, to string) (picked []github.Release, more, ok bool) {
	// key is the version of a tag as compared: written, or loose.
	key := tagVersion
	target := -1
	for i, r := range rels {
		if to == "" {
			if !r.Prerelease {
				target = i
				break
			}
			continue
		}
		if sameVersion(tagVersion(r.Tag), to) {
			target = i
			break
		}
	}
	if target < 0 && to != "" {
		if want := looseVersion(version.Normalize(to)); want != "" {
			for i, r := range rels {
				if looseVersion(tagVersion(r.Tag)) == want {
					target = i
					key = func(tag string) string { return looseVersion(tagVersion(tag)) }
					from = cmp.Or(looseVersion(version.Normalize(from)), from)
					break
				}
			}
		}
	}
	if target < 0 {
		return nil, false, false
	}
	top := key(rels[target].Tag)
	if _, comparable := version.Compare(from, top); from == "" || !comparable {
		return rels[target : target+1], false, true
	}
	var in []github.Release
	for i, r := range rels {
		v := key(r.Tag)
		after, ok1 := version.Compare(v, from)
		upTo, ok2 := version.Compare(v, top)
		if !ok1 || !ok2 || after <= 0 || upTo > 0 {
			continue
		}
		// A prerelease between the two is not what is installed next.
		if r.Prerelease && i != target {
			continue
		}
		in = append(in, r)
	}
	if len(in) == 0 {
		// The version installed is not older: to is still what is offered.
		return rels[target : target+1], false, true
	}
	slices.SortStableFunc(in, func(a, b github.Release) int {
		c, _ := version.Compare(key(b.Tag), key(a.Tag))
		return c
	})
	// The releases read stop before from: older ones may be missing.
	if n := len(rels); n >= 30 {
		if c, ok := version.Compare(key(rels[n-1].Tag), from); ok && c > 0 {
			more = true
		}
	}
	if len(in) > maxNotes {
		in, more = in[:maxNotes], true
	}
	return in, more, true
}

// preMarker is a word of a version that marks a prerelease.
var preMarker = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:alpha|beta|rc|pre|preview|dev|nightly|canary|insider|snapshot)`)

// looseVersion is a version with the words between its numbers left out,
// as "2.56.0.4" of "2.56.0.windows.4": "" when a word marks a prerelease,
// or there are fewer than two numbers.
func looseVersion(v string) string {
	if v == "" || preMarker.MatchString(v) {
		return ""
	}
	var nums []string
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '_' }) {
		n, err := strconv.Atoi(part)
		if err == nil && n >= 0 {
			nums = append(nums, strconv.Itoa(n))
		}
	}
	if len(nums) < 2 {
		return ""
	}
	return strings.Join(nums, ".")
}

// tagPrefix is what a tag may have before its version, as "app-v1.2" or
// "release/1.2".
var tagPrefix = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._ -]*?[-_/@ ][vV]?(\d)`)

// tagVersion is the version a release tag names.
func tagVersion(tag string) string {
	v := version.Normalize(tag)
	if v != "" && v[0] >= '0' && v[0] <= '9' {
		return v
	}
	if m := tagPrefix.FindStringSubmatchIndex(tag); m != nil {
		return version.Normalize(tag[m[2]:])
	}
	return v
}

// sameVersion reports whether a tag's version is a package's version:
// written the same, else the same numbers and suffix.
func sameVersion(tag, v string) bool {
	v = version.Normalize(v)
	if strings.EqualFold(tag, v) {
		return true
	}
	c, ok := version.Compare(tag, v)
	if !ok || c != 0 {
		return false
	}
	// "1.2" and "1.2.0" are the same; "1.2.2-rc.1" and "1.2.2" are not.
	return strings.EqualFold(suffix(tag), suffix(v))
}

// suffix is what follows a version's leading dotted numbers, as "-rc.1".
func suffix(v string) string {
	i := 0
	for i < len(v) && (v[i] >= '0' && v[i] <= '9' || v[i] == '.') {
		i++
	}
	return strings.TrimLeft(v[i:], ".")
}

// githubRepo matches a link into a GitHub repository.
var githubRepo = regexp.MustCompile(`(?i)^https?://(?:www\.)?github\.com/([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)`)

// notRepos are the paths of github.com that are not owners.
var notRepos = map[string]bool{"orgs": true, "sponsors": true, "marketplace": true, "topics": true, "apps": true, "features": true, "settings": true}

// RepoOf returns the "owner/name" of a GitHub repository a link points
// into, or "".
func RepoOf(link string) string {
	m := githubRepo.FindStringSubmatch(strings.TrimSpace(link))
	if m == nil || notRepos[strings.ToLower(m[1])] {
		return ""
	}
	name := strings.TrimSuffix(m[2], ".git")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return m[1] + "/" + name
}

func (d *Details) wingetNotes(ctx context.Context, id, from, to string) (ReleaseNotes, error) {
	if to == "" && d.fetch.WingetLatest != nil {
		v, err := d.fetch.WingetLatest(ctx, id)
		if err != nil {
			return ReleaseNotes{}, err
		}
		to = v
	}
	m, top, err := d.wingetManifest(ctx, id, to)
	if err != nil {
		return ReleaseNotes{}, err
	}
	link := ""
	if u, err := url.Parse(m.ReleaseNotesURL); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
		link = m.ReleaseNotesURL
	}
	repo := cmp.Or(RepoOf(m.ReleaseNotesURL), RepoOf(m.PackageURL), installerRepo(m), installerRepo(top))
	if repo == "" && !strings.EqualFold(top.ManifestType, "singleton") {
		// The installers' links are in a manifest of their own.
		base := ManifestDir(id, to) + "/" + url.PathEscape(id)
		if inst, err := d.getManifest(ctx, base+".installer.yaml"); err == nil {
			repo = installerRepo(inst)
		}
	}
	var why string
	failed := false
	if repo != "" && d.fetch.GitHubReleases != nil {
		gh, err := d.githubNotes(ctx, repo, from, to)
		switch {
		case err == nil && len(gh.Notes) > 0:
			gh.Link = cmp.Or(link, gh.Link)
			return gh, nil
		case err != nil:
			why, failed = "GitHub 的发布说明读取失败："+err.Error(), true
		default:
			why = gh.Reason
		}
	}
	out := ReleaseNotes{Link: link, Repo: repo, failed: failed}
	if body := strings.TrimSpace(m.ReleaseNotes); body != "" {
		out.From = "winget"
		out.Notes = []ReleaseNote{{Version: cmp.Or(m.PackageVersion, top.PackageVersion, to), Body: body, URL: link}}
		return out, nil
	}
	if repo != "" && link == "" {
		out.Link = "https://github.com/" + repo + "/releases"
	}
	out.Reason = cmp.Or(why, "winget 清单没有写更新日志")
	return out, nil
}

// installerRepo is the GitHub repository a manifest's installers are
// downloaded from, or "".
func installerRepo(m manifest) string {
	if r := RepoOf(m.InstallerURL); r != "" {
		return r
	}
	for _, in := range m.Installers {
		if r := RepoOf(in.InstallerURL); r != "" {
			return r
		}
	}
	return ""
}

func (d *Details) scoopNotes(ctx context.Context, ref, from, to string) (ReleaseNotes, error) {
	det, err := d.Get(ctx, PackageRef{Type: catalog.Scoop, ID: ref})
	if err != nil {
		return ReleaseNotes{}, err
	}
	repo := RepoOf(det.Homepage)
	if repo == "" {
		return ReleaseNotes{Reason: "scoop 清单没有更新日志，主页也不是 GitHub 仓库"}, nil
	}
	if d.fetch.GitHubReleases == nil {
		return ReleaseNotes{Repo: repo, Link: "https://github.com/" + repo + "/releases", Reason: "GitHub 来源不可用"}, nil
	}
	return d.githubNotes(ctx, repo, from, cmp.Or(to, det.Version))
}
