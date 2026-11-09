package scoop

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"appshelf/internal/source"
)

func TestDownloadSize(t *testing.T) {
	cases := map[string]int64{
		// scoop 0.5 with its output in a file, as on this machine.
		"downloading https://dl.google.com/go/go1.27.3.windows-amd64.zip (75.4 mb)...": mib(75.4),
		"downloading https://x/y.7z (1.2 gb)...":                                       gib(1.2),
		"downloading https://x/y.zip (512 b)...":                                       512,
		"downloading https://x/y.zip (12.0 kb)...":                                     12 << 10,
		// Other cultures: group separators, a decimal comma.
		"downloading https://x/y.zip (1,023.5 mb)...":      mib(1023.5),
		"downloading https://x/y.zip (1.023,5 mb)...":      mib(1023.5),
		"downloading https://x/y.zip (75,4 mb)...":         mib(75.4),
		"downloading https://x/y.zip (1\u00a0023,5 mb)...": mib(1023.5),
	}
	for line, want := range cases {
		if got, ok := downloadSize(line); !ok || got != want {
			t.Errorf("%q = %d %v, want %d", line, got, ok, want)
		}
	}
	for _, line := range []string{"downloading new version", "downloading https://x/(v2)/y.zip...", "downloading https://x/y.zip (? mb)..."} {
		if _, ok := downloadSize(line); ok {
			t.Errorf("%q has no size", line)
		}
	}
}

func TestAria2Readout(t *testing.T) {
	done, total, ok := aria2Readout("download: [#2089b0 12mib/50mib(24%) cn:5 dl:3.2mib eta:12s]")
	if !ok || done != 12<<20 || total != 50<<20 {
		t.Errorf("one: %d/%d %v", done, total, ok)
	}
	done, total, ok = aria2Readout("download: [dl:4.0mib][#a1 1.5mib/2mib(75%) cn:1][#b2 400kib/1.0gib(0%) cn:4]")
	if !ok || done != mib(1.5)+400<<10 || total != 2<<20+1<<30 {
		t.Errorf("two: %d/%d %v", done, total, ok)
	}
	if _, _, ok := aria2Readout("download results:"); ok {
		t.Error("the results are no readout")
	}
	if _, _, ok := aria2Readout("downloading https://x/y.zip (1 mb)..."); ok {
		t.Error("scoop's own line is no readout")
	}
}

// progressRecorder keeps the progress reported.
type progressRecorder struct {
	bytes.Buffer
	mu  sync.Mutex
	all []source.Progress
}

func (r *progressRecorder) ReportProgress(p source.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all = append(r.all, p)
}

func (r *progressRecorder) last() source.Progress {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.all[len(r.all)-1]
}

// scoop's downloader writes the file in the cache as it comes: progress
// reads its size, under the size the line gave; another app's file does
// not count, nor one left from before.
func TestProgressReadsTheDownload(t *testing.T) {
	cache := t.TempDir()
	old := filepath.Join(cache, "go#1.27.2#aaaa.zip.download")
	os.WriteFile(old, make([]byte, 999), 0o644)
	os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	os.WriteFile(filepath.Join(cache, "gopls#0.1#bbbb.zip.download"), make([]byte, 5000), 0o644)

	rec := &progressRecorder{}
	p := newProgress(rec, "install", "Go", cache)
	p.every = 5 * time.Millisecond
	io.WriteString(p, "INFO  Downloading 'go' [64bit] from main bucket\r\n")
	io.WriteString(p, "Downloading https://dl.google.com/go/go1.27.3.windows-amd64.zip (75.4 MB)...\r\n")
	if got := rec.last(); got.Stage != "正在下载" || got.Total != mib(75.4) || got.Done != 0 {
		t.Fatalf("on the line: %+v", got)
	}
	f, err := os.Create(filepath.Join(cache, "go#1.27.3#0cd68ac.zip.download"))
	if err != nil {
		t.Fatal(err)
	}
	f.Write(make([]byte, 3000))
	waitFor(t, func() bool { return rec.last().Done == 3000 })
	f.Write(make([]byte, 4000))
	waitFor(t, func() bool { return rec.last().Done == 7000 })
	f.Close()
	if got := rec.last(); got.Total != mib(75.4) || got.Stage != "正在下载" {
		t.Errorf("while downloading: %+v", got)
	}

	// The next step stops reading it.
	io.WriteString(p, "Checking hash of go1.27.3.windows-amd64.zip ... OK.\r\n")
	if got := rec.last(); got.Stage != "正在校验" || got.Done != 0 || got.Total != 0 {
		t.Errorf("after: %+v", got)
	}
	n := len(rec.all)
	time.Sleep(30 * time.Millisecond)
	rec.mu.Lock()
	if len(rec.all) != n {
		t.Errorf("still reading the download: %+v", rec.all[n:])
	}
	rec.mu.Unlock()
	p.close()
}

// aria2's readouts come with a carriage return each, as scoop writes them.
func TestProgressReadsAria2(t *testing.T) {
	rec := &progressRecorder{}
	p := newProgress(rec, "update", "go", t.TempDir())
	io.WriteString(p, "Starting download with aria2...\r\n")
	io.WriteString(p, "\rDownload: [#2089b0 0B/0B CN:1 DL:0B]          ")
	io.WriteString(p, "\rDownload: [#2089b0 12MiB/50MiB(24%) CN:5 DL:3.2MiB ETA:12s]   ")
	io.WriteString(p, "\rDownload: [#2089b0 50MiB/50MiB(100%) CN:5 DL:3.2MiB]\r\n")
	if got := rec.last(); got.Stage != "正在下载" || got.Done != 50<<20 || got.Total != 50<<20 {
		t.Errorf("last = %+v", got)
	}
	p.close()
}

func TestCacheDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("SCOOP", "")
	t.Setenv("SCOOP_CACHE", "")
	if got := cacheDir(); got != filepath.Join(home, "scoop", "cache") {
		t.Errorf("default = %q", got)
	}
	t.Setenv("SCOOP", `D:\scoop`)
	if got := cacheDir(); got != filepath.Join(`D:\scoop`, "cache") {
		t.Errorf("SCOOP = %q", got)
	}
	os.MkdirAll(filepath.Join(home, ".config", "scoop"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "scoop", "config.json"), []byte(`{"cache_path": "E:\\cache", "aria2-enabled": false}`), 0o644)
	if got := cacheDir(); got != `E:\cache` {
		t.Errorf("config = %q", got)
	}
	t.Setenv("SCOOP_CACHE", `F:\c`)
	if got := cacheDir(); got != `F:\c` {
		t.Errorf("SCOOP_CACHE = %q", got)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func mib(n float64) int64 { return int64(n * (1 << 20)) }
func gib(n float64) int64 { return int64(n * (1 << 30)) }

func TestAria2Missing(t *testing.T) {
	root, global, cfg := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("SCOOP", root)
	t.Setenv("SCOOP_GLOBAL", global)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	if !aria2Missing() {
		t.Error("no aria2 anywhere")
	}
	os.MkdirAll(filepath.Join(global, "apps", "aria2", "current"), 0o755)
	if aria2Missing() {
		t.Error("aria2 for all users")
	}
	os.RemoveAll(filepath.Join(global, "apps"))
	os.MkdirAll(filepath.Join(root, "apps", "aria2", "current"), 0o755)
	if aria2Missing() {
		t.Error("aria2 for the user")
	}
	os.RemoveAll(filepath.Join(root, "apps"))
	// Turned off, it is not missing.
	os.MkdirAll(filepath.Join(cfg, "scoop"), 0o755)
	os.WriteFile(filepath.Join(cfg, "scoop", "config.json"), []byte(`{"aria2-enabled": false}`), 0o644)
	if aria2Missing() {
		t.Error("aria2 turned off")
	}
	os.WriteFile(filepath.Join(cfg, "scoop", "config.json"), []byte(`{"aria2-enabled": true}`), 0o644)
	if !aria2Missing() {
		t.Error("aria2 on but not installed")
	}
}
