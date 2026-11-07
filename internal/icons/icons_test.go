package icons

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"appshelf/internal/inventory"
)

var pngData = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR fake png")

func TestIconIDs(t *testing.T) {
	winget := map[string]string{
		"Obsidian.Obsidian":                     "obsidian",
		"Microsoft.VisualStudioCode":            "visualstudiocode",
		"Microsoft.VisualStudio.2022.Community": "visualstudio-2022-community",
		"7zip.7zip":                             "7zip",
		"Python.Python.3.12":                    "python-3-12",
		"NoPublisher":                           "nopublisher",
		"Some.Name_With Spaces,Commas/Slash":    "name-with-spaces-commas-slash",
	}
	for in, want := range winget {
		if got := WingetIconID(in); got != want {
			t.Errorf("WingetIconID(%q) = %q, want %q", in, got, want)
		}
	}
	scoop := map[string]string{"7zip": "7zip", "Firefox.App": "firefox", "vscode-portable": "vscode-portable", "python_3.12": "python-3-12"}
	for in, want := range scoop {
		if got := ScoopIconID(in); got != want {
			t.Errorf("ScoopIconID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIconFile(t *testing.T) {
	os.Setenv("APPSHELF_ICON_TEST", `C:\Apps`)
	cases := map[string]string{
		`"C:\Program Files\App\app.exe",0`:  `C:\Program Files\App\app.exe`,
		`C:\Program Files\App\app.exe,-101`: `C:\Program Files\App\app.exe`,
		`C:\Program Files\App\app.ico`:      `C:\Program Files\App\app.ico`,
		`%APPSHELF_ICON_TEST%\x.exe`:        `C:\Apps\x.exe`,
		`C:\Windows\System32\msiexec.exe,0`: "",
		`relative\app.exe`:                  "",
		``:                                  "",
		`C:\Odd,Name\app.exe`:               `C:\Odd,Name\app.exe`,
		` "C:\Quoted Path\tool.exe" `:       `C:\Quoted Path\tool.exe`,
	}
	for in, want := range cases {
		got := IconFile(in)
		if filepath.Separator != '\\' {
			// filepath.IsAbs knows no drive letters elsewhere.
			if want != "" {
				continue
			}
		}
		if got != want {
			t.Errorf("IconFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImageType(t *testing.T) {
	cases := []struct {
		header string
		data   []byte
		want   string
	}{
		{"image/png", pngData, "image/png"},
		{"image/svg+xml; charset=utf-8", []byte("<svg/>"), "image/svg+xml"},
		{"application/octet-stream", pngData, "image/png"},
		{"text/plain", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), "image/svg+xml"},
		{"text/html", []byte("<html><body>nope</body></html>"), ""},
		{"", []byte("GIF89a......"), "image/gif"},
		{"image/vnd.microsoft.icon", []byte{0, 0, 1, 0}, "image/x-icon"},
	}
	for _, c := range cases {
		if got := imageType(c.header, c.data); got != c.want {
			t.Errorf("imageType(%q, %q) = %q, want %q", c.header, c.data, got, c.want)
		}
	}
}

func TestPublicURL(t *testing.T) {
	ok := []string{"https://i.imgur.com/x.png", "http://example.com/a.svg", "https://8.8.8.8/x.png"}
	bad := []string{"file:///C:/x.png", "ftp://example.com/x.png", "http://localhost/x.png", "http://127.0.0.1/x.png", "http://192.168.1.1/x.png", "http://10.0.0.1/x", "http://[::1]/x", "http://169.254.1.1/x", "http://icon.localhost/w/x"}
	for _, s := range ok {
		u, _ := url.Parse(s)
		if err := publicURL(u); err != nil {
			t.Errorf("%s refused: %v", s, err)
		}
	}
	for _, s := range bad {
		u, _ := url.Parse(s)
		if err := publicURL(u); err == nil {
			t.Errorf("%s allowed", s)
		}
	}
}

// server serves the database and icons, counting the requests by path.
type server struct {
	*httptest.Server
	mu    sync.Mutex
	count map[string]int
	files map[string]func(w http.ResponseWriter, r *http.Request)
}

func newServer(t *testing.T) *server {
	s := &server{count: map[string]int{}, files: map[string]func(http.ResponseWriter, *http.Request){}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.count[r.URL.Path]++
		h := s.files[r.URL.Path]
		s.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) serve(path, ctype string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = func(w http.ResponseWriter, r *http.Request) {
		if ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		w.Write(body)
	}
}

func (s *server) hits(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count[path]
}

func allowLocal(t *testing.T) {
	checkURL = func(*url.URL) error { return nil }
	t.Cleanup(func() { checkURL = publicURL })
}

func newTestService(t *testing.T, srv *server, opt Options) *Service {
	allowLocal(t)
	opt.Dir = t.TempDir()
	opt.DatabaseURL = srv.URL + "/db.json"
	opt.AvatarURL = srv.URL + "/avatars"
	return New(opt)
}

func get(t *testing.T, s *Service, path string) (int, string, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://icon.localhost"+path, nil))
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, rec.Header().Get("Content-Type"), body
}

func TestServeFromDatabase(t *testing.T) {
	srv := newServer(t)
	db := fmt.Sprintf(`{"package_count":{},"icons_and_screenshots":{
		"obsidian":{"icon":"%[1]s/img/obsidian.png","images":[]},
		"Owner/Repo":{"icon":"%[1]s/img/repo.svg","images":[]},
		"ripgrep":{"icon":"%[1]s/img/rg.png","images":[]},
		"broken":{"icon":"%[1]s/img/broken.html","images":[]},
		"empty":{"icon":"","images":[]}}}`, srv.URL)
	srv.serve("/db.json", "text/plain", []byte(db))
	srv.serve("/img/obsidian.png", "image/png", pngData)
	srv.serve("/img/repo.svg", "image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	srv.serve("/img/rg.png", "application/octet-stream", pngData)
	srv.serve("/img/broken.html", "text/html", []byte("<html></html>"))
	srv.serve("/avatars/someone", "image/png", pngData)
	s := newTestService(t, srv, Options{})

	code, ctype, body := get(t, s, "/w/Obsidian.Obsidian")
	if code != 200 || ctype != "image/png" || !bytes.Equal(body, pngData) {
		t.Errorf("winget icon: %d %s", code, ctype)
	}
	// Cached on disk: the second request downloads nothing.
	get(t, s, "/w/obsidian.Obsidian")
	if n := srv.hits("/img/obsidian.png"); n != 1 {
		t.Errorf("icon downloaded %d times", n)
	}
	if n := srv.hits("/db.json"); n != 1 {
		t.Errorf("database downloaded %d times", n)
	}

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://icon.localhost/g/owner/repo", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("svg icon: %d %v", rec.Code, rec.Header())
	}
	if code, _, _ := get(t, s, "/s/ripgrep"); code != 200 {
		t.Errorf("scoop icon by sniffing: %d", code)
	}
	// Not in the database: the owner's avatar.
	if code, _, _ := get(t, s, "/g/someone/thing"); code != 200 || srv.hits("/avatars/someone") != 1 {
		t.Errorf("avatar: %d", code)
	}
	// Not an image: 404, and not immediately asked again.
	for range 2 {
		if code, _, _ := get(t, s, "/s/broken"); code != 404 {
			t.Errorf("html icon served: %d", code)
		}
	}
	if n := srv.hits("/img/broken.html"); n != 1 {
		t.Errorf("failed icon asked %d times", n)
	}
	for _, p := range []string{"/w/Nobody.Nothing", "/s/empty", "/x/y", "/w", "/i/unknown", "/w/a/b"} {
		if code, _, _ := get(t, s, p); code != 404 {
			t.Errorf("%s: %d, want 404", p, code)
		}
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "http://icon.localhost/w/Obsidian.Obsidian", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestFetchLimits(t *testing.T) {
	srv := newServer(t)
	big := bytes.Repeat([]byte("x"), maxIconSize+10)
	srv.serve("/big.png", "image/png", append(append([]byte{}, pngData...), big...))
	allowLocal(t)
	f := newFetcher(&http.Client{Timeout: 5 * time.Second}, t.TempDir())
	if _, _, err := f.get(context.Background(), srv.URL+"/big.png"); err == nil {
		t.Error("an icon over the limit downloaded")
	}
	if _, _, err := f.get(context.Background(), "file:///C:/Windows/win.ini"); err == nil {
		t.Error("a file URL downloaded")
	}
	// The real check refuses local servers.
	checkURL = publicURL
	if _, _, err := f.get(context.Background(), srv.URL+"/other.png"); err == nil {
		t.Error("a local server reached")
	}
}

func TestFetchOnce(t *testing.T) {
	srv := newServer(t)
	release := make(chan struct{})
	var calls atomic.Int32
	srv.mu.Lock()
	srv.files["/slow.png"] = func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-release
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngData)
	}
	srv.mu.Unlock()
	allowLocal(t)
	f := newFetcher(&http.Client{Timeout: 5 * time.Second}, t.TempDir())
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if _, _, err := f.get(context.Background(), srv.URL+"/slow.png"); err != nil {
				t.Error(err)
			}
		})
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("downloaded %d times", n)
	}
}

func TestDatabaseCacheAndETag(t *testing.T) {
	srv := newServer(t)
	var notModified atomic.Int32
	srv.mu.Lock()
	srv.files["/db.json"] = func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(`{"icons_and_screenshots":{"a":{"icon":"https://x.example/a.png"}}}`))
	}
	srv.mu.Unlock()
	allowLocal(t)
	dir := t.TempDir()
	client := &http.Client{Timeout: 5 * time.Second}
	d := newDatabase(client, srv.URL+"/db.json", dir)
	m, err := d.icons(context.Background())
	if err != nil || m["a"] == "" {
		t.Fatalf("icons = %v, %v", m, err)
	}
	// A new run reads the file kept, without downloading.
	d2 := newDatabase(client, srv.URL+"/db.json", dir)
	if m, err := d2.icons(context.Background()); err != nil || m["a"] == "" || srv.hits("/db.json") != 1 {
		t.Fatalf("cached icons = %v, %v, %d downloads", m, err, srv.hits("/db.json"))
	}
	// Once old, it asks again with the ETag and keeps the file.
	d2.mu.Lock()
	d2.loadedAt = time.Now().Add(-8 * 24 * time.Hour)
	d2.mu.Unlock()
	if _, err := d2.icons(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for notModified.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if notModified.Load() != 1 {
		t.Error("no conditional request once the database was old")
	}

	// Unreachable and nothing kept: an error, and no second try at once.
	d3 := newDatabase(client, srv.URL+"/missing.json", t.TempDir())
	if _, err := d3.icons(context.Background()); err == nil {
		t.Error("no error without a database")
	}
	if _, err := d3.icons(context.Background()); err == nil || srv.hits("/missing.json") != 1 {
		t.Errorf("retried at once: %d", srv.hits("/missing.json"))
	}
}

func TestInstalledIcons(t *testing.T) {
	srv := newServer(t)
	srv.serve("/db.json", "text/plain", []byte(`{"icons_and_screenshots":{}}`))
	dir := t.TempDir()
	exe := filepath.Join(dir, "app.exe")
	os.WriteFile(exe, []byte("MZ"), 0o644)
	pngFile := filepath.Join(dir, "logo.png")
	os.WriteFile(pngFile, pngData, 0o644)
	var drawn []string
	s := newTestService(t, srv, Options{
		FileIcon: func(path string, size int) ([]byte, error) {
			drawn = append(drawn, path)
			return []byte("drawn:" + path), nil
		},
		PackagedApps: func() (map[string]string, error) {
			return map[string]string{"Microsoft.WindowsCalculator_8wekyb3d8bbwe": `shell:AppsFolder\Microsoft.WindowsCalculator_8wekyb3d8bbwe!App`}, nil
		},
	})
	s.SetInventory([]inventory.Entry{
		{Key: `arp:machine\x64\app`, Kind: inventory.Local, Icon: `"` + exe + `",0`},
		{Key: "winget:git.git", Kind: inventory.Winget, WingetID: "Git.Git", Icon: pngFile},
		{Key: "msix:calc", Kind: inventory.Store, PackageFamily: "Microsoft.WindowsCalculator_8wekyb3d8bbwe"},
		{Key: "reg:none", Kind: inventory.Local},
	})
	if filepath.Separator == '\\' {
		code, _, body := get(t, s, "/i/"+url.PathEscape(`arp:machine\x64\app`))
		if code != 200 || string(body) != "drawn:"+exe {
			t.Errorf("program icon: %d %q", code, body)
		}
		get(t, s, "/i/"+url.PathEscape(`arp:machine\x64\app`))
		if len(drawn) != 1 {
			t.Errorf("drawn %d times", len(drawn))
		}
		// A winget package not in the database: its program's icon, read
		// as it is for a PNG.
		code, _, body = get(t, s, "/w/Git.Git")
		if code != 200 || !bytes.Equal(body, pngData) {
			t.Errorf("winget fallback: %d", code)
		}
	}
	code, _, body := get(t, s, "/i/msix:calc")
	if code != 200 || !strings.HasPrefix(string(body), `drawn:shell:AppsFolder\Microsoft.WindowsCalculator`) {
		t.Errorf("package icon: %d %q", code, body)
	}
	if code, _, _ := get(t, s, "/i/reg:none"); code != 404 {
		t.Errorf("no icon: %d", code)
	}
}

// TestDatabaseReal downloads UniGetUI's database. It needs the network.
func TestDatabaseReal(t *testing.T) {
	d := newDatabase(&http.Client{Timeout: 30 * time.Second}, DatabaseURL, t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	m, err := d.icons(ctx)
	if err != nil {
		t.Skipf("database not reachable here: %v", err)
	}
	t.Logf("%d icons", len(m))
	for _, key := range []string{WingetIconID("Obsidian.Obsidian"), WingetIconID("Microsoft.VisualStudioCode"), WingetIconID("Git.Git")} {
		if m[key] == "" {
			t.Errorf("no icon for %q", key)
		}
	}
}
