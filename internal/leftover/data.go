package leftover

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"appshelf/internal/detect"
)

// DataProgram describes directory ownership for display, not permission to
// clean. Aliases must come from installed-program metadata, never a fuzzy search.
type DataProgram struct {
	Program
	Aliases []string
	Family  string
}

// DataConfig searches only data roots; program files and shortcuts are not data.
func DataConfig(protected ...string) Config {
	cfg := DefaultConfig(protected...)
	cfg.DirRoots = nil
	for _, p := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA"), os.Getenv("ProgramData")} {
		if filepath.IsAbs(p) {
			cfg.DirRoots = append(cfg.DirRoots, p)
		}
	}
	if p := os.Getenv("USERPROFILE"); filepath.IsAbs(p) {
		cfg.DirRoots = append(cfg.DirRoots, filepath.Join(p, "AppData", "LocalLow"))
	}
	return cfg
}

// FindData identifies data by exact normalized names, publisher/name pairs,
// package family and scoop identity. It deliberately never calls contentReasons:
// DLLs and large caches say nothing about who owns the data.
// Results are display-only and must never be passed to Clean.
func FindData(ctx context.Context, cfg Config, p DataProgram, others []Other) []Item {
	names := map[string]bool{}
	addName := func(s string) {
		n := normName(s)
		// Unlike cleanup, a software name such as Steam or Git is useful for
		// display. Shared container names still cannot identify an application.
		if len([]rune(n)) >= 2 && (!genericNames[n] || n == "steam" || n == "git") {
			names[strings.ReplaceAll(n, " ", "")] = true
		}
	}
	addName(p.Name)
	addName(p.ScoopName)
	for _, n := range p.Aliases {
		addName(n)
	}
	publisher := dataPublisher(p.Publisher)
	match := func(s string) bool { return names[strings.ReplaceAll(normName(s), " ", "")] }
	var out []Item
	seen := map[string]bool{}
	add := func(path, reason string, level Level) {
		if ctx.Err() != nil {
			return
		}
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return
		}
		key := cleanPath(path)
		if seen[key] {
			return
		}
		// Only roots' descendants can be candidates. Avoid counting the
		// installation twice, and never attribute another installation to it.
		for _, root := range cfg.DirRoots {
			if key == cleanPath(root) {
				return
			}
		}
		blocked := append([]string{p.Location}, cfg.Protected...)
		for _, o := range others {
			// Bad registry entries sometimes name AppData or Program Files
			// itself. Such a shared root does not own all its descendants.
			loc := cleanPath(o.Location)
			shared := false
			for _, root := range cfg.DirRoots {
				if loc == cleanPath(root) {
					shared = true
				}
			}
			if !shared && loc != "" && len(loc) > 3 && !slices.Contains(detect.SharedDirectories(), loc) {
				blocked = append(blocked, o.Location)
			}
		}
		for _, b := range blocked {
			b = cleanPath(b)
			if b != "" && (key == b || within(key, b) || within(b, key)) {
				return
			}
		}
		seen[key] = true
		out = append(out, Item{ID: idOf(Dir, path, ""), Kind: Dir, Path: path, Level: level, Reasons: []Reason{{Text: reason, Delta: 2}}})
	}
	for _, root := range cfg.DirRoots {
		if ctx.Err() != nil {
			break
		}
		children, _ := os.ReadDir(root)
		for _, child := range children {
			if !child.IsDir() || child.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(root, child.Name())
			if match(child.Name()) {
				add(path, "目录名称与软件或安装元数据相符", Medium)
				continue
			}
			vendor := dataPublisher(child.Name())
			if publisher == "" || vendor != publisher {
				continue
			}
			subs, _ := os.ReadDir(path)
			for _, sub := range subs {
				// Microsoft Edge -> Microsoft/Edge, without treating the
				// shared Microsoft directory as the application's data.
				if sub.IsDir() && (match(sub.Name()) || match(child.Name()+" "+sub.Name())) {
					add(filepath.Join(path, sub.Name()), "发布者目录下的软件数据目录", Medium)
				}
			}
		}
	}
	if safeComponent(p.Family) {
		// PackageFamily is supplied by the installed package identity.
		if root := os.Getenv("LOCALAPPDATA"); filepath.IsAbs(root) {
			add(filepath.Join(root, "Packages", p.Family), "应用包标识对应的数据目录", High)
		}
	}
	if safeComponent(p.ScoopName) {
		for _, root := range cfg.ScoopPersist {
			add(filepath.Join(root, p.ScoopName), "scoop 为它保留的数据", High)
		}
	}
	return out
}

func safeComponent(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `\/:`)
}

// Shared vendors can be traversed, but never attributed wholesale.
func dataPublisher(s string) string {
	s = strings.TrimSpace(s)
	for {
		next := strings.TrimSpace(companyTail.ReplaceAllString(s, ""))
		if next == s {
			break
		}
		s = next
	}
	return normName(strings.TrimSuffix(strings.TrimSuffix(s, "有限公司"), "股份"))
}
