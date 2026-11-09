package scoop

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// stages are the lines scoop starts its steps with, always in English, and
// the stage each one starts. "" keeps the stage it is in.
var stages = []struct{ prefix, stage string }{
	{"checking repo", "正在添加 bucket"},
	{"updating '", "正在更新"},
	{"installing '", "install"},
	{"uninstalling '", "uninstall"},
	{"downloading", "正在下载"},
	{"starting download", "正在下载"},
	{"checking hash", "正在校验"},
	{"extracting", "正在解压"},
	{"running installer", "正在运行安装程序"},
	{"running pre_install", "正在运行安装脚本"},
	{"running post_install", "正在运行安装脚本"},
	{"linking", "正在完成"},
	{"creating shim", "正在完成"},
	{"creating shortcut", "正在完成"},
	{"persisting", "正在完成"},
}

// progress follows scoop's output for the stage of its work, and for how
// far its download has come. Its output sent to a file has no progress
// bar: its own downloader writes "Downloading <url> (75.4 MB)..." and the
// file grows in scoop's cache as <app>#<version>#<hash>.download, written
// as it comes, which progress reads every half second; aria2's readouts,
// which scoop passes on, carry the bytes.
type progress struct {
	log   io.Writer
	verb  string // "install", "update" or "uninstall"
	app   string // the app's name, which starts its files in the cache
	cache string // scoop's cache directory
	start time.Time
	every time.Duration

	mu    sync.Mutex
	line  []byte
	stop  chan struct{} // closes to stop watching the download
	total int64         // the size of the download being watched
}

func newProgress(log io.Writer, verb, app, cache string) *progress {
	source.ReportProgress(log, source.Progress{Stage: "正在准备"})
	return &progress{log: log, verb: verb, app: strings.ToLower(app), cache: cache, start: time.Now(), every: 500 * time.Millisecond}
}

func (p *progress) Write(b []byte) (int, error) {
	n, err := p.log.Write(b)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		if c == '\n' || c == '\r' {
			p.lineDone(strings.ToLower(strings.TrimSpace(runner.StripANSI(string(p.line)))))
			p.line = p.line[:0]
			continue
		}
		p.line = append(p.line, c)
	}
	return n, err
}

func (p *progress) lineDone(line string) {
	if done, total, ok := aria2Readout(line); ok {
		p.unwatch()
		source.ReportProgress(p.log, source.Progress{Stage: "正在下载", Done: done, Total: total})
		return
	}
	for _, s := range stages {
		if !strings.HasPrefix(line, s.prefix) {
			continue
		}
		stage := s.stage
		switch {
		case stage == "install" && p.verb == "update":
			stage = "正在安装新版本"
		case stage == "install":
			stage = "正在安装"
		case stage == "uninstall" && p.verb == "update":
			stage = "正在移除旧版本"
		case stage == "uninstall":
			stage = "正在卸载"
		}
		p.unwatch()
		var total int64
		if size, ok := downloadSize(line); ok {
			total = size
			p.watch(size)
		}
		source.ReportProgress(p.log, source.Progress{Stage: stage, Total: total})
		return
	}
}

// watch starts reading the size of the download, of total bytes, as it
// grows. p.mu is held.
func (p *progress) watch(total int64) {
	if p.cache == "" || p.app == "" {
		return
	}
	stop := make(chan struct{})
	p.stop, p.total = stop, total
	go func() {
		t := time.NewTicker(p.every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			done := p.downloaded()
			p.mu.Lock()
			if p.stop == stop && done > 0 {
				source.ReportProgress(p.log, source.Progress{Stage: "正在下载", Done: done, Total: max(p.total, done)})
			}
			p.mu.Unlock()
		}
	}()
}

// unwatch stops reading the download's size. p.mu is held.
func (p *progress) unwatch() {
	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}
}

// close stops reading the download's size, once scoop ended.
func (p *progress) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.unwatch()
}

// downloaded is the size of the app's file being downloaded: the newest
// <app>#….download in the cache written since the work started, as the
// file says, not as its directory does, which lags behind while it is
// written.
func (p *progress) downloaded() int64 {
	entries, err := os.ReadDir(p.cache)
	if err != nil {
		return 0
	}
	var newest time.Time
	var size int64
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if e.IsDir() || !strings.HasPrefix(name, p.app+"#") || !strings.HasSuffix(name, ".download") {
			continue
		}
		fi, err := os.Stat(filepath.Join(p.cache, e.Name()))
		if err != nil || fi.ModTime().Before(p.start.Add(-time.Minute)) || fi.ModTime().Before(newest) {
			continue
		}
		newest, size = fi.ModTime(), fi.Size()
	}
	return size
}

// sizeOfDownload is the size scoop's downloader writes after the URL, as
// "(75.4 mb)...", in the line lower case; its number is formatted as the
// user's culture does, with one decimal and maybe group separators.
var sizeOfDownload = regexp.MustCompile(`\(([0-9][0-9.,\s\x{00a0}\x{202f}']*)\s*(b|kb|mb|gb)\)\.*$`)

// downloadSize reads the size of "downloading <url> (75.4 mb)...".
func downloadSize(line string) (int64, bool) {
	m := sizeOfDownload.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, ok := cultureNumber(m[1], m[2] != "b")
	if !ok || n <= 0 {
		return 0, false
	}
	return int64(n * float64(unitBytes(m[2]))), true
}

// cultureNumber reads a number as .NET's "n1" writes it in any culture:
// with one decimal, after the last separator, when decimal; the other
// separators group thousands.
func cultureNumber(s string, decimal bool) (float64, bool) {
	var digits []rune
	frac := ""
	runes := []rune(strings.TrimSpace(s))
	last := -1
	for i, r := range runes {
		if r < '0' || r > '9' {
			last = i
		}
	}
	if decimal && last >= 0 && len(runes)-last-1 == 1 {
		frac = string(runes[last+1:])
		runes = runes[:last]
	}
	for _, r := range runes {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) == 0 {
		return 0, false
	}
	text := string(digits)
	if frac != "" {
		text += "." + frac
	}
	n, err := strconv.ParseFloat(text, 64)
	return n, err == nil
}

// aria2Bytes is one download of an aria2 readout, lower case:
// "#2089b0 12mib/50mib(24%)".
var aria2Bytes = regexp.MustCompile(`#[0-9a-f]+ ([0-9.]+)([kmgt]?i?b)/([0-9.]+)([kmgt]?i?b)`)

// aria2Readout reads the bytes of the readouts scoop passes on from aria2,
// "download: [#2089b0 12mib/50mib(24%) cn:5 dl:3.2mib eta:12s]", or with
// several downloads "download: [dl:4mib][#… …][#… …]", added up.
func aria2Readout(line string) (done, total int64, ok bool) {
	rest, found := strings.CutPrefix(line, "download: [")
	if !found {
		return 0, 0, false
	}
	for _, m := range aria2Bytes.FindAllStringSubmatch(rest, -1) {
		d, err1 := strconv.ParseFloat(m[1], 64)
		t, err2 := strconv.ParseFloat(m[3], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		done += int64(d * float64(unitBytes(m[2])))
		total += int64(t * float64(unitBytes(m[4])))
		ok = true
	}
	return done, total, ok
}

// unitBytes is the bytes of a unit scoop or aria2 writes, both by 1024:
// "kb" and "kib" alike.
func unitBytes(unit string) int64 {
	switch strings.TrimSuffix(strings.TrimSuffix(unit, "b"), "i") {
	case "k":
		return 1 << 10
	case "m":
		return 1 << 20
	case "g":
		return 1 << 30
	case "t":
		return 1 << 40
	}
	return 1
}

// cacheDir is scoop's cache directory, found as scoop finds it:
// SCOOP_CACHE, else its config's cache_path, else the cache of its root.
func cacheDir() string {
	if d := os.Getenv("SCOOP_CACHE"); d != "" {
		return d
	}
	if d := configString(scoopConfig(), "cache_path"); d != "" {
		return d
	}
	root := scoopRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, "cache")
}

// scoopRoot is where scoop keeps the user's apps: SCOOP, its config's
// root_path, or ~\scoop.
func scoopRoot() string {
	if d := os.Getenv("SCOOP"); d != "" {
		return d
	}
	if d := configString(scoopConfig(), "root_path"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "scoop")
}

// globalRoot is where scoop keeps the apps for all users: SCOOP_GLOBAL,
// its config's global_path, or %ProgramData%\scoop.
func globalRoot() string {
	if d := os.Getenv("SCOOP_GLOBAL"); d != "" {
		return d
	}
	if d := configString(scoopConfig(), "global_path"); d != "" {
		return d
	}
	if d := os.Getenv("ProgramData"); d != "" {
		return filepath.Join(d, "scoop")
	}
	return ""
}

// aria2Missing reports that scoop has no aria2, for the user or for all
// users, and the user did not turn it off with aria2-enabled false.
func aria2Missing() bool {
	switch v := scoopConfig()["aria2-enabled"].(type) {
	case bool:
		if !v {
			return false
		}
	case string:
		if strings.EqualFold(v, "false") {
			return false
		}
	}
	for _, root := range []string{scoopRoot(), globalRoot()} {
		if root == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "apps", "aria2", "current")); err == nil {
			return false
		}
	}
	return true
}

func configString(cfg map[string]any, key string) string {
	s, _ := cfg[key].(string)
	return s
}

// scoopConfig reads scoop's config.json, under XDG_CONFIG_HOME or
// ~\.config, its keys in lower case.
func scoopConfig() map[string]any {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(dir, "scoop", "config.json"))
	if err != nil {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[strings.ToLower(k)] = v
	}
	return out
}
