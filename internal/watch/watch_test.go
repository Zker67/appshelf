//go:build windows

package watch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// TestWatchRegistryAndDirectory makes a key of its own under
// HKCU\Software and a temporary directory, changes them, and removes the
// key.
func TestWatchRegistryAndDirectory(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	b := make([]byte, 6)
	rand.Read(b)
	path := `Software\AppShelfTest-` + hex.EncodeToString(b)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	k.Close()
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, path+`\Sub`)
		registry.DeleteKey(registry.CURRENT_USER, path)
	})
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan struct{}, 16)
	n, err := Watch(ctx, []Key{
		{Path: path, Subtree: true, Values: true},
		{Path: `Software\AppShelfTest-missing-` + hex.EncodeToString(b)},
	}, []string{dir, filepath.Join(dir, "missing")}, func() { events <- struct{}{} })
	if err != nil || n != 2 {
		t.Fatalf("watching %d, %v; want the key and the directory that exist", n, err)
	}
	expect := func(what string) {
		t.Helper()
		select {
		case <-events:
		case <-time.After(5 * time.Second):
			t.Fatalf("no change seen after %s", what)
		}
		// Changes come in bursts: let the burst end.
		time.Sleep(100 * time.Millisecond)
		for len(events) > 0 {
			<-events
		}
	}

	sub, _, err := registry.CreateKey(registry.CURRENT_USER, path+`\Sub`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	expect("adding a subkey")
	sub.SetStringValue("DisplayVersion", "2")
	sub.Close()
	expect("changing a value of a subkey")

	if err := os.Mkdir(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	expect("adding a directory")

	cancel()
	time.Sleep(100 * time.Millisecond)
	for len(events) > 0 {
		<-events
	}
	os.Mkdir(filepath.Join(dir, "after"), 0o755)
	select {
	case <-events:
		t.Fatal("a change seen after the watch ended")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchNothing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	if _, err := Watch(context.Background(), []Key{{Path: `Software\AppShelfTest-none`}}, nil, func() {}); err == nil {
		t.Fatal("watching nothing must fail")
	}
}

func TestInstalledKeysExist(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, err := Watch(ctx, InstalledKeys(), ScoopDirs(), func() {})
	if err != nil || n < 3 {
		t.Fatalf("watching %d of the installed keys, %v", n, err)
	}
	t.Logf("watching %d keys and directories", n)
}
