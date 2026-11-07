// Package state keeps what AppShelf knows about this machine that the
// sources cannot report again: what it installed from GitHub releases.
// winget and scoop are asked every time instead.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"sync"
	"time"

	"appshelf/internal/jsonfile"
)

// SchemaVersion is the version of the state file.
const SchemaVersion = 1

// GitHubRecord is what AppShelf installed of an app from a GitHub release.
type GitHubRecord struct {
	Tag         string    `json:"tag"`
	AssetName   string    `json:"assetName"`
	Digest      string    `json:"digest,omitempty"`
	InstalledAt time.Time `json:"installedAt"`
}

type file struct {
	SchemaVersion int                     `json:"schemaVersion"`
	GitHub        map[string]GitHubRecord `json:"github"`
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
	s := &Store{path: path, f: file{SchemaVersion: SchemaVersion, GitHub: map[string]GitHubRecord{}}}
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

// Forget drops the records of an app, as when it leaves the catalog.
func (s *Store) Forget(appID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.f.GitHub[appID]; !ok {
		return nil
	}
	delete(s.f.GitHub, appID)
	return s.save()
}

func (s *Store) save() error {
	f := file{SchemaVersion: SchemaVersion, GitHub: maps.Clone(s.f.GitHub)}
	return jsonfile.WriteAtomic(s.path, f)
}
