package catalog

import "slices"

// MergeSummary counts what merging an imported catalog changes.
type MergeSummary struct {
	AddedApps       int
	MergedApps      int // apps already in the catalog, by their source
	AddedCategories int
	AddedTags       int
}

// Merge returns base with incoming merged into it, leaving both unchanged:
//
//   - categories and tags match by name, ignoring case; others are added;
//   - apps match by their source (Source.Key). An app already in base keeps
//     its name, category, source, detection rule and note, gains the
//     imported app's tags, and takes its category and note only when it
//     has none;
//   - other apps are added, with their categories and tags mapped;
//   - an imported ID already used in base gets a new ID.
func Merge(base, incoming *Catalog) (*Catalog, MergeSummary, error) {
	out := base.Clone()
	var sum MergeSummary

	usedCat := map[string]bool{}
	catByName := map[string]string{}
	for _, cat := range out.Categories {
		usedCat[cat.ID] = true
		catByName[foldName(cat.Name)] = cat.ID
	}
	catMap := map[string]string{}
	for _, cat := range incoming.Categories {
		if id, ok := catByName[foldName(cat.Name)]; ok {
			catMap[cat.ID] = id
			continue
		}
		id := cat.ID
		if usedCat[id] {
			id = NewID("c")
		}
		usedCat[id] = true
		catByName[foldName(cat.Name)] = id
		catMap[cat.ID] = id
		out.Categories = append(out.Categories, Category{ID: id, Name: cat.Name})
		sum.AddedCategories++
	}

	usedTag := map[string]bool{}
	tagByName := map[string]string{}
	for _, t := range out.Tags {
		usedTag[t.ID] = true
		tagByName[foldName(t.Name)] = t.ID
	}
	tagMap := map[string]string{}
	for _, t := range incoming.Tags {
		if id, ok := tagByName[foldName(t.Name)]; ok {
			tagMap[t.ID] = id
			continue
		}
		id := t.ID
		if usedTag[id] {
			id = NewID("t")
		}
		usedTag[id] = true
		tagByName[foldName(t.Name)] = id
		tagMap[t.ID] = id
		out.Tags = append(out.Tags, Tag{ID: id, Name: t.Name})
		sum.AddedTags++
	}

	usedApp := map[string]bool{}
	byKey := map[string]int{}
	for i, a := range out.Apps {
		usedApp[a.ID] = true
		byKey[a.Source.Key()] = i
	}
	for _, in := range incoming.Apps {
		tags := make([]string, 0, len(in.TagIDs))
		for _, t := range in.TagIDs {
			tags = append(tags, tagMap[t])
		}
		cat := catMap[in.CategoryID]
		if i, ok := byKey[in.Source.Key()]; ok {
			a := &out.Apps[i]
			for _, t := range tags {
				if !slices.Contains(a.TagIDs, t) {
					a.TagIDs = append(a.TagIDs, t)
				}
			}
			if a.CategoryID == "" {
				a.CategoryID = cat
			}
			if a.Note == "" {
				a.Note = in.Note
			}
			sum.MergedApps++
			continue
		}
		a := in.Clone()
		if usedApp[a.ID] {
			a.ID = NewID("a")
		}
		usedApp[a.ID] = true
		a.CategoryID = cat
		a.TagIDs = tags
		byKey[a.Source.Key()] = len(out.Apps)
		out.Apps = append(out.Apps, a)
		sum.AddedApps++
	}
	if err := out.Validate(); err != nil {
		return nil, MergeSummary{}, err
	}
	return out, sum, nil
}
