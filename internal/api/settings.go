package api

import (
	"slices"

	"appshelf/internal/jsonfile"
)

// settings are the user's preferences of the app itself, in settings.json
// of the data directory.
type settings struct {
	Theme   ThemeSource    `json:"theme,omitempty"`
	Updates updatesChoices `json:"updates,omitzero"`
	// SourceSetupSeen means the first prompt to install winget's module
	// or scoop was answered.
	SourceSetupSeen bool `json:"sourceSetupSeen,omitempty"`
}

// updatesChoices are the update settings kept in settings.json, written so
// that a file without them means the defaults: checking every 6 hours,
// notifications on, closing the window leaving AppShelf in the
// notification area, AppShelf checking for a newer version of itself as it
// starts. Starting at login is kept by the system.
type updatesChoices struct {
	CheckEvery CheckInterval `json:"checkEvery,omitempty"`
	Quiet      bool          `json:"quiet,omitempty"`
	// ExitOnClose quits AppShelf when its window closes.
	ExitOnClose bool `json:"exitOnClose,omitempty"`
	// SelfCheck is when AppShelf checks for a newer version of itself, ""
	// meaning at launch; SelfEvery the hours between checks of
	// SelfCheckInterval, 0 meaning a day.
	SelfCheck SelfCheckMode `json:"selfCheck,omitempty"`
	SelfEvery int           `json:"selfEvery,omitempty"`
}

func (c updatesChoices) selfCheck() SelfCheckMode {
	switch c.SelfCheck {
	case SelfCheckOff, SelfCheckInterval:
		return c.SelfCheck
	}
	return SelfCheckLaunch
}

func (c updatesChoices) selfEvery() int {
	if slices.Contains(selfIntervals, c.SelfEvery) {
		return c.SelfEvery
	}
	return defaultSelfEvery
}

func loadSettings(path string) settings {
	var s settings
	if path != "" {
		_ = jsonfile.Read(path, &s)
	}
	switch s.Theme {
	case ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
	}
	if _, ok := checkIntervals[s.Updates.CheckEvery]; !ok {
		s.Updates.CheckEvery = defaultCheck
	}
	return s
}

// LoadTheme reads the appearance chosen, for the app to start in it.
func LoadTheme(path string) ThemeSource { return loadSettings(path).Theme }

// LoadBackground reads whether AppShelf stays in the notification area when
// its window closes, for the app to start so.
func LoadBackground(path string) bool { return !loadSettings(path).Updates.ExitOnClose }

func saveSettings(path string, s settings) error {
	if path == "" {
		return nil
	}
	return jsonfile.WriteAtomic(path, s)
}
