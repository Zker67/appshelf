// AppShelf is a personal software store for Windows: a catalog of apps
// with categories and tags, installed in batches from winget, scoop and
// GitHub releases, never twice and never the wrong package.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"

	"appshelf/internal/api"
	"appshelf/internal/core"
	"appshelf/internal/detect"
	"appshelf/internal/discover"
	"appshelf/internal/icons"
	"appshelf/internal/procs"
	"appshelf/internal/runner"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
	"appshelf/internal/state"
	"appshelf/internal/uninstall"
	"appshelf/internal/usage"
	"appshelf/internal/watch"
)

var catalogFiles = []mygo.FileFilter{{Name: "AppShelf 清单", Extensions: []string{"json"}}}

func main() {
	app := mygo.App
	// The name decides the data directory, in development as when built.
	app.SetName("AppShelf")
	if !app.RequestSingleInstanceLock() {
		return // the running AppShelf comes to the front
	}
	// AppShelf's own updates are read through a feed that copes with a
	// proxy failing on github.com (selfupdate.go), checked and installed by
	// the api package, without a window of their own.
	useUpdateFeed()

	// Development builds, and copies not installed where they can be
	// replaced, cannot update themselves.
	var canUpdate atomic.Bool
	// The window, once made: the notification area icon and notifications
	// show it again.
	var win atomic.Pointer[mygo.Window]
	showWindow := func() {
		if w := win.Load(); w != nil {
			w.Restore()
			w.Show()
			w.Focus()
		}
	}
	var shelf *api.App
	// showPage shows the window on one of its pages, or its tasks.
	showPage := func(page string) {
		showWindow()
		shelf.ShowPage(page)
	}
	bg := &tray{
		open:   showWindow,
		show:   showPage,
		status: func() api.TrayStatus { return shelf.TrayStatus() },
		check:  func() { shelf.CheckUpdates() },
	}
	bg.updateAll = func() {
		n, err := shelf.UpdateAll(context.Background())
		title, body := fmt.Sprintf("开始更新 %d 个软件", n), "点击查看任务进度"
		if err != nil {
			title, body = "无法全部更新", err.Error()
		}
		note := mygo.NewNotification(mygo.NotificationOptions{ID: "appshelf-tray", Title: title, Body: body})
		note.OnClick(func() { showPage("tasks") })
		if err := note.Show(); err != nil {
			log.Println("notification:", err)
		}
	}
	shelf = api.New(api.Hooks{
		// The pages hear of changes, and the notification area icon.
		Emit: func(c api.Change) {
			api.Changed.Broadcast(c)
			bg.changed()
		},
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
		OpenPath:  mygo.Shell.OpenPath,
		OpenURL:   mygo.Shell.OpenExternal,
		CanUpdate: canUpdate.Load,
		SelfCheck: func(ctx context.Context) (*api.SelfRelease, error) {
			up, err := mygo.Updater.Check(ctx)
			if err != nil || up == nil {
				return nil, err
			}
			return &api.SelfRelease{Version: up.Version, Install: up.Install}, nil
		},
		Relaunch: app.Relaunch,
		SetTheme: func(t api.ThemeSource) { mygo.Theme.SetSource(mygo.ThemeSource(t)) },
		Notify: func(title, body string) {
			// One notification of updates at a time: a newer one replaces it.
			n := mygo.NewNotification(mygo.NotificationOptions{ID: "appshelf-updates", Title: title, Body: body})
			n.OnClick(func() { showPage("updates") })
			if err := n.Show(); err != nil {
				log.Println("notification:", err)
			}
		},
		NotifyTasks: func(title, body string) {
			n := mygo.NewNotification(mygo.NotificationOptions{ID: "appshelf-tasks", Title: title, Body: body})
			n.OnClick(func() { showPage("tasks") })
			if err := n.Show(); err != nil {
				log.Println("notification:", err)
			}
		},
		Active:         app.IsActive,
		SetBackground:  bg.setStay,
		OpenAtLogin:    app.OpenAtLogin,
		SetOpenAtLogin: app.SetOpenAtLogin,
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

		w, sc := winget.New(), scoop.New()
		var gh *github.Source
		// AppShelf never removes itself: its directory is that of its
		// executable.
		selfDir := ""
		if exe, err := os.Executable(); err == nil {
			selfDir = filepath.Dir(exe)
		}
		ctx := context.Background()
		c, err := core.Open(ctx, core.Config{
			Paths: core.Paths{
				Catalog: filepath.Join(dataDir, "catalog.json"),
				State:   filepath.Join(dataDir, "state.json"),
				Logs:    logsDir,
			},
			Sources: func(store *state.Store) []source.Source {
				gh = github.New(github.Options{Store: store, TempDir: filepath.Join(tempDir, "AppShelf"), Token: token, Version: app.Version()})
				return []source.Source{w, sc, gh}
			},
			Post:        shelf.Post,
			Progress:    shelf.Progress,
			Uninstaller: uninstall.New(uninstall.Options{SelfDir: selfDir}, w, sc),
			Installers: func() *procs.Installers {
				return procs.WatchInstallers(procs.Live(), procs.TempDirs())
			},
			InstallerBusy: runner.InstallerBusy,
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
			WingetLatest:   w.LatestVersion,
			ScoopInfo:      sc.Info,
			GitHubRepo:     gh.Repository,
			GitHubRelease:  gh.Describe,
			GitHubReleases: gh.Releases,
		}, filepath.Join(cacheDir, "details"), "")

		shelf.Start(ctx, api.Config{
			Core:    c,
			Sources: api.SourceAPIs{Winget: w, Scoop: sc, GitHub: gh},
			Icons:   iconService,
			Details: details,
			Scan:    detect.Scan,
			Usage:   usage.Read,
			// The installed list of the last run, shown while it is read.
			InstalledCache: filepath.Join(cacheDir, "installed.json"),
			Watch: func(ctx context.Context, changed func()) error {
				_, err := watch.Watch(ctx, watch.InstalledKeys(), watch.ScoopDirs(), changed)
				return err
			},
			Version:  app.Version(),
			DataDir:  dataDir,
			Protect:  []string{dataDir, logsDir, cacheDir, selfDir},
			Settings: settings,
			IconBase: iconBase,

			TokenFile: filepath.Join(dataDir, "github-token.bin"),
			EnvToken:  token,
			Close:     procs.NewCloser(procs.LiveClose()).Close,
		})

		// Staying in the notification area, AppShelf started at login starts
		// there, its window hidden.
		background := api.LoadBackground(settings)
		window := mygo.NewWindow(mygo.WindowOptions{
			Title:           "AppShelf 软件图书馆",
			URL:             "/",
			Width:           1240,
			Height:          800,
			MinWidth:        960,
			MinHeight:       620,
			StateKey:        "main",
			TitleBarStyle:   mygo.TitleBarHidden,
			TitleBarHeight:  44,
			BackgroundColor: "light-dark(#f4f6fb, #0b1020)",
			Hidden:          background && app.WasOpenedAtLogin(),
		})
		win.Store(window)
		bg.setStay(background)
		bg.start()
		// With the icon, closing the window hides it; quitting closes it.
		var quitting atomic.Bool
		app.OnBeforeQuit(func(*mygo.QuitEvent) { quitting.Store(true) })
		window.OnClose(func(e *mygo.CloseEvent) {
			if !quitting.Load() && bg.enabled() {
				e.PreventDefault()
				window.Hide()
			}
		})
		// The page's lists have menus of their own; elsewhere only text has
		// one, to copy and paste: a store window has no Back or Reload.
		window.Page().OnContextMenu(func(e *mygo.ContextMenuEvent) {
			if !e.Editable && e.SelectionText == "" {
				e.PreventDefault()
				return
			}
			e.Menu.Remove(mygo.ContextMenuBack, mygo.ContextMenuForward, mygo.ContextMenuReload, mygo.ContextMenuStop, mygo.ContextMenuPrint)
		})
		app.OnSecondInstance(func([]string, string) { showWindow() })
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// packagedApps lists the Start menu's app packages by family name, for
// their icons: their AppUserModelID is <family>!<app>.
func packagedApps() (map[string]string, error) {
	apps, shellErr := mygo.Shell.InstalledApps()
	out := map[string]string{}
	nameCounts := map[string]int{}
	for _, a := range apps {
		key := icons.ApplicationIconKey(a.Name)
		nameCounts[key]++
		if key != "name:" && nameCounts[key] == 1 {
			out[key] = a.Path
		} else {
			out[key] = ""
		}
		if family, _, ok := strings.Cut(a.ID, "!"); ok {
			if _, seen := out[family]; !seen {
				out[family] = a.Path
			}
		}
	}
	// Non-launchable packages have no Start menu item, but often ship a logo.
	logos, logoErr := icons.PackageLogos(context.Background())
	if logoErr == nil {
		for family, path := range logos {
			// Reading a PNG also avoids dispatching shell drawing to the UI thread.
			out[family] = path
		}
	}
	if shellErr != nil && logoErr != nil {
		return nil, shellErr
	}
	return out, nil
}
