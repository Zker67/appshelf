//go:build windows

package leftover

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Real data roots do not live under Temp, which OwnDirectory deliberately rejects.
func dataTestRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp(os.Getenv("LOCALAPPDATA"), "AppShelfTest-DataContainer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return root
}

func TestFindDataContainers(t *testing.T) {
	for _, tc := range []struct {
		name, install, data, publisher, other, protect string
		want                                           bool
	}{
		{name: "localized name", install: `WidgetCode\Apps`, data: `WidgetCode\User Data`, want: true},
		{name: "nested version", install: `WidgetCode\Apps\11.7.0`, data: `WidgetCode\User Data`, want: true},
		{name: "version install", install: `WidgetCode\app-2.1.0`, data: `WidgetCode\Cache`, want: true},
		{name: "bin install", install: `Vendor\WidgetCode\bin`, data: `Vendor\WidgetCode\config`, publisher: "Vendor", want: true},
		{name: "case and punctuation", install: `WidgetCode\APPS`, data: `WidgetCode\user-data`, want: true},
		{name: "unknown sibling", install: `WidgetCode\Apps`, data: `WidgetCode\OtherProduct`},
		{name: "old version", install: `WidgetCode\Apps`, data: `WidgetCode\1.2.0`},
		{name: "no structural evidence", install: `WidgetCode\Runtime`, data: `WidgetCode\User Data`},
		{name: "shared root", install: `Apps`, data: `User Data`},
		{name: "generic container", install: `Software\Apps`, data: `Software\User Data`},
		{name: "publisher container", install: `Vendor\Apps`, data: `Vendor\User Data`, publisher: "Vendor Inc."},
		{name: "other installation", install: `WidgetCode\Apps`, data: `WidgetCode\User Data`, other: `WidgetCode\OtherProduct`},
		{name: "malformed shared location", install: `WidgetCode\Apps`, data: `WidgetCode\User Data`, other: `.`, want: true},
		{name: "other owns container", install: `WidgetCode\Apps`, data: `WidgetCode\User Data`, other: `WidgetCode`},
		{name: "protected data", install: `WidgetCode\Apps`, data: `WidgetCode\User Data`, protect: `WidgetCode\User Data`},
		{name: "depth bound", install: `WidgetCode\Apps\versions\current\bin\1.2.0`, data: `WidgetCode\User Data`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := dataTestRoot(t)
			mkdir := func(rel string) string {
				path := filepath.Join(root, rel)
				if err := os.MkdirAll(path, 0755); err != nil {
					t.Fatal(err)
				}
				return path
			}
			p := DataProgram{Program: Program{Name: "本地化软件", Publisher: tc.publisher, Location: mkdir(tc.install)}}
			data := mkdir(tc.data)
			cfg := Config{DirRoots: []string{root}}
			var others []Other
			if tc.other != "" {
				others = append(others, Other{Location: mkdir(tc.other)})
			}
			if tc.protect != "" {
				cfg.Protected = []string{mkdir(tc.protect)}
			}
			items := FindData(context.Background(), cfg, p, others)
			if tc.want {
				if len(items) != 1 || items[0].Path != data || items[0].Level != Medium {
					t.Fatalf("got %+v, want %s", items, data)
				}
			} else if len(items) != 0 {
				t.Fatalf("unexpected data: %+v", items)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if got := FindData(ctx, cfg, p, others); len(got) != 0 {
				t.Fatalf("canceled: %+v", got)
			}
		})
	}
}

func TestDataContainerLinksAndScope(t *testing.T) {
	root := dataTestRoot(t)
	outside := dataTestRoot(t)
	app := filepath.Join(root, "WidgetCode")
	install := filepath.Join(app, "Apps")
	data := filepath.Join(app, "User Data")
	if err := os.MkdirAll(install, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "Apps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "User Data"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DirRoots: []string{root}}
	if got := FindData(context.Background(), cfg, DataProgram{Program: Program{Location: filepath.Join(outside, "Apps")}}, nil); len(got) != 0 {
		t.Fatal(got)
	}
	if err := os.Symlink(outside, data); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if got := FindData(context.Background(), cfg, DataProgram{Program: Program{Location: install}}, nil); len(got) != 0 {
		t.Fatal(got)
	}
	link := filepath.Join(root, "LinkedWidget")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if got := FindData(context.Background(), cfg, DataProgram{Program: Program{Location: filepath.Join(link, "Apps")}}, nil); len(got) != 0 {
		t.Fatal(got)
	}
}
