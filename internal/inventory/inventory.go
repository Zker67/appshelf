// Package inventory lists the software installed on this machine, as winget,
// scoop and the registry know it, and tells which of it is in the catalog.
//
// What it finds is shown to the user and helps adding apps; it never decides
// an install: whether an app is installed is for its source and its
// detection rule to say (internal/core).
package inventory

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/detect"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
)

// Kind is where an installed program comes from, as far as AppShelf can
// install it again.
type Kind string

const (
	// Winget is a package of the winget source.
	Winget Kind = "winget"
	// Scoop is an app scoop installed.
	Scoop Kind = "scoop"
	// Local is a program of the registry that no source AppShelf uses knows.
	Local Kind = "local"
	// Store is an app package (MSIX), as of the Microsoft Store.
	Store Kind = "store"
)

// Entry is an installed program.
type Entry struct {
	// Key identifies the entry among the others: "winget:git.git",
	// "arp:machine\x64\<key>", "msix:<package>", "scoop:7zip" or
	// "reg:<registry key>", in lower case.
	Key       string
	Name      string
	Version   string
	Publisher string
	Kind      Kind
	WingetID  string // of a Winget entry
	Bucket    string // of a Scoop entry
	ScoopName string // of a Scoop entry
	Update    string // the newer version its source has, when there is one
	// DisplayName is the program's name in the registry, when known: what
	// detection rules match.
	DisplayName string
	// Icon is the registry's DisplayIcon: a file, maybe with ",index".
	Icon     string
	Location string // the registry's InstallLocation
	// PackageFamily is the family name of a Store entry's package.
	PackageFamily string
}

// RuleName is the name a detection rule would match: the registry's name,
// else the name its source gives.
func (e Entry) RuleName() string { return cmp.Or(e.DisplayName, e.Name) }

// Build merges what winget, scoop and the registry list into entries, sorted
// by name:
//
//   - winget's programs of no source (ARP\<scope>\<arch>\<key>) take their
//     publisher, icon and location from their registry key;
//   - the packages of the winget source take them from the registry entry
//     of their name, preferring the same version;
//   - app packages (MSIX\…) and packages of the msstore source are Store
//     entries;
//   - when winget could not be read (wingetOK false), the registry's entries
//     stand for its programs; otherwise winget lists all of them already;
//   - scoop's apps, which are not in the registry, come last.
func Build(pkgs []winget.Package, wingetOK bool, apps []scoop.App, reg []detect.Entry) []Entry {
	byKey := map[string][]int{}  // scope\name, in lower case
	byName := map[string][]int{} // display name, in lower case
	for i, r := range reg {
		k := strings.ToLower(r.Scope + `\` + r.Name)
		byKey[k] = append(byKey[k], i)
		n := strings.ToLower(r.DisplayName)
		byName[n] = append(byName[n], i)
	}
	used := make([]bool, len(reg))
	join := func(e *Entry, i int) {
		r := reg[i]
		used[i] = true
		e.Publisher = r.Publisher
		e.DisplayName = r.DisplayName
		e.Icon = r.DisplayIcon
		e.Location = r.InstallLocation
		if e.Version == "" {
			e.Version = r.DisplayVersion
		}
	}

	out := make([]Entry, 0, len(pkgs)+len(apps))
	for _, p := range pkgs {
		e := Entry{Name: p.Name, Version: p.Version}
		switch {
		case hasPrefixFold(p.ID, `ARP\`):
			rest := p.ID[len(`ARP\`):]
			e.Key, e.Kind = "arp:"+strings.ToLower(rest), Local
			if i, ok := arpEntry(rest, reg, byKey, used); ok {
				join(&e, i)
			}
		case hasPrefixFold(p.ID, `MSIX\`):
			full := p.ID[len(`MSIX\`):]
			e.Key, e.Kind = "msix:"+strings.ToLower(full), Store
			e.PackageFamily = familyName(full)
		case strings.EqualFold(p.Source, "msstore"):
			e.Key, e.Kind = "msstore:"+strings.ToLower(p.ID), Store
		case strings.EqualFold(p.Source, "winget"):
			e.Key, e.Kind, e.WingetID, e.Update = "winget:"+strings.ToLower(p.ID), Winget, p.ID, p.Update
			if i, ok := nameEntry(p.Name, p.Version, reg, byName, used); ok {
				join(&e, i)
			}
		default:
			// A package of another source, which AppShelf does not use.
			e.Key, e.Kind = "pkg:"+strings.ToLower(p.Source+`\`+p.ID), Local
			if i, ok := nameEntry(p.Name, p.Version, reg, byName, used); ok {
				join(&e, i)
			}
		}
		if e.Name == "" {
			e.Name = cmp.Or(e.DisplayName, p.ID)
		}
		out = append(out, e)
	}
	if !wingetOK {
		for i, r := range reg {
			if used[i] {
				continue
			}
			out = append(out, Entry{
				Key: "reg:" + strings.ToLower(r.Key), Name: r.DisplayName, Version: r.DisplayVersion,
				Publisher: r.Publisher, Kind: Local, DisplayName: r.DisplayName,
				Icon: r.DisplayIcon, Location: r.InstallLocation,
			})
		}
	}
	for _, a := range apps {
		out = append(out, Entry{
			Key: "scoop:" + strings.ToLower(a.Name), Name: a.Name, Version: a.Version,
			Kind: Scoop, Bucket: a.Bucket, ScoopName: a.Name, Update: a.Update,
		})
	}
	slices.SortStableFunc(out, func(a, b Entry) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), strings.Compare(a.Key, b.Key))
	})
	// Keys are unique: winget may list a package twice, for the machine and
	// for the user.
	seen := map[string]int{}
	for i := range out {
		k := out[i].Key
		seen[k]++
		if n := seen[k]; n > 1 {
			out[i].Key = k + "#" + strconv.Itoa(n)
		}
	}
	return out
}

// arpEntry finds the registry key winget names ARP\<scope>\<arch>\<key>,
// preferring the same registry view.
func arpEntry(rest string, reg []detect.Entry, byKey map[string][]int, used []bool) (int, bool) {
	parts := strings.SplitN(rest, `\`, 3)
	if len(parts) != 3 {
		return 0, false
	}
	scope, arch, name := parts[0], parts[1], parts[2]
	found := -1
	for _, i := range byKey[strings.ToLower(scope+`\`+name)] {
		if used[i] {
			continue
		}
		if strings.EqualFold(reg[i].Arch, arch) {
			return i, true
		}
		if found < 0 {
			found = i
		}
	}
	return found, found >= 0
}

// nameEntry finds the registry entry of a winget package by its name,
// which winget takes from the registry, preferring the same version.
func nameEntry(name, version string, reg []detect.Entry, byName map[string][]int, used []bool) (int, bool) {
	found := -1
	for _, i := range byName[strings.ToLower(name)] {
		if used[i] {
			continue
		}
		if version != "" && reg[i].DisplayVersion == version {
			return i, true
		}
		if found < 0 {
			found = i
		}
	}
	return found, found >= 0
}

// familyName returns the family name of an app package from its full
// name, Name_Version_Arch_ResourceID_PublisherID: Name_PublisherID.
func familyName(full string) string {
	parts := strings.Split(full, "_")
	if len(parts) < 5 {
		return ""
	}
	return parts[0] + "_" + parts[len(parts)-1]
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// Match tells which apps of the catalog each entry is, by the entry's key:
// a winget package of the same ID, a scoop app of the same name, or an
// entry an app's detection rule matches by its name.
func Match(entries []Entry, apps []catalog.App) map[string][]string {
	byWinget := map[string][]string{}
	byScoop := map[string][]string{}
	type rule struct {
		re *regexp.Regexp
		id string
	}
	var rules []rule
	for _, a := range apps {
		switch a.Source.Type {
		case catalog.Winget:
			k := strings.ToLower(a.Source.ID)
			byWinget[k] = append(byWinget[k], a.ID)
		case catalog.Scoop:
			k := strings.ToLower(a.Source.Name)
			byScoop[k] = append(byScoop[k], a.ID)
		}
		if a.Detect != nil && a.Detect.DisplayName != "" {
			if re, err := regexp.Compile("(?i)" + a.Detect.DisplayName); err == nil {
				rules = append(rules, rule{re, a.ID})
			}
		}
	}
	out := map[string][]string{}
	for _, e := range entries {
		var ids []string
		switch e.Kind {
		case Winget:
			ids = append(ids, byWinget[strings.ToLower(e.WingetID)]...)
		case Scoop:
			ids = append(ids, byScoop[strings.ToLower(e.ScoopName)]...)
		}
		name := e.RuleName()
		for _, r := range rules {
			if !slices.Contains(ids, r.id) && r.re.MatchString(name) {
				ids = append(ids, r.id)
			}
		}
		if len(ids) > 0 {
			out[e.Key] = ids
		}
	}
	return out
}

// Sources reads what is installed. A nil function is a source this machine
// does not have.
type Sources struct {
	Winget func(ctx context.Context) ([]winget.Package, error)
	Scoop  func(ctx context.Context) ([]scoop.App, error)
	Scan   func() ([]detect.Entry, error)
}

// List is what Load found.
type List struct {
	Entries []Entry
	At      time.Time
	// Notes say what could not be read, for the user.
	Notes []string
}

// Load reads the sources side by side and builds the entries.
func Load(ctx context.Context, src Sources) List {
	var (
		wg               sync.WaitGroup
		pkgs             []winget.Package
		apps             []scoop.App
		reg              []detect.Entry
		wErr, sErr, rErr error
	)
	if src.Winget != nil {
		wg.Go(func() { pkgs, wErr = src.Winget(ctx) })
	}
	if src.Scoop != nil {
		wg.Go(func() { apps, sErr = src.Scoop(ctx) })
	}
	if src.Scan != nil {
		wg.Go(func() { reg, rErr = src.Scan() })
	}
	wg.Wait()
	var notes []string
	if wErr != nil {
		notes = append(notes, wErr.Error())
	}
	if sErr != nil {
		notes = append(notes, "scoop："+sErr.Error())
	}
	if rErr != nil {
		notes = append(notes, rErr.Error())
	}
	wingetOK := src.Winget != nil && wErr == nil
	return List{Entries: Build(pkgs, wingetOK, apps, reg), At: time.Now(), Notes: notes}
}
