package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func sample(t *testing.T) *Catalog {
	t.Helper()
	c := New()
	dev, err := c.AddCategory("开发工具")
	if err != nil {
		t.Fatal(err)
	}
	ai, err := c.AddTag("AI")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutApp(App{Name: "Visual Studio Code", CategoryID: dev.ID, TagIDs: []string{ai.ID}, Source: Source{Type: Winget, ID: "Microsoft.VisualStudioCode"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutApp(App{Name: "7-Zip", Source: Source{Type: Scoop, Bucket: "main", Name: "7zip"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PutApp(App{Name: "GitHub CLI", TagIDs: []string{ai.ID}, Source: Source{Type: GitHub, Repo: "cli/cli"}, Detect: &Detect{DisplayName: "^GitHub CLI"}}); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestValidateApp(t *testing.T) {
	cases := []struct {
		name  string
		app   App
		field string
	}{
		{"empty name", App{Name: " ", Source: Source{Type: Winget, ID: "A.B"}}, "name"},
		{"winget id", App{Name: "x", Source: Source{Type: Winget, ID: "A B"}}, "source.id"},
		{"winget extra field", App{Name: "x", Source: Source{Type: Winget, ID: "A.B", Repo: "a/b"}}, "source"},
		{"scoop bucket", App{Name: "x", Source: Source{Type: Scoop, Bucket: "", Name: "git"}}, "source.bucket"},
		{"scoop name", App{Name: "x", Source: Source{Type: Scoop, Bucket: "main", Name: "a;b"}}, "source.name"},
		{"scoop url", App{Name: "x", Source: Source{Type: Scoop, Bucket: "mine", Name: "a", BucketURL: "http://x"}}, "source.bucketUrl"},
		{"github repo", App{Name: "x", Source: Source{Type: GitHub, Repo: "nope"}, Detect: &Detect{DisplayName: "x"}}, "source.repo"},
		{"github detect required", App{Name: "x", Source: Source{Type: GitHub, Repo: "a/b"}}, "detect"},
		{"github pattern", App{Name: "x", Source: Source{Type: GitHub, Repo: "a/b", AssetPattern: "("}, Detect: &Detect{DisplayName: "x"}}, "source.assetPattern"},
		{"github args newline", App{Name: "x", Source: Source{Type: GitHub, Repo: "a/b", InstallerArgs: "/S\n/D"}, Detect: &Detect{DisplayName: "x"}}, "source.installerArgs"},
		{"detect regex", App{Name: "x", Source: Source{Type: Winget, ID: "A.B"}, Detect: &Detect{DisplayName: "["}}, "detect.displayName"},
		{"detect empty", App{Name: "x", Source: Source{Type: Winget, ID: "A.B"}, Detect: &Detect{}}, "detect"},
		{"unknown type", App{Name: "x", Source: Source{Type: "choco"}}, "source.type"},
	}
	for _, tc := range cases {
		err := ValidateApp(tc.app)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: got %v, want a ValidationError", tc.name, err)
			continue
		}
		if ve.Field != tc.field {
			t.Errorf("%s: field %q, want %q (%v)", tc.name, ve.Field, tc.field, err)
		}
	}
	ok := []App{
		{Name: "Notepad++", Source: Source{Type: Winget, ID: "Notepad++.Notepad++"}},
		{Name: "x", Source: Source{Type: Scoop, Bucket: "extras", Name: "vscode"}},
		{Name: "x", Source: Source{Type: GitHub, Repo: "cli/cli", AssetPattern: `windows_amd64\.msi$`, InstallerArgs: "/quiet"}, Detect: &Detect{ExePath: `%ProgramFiles%\GitHub CLI\gh.exe`}},
	}
	for _, a := range ok {
		if err := ValidateApp(a); err != nil {
			t.Errorf("%+v: %v", a, err)
		}
	}
}

func TestValidateCatalog(t *testing.T) {
	c := sample(t)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}

	dup := c.Clone()
	dup.Apps = append(dup.Apps, App{ID: "a_other", Name: "Code again", Source: Source{Type: Winget, ID: "microsoft.visualstudiocode"}})
	if err := dup.Validate(); err == nil || !strings.Contains(err.Error(), "来源相同") {
		t.Errorf("same source in another case: %v", err)
	}

	badRef := c.Clone()
	badRef.Apps[0].CategoryID = "c_missing"
	if err := badRef.Validate(); err == nil {
		t.Error("an app in a missing category passed")
	}

	badTag := c.Clone()
	badTag.Apps[0].TagIDs = append(badTag.Apps[0].TagIDs, "t_missing")
	if err := badTag.Validate(); err == nil {
		t.Error("an app with a missing tag passed")
	}

	dupName := c.Clone()
	dupName.Categories = append(dupName.Categories, Category{ID: "c_x", Name: "开发工具"})
	if err := dupName.Validate(); err == nil {
		t.Error("two categories with one name passed")
	}

	dupID := c.Clone()
	dupID.Apps[1].ID = dupID.Apps[0].ID
	if err := dupID.Validate(); err == nil {
		t.Error("two apps with one ID passed")
	}
}

func TestPutAppNormalizes(t *testing.T) {
	c := New()
	a, err := c.PutApp(App{Name: "  Git  ", Source: Source{Type: Winget, ID: " Git.Git ", Repo: "ignored/by-type"}, Detect: &Detect{}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.Name != "Git" || a.Source.ID != "Git.Git" || a.Source.Repo != "" || a.Detect != nil {
		t.Errorf("stored %+v", a)
	}
	a.Name = "Git for Windows"
	if _, err := c.PutApp(a); err != nil {
		t.Fatal(err)
	}
	if len(c.Apps) != 1 || c.Apps[0].Name != "Git for Windows" {
		t.Errorf("replace: %+v", c.Apps)
	}
	if _, err := c.PutApp(App{Name: "Git again", Source: Source{Type: Winget, ID: "git.git"}}); err == nil {
		t.Error("a second app with the same source was added")
	}
	if len(c.Apps) != 1 {
		t.Errorf("a failed put changed the catalog: %+v", c.Apps)
	}
}

func TestDeleteCategoryAndTag(t *testing.T) {
	c := sample(t)
	cat := c.Categories[0]
	tag := c.Tags[0]
	if n := c.CategoryUsage(cat.ID); n != 1 {
		t.Errorf("category usage %d", n)
	}
	if n := c.TagUsage(tag.ID); n != 2 {
		t.Errorf("tag usage %d", n)
	}
	c.DeleteCategory(cat.ID)
	c.DeleteTag(tag.ID)
	if len(c.Categories) != 0 || len(c.Tags) != 0 {
		t.Errorf("left %+v %+v", c.Categories, c.Tags)
	}
	for _, a := range c.Apps {
		if a.CategoryID != "" || len(a.TagIDs) != 0 {
			t.Errorf("app keeps a deleted category or tag: %+v", a)
		}
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	empty, err := Load(path)
	if err != nil || len(empty.Apps) != 0 {
		t.Fatalf("missing file: %+v %v", empty, err)
	}
	c := sample(t)
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.Marshal()
	b, _ := back.Marshal()
	if string(a) != string(b) {
		t.Errorf("round trip changed the catalog:\n%s\n%s", a, b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left: %v", entries)
	}
}

func TestParse(t *testing.T) {
	c := sample(t)
	data, _ := c.Marshal()
	if _, err := Parse(append([]byte("\xef\xbb\xbf"), data...)); err != nil {
		t.Errorf("with a byte order mark: %v", err)
	}
	for name, text := range map[string]string{
		"not json":   "{",
		"no version": `{"apps":[]}`,
		"too new":    `{"schemaVersion":99,"apps":[]}`,
		"bad app":    `{"schemaVersion":1,"apps":[{"id":"a_1","name":"x","source":{"type":"winget","id":"bad id"}}]}`,
	} {
		if _, err := Parse([]byte(text)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestMerge(t *testing.T) {
	base := sample(t)
	in := New()
	inCat, _ := in.AddCategory("开发工具") // same name, other ID
	inDesign, _ := in.AddCategory("设计")
	inAI, _ := in.AddTag("ai") // same name ignoring case
	inNew, _ := in.AddTag("常用")
	// The same source as base's VS Code, in another case, with another tag.
	if _, err := in.PutApp(App{Name: "Code", CategoryID: inDesign.ID, TagIDs: []string{inNew.ID}, Source: Source{Type: Winget, ID: "microsoft.visualstudiocode"}}); err != nil {
		t.Fatal(err)
	}
	// 7-Zip is uncategorized in base: it takes the imported category.
	if _, err := in.PutApp(App{Name: "7z", CategoryID: inCat.ID, Source: Source{Type: Scoop, Bucket: "main", Name: "7zip"}}); err != nil {
		t.Fatal(err)
	}
	figma, err := in.PutApp(App{Name: "Figma", CategoryID: inDesign.ID, TagIDs: []string{inAI.ID, inNew.ID}, Source: Source{Type: Winget, ID: "Figma.Figma"}})
	if err != nil {
		t.Fatal(err)
	}
	// An imported app whose ID base already uses gets a new one.
	in.Apps[len(in.Apps)-1].ID = base.Apps[0].ID

	before, _ := base.Marshal()
	out, sum, err := Merge(base, in)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := base.Marshal()
	if string(before) != string(after) {
		t.Error("merge changed base")
	}
	want := MergeSummary{AddedApps: 1, MergedApps: 2, AddedCategories: 1, AddedTags: 1}
	if sum != want {
		t.Errorf("summary %+v, want %+v", sum, want)
	}
	if len(out.Categories) != 2 || len(out.Tags) != 2 || len(out.Apps) != 4 {
		t.Fatalf("out %d categories, %d tags, %d apps", len(out.Categories), len(out.Tags), len(out.Apps))
	}
	code := out.Apps[0]
	if code.Name != "Visual Studio Code" || code.CategoryID != base.Categories[0].ID || len(code.TagIDs) != 2 {
		t.Errorf("VS Code after merge: %+v", code)
	}
	zip := out.Apps[1]
	if zip.Name != "7-Zip" || zip.CategoryID != base.Categories[0].ID {
		t.Errorf("7-Zip after merge: %+v", zip)
	}
	f := out.Apps[3]
	if f.Name != figma.Name || f.ID == base.Apps[0].ID || len(f.TagIDs) != 2 || !slices.Contains(f.TagIDs, base.Tags[0].ID) {
		t.Errorf("Figma after merge: %+v", f)
	}
	design, ok := out.Category(f.CategoryID)
	if !ok || design.Name != "设计" {
		t.Errorf("Figma's category %+v", design)
	}

	// Merging the result again changes nothing more.
	again, sum2, err := Merge(out, in)
	if err != nil {
		t.Fatal(err)
	}
	if sum2.AddedApps != 0 || sum2.AddedCategories != 0 || sum2.AddedTags != 0 || len(again.Apps) != 4 {
		t.Errorf("second merge %+v, %d apps", sum2, len(again.Apps))
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	c := sample(t)
	data, _ := c.Marshal()
	in, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	out, sum, err := Merge(c, in)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.Marshal()
	b, _ := out.Marshal()
	if string(a) != string(b) || sum.AddedApps != 0 || sum.MergedApps != len(c.Apps) {
		t.Errorf("importing an export changed the catalog: %+v", sum)
	}
}
