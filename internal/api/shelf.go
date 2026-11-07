package api

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/core"
	"appshelf/internal/source"
	"appshelf/internal/state"
)

// Shelf is the catalog: its apps with their states, categories and tags,
// adding, editing and deleting, importing and exporting.
type Shelf struct{ a *App }

// Snapshot returns the catalog and what is known of its apps.
func (s *Shelf) Snapshot() ShelfSnapshot {
	a := s.a
	var out ShelfSnapshot
	<-a.ready
	pol := a.core.Store().Policies()
	now := time.Now()
	a.read(func() {
		c := a.core
		cat := c.Catalog()
		out.Apps = make([]ShelfApp, 0, len(cat.Apps))
		for _, app := range cat.Apps {
			sa := ShelfApp{App: app.Clone(), Checking: c.Checking(app.ID), Pending: c.Queue.Pending(app.ID), Icon: iconOf(app.Source)}
			if st, ok := c.Status(app.ID); ok {
				sa.Status = statusOf(st)
				if st.State == source.UpdateAvailable {
					sa.Hold = holdOf(pol[app.Source.Key()].Hold(st.Latest, now))
				}
			}
			if app.CategoryID == "" {
				out.Uncategorized++
			}
			out.Apps = append(out.Apps, sa)
		}
		for _, k := range cat.Categories {
			out.Categories = append(out.Categories, CategoryInfo{ID: k.ID, Name: k.Name, Count: cat.CategoryUsage(k.ID)})
		}
		for _, t := range cat.Tags {
			out.Tags = append(out.Tags, TagInfo{ID: t.ID, Name: t.Name, Count: cat.TagUsage(t.ID)})
		}
		out.Refreshing = c.Refreshing()
		out.LastRefresh = c.LastRefresh()
		out.Notes = slices.Clone(c.Notes)
	})
	return out
}

// Refresh checks the sources and reads the state of every app again, then
// what is installed: "检查更新", a check for updates as Updates.Check.
func (s *Shelf) Refresh() {
	<-s.a.ready
	s.a.checkNow()
}

// SaveApp adds an app, when its ID is empty, or changes one, and returns it
// as saved. Its state is read again.
func (s *Shelf) SaveApp(app catalog.App) (catalog.App, error) {
	var saved catalog.App
	var err error
	s.a.write([]ChangeKind{ChangedShelf, ChangedInstalled}, func() { saved, err = s.a.core.SaveApp(app) })
	return saved, err
}

// DeleteApps removes apps from the catalog and returns a token that
// UndoDelete takes to bring them back.
func (s *Shelf) DeleteApps(ids []string) (string, error) {
	a := s.a
	var del deletion
	var err error
	a.write([]ChangeKind{ChangedShelf, ChangedInstalled}, func() {
		c := a.core
		del.records = map[string]state.GitHubRecord{}
		for _, id := range ids {
			app, ok := c.Catalog().App(id)
			if !ok {
				continue
			}
			del.apps = append(del.apps, app)
			if r, ok := c.Store().GitHub(id); ok {
				del.records[id] = r
			}
		}
		if len(del.apps) == 0 {
			err = errors.New("这些软件已不在清单里")
			return
		}
		err = c.DeleteApps(ids)
	})
	if err != nil {
		return "", err
	}
	return a.remember(func(token string) { a.deleted[token] = del }), nil
}

// UndoDelete brings back the apps a deletion removed, with what AppShelf
// installed of them. It returns how many came back: an app whose source
// the catalog has again, or whose category is gone, stays out.
func (s *Shelf) UndoDelete(token string) (int, error) {
	a := s.a
	a.mu.Lock()
	del, ok := a.deleted[token]
	delete(a.deleted, token)
	a.mu.Unlock()
	if !ok {
		return 0, errors.New("已无法撤销")
	}
	cat := map[string]bool{}
	var added []catalog.App
	var errs []error
	a.write([]ChangeKind{ChangedShelf, ChangedInstalled}, func() {
		c := a.core
		for _, k := range c.Catalog().Categories {
			cat[k.ID] = true
		}
		apps := make([]catalog.App, 0, len(del.apps))
		for _, app := range del.apps {
			app = app.Clone()
			if app.CategoryID != "" && !cat[app.CategoryID] {
				app.CategoryID = ""
			}
			app.TagIDs = slices.DeleteFunc(app.TagIDs, func(t string) bool { _, ok := c.Catalog().Tag(t); return !ok })
			apps = append(apps, app)
		}
		added, errs = c.AddApps(apps)
		for _, app := range added {
			if r, ok := del.records[app.ID]; ok {
				c.Store().PutGitHub(app.ID, r)
			}
		}
	})
	if len(added) == 0 {
		return 0, errors.Join(errs...)
	}
	return len(added), nil
}

// SetNote changes an app's own note, in Markdown, and returns it as saved;
// "" removes it. SaveApp keeps an app's note.
func (s *Shelf) SetNote(id, note string) (string, error) {
	var saved string
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() {
		if err = s.a.core.SetNote(id, note); err == nil {
			app, _ := s.a.core.Catalog().App(id)
			saved = app.Note
		}
	})
	return saved, err
}

// SetCategory puts apps in a category, or none for "".
func (s *Shelf) SetCategory(ids []string, categoryID string) error {
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() {
		err = s.a.core.EditApps(ids, func(app *catalog.App) { app.CategoryID = categoryID })
	})
	return err
}

// SetTag gives apps a tag, or takes it from them.
func (s *Shelf) SetTag(ids []string, tagID string, on bool) error {
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() {
		err = s.a.core.EditApps(ids, func(app *catalog.App) {
			app.TagIDs = slices.DeleteFunc(app.TagIDs, func(t string) bool { return t == tagID })
			if on {
				app.TagIDs = append(app.TagIDs, tagID)
			}
		})
	})
	return err
}

// AddCategory adds a category and returns it.
func (s *Shelf) AddCategory(name string) (CategoryInfo, error) {
	var out CategoryInfo
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() { out, err = s.a.addCategory(name) })
	return out, err
}

func (a *App) addCategory(name string) (CategoryInfo, error) {
	if err := a.core.AddCategory(name); err != nil {
		return CategoryInfo{}, err
	}
	name = strings.TrimSpace(name)
	for _, k := range a.core.Catalog().Categories {
		if k.Name == name {
			return CategoryInfo{ID: k.ID, Name: k.Name}, nil
		}
	}
	return CategoryInfo{}, errors.New("分类没有保存")
}

// RenameCategory renames a category.
func (s *Shelf) RenameCategory(id, name string) error {
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() { err = s.a.core.RenameCategory(id, name) })
	return err
}

// DeleteCategory removes a category; its apps become uncategorized.
func (s *Shelf) DeleteCategory(id string) error {
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() { err = s.a.core.DeleteCategory(id) })
	return err
}

// AddTag adds a tag and returns it.
func (s *Shelf) AddTag(name string) (TagInfo, error) {
	var out TagInfo
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() { out, err = s.a.addTag(name) })
	return out, err
}

func (a *App) addTag(name string) (TagInfo, error) {
	if err := a.core.AddTag(name); err != nil {
		return TagInfo{}, err
	}
	name = strings.TrimSpace(name)
	for _, t := range a.core.Catalog().Tags {
		if t.Name == name {
			return TagInfo{ID: t.ID, Name: t.Name}, nil
		}
	}
	return TagInfo{}, errors.New("标签没有保存")
}

// RenameTag renames a tag.
func (s *Shelf) RenameTag(id, name string) error {
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() { err = s.a.core.RenameTag(id, name) })
	return err
}

// DeleteTag removes a tag from the catalog and its apps.
func (s *Shelf) DeleteTag(id string) error {
	var err error
	s.a.write([]ChangeKind{ChangedShelf}, func() { err = s.a.core.DeleteTag(id) })
	return err
}

// AddPackages adds packages found by searching, or installed, with a
// category and tags, creating those that are new. Each package is added on
// its own: one that cannot be says why, the others are added.
func (s *Shelf) AddPackages(req AddRequest) (AddResult, error) {
	a := s.a
	out := AddResult{Added: []string{}, Failed: []AddFailure{}}
	if len(req.Items) == 0 {
		return out, errors.New("没有要加入的软件")
	}
	var err error
	a.write([]ChangeKind{ChangedShelf, ChangedInstalled}, func() {
		c := a.core
		catID := req.CategoryID
		if name := strings.TrimSpace(req.NewCategory); name != "" {
			if id := findCategory(c.Catalog(), name); id != "" {
				catID = id
			} else {
				var k CategoryInfo
				if k, err = a.addCategory(name); err != nil {
					return
				}
				catID = k.ID
			}
		}
		tags := slices.Clone(req.TagIDs)
		for _, name := range req.NewTags {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id := findTag(c.Catalog(), name)
			if id == "" {
				var t TagInfo
				if t, err = a.addTag(name); err != nil {
					return
				}
				id = t.ID
			}
			if !slices.Contains(tags, id) {
				tags = append(tags, id)
			}
		}
		apps := make([]catalog.App, 0, len(req.Items))
		for _, it := range req.Items {
			apps = append(apps, catalog.App{Name: strings.TrimSpace(it.Name), CategoryID: catID, TagIDs: slices.Clone(tags), Source: it.Source, Detect: it.Detect})
		}
		added, errs := c.AddApps(apps)
		for _, app := range added {
			out.Added = append(out.Added, app.ID)
		}
		for i, e := range errs {
			if e != nil {
				out.Failed = append(out.Failed, AddFailure{Name: req.Items[i].Name, Error: e.Error()})
			}
		}
	})
	return out, err
}

func findCategory(cat *catalog.Catalog, name string) string {
	for _, k := range cat.Categories {
		if strings.EqualFold(k.Name, name) {
			return k.ID
		}
	}
	return ""
}

func findTag(cat *catalog.Catalog, name string) string {
	for _, t := range cat.Tags {
		if strings.EqualFold(t.Name, name) {
			return t.ID
		}
	}
	return ""
}

// Export saves the catalog to a file the user chooses, and returns its
// path; "" when the user canceled.
func (s *Shelf) Export(ctx context.Context) (string, error) {
	a := s.a
	<-a.ready
	if a.hooks.SaveFile == nil {
		return "", errors.New("无法打开保存对话框")
	}
	path, err := a.hooks.SaveFile(ctx, "导出清单", "appshelf-catalog.json")
	if err != nil || path == "" {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		path += ".json"
	}
	a.read(func() { err = a.core.Export(path) })
	if err != nil {
		return "", err
	}
	return path, nil
}

// ReadImport reads a catalog file the user chooses and returns what
// merging it would change, nothing changed yet; null when the user
// canceled.
func (s *Shelf) ReadImport(ctx context.Context) (*ImportPreview, error) {
	a := s.a
	<-a.ready
	if a.hooks.OpenFile == nil {
		return nil, errors.New("无法打开文件对话框")
	}
	path, err := a.hooks.OpenFile(ctx, "导入清单")
	if err != nil || path == "" {
		return nil, err
	}
	return s.readImport(path)
}

func (s *Shelf) readImport(path string) (*ImportPreview, error) {
	a := s.a
	var im *core.Import
	var err error
	a.read(func() { im, err = a.core.ReadImport(path) })
	if err != nil {
		return nil, err
	}
	token := a.remember(func(token string) { a.imports[token] = im })
	return &ImportPreview{Token: token, File: filepath.Base(path), Counts: countsOf(im.Summary)}, nil
}

// ApplyImport merges a catalog file read by ReadImport into the catalog, as
// it is now.
func (s *Shelf) ApplyImport(token string) (MergeCounts, error) {
	a := s.a
	a.mu.Lock()
	im, ok := a.imports[token]
	delete(a.imports, token)
	a.mu.Unlock()
	if !ok {
		return MergeCounts{}, errors.New("请重新选择要导入的文件")
	}
	var out MergeCounts
	var err error
	a.write([]ChangeKind{ChangedShelf, ChangedInstalled}, func() {
		sum, e := a.core.ApplyImport(im)
		out, err = countsOf(sum), e
	})
	return out, err
}
