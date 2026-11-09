package api

import (
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/queue"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
)

// The types here are what the page receives and sends: mygo generate makes
// them TypeScript types of the same names. Their texts are the user's, in
// Chinese; the page words the states itself.

// ChangeKind names what changed, for the page to read it again.
type ChangeKind string

const (
	// ChangedShelf is the catalog, its apps' states, or their being read.
	ChangedShelf ChangeKind = "shelf"
	// ChangedTasks is the install and update tasks.
	ChangedTasks ChangeKind = "tasks"
	// ChangedSources is the sources' availability and GitHub's quota.
	ChangedSources ChangeKind = "sources"
	// ChangedInstalled is the list of installed programs.
	ChangedInstalled ChangeKind = "installed"
	// ChangedUpdates is the update checks, the packages' update modes and
	// the history; the page also reads the updates again on the kinds above.
	ChangedUpdates ChangeKind = "updates"
)

// Change tells the page what changed. Changes close together come as one.
type Change struct {
	Kinds    []ChangeKind `json:"kinds"`
	Revision int64        `json:"revision"`
}

// StatusState is what is known of an app on this machine.
type StatusState string

const (
	StateUnknown         StatusState = "unknown"
	StateNotInstalled    StatusState = "notInstalled"
	StateInstalled       StatusState = "installed"
	StateUpdateAvailable StatusState = "updateAvailable"
	StateUnavailable     StatusState = "unavailable"
)

// AppStatus is the state of an app, as its source and detection rule say.
type AppStatus struct {
	State   StatusState `json:"state"`
	Version string      `json:"version,omitempty"`
	Latest  string      `json:"latest,omitempty"`
	Asset   string      `json:"asset,omitempty"`
	// External means installed some other way than its source.
	External bool   `json:"external,omitempty"`
	Note     string `json:"note,omitempty"`
}

func statusOf(st source.Status) *AppStatus {
	s := StateUnknown
	switch st.State {
	case source.NotInstalled:
		s = StateNotInstalled
	case source.Installed:
		s = StateInstalled
	case source.UpdateAvailable:
		s = StateUpdateAvailable
	case source.Unavailable:
		s = StateUnavailable
	}
	return &AppStatus{State: s, Version: st.Version, Latest: st.Latest, Asset: st.Asset, External: st.External, Note: st.Note}
}

// ShelfApp is an app of the catalog with its state.
type ShelfApp struct {
	App catalog.App `json:"app"`
	// Status is null until the state was read.
	Status *AppStatus `json:"status"`
	// Checking means the state is being read.
	Checking bool `json:"checking"`
	// Pending means the app waits or runs in the tasks.
	Pending bool `json:"pending"`
	// Icon is the app's path on the icon scheme.
	Icon string `json:"icon"`
	// Hold is why its update is not offered: ignored, skipped or put off.
	Hold UpdateHold `json:"hold,omitempty"`
}

// CategoryInfo is a category and how many apps it has.
type CategoryInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TagInfo is a tag and how many apps have it.
type TagInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ShelfSnapshot is the catalog and what is known of its apps.
type ShelfSnapshot struct {
	Apps          []ShelfApp     `json:"apps"`
	Categories    []CategoryInfo `json:"categories"`
	Tags          []TagInfo      `json:"tags"`
	Uncategorized int            `json:"uncategorized"`
	// Refreshing means all states are being read.
	Refreshing  bool      `json:"refreshing"`
	LastRefresh time.Time `json:"lastRefresh,omitzero"`
	// Notes are messages from opening, as a file that could not be read.
	Notes []string `json:"notes"`
}

// MergeCounts is what importing a catalog changes.
type MergeCounts struct {
	AddedApps       int `json:"addedApps"`
	MergedApps      int `json:"mergedApps"`
	AddedCategories int `json:"addedCategories"`
	AddedTags       int `json:"addedTags"`
}

func countsOf(s catalog.MergeSummary) MergeCounts {
	return MergeCounts{AddedApps: s.AddedApps, MergedApps: s.MergedApps, AddedCategories: s.AddedCategories, AddedTags: s.AddedTags}
}

// ImportPreview is a catalog file read for importing, not merged yet.
type ImportPreview struct {
	Token  string      `json:"token"`
	File   string      `json:"file"`
	Counts MergeCounts `json:"counts"`
}

// AddItem is a package to add to the catalog.
type AddItem struct {
	Name   string          `json:"name"`
	Source catalog.Source  `json:"source"`
	Detect *catalog.Detect `json:"detect,omitempty"`
}

// AddRequest adds packages with a category and tags, existing or new.
type AddRequest struct {
	Items       []AddItem `json:"items"`
	CategoryID  string    `json:"categoryId,omitempty"`
	NewCategory string    `json:"newCategory,omitempty"`
	TagIDs      []string  `json:"tagIds,omitempty"`
	NewTags     []string  `json:"newTags,omitempty"`
}

// AddFailure is a package that could not be added.
type AddFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

// AddResult is what adding did: the IDs of the apps added, and why the
// others were not.
type AddResult struct {
	Added  []string     `json:"added"`
	Failed []AddFailure `json:"failed"`
}

// TaskAction is installing, updating or removing.
type TaskAction string

const (
	ActionInstall   TaskAction = "install"
	ActionUpgrade   TaskAction = "upgrade"
	ActionUninstall TaskAction = "uninstall"
)

// queue returns the action of installing or updating apps of the catalog;
// ok is false for any other, as removing, which goes by the installed list.
func (a TaskAction) queue() (queue.Action, bool) {
	switch a {
	case ActionInstall:
		return queue.Install, true
	case ActionUpgrade:
		return queue.Upgrade, true
	}
	return 0, false
}

// TaskState is where a task is.
type TaskState string

const (
	TaskWaiting   TaskState = "waiting"
	TaskRunning   TaskState = "running"
	TaskSucceeded TaskState = "succeeded"
	TaskSkipped   TaskState = "skipped"
	TaskFailed    TaskState = "failed"
)

// TaskEntry is an install or update in the queue.
type TaskEntry struct {
	ID      int        `json:"id"`
	AppID   string     `json:"appId"`
	Name    string     `json:"name"`
	Icon    string     `json:"icon"`
	Action  TaskAction `json:"action"`
	State   TaskState  `json:"state"`
	Message string     `json:"message,omitempty"`
	HasLog  bool       `json:"hasLog"`
	Started time.Time  `json:"started,omitzero"`
	Ended   time.Time  `json:"ended,omitzero"`
	// Stage is what a running task does now, as "正在下载".
	Stage string `json:"stage,omitempty"`
	// Done and Total are the bytes of its download, when known: Total 0
	// means the size is unknown.
	Done  int64 `json:"done,omitempty"`
	Total int64 `json:"total,omitempty"`
	// Percent is how far its stage has come, 0 to 100, when the source
	// reports a percentage without bytes; 0 when unknown.
	Percent float64 `json:"percent,omitempty"`
	// Interactive means a failed removal can run again with its
	// uninstaller's window.
	Interactive bool `json:"interactive,omitempty"`
	// Leftovers means the program removed can be searched for what it
	// left behind.
	Leftovers bool `json:"leftovers,omitempty"`
}

// LeftoverKind is what a leftover is.
type LeftoverKind string

const (
	LeftoverDir      LeftoverKind = "dir"
	LeftoverShortcut LeftoverKind = "shortcut"
	LeftoverRegKey   LeftoverKind = "regkey"
	LeftoverRegValue LeftoverKind = "regvalue"
)

// LeftoverLevel is how sure a leftover is.
type LeftoverLevel string

const (
	LeftoverHigh   LeftoverLevel = "high"
	LeftoverMedium LeftoverLevel = "medium"
	LeftoverLow    LeftoverLevel = "low"
)

// LeftoverReason is one thing for or against a leftover, with its points.
type LeftoverReason struct {
	Text  string `json:"text"`
	Delta int    `json:"delta"`
}

// LeftoverItem is something a removed program may have left behind.
type LeftoverItem struct {
	ID   string       `json:"id"`
	Kind LeftoverKind `json:"kind"`
	// Path is a file's or a registry key's; Value the name of a registry
	// value.
	Path    string           `json:"path"`
	Value   string           `json:"value,omitempty"`
	Score   int              `json:"score"`
	Level   LeftoverLevel    `json:"level"`
	Reasons []LeftoverReason `json:"reasons"`
	// Checked is whether it is chosen at first: the sure ones only.
	Checked bool `json:"checked"`
}

// LeftoverScan is what was found of a removed program, under a token to
// clean it with.
type LeftoverScan struct {
	Token string         `json:"token"`
	Name  string         `json:"name"`
	Items []LeftoverItem `json:"items"`
}

// LeftoverOutcome is what cleaning one leftover did.
type LeftoverOutcome struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// LeftoverResult is what cleaning did.
type LeftoverResult struct {
	Outcomes []LeftoverOutcome `json:"outcomes"`
	Cleaned  int               `json:"cleaned"`
	Failed   int               `json:"failed"`
	// BackupDir is where the registry was exported first, "" when none.
	BackupDir string `json:"backupDir,omitempty"`
}

// UninstallPreview is what removing a program would do.
type UninstallPreview struct {
	Key       string        `json:"key"`
	Name      string        `json:"name"`
	Version   string        `json:"version,omitempty"`
	Publisher string        `json:"publisher,omitempty"`
	Icon      string        `json:"icon"`
	Kind      InstalledKind `json:"kind"`
	Runnable  bool          `json:"runnable"`
	// Plan is "卸载", or why it will not run.
	Plan string `json:"plan"`
	// Method is how it is removed, as "winget"; Installer the installer
	// that made a program of the registry, as "Inno Setup".
	Method    string `json:"method"`
	Installer string `json:"installer,omitempty"`
	// Command is what runs; Quiet means without asking.
	Command string `json:"command"`
	Quiet   bool   `json:"quiet"`
	// Notes are what to know first, as a program running.
	Notes    []string `json:"notes"`
	Location string   `json:"location,omitempty"`
	Size     int64    `json:"size,omitempty"`
	// Signer is who signed its uninstaller, Signed whether validly.
	Signer string `json:"signer,omitempty"`
	Signed bool   `json:"signed,omitempty"`
	// InShelf are the catalog's apps that are this program: they stay in
	// the catalog.
	InShelf []string `json:"inShelf"`
}

// TaskLog is part of a task's output, as clean lines.
type TaskLog struct {
	// Lines are the lines from Start on; ask from Next for the new ones.
	// Start is larger than asked when older lines were dropped.
	Lines []string `json:"lines"`
	Start int      `json:"start"`
	Next  int      `json:"next"`
	// Live is the line being written, which may still change.
	Live string `json:"live,omitempty"`
	// Done means the task ended: no more lines come.
	Done bool `json:"done"`
}

// TaskList is the queue.
type TaskList struct {
	Items []TaskEntry `json:"items"`
	// Running means the queue works through its items; Stopping that it
	// stops after the current one.
	Running  bool `json:"running"`
	Stopping bool `json:"stopping"`
}

// SourceInfo is whether a source can be used here.
type SourceInfo struct {
	Type    catalog.SourceType `json:"type"`
	Checked bool               `json:"checked"`
	OK      bool               `json:"ok"`
	Reason  string             `json:"reason,omitempty"`
	// Fixable means the source is unavailable and AppShelf can fix it:
	// install winget's module, or scoop.
	Fixable bool `json:"fixable,omitempty"`
}

// RateInfo is a GitHub request quota as last reported.
type RateInfo struct {
	Known     bool      `json:"known"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     time.Time `json:"reset,omitzero"`
}

func rateOf(r github.Rate) RateInfo {
	return RateInfo{Known: r.Known, Limit: r.Limit, Remaining: r.Remaining, Reset: r.Reset}
}

// SourcesOverview is the sources' state.
type SourcesOverview struct {
	Sources []SourceInfo `json:"sources"`
	// SetupSeen means the first prompt to set up the sources was answered.
	SetupSeen   bool `json:"setupSeen"`
	GitHubToken bool `json:"githubToken"`
	// GitHubTokenFrom is where the token comes from; the token itself
	// never leaves Go.
	GitHubTokenFrom  TokenSource `json:"githubTokenFrom"`
	GitHubRate       RateInfo    `json:"githubRate"`
	GitHubSearchRate RateInfo    `json:"githubSearchRate"`
}

// SearchHit is a package a search found, with what this machine and the
// catalog know of it.
type SearchHit struct {
	Type catalog.SourceType `json:"type"`
	// ID is a winget ID, a scoop bucket/name or a GitHub owner/name.
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Stars    int    `json:"stars,omitzero"`
	Archived bool   `json:"archived,omitzero"`
	Icon     string `json:"icon"`
	// InShelf is the ID of the catalog's app with this source.
	InShelf string `json:"inShelf,omitempty"`
	// Installed means found among the installed programs, for display.
	Installed bool `json:"installed,omitempty"`
}

// SearchBatch is the results of one source.
type SearchBatch struct {
	Type catalog.SourceType `json:"type"`
	Hits []SearchHit        `json:"hits"`
	// Error says why the source found nothing; Unavailable that it cannot
	// be used here.
	Error       string `json:"error,omitempty"`
	Unavailable bool   `json:"unavailable,omitempty"`
}

// GitHubSearch is what a search of GitHub found, with the quota left.
type GitHubSearch struct {
	Hits []SearchHit `json:"hits"`
	Rate RateInfo    `json:"rate"`
}

// GitHubCheck is the release a GitHub app would install from.
type GitHubCheck struct {
	Tag   string `json:"tag"`
	Asset string `json:"asset"`
}

// InstalledKind is where an installed program comes from.
type InstalledKind string

const (
	InstalledWinget InstalledKind = "winget"
	InstalledScoop  InstalledKind = "scoop"
	InstalledLocal  InstalledKind = "local"
	InstalledStore  InstalledKind = "store"
)

// InstalledEntry is a program installed on this machine.
type InstalledEntry struct {
	Key       string        `json:"key"`
	Name      string        `json:"name"`
	Version   string        `json:"version,omitempty"`
	Publisher string        `json:"publisher,omitempty"`
	Kind      InstalledKind `json:"kind"`
	// Source is the source to add it with: a winget package or a scoop app.
	Source *catalog.Source `json:"source"`
	// Update is the newer version its source has.
	Update string `json:"update,omitempty"`
	// InShelf are the IDs of the catalog's apps that are this program.
	InShelf []string `json:"inShelf"`
	Icon    string   `json:"icon"`
	// Rule is a detection rule that finds this program, for adding it with
	// a source found by searching.
	Rule string `json:"rule"`
	// Mode is how its updates are handled, of a program with a source.
	Mode UpdateMode `json:"mode,omitempty"`
	// Hold is why its update is not offered.
	Hold UpdateHold `json:"hold,omitempty"`
	// Size is the registry's estimate in bytes; Installed the date it was
	// installed, YYYY-MM-DD; Location its directory. Each is empty when
	// unknown.
	Size      int64  `json:"size,omitempty"`
	Installed string `json:"installed,omitempty"`
	Location  string `json:"location,omitempty"`
	// LastUsed is when the user last opened it, as Windows recorded its
	// launches from the Start menu, the taskbar, the desktop and Explorer;
	// the time it was read when it is running. Empty when no record says.
	LastUsed time.Time `json:"lastUsed,omitzero"`
}

// InstalledList is what is installed.
type InstalledList struct {
	Entries []InstalledEntry `json:"entries"`
	Loaded  bool             `json:"loaded"`
	Loading bool             `json:"loading"`
	// Partial means only the registry's programs are listed yet: sources,
	// updates, scoop and the Store's apps are still being read.
	Partial bool      `json:"partial"`
	At      time.Time `json:"at,omitzero"`
	Notes   []string  `json:"notes"`
	// UsageTracked means Windows records the programs the user opens; off,
	// no entry has a LastUsed.
	UsageTracked bool `json:"usageTracked"`
}

// TokenSource is where the GitHub token comes from.
type TokenSource string

const (
	TokenNone  TokenSource = ""      // no token
	TokenSaved TokenSource = "saved" // entered in the settings, kept encrypted
	TokenEnv   TokenSource = "env"   // GH_TOKEN or GITHUB_TOKEN
)

// ThemeSource is the appearance chosen.
type ThemeSource string

const (
	ThemeSystem ThemeSource = "system"
	ThemeLight  ThemeSource = "light"
	ThemeDark   ThemeSource = "dark"
)

// AboutInfo is AppShelf itself.
type AboutInfo struct {
	Version string `json:"version"`
	// CanUpdate means this copy can update itself.
	CanUpdate bool        `json:"canUpdate"`
	LogsDir   string      `json:"logsDir"`
	DataDir   string      `json:"dataDir"`
	Theme     ThemeSource `json:"theme"`
	// IconBase is the root of the icon scheme, for the icons' paths.
	IconBase string `json:"iconBase"`
}
