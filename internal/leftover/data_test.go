//go:build windows

package leftover

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFindDataOwnership(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	mkdir := func(name string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cache := mkdir("Widget")
	for i := range 250 {
		if err := os.WriteFile(filepath.Join(cache, fmt.Sprintf("%d.dll", i)), []byte("cache"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	alias := mkdir("WidgetCode")
	vendor := mkdir(`Microsoft\Edge`)
	mkdir(`Microsoft\Other`)
	family := mkdir(`Packages\Vendor.Widget_publisher`)
	persist := mkdir(`persist\widget`)
	mkdir("WidgetExtra")
	cfg := Config{DirRoots: []string{root}, ScoopPersist: []string{filepath.Join(root, "persist")}}
	for _, tc := range []struct {
		name string
		p    DataProgram
		want []string
	}{
		{"large cache and alias", DataProgram{Program: Program{Name: "Widget 1.2 (x64)"}, Aliases: []string{"WidgetCode"}}, []string{cache, alias}},
		{"shared publisher", DataProgram{Program: Program{Name: "Microsoft Edge", Publisher: "Microsoft Corporation"}}, []string{vendor}},
		{"store identity", DataProgram{Program: Program{Name: "Localized name"}, Family: "Vendor.Widget_publisher"}, []string{family}},
		{"scoop identity", DataProgram{Program: Program{Name: "Localized name", ScoopName: "widget"}}, []string{cache, persist}},
		{"unsafe identity", DataProgram{Family: `..\Widget`}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := FindData(context.Background(), cfg, tc.p, []Other{{Location: root}})
			if len(items) != len(tc.want) {
				t.Fatalf("got %+v, want %v", items, tc.want)
			}
			for _, want := range tc.want {
				found := false
				for _, it := range items {
					if it.Path == want {
						found = true
					}
				}
				if !found {
					t.Errorf("missing %s", want)
				}
			}
		})
	}
	// Display must not weaken the cleanup score of a directory full of DLLs.
	f := &finder{cfg: cfg, p: Program{Name: "Widget"}, names: nameForms("Widget", ""), seen: map[string]bool{}}
	f.dirs()
	for _, it := range f.items {
		if it.Path == cache {
			score := 0
			for _, r := range it.Reasons {
				score += r.Delta
			}
			if levelOf(score) != Low {
				t.Fatal("cleanup scoring changed")
			}
		}
	}
	for _, tc := range []struct {
		name   string
		cfg    Config
		p      DataProgram
		others []Other
	}{
		{"own", cfg, DataProgram{Program: Program{Name: "Widget", Location: cache}}, nil},
		{"protected", Config{DirRoots: []string{root}, Protected: []string{cache}}, DataProgram{Program: Program{Name: "Widget"}}, nil},
		{"other", cfg, DataProgram{Program: Program{Name: "Widget"}}, []Other{{Location: cache}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FindData(context.Background(), tc.cfg, tc.p, tc.others); len(got) != 0 {
				t.Fatalf("protected data: %+v", got)
			}
		})
	}
}
