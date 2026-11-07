package icons

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"appshelf/internal/inventory"
)

func TestInstalledIconDoesNotWaitForNetwork(t *testing.T) {
	srv := newServer(t)
	file := filepath.Join(t.TempDir(), "app.png")
	if err := os.WriteFile(file, pngData, 0o600); err != nil {
		t.Fatal(err)
	}
	s := newTestService(t, srv, Options{})
	s.SetInventory([]inventory.Entry{{Key: "app", Kind: inventory.Winget, WingetID: "Vendor.App", Icon: file}})
	for _, segments := range [][]string{{"i", "app"}, {"w", "Vendor.App"}} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // No network request can succeed, but local icons still should.
		data, _, err := s.Icon(ctx, segments)
		if err != nil || !bytes.Equal(data, pngData) {
			t.Fatalf("local icon: %v", err)
		}
	}
	if srv.hits("/db.json") != 0 {
		t.Fatal("local icon depended on network database")
	}
}

func TestLocalIconCoalescesAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.exe")
	if err := os.WriteFile(file, []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	start := make(chan struct{})
	s := New(Options{Dir: dir, FileIcon: func(string, int) ([]byte, error) {
		calls.Add(1)
		<-start
		return pngData, nil
	}})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, _, err := s.fromFile(file); err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("shell called %d times", calls.Load())
	}
	restarted := New(Options{Dir: dir, FileIcon: func(string, int) ([]byte, error) {
		t.Error("persistent icon not reused")
		return nil, ErrNoIcon
	}})
	if _, _, err := restarted.fromFile(file); err != nil {
		t.Fatal(err)
	}
	// File changes invalidate memory and disk caches.
	if err := os.WriteFile(file, []byte("MZchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.fromFile(file); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("changed file reused old icon")
	}
}

func TestPackageEnumerationSharedAndCancelable(t *testing.T) {
	var scans, draws atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	s := New(Options{Dir: t.TempDir(), PackagedApps: func() (map[string]string, error) {
		scans.Add(1)
		close(started)
		<-release
		return map[string]string{"Family": "shell:AppsFolder\\Family!App"}, nil
	}, FileIcon: func(string, int) ([]byte, error) { draws.Add(1); return pngData, nil }})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, _, err := s.packaged(context.Background(), "family"); err != nil {
				t.Error(err)
			}
		})
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.packaged(ctx, "family"); err != context.Canceled {
		t.Errorf("canceled wait: %v", err)
	}
	close(release)
	wg.Wait()
	s.SetInventory(nil)
	if _, _, err := s.packaged(context.Background(), "family"); err != nil {
		t.Fatal(err)
	}
	if scans.Load() != 1 || draws.Load() != 1 {
		t.Fatalf("scans=%d draws=%d", scans.Load(), draws.Load())
	}
}

func TestInstalledDirectoryAndPackageLogoFallbacks(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "icon.png")
	if err := os.WriteFile(file, pngData, 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Dir: t.TempDir()})
	if _, _, err := s.installedFile(inventory.Entry{Icon: filepath.Join(dir, "missing.exe"), Location: dir}); err != nil {
		t.Fatal(err)
	}
	logo := filepath.Join(dir, "StoreLogo.scale-200.png")
	if err := os.WriteFile(logo, pngData, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := packageLogo(dir, "StoreLogo.png"); got != logo {
		t.Fatalf("qualified asset: %q", got)
	}
	for _, path := range []string{"../escape.png", file, "ms-resource:Logo"} {
		if got := packageLogo(dir, path); got != "" {
			t.Fatalf("unsafe logo %q: %q", path, got)
		}
	}
}

func TestTransientDownloadFailureExpires(t *testing.T) {
	srv := newServer(t)
	allowLocal(t)
	f := newFetcher(srv.Client(), t.TempDir())
	u := srv.URL + "/recover.png"
	if _, _, err := f.get(context.Background(), u); err == nil {
		t.Fatal("expected initial failure")
	}
	srv.serve("/recover.png", "image/png", pngData)
	old := time.Now().Add(-failTTL - time.Second)
	if err := os.Chtimes(f.base(u)+".fail", old, old); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.get(context.Background(), u); err != nil {
		t.Fatalf("did not recover: %v", err)
	}
	if srv.hits("/recover.png") != 2 {
		t.Fatal("unexpected retry count")
	}
}

func TestQueuedDownloadHasDeadline(t *testing.T) {
	srv := newServer(t)
	allowLocal(t)
	f := newFetcher(srv.Client(), t.TempDir())
	for range maxDownloads {
		f.sem <- struct{}{}
	}
	_, _, err := f.download(srv.URL + "/queued.png")
	if err != context.DeadlineExceeded {
		t.Fatalf("queue did not expire: %v", err)
	}
	if srv.hits("/queued.png") != 0 {
		t.Fatal("expired request reached network")
	}
}

func TestLocalExtractionFailureCanRecover(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.exe")
	if err := os.WriteFile(file, []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	s := New(Options{Dir: t.TempDir(), FileIcon: func(string, int) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, ErrNoIcon
		}
		return pngData, nil
	}})
	if _, _, err := s.fromFile(file); err == nil {
		t.Fatal("expected initial failure")
	}
	for key, img := range s.local {
		img.at = time.Now().Add(-31 * time.Second)
		s.local[key] = img
	}
	if _, _, err := s.fromFile(file); err != nil {
		t.Fatalf("cached failure never recovered: %v", err)
	}
}

func TestPackageLogosReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	start := time.Now()
	logos, err := PackageLogos(context.Background())
	if err != nil {
		t.Skipf("package metadata unavailable: %v", err)
	}
	t.Logf("%d package logos in %s", len(logos), time.Since(start))
	for _, path := range logos {
		if fi, err := os.Stat(path); err != nil || fi.IsDir() {
			t.Fatalf("invalid logo: %q", path)
		}
	}
}
