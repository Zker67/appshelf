package usage

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"appshelf/internal/detect"
)

func TestRot13(t *testing.T) {
	if got := rot13(`{6Q809377-6NS0-444O-8957-N3773S02200R}\Ncc\ncc.rkr`); got != `{6D809377-6AF0-444B-8957-A3773F02200E}\App\app.exe` {
		t.Errorf("rot13 = %q", got)
	}
}

func TestLastRun(t *testing.T) {
	want := time.Date(2026, 11, 9, 8, 30, 0, 0, time.UTC)
	data := make([]byte, 72)
	binary.LittleEndian.PutUint64(data[60:], uint64(want.UnixNano()/100+116444736000000000))
	if got := lastRun(data); !got.Equal(want) {
		t.Errorf("lastRun = %v, want %v", got, want)
	}
	if !lastRun(make([]byte, 72)).IsZero() || !lastRun(make([]byte, 16)).IsZero() {
		t.Error("no time must be zero")
	}
}

func TestParseName(t *testing.T) {
	cases := map[string]entryName{
		`{7C5A40EF-A0FB-4BFC-874A-C0F2E0B9FA8E}\Steam\Steam.exe`: {Folder: "{7C5A40EF-A0FB-4BFC-874A-C0F2E0B9FA8E}", Rest: `Steam\Steam.exe`},
		`C:\App\Bandizip\Bandizip.exe`:                           {Rest: `C:\App\Bandizip\Bandizip.exe`},
		`Microsoft.ScreenSketch_8wekyb3d8bbwe!App`:               {AppID: "Microsoft.ScreenSketch_8wekyb3d8bbwe!App"},
		`com.ccswitch.desktop`:                                   {AppID: "com.ccswitch.desktop"},
	}
	for in, want := range cases {
		if got, ok := parseName(in); !ok || got != want {
			t.Errorf("parseName(%q) = %+v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "UEME_CTLSESSION", `relative\x.exe`} {
		if _, ok := parseName(in); ok {
			t.Errorf("parseName(%q) must be nothing", in)
		}
	}
}

func TestLastUsed(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 12, 0, 0, 0, time.UTC) }
	now := day(9)
	rec := Record{
		At: now,
		Launches: []Launch{
			{Path: `C:\Tools\Editor\editor.exe`, At: day(1)},
			{Path: `c:/tools/editor/bin/helper.exe`, At: day(3)},
			{Path: `C:\Tools\Editor\unins000.exe`, At: day(8)},
			{Path: `C:\Tools\EditorPlus\plus.exe`, At: day(7)},
			{AppID: "Microsoft.ScreenSketch_8wekyb3d8bbwe!App", At: day(5)},
			{Path: `C:\Program Files\WindowsApps\Microsoft.Windows.Photos_2026.1.0_x64__8wekyb3d8bbwe\Photos.exe`, At: day(4)},
			{Path: `C:\Users\me\scoop\apps\ripgrep\current\rg.exe`, At: day(2)},
			{Path: `C:\Program Files\Shared.exe`, At: day(6)},
		},
		Running: []string{`C:\Tools\Busy\busy.exe`},
	}
	cases := []struct {
		name string
		p    Program
		want time.Time
	}{
		{"latest of its directory, not its uninstaller nor a sibling", Program{Dirs: []string{`C:\Tools\Editor\`}}, day(3)},
		{"running is now", Program{Dirs: []string{`C:\Tools\Busy`}}, now},
		{"app package by its ID", Program{Family: "Microsoft.ScreenSketch_8wekyb3d8bbwe"}, day(5)},
		{"app package by its directory", Program{Family: "microsoft.windows.photos_8wekyb3d8bbwe"}, day(4)},
		{"scoop app", Program{ScoopName: "ripgrep"}, day(2)},
		{"a shared directory is no program's", Program{Dirs: []string{os.Getenv("ProgramFiles")}}, time.Time{}},
		{"never opened", Program{Dirs: []string{`C:\Tools\Unused`}}, time.Time{}},
		{"nothing to tell it by", Program{}, time.Time{}},
	}
	for _, c := range cases {
		if got := rec.LastUsed(c.p); !got.Equal(c.want) {
			t.Errorf("%s: LastUsed = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestReadThisMachine reads this user's launches and gives them to the
// programs of the registry. It changes nothing.
func TestReadThisMachine(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	rec, err := Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Tracked() {
		t.Skip("Windows records no launch for this user")
	}
	paths, ids := 0, 0
	for _, l := range rec.Launches {
		if l.Path != "" {
			paths++
			if !filepath.IsAbs(l.Path) {
				t.Errorf("launch of a relative path %q", l.Path)
			}
		} else {
			ids++
		}
	}
	t.Logf("%d launches: %d of a file, %d of an app package; %d processes running", len(rec.Launches), paths, ids, len(rec.Running))
	entries, err := detect.Scan()
	if err != nil {
		t.Fatal(err)
	}
	type used struct {
		name string
		at   time.Time
	}
	var found []used
	for _, e := range entries {
		dirs := []string{e.InstallLocation, filepath.Dir(detect.CommandFile(e.DisplayIcon)), filepath.Dir(detect.CommandFile(e.UninstallString))}
		if at := rec.LastUsed(Program{Dirs: dirs}); !at.IsZero() {
			found = append(found, used{e.DisplayName, at})
		}
	}
	if len(found) == 0 {
		t.Fatal("launches recorded, but no program of the registry has one")
	}
	slices.SortFunc(found, func(a, b used) int { return b.at.Compare(a.at) })
	t.Logf("%d of %d programs of the registry were opened; the latest:", len(found), len(entries))
	for _, f := range found[:min(10, len(found))] {
		t.Logf("  %s  %s", f.at.Local().Format("2006-01-02 15:04"), f.name)
	}
}
