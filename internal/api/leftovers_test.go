//go:build windows

package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appshelf/internal/source/winget"
)

// TestLeftoversAfterUninstall removes a made-up program, finds the
// directory it "left" in %LOCALAPPDATA% and cleans it to the Recycle Bin.
func TestLeftoversAfterUninstall(t *testing.T) {
	e := newEnv(t)
	b := make([]byte, 5)
	rand.Read(b)
	name := "AppShelfTestLeftover" + hex.EncodeToString(b)
	id := "AppShelfTest." + name
	left := filepath.Join(os.Getenv("LOCALAPPDATA"), name)
	if err := os.MkdirAll(left, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(left) })
	os.WriteFile(filepath.Join(left, "settings.json"), []byte("{}"), 0o644)

	e.wapi.mu.Lock()
	e.wapi.pkgs = append(e.wapi.pkgs, winget.Package{ID: id, Name: name, Version: "1.0", Source: "winget"})
	e.wapi.mu.Unlock()
	e.installed.Refresh()
	key := "winget:" + strings.ToLower(id)
	e.until(func() bool {
		for _, en := range e.installed.List().Entries {
			if en.Key == key {
				return true
			}
		}
		return false
	})

	// Before the removal succeeded there is nothing to look for.
	if _, err := e.tasks.FindLeftovers(context.Background(), 999); err == nil {
		t.Error("an unknown task was searched")
	}
	if _, err := e.installed.StartUninstall(context.Background(), []string{key}); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool {
		l := e.tasks.List()
		return len(l.Items) == 1 && l.Items[0].State == TaskSucceeded && !l.Running
	})
	task := e.tasks.List().Items[0]
	if !task.Leftovers {
		t.Fatalf("task = %+v", task)
	}
	scan, err := e.tasks.FindLeftovers(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var found *LeftoverItem
	for i, it := range scan.Items {
		if strings.EqualFold(it.Path, left) {
			found = &scan.Items[i]
		}
	}
	if found == nil || found.Kind != LeftoverDir || found.Score != 2 || found.Level != LeftoverMedium || found.Checked {
		t.Fatalf("scan = %+v", scan)
	}
	if _, err := e.tasks.CleanLeftovers(context.Background(), scan.Token, nil); err == nil {
		t.Error("cleaning nothing must fail")
	}
	if _, err := e.tasks.CleanLeftovers(context.Background(), "expired", []string{found.ID}); err == nil {
		t.Error("an unknown token was accepted")
	}
	res, err := e.tasks.CleanLeftovers(context.Background(), scan.Token, []string{found.ID, "not-a-find"})
	if err != nil || res.Cleaned != 1 || res.Failed != 0 || len(res.Outcomes) != 1 || res.BackupDir != "" {
		t.Fatalf("clean = %+v, %v", res, err)
	}
	if _, err := os.Stat(left); err == nil {
		t.Fatal("the directory is still there")
	}
	if err := e.tasks.OpenBackup(scan.Token); err == nil {
		t.Error("no registry was saved: there is no backup to open")
	}
}
