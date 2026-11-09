package api

import (
	"errors"
	"net/url"
	"strings"
)

// About is AppShelf itself: its version and updates, its files, its
// appearance, and opening web pages.
type About struct{ a *App }

// Info returns AppShelf's version and where its files are.
func (s *About) Info() AboutInfo {
	a := s.a
	<-a.ready
	out := AboutInfo{
		Version:   a.cfg.Version,
		CanUpdate: a.canUpdate(),
		DataDir:   a.cfg.DataDir,
		IconBase:  a.cfg.IconBase,
	}
	a.read(func() { out.LogsDir = a.core.Paths().Logs })
	a.mu.Lock()
	out.Theme = a.settings.Theme
	a.mu.Unlock()
	return out
}

// OpenLogs opens the directory of the install logs.
func (s *About) OpenLogs() error {
	a := s.a
	<-a.ready
	var dir string
	a.read(func() { dir = a.core.LogsDir() })
	return a.openPath(dir)
}

// OpenDataDir opens the directory of the catalog and AppShelf's files.
func (s *About) OpenDataDir() error {
	<-s.a.ready
	return s.a.openPath(s.a.cfg.DataDir)
}

func (a *App) openPath(path string) error {
	if a.hooks.OpenPath == nil || path == "" {
		return errors.New("无法打开目录")
	}
	return a.hooks.OpenPath(path)
}

// OpenURL opens a web page in the default browser: http and https only.
func (s *About) OpenURL(link string) error {
	a := s.a
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("只能打开 http 或 https 网址")
	}
	if a.hooks.OpenURL == nil {
		return errors.New("无法打开网址")
	}
	return a.hooks.OpenURL(u.String())
}

// SetTheme follows the system's appearance, or forces light or dark, and
// keeps the choice.
func (s *About) SetTheme(theme ThemeSource) error {
	a := s.a
	<-a.ready
	switch theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		return errors.New("未知的外观")
	}
	a.mu.Lock()
	a.settings.Theme = theme
	st := a.settings
	a.mu.Unlock()
	if a.hooks.SetTheme != nil {
		a.hooks.SetTheme(theme)
	}
	return saveSettings(a.cfg.Settings, st)
}

func (a *App) canUpdate() bool {
	return a.hooks.CanUpdate != nil && a.hooks.SelfCheck != nil && a.hooks.CanUpdate()
}
