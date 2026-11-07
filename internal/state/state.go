// Package state keeps what AppShelf knows about this machine that the
// sources cannot report again: what it installed from GitHub releases, how
// the user wants each package updated, and the updates done. winget and
// scoop are asked every time instead.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"appshelf/internal/jsonfile"
)

// SchemaVersion is the version of the state file. Fields that can be left
// out are added without changing it, as for the catalog.
const SchemaVersion = 1

// MaxHistory is how many updates the history keeps.
const MaxHistory = 200

// GitHubRecord is what AppShelf installed of an app from a GitHub release.
type GitHubRecord struct {
	Tag         string    `json:"tag"`
	AssetName   string    `json:"assetName"`
	Digest      string    `json:"digest,omitempty"`
	InstalledAt time.Time `json:"installedAt"`
}

// UpdateMode is how the updates of a package are handled.
type UpdateMode string

const (
	// ModeNotify tells the user of updates, who starts them: the default.
	ModeNotify UpdateMode = ""
	// ModeAuto updates the package when a check finds an update, when it
	// can be done without asking anything of the user.
	ModeAuto UpdateMode = "auto"
	// ModeIgnore never offers the package's updates.
	ModeIgnore UpdateMode = "ignore"
)

// UpdatePolicy is how the user wants a package updated, by its source key.
type UpdatePolicy struct {
	Mode UpdateMode `json:"mode,omitempty"`
	// Skip is a version not to offer: a newer one is offered again.
	Skip string `json:"skip,omitempty"`
	// Until puts off offering updates until then.
	Until time.Time `json:"until,omitzero"`
	// Name is the package's name, to list it when it is not installed.
	Name string `json:"name,omitempty"`
}

func (p UpdatePolicy) empty() bool { return p.Mode == ModeNotify && p.Skip == "" && p.Until.IsZero() }

// Hold is why an available update is not offered.
type Hold string

const (
	HoldNone   Hold = ""
	HoldIgnore Hold = "ignore" // the package's updates are ignored
	HoldSkip   Hold = "skip"   // this version is skipped
	HoldSnooze Hold = "snooze" // updates are put off for now
)

// Hold says whether an update to latest is held back at now, and why.
func (p UpdatePolicy) Hold(latest string, now time.Time) Hold {
	switch {
	case p.Mode == ModeIgnore:
		return HoldIgnore
	case p.Skip != "" && latest != "" && strings.EqualFold(p.Skip, latest):
		return HoldSkip
	case now.Before(p.Until):
		return HoldSnooze
	}
	return HoldNone
}

// UpdateRecord is an update done, for the history.
type UpdateRecord struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// Result is "succeeded", "skipped" or "failed".
	Result  string    `json:"result"`
	Message string    `json:"message,omitempty"`
	Auto    bool      `json:"auto,omitempty"`
	At      time.Time `json:"at"`
}

type file struct {
	SchemaVersion int                     `json:"schemaVersion"`
	GitHub        map[string]GitHubRecord `json:"github"`
	Updates       map[string]UpdatePolicy `json:"updates,omitempty"`
	// History is the updates done, newest first.
	History []UpdateRecord `json:"history,omitempty"`
}

// Store is the state file, safe for use from several goroutines. Each
// change is written at once.
type Store struct {
	path string
	mu   sync.Mutex
	f    file
}

// Open reads the state at path; a missing file is an empty state.
func Open(path string) (*Store, error) {
	s := &Store{path: path, f: file{SchemaVersion: SchemaVersion, GitHub: map[string]GitHubRecord{}, Updates: map[string]UpdatePolicy{}}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &f); err != nil {
		return nil, fmt.Errorf("本机状态文件无法读取：%w", err)
	}
	if f.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("本机状态文件版本 %d 比程序支持的 %d 新", f.SchemaVersion, SchemaVersion)
	}
	if f.GitHub == nil {
		f.GitHub = map[string]GitHubRecord{}
	}
	updates := map[string]UpdatePolicy{}
	for k, p := range f.Updates {
		if k = normalKey(k); k != "" && !p.empty() {
			updates[k] = p
		}
	}
	f.Updates = updates
	if len(f.History) > MaxHistory {
		f.History = f.History[:MaxHistory]
	}
	f.SchemaVersion = SchemaVersion
	s.f = f
	return s, nil
}

// GitHub returns the record of an app, by its ID.
func (s *Store) GitHub(appID string) (GitHubRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.f.GitHub[appID]
	return r, ok
}

// PutGitHub records what was installed of an app and saves the state.
func (s *Store) PutGitHub(appID string, r GitHubRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.f.GitHub[appID] = r
	return s.save()
}

// Forget drops the records of an app, as when it leaves the catalog. The
// update policy of its package stays: it is the package's, by source.
func (s *Store) Forget(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.f.GitHub[appID]; !ok {
		return nil
	}
	delete(s.f.GitHub, appID)
	return s.save()
}

func normalKey(key string) string { return strings.ToLower(strings.TrimSpace(key)) }

// Policy returns the update policy of a package, by its source key.
func (s *Store) Policy(key string) UpdatePolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Updates[normalKey(key)]
}

// Policies returns the update policies, by source key.
func (s *Store) Policies() map[string]UpdatePolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.f.Updates)
}

// SetPolicy changes the update policy of a package and saves the state; a
// policy left as the default is removed.
func (s *Store) SetPolicy(key string, fn func(p *UpdatePolicy)) error {
	key = normalKey(key)
	if key == "" {
		return errors.New("缺少软件来源")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, had := s.f.Updates[key]
	p := old
	fn(&p)
	if p.empty() {
		if !had {
			return nil
		}
		delete(s.f.Updates, key)
	} else {
		s.f.Updates[key] = p
	}
	if err := s.save(); err != nil {
		if had {
			s.f.Updates[key] = old
		} else {
			delete(s.f.Updates, key)
		}
		return err
	}
	return nil
}

// AddHistory records an update done, keeping the newest MaxHistory.
func (s *Store) AddHistory(r UpdateRecord) error {
	r.Key = normalKey(r.Key)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.f.History = slices.Insert(s.f.History, 0, r)
	if len(s.f.History) > MaxHistory {
		s.f.History = s.f.History[:MaxHistory]
	}
	return s.save()
}

// History returns the updates done, newest first.
func (s *Store) History() []UpdateRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.f.History)
}

// ClearHistory forgets the updates done.
func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.f.History) == 0 {
		return nil
	}
	s.f.History = nil
	return s.save()
}

func (s *Store) save() error {
	f := file{SchemaVersion: SchemaVersion, GitHub: maps.Clone(s.f.GitHub), Updates: maps.Clone(s.f.Updates), History: slices.Clone(s.f.History)}
	return jsonfile.WriteAtomic(s.path, f)
}
