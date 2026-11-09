// Package leftover finds what a removed program may have left behind, as
// its directory, its settings in the registry and its shortcuts, and
// scores each find with the reasons for and against it, so that the user
// chooses what to clean. It never cleans on its own: Clean removes only
// the finds it is given, moving files to the Recycle Bin and exporting
// registry keys before deleting them.
//
// The scoring follows Bulk Crap Uninstaller's ConfidenceRecords
// (Apache-2.0); see references/external-repos.
package leftover

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"appshelf/internal/detect"
)

// Kind is what a find is.
type Kind string

const (
	Dir      Kind = "dir"
	Shortcut Kind = "shortcut"
	RegKey   Kind = "regkey"   // a key under HKEY_CURRENT_USER
	RegValue Kind = "regvalue" // a value of a key under HKEY_CURRENT_USER
)

// Level is how sure a find is.
type Level string

const (
	High   Level = "high"   // 4 points or more: checked at first
	Medium Level = "medium" // 1 to 3
	Low    Level = "low"    // 0 or less
)

// Reason is one thing for or against a find, with its points.
type Reason struct {
	Text  string
	Delta int
}

// Item is a find.
type Item struct {
	ID   string
	Kind Kind
	// Path is a file's or directory's path, or a registry key's path under
	// HKEY_CURRENT_USER; Value is the name of a RegValue.
	Path, Value string
	Score       int
	Level       Level
	Reasons     []Reason
}

// Program is what is known of the program removed.
type Program struct {
	Name, Publisher string
	Location        string // where it was installed, when known
	ScoopName       string // of an app scoop removed
}

// Other is a program still installed: what it uses is no leftover.
type Other struct {
	Name, Publisher, Location string
}

// Config is where to look.
type Config struct {
	// DirRoots are the directories whose subdirectories may be the
	// program's: Program Files, ProgramData, AppData.
	DirRoots []string
	// RegRoot is the key under HKEY_CURRENT_USER whose subkeys may be the
	// program's settings: Software.
	RegRoot string
	// ShortcutDirs are the Start Menu's and the Desktop's directories.
	ShortcutDirs []string
	// RunKey is the key under HKEY_CURRENT_USER of programs started at
	// login.
	RunKey string
	// ScoopPersist are scoop's persist directories.
	ScoopPersist []string
	// Protected are directories never cleaned, nor any directory holding
	// them: AppShelf's own.
	Protected []string

	// links reads where shortcuts point; replaced in tests.
	links func(ctx context.Context, paths []string) map[string]string
}

// DefaultConfig looks where programs usually leave files and settings,
// protecting the directories given.
func DefaultConfig(protected ...string) Config {
	env := os.Getenv
	local := env("LOCALAPPDATA")
	var roots []string
	for _, r := range []string{env("ProgramFiles"), env("ProgramFiles(x86)"), env("ProgramData"), env("APPDATA"), local, filepath.Join(local, "Programs")} {
		if r != "" && r != "Programs" && !slices.Contains(roots, r) {
			roots = append(roots, r)
		}
	}
	var shortcuts []string
	for _, d := range []string{
		filepath.Join(env("APPDATA"), `Microsoft\Windows\Start Menu\Programs`),
		filepath.Join(env("ProgramData"), `Microsoft\Windows\Start Menu\Programs`),
		filepath.Join(env("USERPROFILE"), "Desktop"),
		filepath.Join(env("PUBLIC"), "Desktop"),
	} {
		if filepath.IsAbs(d) {
			shortcuts = append(shortcuts, d)
		}
	}
	var persist []string
	scoop := env("SCOOP")
	if scoop == "" && env("USERPROFILE") != "" {
		scoop = filepath.Join(env("USERPROFILE"), "scoop")
	}
	global := env("SCOOP_GLOBAL")
	if global == "" && env("ProgramData") != "" {
		global = filepath.Join(env("ProgramData"), "scoop")
	}
	for _, s := range []string{scoop, global} {
		if s != "" {
			persist = append(persist, filepath.Join(s, "persist"))
		}
	}
	return Config{
		DirRoots: roots, RegRoot: "Software", ShortcutDirs: shortcuts,
		RunKey: `Software\Microsoft\Windows\CurrentVersion\Run`, ScoopPersist: persist,
		Protected: protected, links: readLinks,
	}
}

// Find looks for what p left behind. others are the programs still
// installed.
func Find(ctx context.Context, cfg Config, p Program, others []Other) []Item {
	f := &finder{cfg: cfg, p: p, others: others, seen: map[string]bool{}}
	f.names = nameForms(p.Name, p.ScoopName)
	f.publisher = normPublisher(p.Publisher)
	f.location()
	f.dirs()
	f.scoop()
	f.registry()
	f.shortcuts(ctx)
	f.run()
	for i := range f.items {
		it := &f.items[i]
		it.ID = idOf(it.Kind, it.Path, it.Value)
		it.Score = 0
		for _, r := range it.Reasons {
			it.Score += r.Delta
		}
		it.Level = levelOf(it.Score)
	}
	slices.SortStableFunc(f.items, func(a, b Item) int { return b.Score - a.Score })
	return f.items
}

func levelOf(score int) Level {
	switch {
	case score >= 4:
		return High
	case score >= 1:
		return Medium
	}
	return Low
}

func idOf(k Kind, path, value string) string {
	h := sha1.Sum([]byte(string(k) + "\x00" + strings.ToLower(path) + "\x00" + strings.ToLower(value)))
	return hex.EncodeToString(h[:8])
}

type finder struct {
	cfg       Config
	p         Program
	others    []Other
	names     []string // the program's name in its forms
	publisher string
	items     []Item
	seen      map[string]bool
}

func (f *finder) add(it Item) {
	key := string(it.Kind) + "\x00" + strings.ToLower(it.Path) + "\x00" + strings.ToLower(it.Value)
	if f.seen[key] {
		return
	}
	f.seen[key] = true
	f.items = append(f.items, it)
}

// Reasons, with their points.
var (
	explicitDir     = Reason{"这是它的安装目录", 4}
	explicitLink    = Reason{"快捷方式指向它的安装目录", 4}
	explicitRun     = Reason{"开机启动项指向它的安装目录", 4}
	explicitPersist = Reason{"scoop 为它保留的数据", 4}
	namePerfect     = Reason{"名称与软件相同", 2}
	nameDodgy       = Reason{"名称只是部分相同", -2}
	underPublisher  = Reason{"在发布者的目录下", 2}
	emptyDir        = Reason{"目录是空的", 4}
	hasPrograms     = Reason{"里面还有程序文件", -4}
	manyFiles       = Reason{"文件很多", -2}
	nameUsed        = Reason{"还有其他已装软件用这个名称", -4}
	publisherUsed   = Reason{"这个发布者还有其他已装软件", -4}
	targetGone      = Reason{"指向的程序已不存在", 2}
	onlyLinks       = Reason{"里面只剩它的快捷方式", 4}
)

// location lists the program's own directory, when it is still there.
func (f *finder) location() {
	loc := f.p.Location
	if loc == "" || !detect.OwnDirectory(loc) || f.protected(loc) {
		return
	}
	if fi, err := os.Stat(loc); err != nil || !fi.IsDir() {
		return
	}
	reasons := []Reason{explicitDir}
	f.add(Item{Kind: Dir, Path: filepath.Clean(loc), Reasons: append(reasons, contentReasons(loc)...)})
}

// dirs lists the directories of the roots named as the program, directly
// or under its publisher's directory.
func (f *finder) dirs() {
	for _, root := range f.cfg.DirRoots {
		children, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, c := range children {
			if !c.IsDir() {
				continue
			}
			path := filepath.Join(root, c.Name())
			if m := f.match(c.Name()); m != nil {
				f.dir(path, []Reason{*m})
				continue
			}
			if f.publisher == "" || normPublisher(c.Name()) != f.publisher {
				continue
			}
			subs, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			found := 0
			for _, s := range subs {
				if !s.IsDir() {
					continue
				}
				if m := f.match(s.Name()); m != nil {
					f.dir(filepath.Join(path, s.Name()), []Reason{*m, underPublisher})
					found++
				}
			}
			// The publisher's directory itself, once it holds nothing else.
			if len(subs) == 0 || found == len(subs) && !f.publisherUsed() {
				reasons := []Reason{underPublisher}
				if len(subs) == 0 {
					reasons = append(reasons, emptyDir)
				}
				f.dir(path, reasons)
			}
		}
	}
}

func (f *finder) dir(path string, reasons []Reason) {
	if f.protected(path) {
		return
	}
	if f.nameUsedBy(filepath.Base(path)) {
		reasons = append(reasons, nameUsed)
	}
	if len(reasons) > 0 && reasons[0] == underPublisher && f.publisherUsed() {
		reasons = append(reasons, publisherUsed)
	}
	f.add(Item{Kind: Dir, Path: path, Reasons: append(reasons, contentReasons(path)...)})
}

// contentReasons look into a directory: empty, holding programs, or many
// files. At most 5000 entries are read.
func contentReasons(dir string) []Reason {
	files, programs := 0, false
	seen := 0
	filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		seen++
		if seen > 5000 {
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		files++
		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".exe", ".dll", ".sys", ".msi":
			programs = true
		}
		return nil
	})
	var out []Reason
	switch {
	case files == 0:
		out = append(out, emptyDir)
	case programs:
		out = append(out, hasPrograms)
	}
	if files > 200 {
		out = append(out, manyFiles)
	}
	return out
}

// scoop lists the data scoop keeps of an app it removed.
func (f *finder) scoop() {
	if f.p.ScoopName == "" || strings.ContainsAny(f.p.ScoopName, `\/.`) {
		return
	}
	for _, root := range f.cfg.ScoopPersist {
		path := filepath.Join(root, f.p.ScoopName)
		if fi, err := os.Stat(path); err == nil && fi.IsDir() && !f.protected(path) {
			f.add(Item{Kind: Dir, Path: path, Reasons: []Reason{explicitPersist}})
		}
	}
}

// protected reports a path never to clean: outside the roots it was found
// in, a directory every program shares, Windows, AppShelf's own, or one of
// another program's.
func (f *finder) protected(path string) bool {
	return Protected(f.cfg, path, f.others, f.p.Location)
}

// Protected reports a path never to clean: a drive, Windows, a directory
// every program shares or one of cfg's roots, AppShelf's own directories
// or one holding them, or a directory holding another program's or inside
// it. own is the removed program's location, which is not another's.
func Protected(cfg Config, path string, others []Other, own string) bool {
	p := cleanPath(path)
	if len(p) <= 3 || !filepath.IsAbs(path) || !detect.OwnDirectory(path) {
		return true
	}
	for _, r := range append(slices.Clone(cfg.DirRoots), append(cfg.ShortcutDirs, cfg.ScoopPersist...)...) {
		if p == cleanPath(r) {
			return true
		}
	}
	for _, d := range cfg.Protected {
		if d = cleanPath(d); d != "" && (p == d || within(d, p) || within(p, d)) {
			return true
		}
	}
	ownLoc := cleanPath(own)
	for _, o := range others {
		loc := cleanPath(o.Location)
		if loc == "" || loc == ownLoc || len(loc) <= 3 {
			continue
		}
		if p == loc || within(loc, p) || within(p, loc) {
			return true
		}
	}
	return false
}

// within reports whether path is inside dir.
func within(path, dir string) bool {
	return dir != "" && strings.HasPrefix(path, strings.TrimRight(dir, `\`)+`\`)
}

func cleanPath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "" {
		return ""
	}
	return strings.ToLower(strings.TrimRight(filepath.Clean(p), `\`))
}

// nameUsedBy reports another installed program named as name.
func (f *finder) nameUsedBy(name string) bool {
	n := normName(name)
	if n == "" {
		return false
	}
	for _, o := range f.others {
		on := normName(o.Name)
		if on == n || len(n) >= 4 && strings.Contains(on, n) {
			return true
		}
	}
	return false
}

func (f *finder) publisherUsed() bool {
	if f.publisher == "" {
		return false
	}
	for _, o := range f.others {
		if normPublisher(o.Publisher) == f.publisher {
			return true
		}
	}
	return false
}

// match tells how name, of a directory, key or shortcut, is the program's:
// nil when it is not.
func (f *finder) match(name string) *Reason {
	n := normName(name)
	if len([]rune(n)) < 2 || genericNames[n] {
		return nil
	}
	compact := strings.ReplaceAll(n, " ", "")
	for _, want := range f.names {
		if n == want || compact == strings.ReplaceAll(want, " ", "") {
			return &namePerfect
		}
	}
	for _, want := range f.names {
		short, long := n, want
		if len(short) > len(long) {
			short, long = long, short
		}
		if len([]rune(short)) >= 4 && strings.Contains(long, short) && len(short)*2 >= len(long) {
			return &nameDodgy
		}
	}
	return nil
}

// genericNames are names many programs share, never one program's.
var genericNames = map[string]bool{}

func init() {
	for _, n := range []string{
		"microsoft", "windows", "common files", "temp", "tmp", "cache", "caches", "programs", "packages",
		"google", "mozilla", "apple", "adobe", "intel", "nvidia", "amd", "realtek", "oracle", "java",
		"app", "apps", "application", "applications", "data", "config", "settings", "user data", "logs",
		"update", "updater", "installer", "uninstall", "setup", "tools", "bin", "lib", "shared", "common",
		"crashpad", "crashdumps", "plugins", "default", "local", "roaming", "locallow", "package cache",
		"policies", "classes", "clients", "wow6432node", "registeredapplications", "appdatalow",
		"electron", "node", "python", "git", "steam", "games", "software",
	} {
		genericNames[n] = true
	}
}

var (
	versionTail = regexp.MustCompile(`(?i)(\s+(v(ersion)?\s*)?\d+([.\-_+]\w+)*|\s*\((x64|x86|64[- ]?bit|32[- ]?bit|amd64|arm64)\)|\s+(x64|x86|64[- ]?bit|32[- ]?bit)|\s+版本.*)+$`)
	companyTail = regexp.MustCompile(`(?i)[\s,.]+(inc|llc|ltd|limited|co|corp|corporation|gmbh|ag|s\.?a|pty|bv|oy|ab)\.?$`)
)

// normName makes a program's name comparable: lower case, without its
// version, architecture and punctuation.
func normName(s string) string {
	s = strings.TrimSpace(s)
	for {
		t := strings.TrimSpace(versionTail.ReplaceAllString(s, ""))
		if t == s || t == "" {
			break
		}
		s = t
	}
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return b.String()
}

// normPublisher makes a publisher comparable, without "Inc." and the like.
func normPublisher(s string) string {
	s = strings.TrimSpace(s)
	for {
		t := strings.TrimSpace(companyTail.ReplaceAllString(s, ""))
		if t == s || t == "" {
			break
		}
		s = t
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, "有限公司"), "股份")
	n := normName(s)
	if genericNames[n] && n != "" {
		// "Microsoft" and the like publish too much to tell.
		return ""
	}
	return n
}

// nameForms are the program's names to compare with.
func nameForms(name, scoopName string) []string {
	var out []string
	for _, n := range []string{name, scoopName} {
		if n := normName(n); n != "" && !slices.Contains(out, n) && !genericNames[n] {
			out = append(out, n)
		}
	}
	return out
}
