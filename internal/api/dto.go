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

// TaskAction is installing or updating.
type TaskAction string

const (
	ActionInstall TaskAction = "install"
	ActionUpgrade TaskAction = "upgrade"
)

func (a TaskAction) queue() queue.Action {
	if a == ActionUpgrade {
		return queue.Upgrade
	}
	return queue.Install
}

// PreviewEntry is what installing or updating an app would do.
type PreviewEntry struct {
	AppID    string `json:"appId"`
	Name     string `json:"name"`
	Icon     string `json:"icon"`
	Runnable bool   `json:"runnable"`
	// Plan is "安装", "更新 1.0 → 1.1", or why not.
	Plan   string         `json:"plan"`
	Source catalog.Source `json:"source"`
	// Asset is the installer a GitHub release gives, of Release.
	Asset   string `json:"asset,omitempty"`
	Release string `json:"release,omitempty"`
	// Note is what the source said of the app, as a limit it reached.
	Note string `json:"note,omitempty"`
	// Running means a program of the app runs: updating may close it.
	Running bool `json:"running,omitempty"`
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
	// Fixable means AppShelf can fix it: install winget's module.
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
	Sources          []SourceInfo `json:"sources"`
	CanInstallModule bool         `json:"canInstallModule"`
	GitHubToken      bool         `json:"githubToken"`
	GitHubRate       RateInfo     `json:"githubRate"`
	GitHubSearchRate RateInfo     `json:"githubSearchRate"`
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
}

// InstalledList is what is installed.
type InstalledList struct {
	Entries []InstalledEntry `json:"entries"`
	Loaded  bool             `json:"loaded"`
	Loading bool             `json:"loading"`
	At      time.Time        `json:"at,omitzero"`
	Notes   []string         `json:"notes"`
}

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
