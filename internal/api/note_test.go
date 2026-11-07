package api

import (
	"slices"
	"testing"

	"appshelf/internal/catalog"
)

func TestSetNote(t *testing.T) {
	e := newEnv(t)
	saved, err := e.shelf.SaveApp(catalog.App{Name: "Git", Source: catalog.Source{Type: catalog.Winget, ID: "Git.Git"}})
	if err != nil {
		t.Fatal(err)
	}
	n := e.changeCount()
	note, err := e.shelf.SetNote(saved.ID, "## 安装后\r\n- [ ] 设置用户名\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if note != "## 安装后\n- [ ] 设置用户名" {
		t.Errorf("saved note = %q", note)
	}
	changes := e.changesSince(n)
	if len(changes) == 0 || !slices.Contains(changes[0].Kinds, ChangedShelf) {
		t.Errorf("changes after the note = %+v", changes)
	}
	if got := e.shelf.Snapshot().Apps[0].App.Note; got != note {
		t.Errorf("snapshot note = %q", got)
	}

	// Saving the app from the editor keeps the note.
	saved.Name = "Git for Windows"
	if _, err := e.shelf.SaveApp(saved); err != nil {
		t.Fatal(err)
	}
	if got := e.shelf.Snapshot().Apps[0].App; got.Note != note || got.Name != "Git for Windows" {
		t.Errorf("after editing: %+v", got)
	}

	if _, err := e.shelf.SetNote("a_missing", "x"); err == nil {
		t.Error("a note saved for a missing app")
	}
	if note, err := e.shelf.SetNote(saved.ID, "  "); err != nil || note != "" || e.shelf.Snapshot().Apps[0].App.Note != "" {
		t.Errorf("clearing the note: %q, %v", note, err)
	}
}
