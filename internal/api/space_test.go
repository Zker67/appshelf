//go:build windows

package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appshelf/internal/detect"
	"appshelf/internal/inventory"
	"appshelf/internal/leftover"
	"appshelf/internal/source/winget"
)

// TestSpaceMeasuresTheProgramAndItsData measures a made-up program's
// directory and finds the data directory named as it in %LOCALAPPDATA%;
// both open by key and ID, never by path.
func TestSpaceMeasuresTheProgramAndItsData(t *testing.T) {
	e := newEnv(t)
	b := make([]byte, 5)
	rand.Read(b)
	name := "AppShelfTestSpace" + hex.EncodeToString(b)
	// Temp is a directory every program shares: installed where per-user
	// installers put programs.
	install := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", name)
	if err := os.MkdirAll(filepath.Join(install, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(install) })
	os.WriteFile(filepath.Join(install, "bin", "app.exe"), make([]byte, 3000), 0o644)
	os.WriteFile(filepath.Join(install, "readme.txt"), make([]byte, 1000), 0o644)
	data := filepath.Join(os.Getenv("LOCALAPPDATA"), name)
	if err := os.MkdirAll(filepath.Join(data, "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	os.WriteFile(filepath.Join(data, "cache", "blob"), make([]byte, 500), 0o644)

	e.mu.Lock()
	e.registry = []detect.Entry{{DisplayName: name, DisplayVersion: "1.0", Publisher: "Test Co", Name: name, Scope: "User", Arch: "X64", Key: `HKCU\` + name, InstallLocation: install, EstimatedSize: 9000}}
	e.mu.Unlock()
	e.listARP(`ARP\User\X64\`+name, name)
	e.installed.Refresh()
	key := `arp:user\x64\` + strings.ToLower(name)
	e.until(func() bool { _, _, err := e.app.entry(key); return err == nil })

	sp, err := e.installed.Space(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !sp.Measured || sp.Program != 4000 || sp.Registered != 9000 || sp.From != LocationRegistry || !sp.Openable || !strings.EqualFold(sp.Location, install) {
		t.Errorf("space = %+v", sp)
	}
	var found *SpaceItem
	for i, d := range sp.Data {
		if strings.EqualFold(d.Path, data) {
			found = &sp.Data[i]
		}
		if strings.EqualFold(d.Path, install) {
			t.Errorf("the program's own directory counted as data: %+v", d)
		}
	}
	if found == nil || found.Size != 500 || sp.DataSize < 500 || len(found.Reasons) == 0 {
		t.Fatalf("data = %+v", sp.Data)
	}

	if err := e.installed.OpenLocation(key, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.installed.OpenLocation(key, found.ID); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	opened := e.opened
	e.mu.Unlock()
	if len(opened) != 2 || !strings.EqualFold(opened[0], install) || !strings.EqualFold(opened[1], data) {
		t.Errorf("opened %v", opened)
	}
	// Only what Space found opens; an unknown program does not.
	if err := e.installed.OpenLocation(key, "not-a-find"); err == nil {
		t.Error("an unknown ID opened")
	}
	if err := e.installed.OpenLocation("arp:user\\x64\\nothing", ""); err == nil {
		t.Error("an unknown program opened")
	}
	if _, err := e.installed.Space(context.Background(), "arp:user\\x64\\nothing"); err == nil {
		t.Error("an unknown program measured")
	}

	// Its directory gone: not measured, not openable, said why.
	os.RemoveAll(install)
	sp, err = e.installed.Space(context.Background(), key)
	if err != nil || sp.Measured || sp.Openable || sp.Note == "" {
		t.Errorf("gone: %+v, %v", sp, err)
	}
	if err := e.installed.OpenLocation(key, ""); err == nil {
		t.Error("a missing directory opened")
	}
}

// A directory every program shares is not measured, though it opens.
func TestSpaceSkipsSharedDirectories(t *testing.T) {
	e := newEnv(t)
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		t.Skip("no Program Files")
	}
	e.mu.Lock()
	e.registry = []detect.Entry{{DisplayName: "Shared Thing", DisplayVersion: "1", Name: "SharedThing", Scope: "Machine", Arch: "X64", Key: `HKLM\SharedThing`, InstallLocation: pf}}
	e.mu.Unlock()
	e.listARP(`ARP\Machine\X64\SharedThing`, "Shared Thing")
	e.installed.Refresh()
	key := `arp:machine\x64\sharedthing`
	e.until(func() bool { _, _, err := e.app.entry(key); return err == nil })
	sp, err := e.installed.Space(context.Background(), key)
	if err != nil || sp.Measured || !sp.Openable || !strings.Contains(sp.Note, "共用") {
		t.Errorf("shared: %+v, %v", sp, err)
	}
}

// listARP makes winget list a program of the registry.
func (e *env) listARP(id, name string) {
	e.wapi.mu.Lock()
	e.wapi.pkgs = append(e.wapi.pkgs, winget.Package{ID: id, Name: name, Version: "1"})
	e.wapi.mu.Unlock()
}

func TestDataDirs(t *testing.T) {
	own := `C:\Apps\Foo`
	items := []leftover.Item{
		{ID: "1", Kind: leftover.Dir, Path: own, Level: leftover.High},
		{ID: "2", Kind: leftover.Dir, Path: `C:\Users\u\AppData\Local\Foo`, Level: leftover.Medium},
		{ID: "duplicate", Kind: leftover.Dir, Path: `C:\Users\u\AppData\Local\Foo`, Level: leftover.Medium},
		{ID: "3", Kind: leftover.Dir, Path: `C:\Users\u\AppData\Local\Foo\Cache`, Level: leftover.High},
		{ID: "4", Kind: leftover.Dir, Path: `C:\Users\u\AppData\Roaming\Foo`, Level: leftover.High},
		{ID: "5", Kind: leftover.Dir, Path: `C:\Apps`, Level: leftover.Medium},
		{ID: "6", Kind: leftover.Dir, Path: `C:\Users\u\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Foo`, Level: leftover.High},
		{ID: "7", Kind: leftover.Dir, Path: `C:\ProgramData\FooBar`, Level: leftover.Low},
		{ID: "8", Kind: leftover.RegKey, Path: `Software\Foo`, Level: leftover.High},
	}
	got := dataDirs(items, own, []string{`C:\Users\u\AppData\Roaming\Microsoft\Windows\Start Menu\Programs`})
	var ids []string
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	if strings.Join(ids, ",") != "4,2" {
		t.Errorf("data dirs = %v", ids)
	}
}

func TestSpaceMeasureMissingAndCanceled(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), make([]byte, 123), 0600); err != nil {
		t.Fatal(err)
	}
	got := measure(context.Background(), []string{root, filepath.Join(root, "missing")})
	if got[0].bytes != 123 || got[0].partial || !got[1].partial {
		t.Fatalf("sizes: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := measure(ctx, []string{root}); got[0].bytes != 0 {
		t.Fatal(got)
	}
}

// Opt-in, read-only smoke check of installed program metadata and data paths.
func TestSpaceLocalReadOnly(t *testing.T) {
	if os.Getenv("APPSHELF_SPACE_TEST") != "1" {
		t.Skip("set APPSHELF_SPACE_TEST=1 for local read-only scan")
	}
	reg, err := detect.Scan()
	if err != nil {
		t.Fatal(err)
	}
	entries := inventory.Build(nil, false, nil, reg)
	var others []leftover.Other
	for _, e := range entries {
		others = append(others, leftover.Other{Name: e.Name, Location: e.Location})
	}
	cfg := leftover.DataConfig()
	for _, e := range entries {
		if !strings.Contains(strings.ToLower(e.Name), "docker desktop") && !strings.EqualFold(e.Name, "Bitwarden") {
			continue
		}
		p := leftover.DataProgram{Program: leftover.Program{Name: e.Name, Publisher: e.Publisher, Location: e.Location}, Aliases: []string{filepath.Base(e.Location)}}
		// Exclude this program, as Installed.Space does.
		var rest []leftover.Other
		for _, o := range others {
			if !strings.EqualFold(o.Location, e.Location) {
				rest = append(rest, o)
			}
		}
		dirs := dataDirs(leftover.FindData(context.Background(), cfg, p, rest), e.Location, nil)
		for _, d := range dirs {
			size := measure(context.Background(), []string{d.Path})[0]
			t.Logf("%s: %s bytes=%d partial=%v", e.Name, d.Path, size.bytes, size.partial)
		}
		if len(dirs) == 0 {
			t.Errorf("no data found for %s", e.Name)
		}
	}
	families, _ := filepath.Glob(filepath.Join(os.Getenv("LOCALAPPDATA"), "Packages", "TheBrowserCompany.Dia_*"))
	for _, family := range families {
		p := leftover.DataProgram{Program: leftover.Program{Name: "Dia"}, Family: filepath.Base(family)}
		dirs := leftover.FindData(context.Background(), cfg, p, nil)
		if len(dirs) == 0 {
			t.Fatal("Dia data missing")
		}
		for _, d := range dirs {
			size := measure(context.Background(), []string{d.Path})[0]
			t.Logf("Dia: %s bytes=%d partial=%v", d.Path, size.bytes, size.partial)
		}
	}
}
