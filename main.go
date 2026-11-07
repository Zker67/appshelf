// AppShelf is a personal software store for Windows: a catalog of apps
// with categories and tags, installed in batches from winget, scoop and
// GitHub releases, never twice and never the wrong package.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
	"github.com/egoist/mygo/ui"

	"appshelf/internal/core"
	"appshelf/internal/source"
	"appshelf/internal/source/github"
	"appshelf/internal/source/scoop"
	"appshelf/internal/source/winget"
	"appshelf/internal/state"
	"appshelf/internal/view"
)

func main() {
	app := mygo.App
	// The name decides the data directory, in development as when built.
	app.SetName("AppShelf")
	if !app.RequestSingleInstanceLock() {
		return // the running AppShelf comes to the front
	}
	mygo.Use(native.Plugin)

	var win atomic.Pointer[mygo.Window]
	post := func(fn func()) {
		if w := win.Load(); w != nil {
			w.Update(fn)
		}
	}

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
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}

		c, err := core.Open(context.Background(), core.Config{
			Paths: core.Paths{
				Catalog: filepath.Join(dataDir, "catalog.json"),
				State:   filepath.Join(dataDir, "state.json"),
				Logs:    logsDir,
			},
			Sources: func(store *state.Store) []source.Source {
				return []source.Source{
					winget.New(),
					scoop.New(),
					github.New(github.Options{Store: store, TempDir: filepath.Join(tempDir, "AppShelf"), Token: token, Version: app.Version()}),
				}
			},
			Post: post,
		})
		if err != nil {
			mygo.Dialog.Error("AppShelf 无法启动", err.Error())
			app.Quit()
			return
		}

		// Development builds, and copies not installed where they can be
		// replaced, cannot update themselves.
		var checkUpdates func()
		if mygo.Updater.Enabled() {
			checkUpdates = updater.CheckForUpdates
		}

		var w *mygo.Window
		v := view.New(c, view.Hooks{
			Post: post,
			OpenFile: func(title string) (string, error) {
				paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
					Parent:  w,
					Title:   title,
					Filters: []mygo.FileFilter{{Name: "AppShelf 清单", Extensions: []string{"json"}}},
				})
				if err != nil || len(paths) == 0 {
					return "", err
				}
				return paths[0], nil
			},
			SaveFile: func(title, name string) (string, error) {
				return mygo.Dialog.Save(mygo.SaveDialogOptions{
					Parent:      w,
					Title:       title,
					DefaultPath: name,
					Filters:     []mygo.FileFilter{{Name: "AppShelf 清单", Extensions: []string{"json"}}},
				})
			},
			OpenPath:     mygo.Shell.OpenPath,
			CheckUpdates: checkUpdates,
			Version:      app.Version(),
		})
		w = mygo.NewWindow(mygo.WindowOptions{
			Title:     "AppShelf",
			Width:     1120,
			Height:    720,
			MinWidth:  880,
			MinHeight: 560,
			StateKey:  "main",
			Content:   ui.View(v.Build),
		})
		win.Store(w)
		app.OnSecondInstance(func([]string, string) {
			w.Restore()
			w.Show()
			w.Focus()
		})
		c.Refresh()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
