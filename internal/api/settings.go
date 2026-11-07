package api

import "appshelf/internal/jsonfile"

// settings are the user's preferences of the app itself, in settings.json
// of the data directory.
type settings struct {
	Theme ThemeSource `json:"theme,omitempty"`
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
	return s
}

// LoadTheme reads the appearance chosen, for the app to start in it.
func LoadTheme(path string) ThemeSource { return loadSettings(path).Theme }

func saveSettings(path string, s settings) error {
	if path == "" {
		return nil
	}
	return jsonfile.WriteAtomic(path, s)
}
