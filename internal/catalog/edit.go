package catalog

import (
	"slices"
	"strings"
)

// Clone returns a deep copy of the catalog.
func (c *Catalog) Clone() *Catalog {
	out := &Catalog{
		SchemaVersion: c.SchemaVersion,
		Categories:    slices.Clone(c.Categories),
		Tags:          slices.Clone(c.Tags),
		Apps:          make([]App, len(c.Apps)),
	}
	if out.Categories == nil {
		out.Categories = []Category{}
	}
	if out.Tags == nil {
		out.Tags = []Tag{}
	}
	for i, a := range c.Apps {
		out.Apps[i] = a.Clone()
	}
	return out
}

// Clone returns a deep copy of the app.
func (a App) Clone() App {
	a.TagIDs = slices.Clone(a.TagIDs)
	if a.Detect != nil {
		d := *a.Detect
		a.Detect = &d
	}
	return a
}

// App returns the app with the ID.
func (c *Catalog) App(id string) (App, bool) {
	for _, a := range c.Apps {
		if a.ID == id {
			return a.Clone(), true
		}
	}
	return App{}, false
}

// Category returns the category with the ID.
func (c *Catalog) Category(id string) (Category, bool) {
	for _, cat := range c.Categories {
		if cat.ID == id {
			return cat, true
		}
	}
	return Category{}, false
}

// Tag returns the tag with the ID.
func (c *Catalog) Tag(id string) (Tag, bool) {
	for _, t := range c.Tags {
		if t.ID == id {
			return t, true
		}
	}
	return Tag{}, false
}

// apply runs edit on a copy and keeps the copy only when it is valid.
func (c *Catalog) apply(edit func(n *Catalog) error) error {
	n := c.Clone()
	if err := edit(n); err != nil {
		return err
	}
	if err := n.Validate(); err != nil {
		return err
	}
	*c = *n
	return nil
}

// AddCategory adds a category with the name.
func (c *Catalog) AddCategory(name string) (Category, error) {
	cat := Category{ID: NewID("c"), Name: strings.TrimSpace(name)}
	err := c.apply(func(n *Catalog) error {
		n.Categories = append(n.Categories, cat)
		return nil
	})
	return cat, err
}

// RenameCategory renames a category.
func (c *Catalog) RenameCategory(id, name string) error {
	return c.apply(func(n *Catalog) error {
		for i := range n.Categories {
			if n.Categories[i].ID == id {
				n.Categories[i].Name = strings.TrimSpace(name)
				return nil
			}
		}
		return &ValidationError{Message: "分类不存在"}
	})
}

// CategoryUsage counts the apps in a category.
func (c *Catalog) CategoryUsage(id string) int {
	n := 0
	for _, a := range c.Apps {
		if a.CategoryID == id {
			n++
		}
	}
	return n
}

// DeleteCategory removes a category; its apps become uncategorized.
func (c *Catalog) DeleteCategory(id string) {
	c.Categories = slices.DeleteFunc(c.Categories, func(cat Category) bool { return cat.ID == id })
	for i := range c.Apps {
		if c.Apps[i].CategoryID == id {
			c.Apps[i].CategoryID = ""
		}
	}
}

// AddTag adds a tag with the name.
func (c *Catalog) AddTag(name string) (Tag, error) {
	tag := Tag{ID: NewID("t"), Name: strings.TrimSpace(name)}
	err := c.apply(func(n *Catalog) error {
		n.Tags = append(n.Tags, tag)
		return nil
	})
	return tag, err
}

// RenameTag renames a tag.
func (c *Catalog) RenameTag(id, name string) error {
	return c.apply(func(n *Catalog) error {
		for i := range n.Tags {
			if n.Tags[i].ID == id {
				n.Tags[i].Name = strings.TrimSpace(name)
				return nil
			}
		}
		return &ValidationError{Message: "标签不存在"}
	})
}

// TagUsage counts the apps with a tag.
func (c *Catalog) TagUsage(id string) int {
	n := 0
	for _, a := range c.Apps {
		if slices.Contains(a.TagIDs, id) {
			n++
		}
	}
	return n
}

// DeleteTag removes a tag from the catalog and from every app.
func (c *Catalog) DeleteTag(id string) {
	c.Tags = slices.DeleteFunc(c.Tags, func(t Tag) bool { return t.ID == id })
	for i := range c.Apps {
		c.Apps[i].TagIDs = slices.DeleteFunc(c.Apps[i].TagIDs, func(t string) bool { return t == id })
	}
}

// PutApp adds the app when its ID is empty, giving it one, or replaces the
// app with its ID. It returns the app as stored.
func (c *Catalog) PutApp(app App) (App, error) {
	app = normalizeApp(app)
	if app.ID == "" {
		app.ID = NewID("a")
	}
	err := c.apply(func(n *Catalog) error {
		for i := range n.Apps {
			if n.Apps[i].ID == app.ID {
				n.Apps[i] = app
				return nil
			}
		}
		n.Apps = append(n.Apps, app)
		return nil
	})
	return app.Clone(), err
}

// DeleteApp removes the app with the ID.
func (c *Catalog) DeleteApp(id string) {
	c.Apps = slices.DeleteFunc(c.Apps, func(a App) bool { return a.ID == id })
}

// normalizeApp trims what the user typed and drops the fields its source
// type does not use.
func normalizeApp(a App) App {
	a = a.Clone()
	a.Name = strings.TrimSpace(a.Name)
	s := a.Source
	s.ID = strings.TrimSpace(s.ID)
	s.Bucket = strings.TrimSpace(s.Bucket)
	s.Name = strings.TrimSpace(s.Name)
	s.BucketURL = strings.TrimSpace(s.BucketURL)
	s.Repo = strings.Trim(strings.TrimSpace(s.Repo), "/")
	s.AssetPattern = strings.TrimSpace(s.AssetPattern)
	s.InstallerArgs = strings.TrimSpace(s.InstallerArgs)
	switch s.Type {
	case Winget:
		s = Source{Type: Winget, ID: s.ID}
	case Scoop:
		s = Source{Type: Scoop, Bucket: s.Bucket, Name: s.Name, BucketURL: s.BucketURL}
	case GitHub:
		s = Source{Type: GitHub, Repo: s.Repo, AssetPattern: s.AssetPattern, InstallerArgs: s.InstallerArgs}
	}
	a.Source = s
	if a.Detect != nil {
		d := Detect{DisplayName: strings.TrimSpace(a.Detect.DisplayName), ExePath: strings.TrimSpace(a.Detect.ExePath)}
		if d.DisplayName == "" && d.ExePath == "" {
			a.Detect = nil
		} else {
			a.Detect = &d
		}
	}
	var tags []string
	for _, t := range a.TagIDs {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	a.TagIDs = tags
	a.Note = NormalizeNote(a.Note)
	return a
}

// NormalizeNote makes a note's line breaks "\n" and drops the blank lines
// and spaces around it, keeping those inside, as Markdown's two spaces that
// break a line. A note of spaces is none.
func NormalizeNote(note string) string {
	note = strings.ReplaceAll(note, "\r\n", "\n")
	note = strings.ReplaceAll(note, "\r", "\n")
	note = strings.TrimRight(note, " \t\n")
	for {
		line, rest, ok := strings.Cut(note, "\n")
		if !ok || strings.TrimSpace(line) != "" {
			return note
		}
		note = rest
	}
}

// SetNote changes the note of an app; "" removes it.
func (c *Catalog) SetNote(id, note string) error {
	return c.apply(func(n *Catalog) error {
		for i := range n.Apps {
			if n.Apps[i].ID == id {
				n.Apps[i].Note = NormalizeNote(note)
				return nil
			}
		}
		return &ValidationError{Message: "软件不在清单里"}
	})
}
