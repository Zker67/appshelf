// Package source defines what AppShelf asks of an install source (winget,
// scoop, GitHub releases): whether it is available, what of the catalog is
// installed or has an update, and installing and updating.
package source

import (
	"context"
	"io"

	"appshelf/internal/catalog"
	"appshelf/internal/state"
)

// State is what is known of an app on this machine.
type State int

const (
	// Unknown means the state could not be established: AppShelf must not
	// install the app.
	Unknown State = iota
	NotInstalled
	Installed
	UpdateAvailable
	// Unavailable means the app's source cannot be used on this machine.
	Unavailable
)

func (s State) String() string {
	switch s {
	case NotInstalled:
		return "未安装"
	case Installed:
		return "已安装"
	case UpdateAvailable:
		return "可更新"
	case Unavailable:
		return "不可用"
	}
	return "未知"
}

// Status is the state of one app.
type Status struct {
	State   State
	Version string // installed version, when known
	Latest  string // the newer version, when State is UpdateAvailable
	// Asset is the installer a GitHub release would install.
	Asset string
	// External is true when the app was found by its detection rule, not
	// by its source: it is installed some other way.
	External bool
	// Note explains the state: why it is Unknown or Unavailable, or what is
	// worth knowing about an installed app, as an update check that failed.
	Note string
}

// Blocked reports whether an install or update must not run: the state is
// unknown or the source unavailable.
func (s Status) Blocked() bool { return s.State == Unknown || s.State == Unavailable }

// IsInstalled reports whether the app is on the machine, by any means.
func (s Status) IsInstalled() bool { return s.State == Installed || s.State == UpdateAvailable }

// UnknownStatus returns an Unknown status with the reason.
func UnknownStatus(reason string) Status { return Status{State: Unknown, Note: reason} }

// Availability tells whether a source can be used.
type Availability struct {
	OK     bool
	Reason string // why not, or a note when OK
	// Fixable means AppShelf can fix it, as by installing the WinGet
	// PowerShell module.
	Fixable bool
}

// Result is what an install or update did, beyond succeeding.
type Result struct {
	// AlreadyInstalled means the source found the app installed and did
	// nothing.
	AlreadyInstalled bool
	// RebootRequired means the installer asks to restart Windows.
	RebootRequired bool
	// GitHub is what was installed from a release, to remember once the
	// app is found installed.
	GitHub *state.GitHubRecord
}

// Progress is how far an install, update or uninstall has come, as far as
// it can be read: a stage, and the bytes of a download.
type Progress struct {
	Stage string // what it does now, as "正在下载"
	Done  int64  // bytes downloaded, 0 when none or unknown
	Total int64  // the size of the download, 0 when unknown
	// Percent is how far the stage has come, 0 to 100, as the source
	// reports it without bytes; 0 when unknown.
	Percent float64
}

// ProgressReporter is a log that also shows the progress of the work it
// records.
type ProgressReporter interface {
	ReportProgress(p Progress)
}

// ReportProgress tells the log of an install or update how far it has
// come, when the log shows progress; other logs ignore it.
func ReportProgress(log io.Writer, p Progress) {
	if r, ok := log.(ProgressReporter); ok {
		r.ReportProgress(p)
	}
}

// SearchResult is a package a search found, to fill in an app's source.
type SearchResult struct {
	ID      string // winget's package ID, or scoop's bucket/name
	Name    string
	Version string
	Source  string // winget's source, or scoop's bucket
}

// Source is one install source. Its methods are safe to call from several
// goroutines; apps passed to it are of its type.
type Source interface {
	Type() catalog.SourceType
	// Check tells whether the source can be used on this machine.
	Check(ctx context.Context) Availability
	// Statuses returns the state of each app, by app ID. An error means no
	// state could be read at all.
	Statuses(ctx context.Context, apps []catalog.App) (map[string]Status, error)
	// Recheck reads the state of one app again, as before and after
	// installing it.
	Recheck(ctx context.Context, app catalog.App) (Status, error)
	Install(ctx context.Context, app catalog.App, log io.Writer) (Result, error)
	Upgrade(ctx context.Context, app catalog.App, log io.Writer) (Result, error)
}

// Searcher is a source that finds packages by a query.
type Searcher interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}
