package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
)

func sourceInfo(e *env, t catalog.SourceType) SourceInfo {
	for _, s := range e.sources.Overview().Sources {
		if s.Type == t {
			return s
		}
	}
	e.t.Fatalf("no source %s", t)
	return SourceInfo{}
}

func TestSourceSetup(t *testing.T) {
	e := newEnv(t)
	if e.sources.Overview().SetupSeen {
		t.Fatal("the setup prompt is answered before it is shown")
	}

	e.winget.mu.Lock()
	e.winget.avail = source.Availability{Reason: "缺少 PowerShell 模块", Fixable: true}
	e.winget.fixErr = errors.New("网络不通")
	e.winget.mu.Unlock()
	e.shelf.Refresh()
	e.until(func() bool { return !sourceInfo(e, catalog.Winget).OK && !e.shelf.Snapshot().Refreshing })
	if info := sourceInfo(e, catalog.Winget); !info.Fixable {
		t.Fatalf("missing module = %+v, want fixable", info)
	}

	// A failed setup says why and leaves the source unavailable.
	if err := e.sources.Fix(catalog.Winget); err == nil || !strings.Contains(err.Error(), "网络不通") {
		t.Fatalf("failed fix = %v", err)
	}
	if info := sourceInfo(e, catalog.Winget); info.OK || !info.Fixable {
		t.Fatalf("after a failed fix = %+v", info)
	}

	e.winget.mu.Lock()
	e.winget.fixErr = nil
	e.winget.mu.Unlock()
	if err := e.sources.Fix(catalog.Winget); err != nil {
		t.Fatal(err)
	}
	if info := sourceInfo(e, catalog.Winget); !info.OK || info.Fixable {
		t.Fatalf("after the fix = %+v, want available", info)
	}
	if e.winget.fixes != 2 {
		t.Fatalf("fixes = %d", e.winget.fixes)
	}
	logs, _ := os.ReadDir(filepath.Join(e.dir, "logs"))
	if len(logs) == 0 || !strings.Contains(logs[0].Name(), "setup-winget") {
		t.Fatalf("logs = %v", logs)
	}

	// An available source is not offered to be fixed, even if it says so.
	e.winget.mu.Lock()
	e.winget.avail = source.Availability{OK: true, Fixable: true}
	e.winget.mu.Unlock()
	e.shelf.Refresh()
	e.until(func() bool { return !e.shelf.Snapshot().Refreshing })
	if info := sourceInfo(e, catalog.Winget); info.Fixable {
		t.Fatalf("available source = %+v, want not fixable", info)
	}

	// A source the core does not have cannot be fixed.
	if err := e.sources.Fix(catalog.Scoop); err == nil {
		t.Fatal("fixing a source that is not set up succeeded")
	}

	if err := e.sources.DismissSetup(); err != nil {
		t.Fatal(err)
	}
	if !e.sources.Overview().SetupSeen {
		t.Fatal("the dismissed prompt is not kept")
	}
	if !loadSettings(filepath.Join(e.dir, "settings.json")).SourceSetupSeen {
		t.Fatal("settings.json does not keep the dismissed prompt")
	}
}

func TestGitHubToken(t *testing.T) {
	e := newEnv(t)
	file := filepath.Join(e.dir, "github-token.bin")
	if from := e.sources.Overview().GitHubTokenFrom; from != TokenEnv {
		t.Fatalf("from = %q, want the environment's", from)
	}

	// What is not a token is not sent to GitHub; one GitHub refuses is
	// not kept.
	for _, bad := range []string{"", "  ", "ghp_good x", "ghp_good\nX-Evil: 1", strings.Repeat("a", 300)} {
		if err := e.sources.SetGitHubToken(context.Background(), bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if err := e.sources.SetGitHubToken(context.Background(), "ghp_refused"); !errors.Is(err, github.ErrBadToken) {
		t.Fatalf("refused token = %v", err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("a refused token was saved")
	}
	if e.gh.carried() != "" || e.sources.Overview().GitHubTokenFrom != TokenEnv {
		t.Fatal("a refused token changed the token in use")
	}

	n := e.changeCount()
	if err := e.sources.SetGitHubToken(context.Background(), "  ghp_good123  "); err != nil {
		t.Fatal(err)
	}
	if got := e.gh.carried(); got != "ghp_good123" {
		t.Fatalf("carried = %q", got)
	}
	if from := e.sources.Overview().GitHubTokenFrom; from != TokenSaved {
		t.Fatalf("from = %q, want saved", from)
	}
	if data, err := os.ReadFile(file); err != nil || strings.Contains(string(data), "ghp_good123") {
		t.Fatalf("saved file: %v, in clear: %v", err, strings.Contains(string(data), "ghp_good123"))
	}
	if !slices.ContainsFunc(e.changesSince(n), func(c Change) bool { return slices.Contains(c.Kinds, ChangedSources) }) {
		t.Error("the pages are not told the sources changed")
	}
	e.until(func() bool { return !e.shelf.Snapshot().Refreshing })

	// A new start uses the saved token, before the environment's.
	again := New(Hooks{})
	again.cfg = Config{TokenFile: file, EnvToken: "ghp_fromenv", Sources: SourceAPIs{GitHub: &fakeGitHub{}}}
	again.loadToken()
	if again.tokenFrom != TokenSaved || again.cfg.Sources.GitHub.(*fakeGitHub).carried() != "ghp_good123" {
		t.Fatalf("restart: from %q", again.tokenFrom)
	}

	if err := e.sources.ClearGitHubToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("the saved token was not deleted")
	}
	if e.gh.carried() != "ghp_fromenv" || e.sources.Overview().GitHubTokenFrom != TokenEnv {
		t.Fatalf("after clearing: carried %q", e.gh.carried())
	}
}
