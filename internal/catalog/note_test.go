package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeNote(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"  \n\t\n ":        "",
		"# 标题\r\n正文\r\n":   "# 标题\n正文",
		"\n\n  \n    代码\n": "    代码",
		"第一行  \n第二行\r旧换行\n\n\n":     "第一行  \n第二行\n旧换行",
		"- [ ] 待办\n- [x] 完成   \n\n": "- [ ] 待办\n- [x] 完成",
	}
	for in, want := range cases {
		if got := NormalizeNote(in); got != want {
			t.Errorf("NormalizeNote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetNote(t *testing.T) {
	c := sample(t)
	id := c.Apps[0].ID
	if err := c.SetNote(id, "\r\n**常用**，记得登录\r\n"); err != nil {
		t.Fatal(err)
	}
	if a, _ := c.App(id); a.Note != "**常用**，记得登录" {
		t.Errorf("note = %q", a.Note)
	}
	if err := c.SetNote(id, "   "); err != nil {
		t.Fatal(err)
	}
	if a, _ := c.App(id); a.Note != "" {
		t.Errorf("blank note kept: %q", a.Note)
	}
	if err := c.SetNote("a_missing", "x"); err == nil {
		t.Error("a note saved for a missing app")
	}

	long := strings.Repeat("字", MaxNoteLen+1)
	err := c.SetNote(id, long)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "note" {
		t.Errorf("too long note: %v", err)
	}
	if a, _ := c.App(id); a.Note != "" {
		t.Error("a failed edit changed the catalog")
	}
	if err := c.SetNote(id, strings.Repeat("字", MaxNoteLen)); err != nil {
		t.Errorf("note of the longest length: %v", err)
	}
	if err := c.SetNote(id, "a\x00b"); err == nil {
		t.Error("a note with NUL saved")
	}
}

func TestNoteRoundTripAndMerge(t *testing.T) {
	base := sample(t)
	vscode, zip := base.Apps[0].ID, base.Apps[1].ID
	if err := base.SetNote(vscode, "本机的备注"); err != nil {
		t.Fatal(err)
	}

	in := sample(t)
	if err := in.SetNote(in.Apps[0].ID, "导入的备注"); err != nil {
		t.Fatal(err)
	}
	if err := in.SetNote(in.Apps[1].ID, "## 7-Zip\n- 右键菜单"); err != nil {
		t.Fatal(err)
	}
	if _, err := in.PutApp(App{Name: "Git", Source: Source{Type: Winget, ID: "Git.Git"}, Note: "新软件的备注"}); err != nil {
		t.Fatal(err)
	}
	data, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"note": "新软件的备注"`) {
		t.Errorf("exported file has no note:\n%s", data)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	out, _, err := Merge(base, parsed)
	if err != nil {
		t.Fatal(err)
	}
	notes := map[string]string{}
	for _, a := range out.Apps {
		notes[a.Name] = a.Note
	}
	if a, _ := out.App(vscode); a.Note != "本机的备注" {
		t.Errorf("own note replaced: %q", a.Note)
	}
	if a, _ := out.App(zip); a.Note != "## 7-Zip\n- 右键菜单" {
		t.Errorf("empty note not filled: %q", a.Note)
	}
	if notes["Git"] != "新软件的备注" {
		t.Errorf("added app's note = %q", notes["Git"])
	}

	// An app without a note writes no note field.
	plain, _ := New().Marshal()
	c := sample(t)
	data, _ = c.Marshal()
	if strings.Contains(string(data), `"note"`) || strings.Contains(string(plain), `"note"`) {
		t.Errorf("empty note written:\n%s", data)
	}
}
