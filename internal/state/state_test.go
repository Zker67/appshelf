package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GitHub("a_1"); ok {
		t.Fatal("an empty state has a record")
	}
	rec := GitHubRecord{Tag: "v1.2.0", AssetName: "app-1.2.0-x64.msi", Digest: "sha256:ab", InstalledAt: time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)}
	if err := s.PutGitHub("a_1", rec); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := again.GitHub("a_1"); !ok || got != rec {
		t.Errorf("reopened: %+v %v", got, ok)
	}
	if err := again.Forget("a_1"); err != nil {
		t.Fatal(err)
	}
	third, _ := Open(path)
	if _, ok := third.GitHub("a_1"); ok {
		t.Error("a forgotten record came back")
	}
}

func TestOpenRejectsBadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte("{"), 0o644)
	if _, err := Open(path); err == nil {
		t.Error("a broken file opened")
	}
	os.WriteFile(path, []byte(`{"schemaVersion":9}`), 0o644)
	if _, err := Open(path); err == nil {
		t.Error("a newer file opened")
	}
}

func TestUpdatePolicies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	if err := s.SetPolicy(" Winget:Git.Git ", func(p *UpdatePolicy) { p.Mode, p.Name = ModeAuto, "Git" }); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPolicy("scoop:main/7zip", func(p *UpdatePolicy) { p.Skip, p.Until = "24.08", until }); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := again.Policy("winget:git.git"); p.Mode != ModeAuto || p.Name != "Git" {
		t.Errorf("auto policy: %+v", p)
	}
	p := again.Policy("SCOOP:main/7zip")
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	if h := p.Hold("24.08", now); h != HoldSkip {
		t.Errorf("skipped version: %q", h)
	}
	if h := p.Hold("24.09", now); h != HoldSnooze {
		t.Errorf("newer version while snoozed: %q", h)
	}
	if h := p.Hold("24.09", until.Add(time.Hour)); h != HoldNone {
		t.Errorf("newer version after the snooze: %q", h)
	}
	if h := (UpdatePolicy{Mode: ModeIgnore}).Hold("1", now); h != HoldIgnore {
		t.Errorf("ignored: %q", h)
	}
	// Back to the default, the policy is removed.
	if err := again.SetPolicy("scoop:main/7zip", func(p *UpdatePolicy) { *p = UpdatePolicy{} }); err != nil {
		t.Fatal(err)
	}
	if len(again.Policies()) != 1 {
		t.Errorf("policies: %+v", again.Policies())
	}
	if err := again.SetPolicy("  ", func(p *UpdatePolicy) { p.Mode = ModeIgnore }); err == nil {
		t.Error("a policy without a key was kept")
	}
	// Forgetting an app's GitHub record keeps its package's policy.
	again.Forget("a_1")
	if again.Policy("winget:git.git").Mode != ModeAuto {
		t.Error("the policy went with Forget")
	}
}

func TestHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := Open(path)
	for i := range MaxHistory + 5 {
		if err := s.AddHistory(UpdateRecord{Key: "winget:Git.Git", Name: "Git", To: string(rune('a' + i%26)), Result: "succeeded", At: time.Unix(int64(i), 0)}); err != nil {
			t.Fatal(err)
		}
	}
	again, _ := Open(path)
	h := again.History()
	if len(h) != MaxHistory || h[0].At.Unix() != MaxHistory+4 || h[0].Key != "winget:git.git" {
		t.Fatalf("history: %d, newest %v %q", len(h), h[0].At, h[0].Key)
	}
	if err := again.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	third, _ := Open(path)
	if len(third.History()) != 0 {
		t.Error("cleared history came back")
	}
}

func TestOldFileOpens(t *testing.T) {
	// A file written before the update policies opens, and keeps its records.
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte(`{"schemaVersion":1,"github":{"a_1":{"tag":"v1","assetName":"a.msi","installedAt":"2026-11-07T00:00:00Z"}}}`), 0o644)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GitHub("a_1"); !ok {
		t.Error("lost the GitHub record")
	}
	if err := s.SetPolicy("github:cli/cli", func(p *UpdatePolicy) { p.Mode = ModeIgnore }); err != nil {
		t.Fatal(err)
	}
	again, _ := Open(path)
	if _, ok := again.GitHub("a_1"); !ok || again.Policy("github:cli/cli").Mode != ModeIgnore {
		t.Error("saving a policy lost something")
	}
}
