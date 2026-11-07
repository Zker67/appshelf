package core

import (
	"testing"

	"appshelf/internal/catalog"
)

func TestNotes(t *testing.T) {
	h := newHarness(t, newFake(catalog.Winget))
	c := h.c
	a := wingetApp("A", "A.A")
	a.Note = "新建时的备注"
	saved, err := c.SaveApp(a)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Note != "新建时的备注" {
		t.Errorf("new app's note = %q", saved.Note)
	}

	if err := c.SetNote(saved.ID, "- [ ] 配置代理\n"); err != nil {
		t.Fatal(err)
	}
	// Editing the app does not change its note, even when the editor sends
	// none or an old one.
	edit := saved
	edit.Name, edit.Note = "A 改名", ""
	if saved, err = c.SaveApp(edit); err != nil {
		t.Fatal(err)
	}
	if saved.Note != "- [ ] 配置代理" || saved.Name != "A 改名" {
		t.Errorf("after editing: %+v", saved)
	}

	// The note is in the catalog's file.
	file, err := catalog.Load(h.dir + "/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := file.App(saved.ID); got.Note != "- [ ] 配置代理" {
		t.Errorf("saved note = %q", got.Note)
	}

	if err := c.SetNote("a_missing", "x"); err == nil {
		t.Error("a note saved for a missing app")
	}
	if err := c.SetNote(saved.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Catalog().App(saved.ID); got.Note != "" {
		t.Errorf("note not removed: %q", got.Note)
	}
}
