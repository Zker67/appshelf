package api

import (
	"time"

	"appshelf/internal/catalog"
	"appshelf/internal/state"
)

// The types of the Updates service, as those of dto.go: mygo generate
// makes them TypeScript types of the same names.

// UpdateMode is how the updates of a package are handled.
type UpdateMode string

const (
	// UpdateNotify offers its updates, which the user starts: the default.
	UpdateNotify UpdateMode = "notify"
	// UpdateAuto updates it after a check, when AppShelf can do that
	// without asking anything of the user.
	UpdateAuto UpdateMode = "auto"
	// UpdateIgnore never offers its updates.
	UpdateIgnore UpdateMode = "ignore"
)

func modeOf(m state.UpdateMode) UpdateMode {
	switch m {
	case state.ModeAuto:
		return UpdateAuto
	case state.ModeIgnore:
		return UpdateIgnore
	}
	return UpdateNotify
}

// UpdateHold is why an available update is not offered.
type UpdateHold string

const (
	HoldNone    UpdateHold = ""
	HoldIgnored UpdateHold = "ignore" // its updates are ignored
	HoldSkipped UpdateHold = "skip"   // this version is skipped
	HoldSnoozed UpdateHold = "snooze" // put off until a time
)

// UpdateItem is a package with an update: an app of the catalog, an
// installed winget or scoop package, or both, once.
type UpdateItem struct {
	// Key is the package's source key, as "winget:git.git": what the
	// other methods of Updates take.
	Key     string         `json:"key"`
	Name    string         `json:"name"`
	Icon    string         `json:"icon"`
	Source  catalog.Source `json:"source"`
	Version string         `json:"version,omitempty"`
	Latest  string         `json:"latest"`
	// Major means the first number of the version goes up.
	Major bool       `json:"major,omitempty"`
	Mode  UpdateMode `json:"mode"`
	Hold  UpdateHold `json:"hold,omitempty"`
	// Skip is the version skipped; Until when updates are put off to.
	Skip  string    `json:"skip,omitempty"`
	Until time.Time `json:"until,omitzero"`
	// CanAuto means AppShelf can update it on its own; AutoNote says why
	// not.
	CanAuto  bool   `json:"canAuto"`
	AutoNote string `json:"autoNote,omitempty"`
	// Running means a program of it runs: updating may close it.
	Running bool `json:"running,omitempty"`
	// Pending means it waits or runs in the tasks.
	Pending bool `json:"pending,omitempty"`
	// AppID is the catalog's app of the package, when there is one.
	AppID string `json:"appId,omitempty"`
	// Note is what its source said of it.
	Note string `json:"note,omitempty"`
}

// UpdateRule is a package whose updates are not handled the default way:
// updated on its own, ignored, a version skipped or put off.
type UpdateRule struct {
	Key    string         `json:"key"`
	Name   string         `json:"name"`
	Icon   string         `json:"icon"`
	Source catalog.Source `json:"source"`
	Mode   UpdateMode     `json:"mode"`
	Skip   string         `json:"skip,omitempty"`
	Until  time.Time      `json:"until,omitzero"`
	// Installed means found on this machine or in the catalog.
	Installed bool   `json:"installed"`
	CanAuto   bool   `json:"canAuto"`
	AutoNote  string `json:"autoNote,omitempty"`
}

// SelfUpdate is AppShelf's own update, as its update window found it.
type SelfUpdate struct {
	// CanUpdate means this copy can update itself.
	CanUpdate bool   `json:"canUpdate"`
	Version   string `json:"version"`
	// Available is the newer version found, or "".
	Available string `json:"available,omitempty"`
}

// UpdateList is the updates and how they are checked.
type UpdateList struct {
	Items []UpdateItem `json:"items"`
	Rules []UpdateRule `json:"rules"`
	// Count is how many updates are offered: held ones are not.
	Count int `json:"count"`
	// Loaded means the catalog's states and the installed programs were
	// read; Checking that a check is under way.
	Loaded    bool       `json:"loaded"`
	Checking  bool       `json:"checking"`
	CheckedAt time.Time  `json:"checkedAt,omitzero"`
	NextCheck time.Time  `json:"nextCheck,omitzero"`
	Self      SelfUpdate `json:"self"`
}

// UpdateRecord is an update done.
type UpdateRecord struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Result is "succeeded", "skipped" or "failed".
	Result  TaskState `json:"result"`
	Message string    `json:"message,omitempty"`
	// Auto means AppShelf started it after a check.
	Auto bool      `json:"auto,omitempty"`
	At   time.Time `json:"at"`
}

// CheckInterval is how often the updates are checked while AppShelf runs.
type CheckInterval string

const (
	CheckOff     CheckInterval = "off"
	Check1h      CheckInterval = "1h"
	Check3h      CheckInterval = "3h"
	Check6h      CheckInterval = "6h"
	Check12h     CheckInterval = "12h"
	Check24h     CheckInterval = "24h"
	defaultCheck               = Check6h
)

var checkIntervals = map[CheckInterval]time.Duration{
	CheckOff: 0, Check1h: time.Hour, Check3h: 3 * time.Hour, Check6h: 6 * time.Hour,
	Check12h: 12 * time.Hour, Check24h: 24 * time.Hour,
}

// UpdateSettings are the user's choices for updates.
type UpdateSettings struct {
	CheckEvery CheckInterval `json:"checkEvery"`
	// Notify shows a system notification when a check finds updates, or
	// updates ran on their own, while the window is not in front.
	Notify bool `json:"notify"`
	// Background keeps AppShelf in the notification area when its window
	// closes, checking as it runs; else closing the window quits it. On out
	// of the box. The icon shows while AppShelf runs either way.
	Background bool `json:"background"`
	// OpenAtLogin starts AppShelf when the user logs in: in the
	// notification area with Background, else with its window.
	OpenAtLogin bool `json:"openAtLogin"`
	// CanBackground means this build has the notification area icon and
	// starting at login.
	CanBackground bool `json:"canBackground"`
	// SelfChecks checks for a newer AppShelf every day; SelfDownloads
	// installs it the next time AppShelf starts, without asking.
	SelfChecks    bool `json:"selfChecks"`
	SelfDownloads bool `json:"selfDownloads"`
	CanSelfUpdate bool `json:"canSelfUpdate"`
}
