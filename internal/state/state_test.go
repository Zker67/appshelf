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
