// AppShelf is a personal software store for Windows: a catalog of apps
// with categories and tags, installed in batches from winget, scoop and
// GitHub releases, never twice and never the wrong package.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

	"appshelf/internal/api"
	"appshelf/internal/core"
	"appshelf/internal/detect"
	"appshelf/internal/discover"
	"appshelf/internal/icons"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
	"appshelf/internal/state"
)

var catalogFiles = []mygo.FileFilter{{Name: "AppShelf 清单", Extensions: []string{"json"}}}

func main() {
	app := mygo.App
	// The name decides the data directory, in development as when built.
	app.SetName("AppShelf")
	if !app.RequestSingleInstanceLock() {
		return // the running AppShelf comes to the front
	}
	mygo.Use(updater.Plugin)

	// Development builds, and copies not installed where they can be
	// replaced, cannot update themselves.
	var canUpdate atomic.Bool
	shelf := api.New(api.Hooks{
		SaveFile: func(ctx context.Context, title, name string) (string, error) {
			return mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: mygo.CallerWindow(ctx), Title: title, DefaultPath: name, Filters: catalogFiles})
		},
		OpenFile: func(ctx context.Context, title string) (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Title: title, Filters: catalogFiles})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		},
		OpenPath:     mygo.Shell.OpenPath,
		OpenURL:      mygo.Shell.OpenExternal,
		CanUpdate:    canUpdate.Load,
		CheckUpdates: updater.CheckForUpdates,
		SetTheme:     func(t api.ThemeSource) { mygo.Theme.SetSource(mygo.ThemeSource(t)) },
	})
	mygo.Bind(shelf.Services()...)

	app.WhenReady(func() {
		dataDir, err := app.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		logsDir, err := app.Path(mygo.PathLogs)
		if err != nil {
			logsDir = filepath.Join(dataDir, "logs")
		}
		tempDir, err := app.Path(mygo.PathTemp)
		if err != nil {
			tempDir = os.TempDir()
		}
		cacheDir, err := app.Path(mygo.PathCache)
		if err != nil {
			cacheDir = filepath.Join(dataDir, "cache")
		}
		settings := filepath.Join(dataDir, "settings.json")
		mygo.Theme.SetSource(mygo.ThemeSource(api.LoadTheme(settings)))
		canUpdate.Store(mygo.Updater.Enabled())
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}

		var (
			w  *winget.Source
			sc *scoop.Source
			gh *github.Source
		)
		ctx := context.Background()
		c, err := core.Open(ctx, core.Config{
			Paths: core.Paths{
				Catalog: filepath.Join(dataDir, "catalog.json"),
				State:   filepath.Join(dataDir, "state.json"),
				Logs:    logsDir,
			},
			Sources: func(store *state.Store) []source.Source {
				w, sc = winget.New(), scoop.New()
				gh = github.New(github.Options{Store: store, TempDir: filepath.Join(tempDir, "AppShelf"), Token: token, Version: app.Version()})
				return []source.Source{w, sc, gh}
			},
			Post: shelf.Post,
		})
		if err != nil {
			mygo.Dialog.Error("AppShelf 无法启动", err.Error())
			app.Quit()
			return
		}

		// Icons come from Go on a scheme of their own, registered before
		// the window that shows them.
		iconService := icons.New(icons.Options{
			Dir:          filepath.Join(cacheDir, "icons"),
			FileIcon:     mygo.Shell.FileIcon,
			PackagedApps: packagedApps,
		})
		mygo.Protocol.Handle("icon", iconService)
		iconBase := "icon://localhost"
		if runtime.GOOS == "windows" {
			iconBase = "http://icon.localhost"
		}
		details := discover.NewDetails(discover.Fetchers{
			WingetLatest:  w.LatestVersion,
			ScoopInfo:     sc.Info,
			GitHubRepo:    gh.Repository,
			GitHubRelease: gh.Describe,
		}, filepath.Join(cacheDir, "details"), "")

		shelf.Start(ctx, api.Config{
			Core:     c,
			Sources:  api.SourceAPIs{Winget: w, Scoop: sc, GitHub: gh},
			Icons:    iconService,
			Details:  details,
			Scan:     detect.Scan,
			Version:  app.Version(),
			DataDir:  dataDir,
			Settings: settings,
			IconBase: iconBase,
		})

		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "AppShelf 软件商城",
			URL:             "/",
			Width:           1240,
			Height:          800,
			MinWidth:        960,
			MinHeight:       620,
			StateKey:        "main",
			TitleBarStyle:   mygo.TitleBarHidden,
			TitleBarHeight:  44,
			BackgroundColor: "light-dark(#f4f6fb, #0b1020)",
		})
		// The page's lists have menus of their own; elsewhere only text has
		// one, to copy and paste: a store window has no Back or Reload.
		win.Page().OnContextMenu(func(e *mygo.ContextMenuEvent) {
			if !e.Editable && e.SelectionText == "" {
				e.PreventDefault()
				return
			}
			e.Menu.Remove(mygo.ContextMenuBack, mygo.ContextMenuForward, mygo.ContextMenuReload, mygo.ContextMenuStop, mygo.ContextMenuPrint)
		})
		app.OnSecondInstance(func([]string, string) {
			win.Restore()
			win.Show()
			win.Focus()
		})
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// packagedApps lists the Start menu's app packages by family name, for
// their icons: their AppUserModelID is <family>!<app>.
func packagedApps() (map[string]string, error) {
	apps, err := mygo.Shell.InstalledApps()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, a := range apps {
		if family, _, ok := strings.Cut(a.ID, "!"); ok {
			if _, seen := out[family]; !seen {
				out[family] = a.Path
			}
		}
	}
	return out, nil
}
