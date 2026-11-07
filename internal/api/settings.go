package api

import "appshelf/internal/jsonfile"

// settings are the user's preferences of the app itself, in settings.json
// of the data directory.
type settings struct {
	Theme   ThemeSource    `json:"theme,omitempty"`
	Updates updatesChoices `json:"updates,omitzero"`
}

// updatesChoices are the update settings kept in settings.json, written so
// that a file without them means the defaults: checking every 6 hours,
// notifications on, no notification area icon. Starting at login and
// AppShelf's own checks are kept by the system and MyGo's updater.
type updatesChoices struct {
	CheckEvery CheckInterval `json:"checkEvery,omitempty"`
	Quiet      bool          `json:"quiet,omitempty"`
	Background bool          `json:"background,omitempty"`
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
func LoadBackground(path string) bool { return loadSettings(path).Updates.Background }

func saveSettings(path string, s settings) error {
	if path == "" {
		return nil
	}
	return jsonfile.WriteAtomic(path, s)
}
