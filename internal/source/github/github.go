// Package github installs apps from the latest release of a GitHub
// repository: it picks the Windows installer of the release, checks it
// against the size and digest GitHub gives, and runs it. Whether an app is
// installed comes from its detection rule, which is required, and from
// what AppShelf recorded when it installed it.
package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/runner"
	"appshelf/internal/source"
	"appshelf/internal/state"
	"appshelf/internal/version"
)

// DefaultAPI is GitHub's REST API.
const DefaultAPI = "https://api.github.com"

// Release is a release as the API returns it.
type Release struct {
	Tag        string  `json:"tag_name"`
	Name       string  `json:"name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Rate is the request quota GitHub reported last.
type Rate struct {
	Known     bool
	Limit     int
	Remaining int
	Reset     time.Time
}

// RateLimitError reports the request quota used up.
type RateLimitError struct{ Reset time.Time }

func (e *RateLimitError) Error() string {
	if e.Reset.IsZero() {
		return "GitHub 请求额度已用完"
	}
	return "GitHub 请求额度已用完，" + e.Reset.Local().Format("15:04") + " 恢复"
}

// Options configure the source.
type Options struct {
	Store   *state.Store
	TempDir string // where installers are downloaded, then removed
	API     string // DefaultAPI when empty
	Token   string // optional; raises the quota from 60 requests an hour
	Version string // AppShelf's version, for the User-Agent
}

// Source is the GitHub releases source.
type Source struct {
	api, token, agent string
	store             *state.Store
	tempDir           string
	machine           string
	apiClient         *http.Client
	dlClient          *http.Client
	// scan and shellRun are the registry and the installer launch, which
	// tests replace.
	scan     func() ([]detect.Entry, error)
	shellRun func(file, params, dir string) (uint32, error)

	mu   sync.Mutex
	rate Rate
}

// New returns the GitHub source.
func New(opt Options) *Source {
	api := strings.TrimRight(opt.API, "/")
	if api == "" {
		api = DefaultAPI
	}
	agent := "AppShelf"
	if opt.Version != "" {
		agent += "/" + opt.Version
	}
	return &Source{
		api: api, token: opt.Token, agent: agent,
		store: opt.Store, tempDir: opt.TempDir, machine: runtime.GOARCH,
		apiClient: &http.Client{Timeout: 30 * time.Second},
		dlClient:  &http.Client{},
		scan:      detect.Scan,
		shellRun:  runner.ShellRun,
	}
}

func (s *Source) Type() catalog.SourceType { return catalog.GitHub }

// Check is always available: network problems show on each app.
func (s *Source) Check(context.Context) source.Availability {
	if s.token != "" {
		return source.Availability{OK: true, Reason: "已设置 GitHub token"}
	}
	return source.Availability{OK: true, Reason: "未设置 GitHub token，每小时最多 60 次请求"}
}

// HasToken reports whether requests carry a token.
func (s *Source) HasToken() bool { return s.token != "" }

// Rate returns the quota GitHub reported last.
func (s *Source) Rate() Rate {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rate
}

// Latest returns the latest release of a repository, which GitHub chooses
// among those neither drafts nor prereleases.
func (s *Source) Latest(ctx context.Context, repo string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.api+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", s.agent)
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.apiClient.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("无法连接 GitHub：%w", err)
	}
	defer resp.Body.Close()
	rate := s.noteRate(resp.Header)
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound:
		return Release{}, errors.New("仓库不存在，或者没有正式版 release")
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) && rate.Known && rate.Remaining == 0:
		return Release{}, &RateLimitError{Reset: rate.Reset}
	case resp.StatusCode == http.StatusUnauthorized:
		return Release{}, errors.New("GitHub token 无效")
	default:
		return Release{}, fmt.Errorf("GitHub 返回 %d", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("GitHub 的回复无法解析：%w", err)
	}
	if rel.Tag == "" || rel.Draft || rel.Prerelease {
		return Release{}, errors.New("没有正式版 release")
	}
	return rel, nil
}

func (s *Source) noteRate(h http.Header) Rate {
	limit, err1 := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, err2 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err3 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err1 == nil && err2 == nil && err3 == nil {
		s.rate = Rate{Known: true, Limit: limit, Remaining: remaining, Reset: time.Unix(reset, 0)}
	}
	return s.rate
}

// Describe returns the latest release of an app and the installer
// AppShelf would install from it, for the user to confirm.
func (s *Source) Describe(ctx context.Context, app catalog.App) (Release, Asset, error) {
	rel, err := s.Latest(ctx, app.Source.Repo)
	if err != nil {
		return Release{}, Asset{}, err
	}
	asset, err := ChooseAsset(rel.Assets, app.Source.AssetPattern, s.machine)
	return rel, asset, err
}

// Statuses reads the registry once and the latest release of each app,
// four at a time.
func (s *Source) Statuses(ctx context.Context, apps []catalog.App) (map[string]source.Status, error) {
	out := map[string]source.Status{}
	if len(apps) == 0 {
		return out, nil
	}
	entries, scanErr := s.scan()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, app := range apps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rel, relErr := s.Latest(ctx, app.Source.Repo)
			st := s.statusFor(app, rel, relErr, entries, scanErr)
			mu.Lock()
			out[app.ID] = st
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out, nil
}

// Recheck reads one app's state.
func (s *Source) Recheck(ctx context.Context, app catalog.App) (source.Status, error) {
	entries, scanErr := s.scan()
	rel, relErr := s.Latest(ctx, app.Source.Repo)
	return s.statusFor(app, rel, relErr, entries, scanErr), nil
}

// statusFor works out an app's state from its detection rule, AppShelf's
// record and the latest release.
func (s *Source) statusFor(app catalog.App, rel Release, relErr error, entries []detect.Entry, scanErr error) source.Status {
	if app.Detect == nil {
		return source.UnknownStatus("GitHub 来源缺少检测规则")
	}
	if scanErr != nil {
		return source.UnknownStatus("无法读取注册表：" + scanErr.Error())
	}
	det, err := detect.Match(*app.Detect, entries)
	if err != nil {
		return source.UnknownStatus(err.Error())
	}
	var asset Asset
	var assetErr error
	if relErr == nil {
		asset, assetErr = ChooseAsset(rel.Assets, app.Source.AssetPattern, s.machine)
	}
	rec, hasRec := s.store.GitHub(app.ID)

	if !det.Installed {
		switch {
		case relErr != nil:
			return source.Status{State: source.NotInstalled, Note: "无法读取最新 release：" + relErr.Error()}
		case assetErr != nil:
			return source.UnknownStatus(assetErr.Error())
		}
		return source.Status{State: source.NotInstalled, Latest: rel.Tag, Asset: asset.Name}
	}

	installed := det.Version
	if installed == "" && hasRec {
		installed = rec.Tag
	}
	st := source.Status{State: source.Installed, Version: installed}
	switch {
	case relErr != nil:
		st.Note = "无法检查更新：" + relErr.Error()
		return st
	case assetErr != nil:
		st.Note = "无法检查更新：" + assetErr.Error()
		return st
	case hasRec && version.Normalize(rec.Tag) == version.Normalize(rel.Tag):
		// AppShelf installed the latest release, whatever version the
		// installer wrote in the registry.
		return st
	}
	if cmp, ok := version.Compare(rel.Tag, installed); ok {
		if cmp > 0 {
			st.State, st.Latest, st.Asset = source.UpdateAvailable, rel.Tag, asset.Name
		}
		return st
	}
	if hasRec {
		// The versions do not compare: AppShelf's record of another
		// release means a newer one is out.
		st.State, st.Latest, st.Asset = source.UpdateAvailable, rel.Tag, asset.Name
		return st
	}
	st.Note = fmt.Sprintf("无法比较版本（已装 %s，最新 %s）", orUnknown(installed), rel.Tag)
	return st
}

func orUnknown(v string) string {
	if v == "" {
		return "版本未知"
	}
	return v
}

// Install downloads the installer of the latest release, checks it, runs
// it and waits for it.
func (s *Source) Install(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	if log == nil {
		log = io.Discard
	}
	rel, asset, err := s.Describe(ctx, app)
	if err != nil {
		return source.Result{}, err
	}
	fmt.Fprintf(log, "> %s %s：%s\n", app.Source.Repo, rel.Tag, asset.Name)
	path, err := s.download(ctx, asset, log)
	if err != nil {
		return source.Result{}, err
	}
	defer os.Remove(path)
	code, err := s.runInstaller(path, app.Source.InstallerArgs, log)
	if err != nil {
		return source.Result{}, err
	}
	fmt.Fprintf(log, "安装程序退出码：%d\n", code)
	res := source.Result{GitHub: &state.GitHubRecord{Tag: rel.Tag, AssetName: asset.Name, Digest: asset.Digest, InstalledAt: time.Now().UTC()}}
	switch code {
	case 0:
	case 3010, 1641: // Windows Installer: restart required, or started
		res.RebootRequired = true
	case 1602:
		return source.Result{}, errors.New("安装被取消")
	case 1618:
		return source.Result{}, errors.New("另一个安装正在进行，请稍后重试")
	default:
		return source.Result{}, fmt.Errorf("安装程序以退出码 %d 结束", code)
	}
	return res, nil
}

// Upgrade installs the latest release over the installed one.
func (s *Source) Upgrade(ctx context.Context, app catalog.App, log io.Writer) (source.Result, error) {
	return s.Install(ctx, app, log)
}

// download saves an asset in the temporary directory, checking its size
// and its SHA-256 digest when GitHub gives one.
func (s *Source) download(ctx context.Context, a Asset, log io.Writer) (string, error) {
	name := filepath.Base(strings.ReplaceAll(a.Name, `\`, "/"))
	if name == "." || name == "/" || name == "" {
		return "", errors.New("安装包文件名无效")
	}
	if err := os.MkdirAll(s.tempDir, 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.agent)
	resp, err := s.dlClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败：服务器返回 %d", resp.StatusCode)
	}
	f, err := os.CreateTemp(s.tempDir, "appshelf-*-"+name)
	if err != nil {
		return "", err
	}
	path := f.Name()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	fail := func(e error) (string, error) {
		os.Remove(path)
		return "", e
	}
	if err != nil {
		return fail(fmt.Errorf("下载失败：%w", err))
	}
	if a.Size > 0 && n != a.Size {
		return fail(fmt.Errorf("下载的大小 %d 与 GitHub 给出的 %d 不符", n, a.Size))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if want, ok := strings.CutPrefix(strings.ToLower(a.Digest), "sha256:"); ok {
		if want != sum {
			return fail(errors.New("下载的文件与 GitHub 给出的 SHA-256 不符，已删除"))
		}
		fmt.Fprintf(log, "已下载 %d 字节，SHA-256 与 GitHub 一致\n", n)
	} else {
		fmt.Fprintf(log, "已下载 %d 字节（GitHub 没有给出 SHA-256：%s）\n", n, sum)
	}
	return path, nil
}

// runInstaller runs an .msi with Windows Installer, passively unless args
// say otherwise, or an .exe with args as its command line.
func (s *Source) runInstaller(path, args string, log io.Writer) (uint32, error) {
	file, params := path, args
	if strings.EqualFold(filepath.Ext(path), ".msi") {
		if args == "" {
			args = "/passive"
		}
		file = filepath.Join(os.Getenv("SystemRoot"), "System32", "msiexec.exe")
		params = `/i "` + path + `" /norestart ` + args
	}
	fmt.Fprintf(log, "> %s %s\n", file, params)
	code, err := s.shellRun(file, params, filepath.Dir(path))
	if err != nil {
		return 0, err
	}
	return code, nil
}
