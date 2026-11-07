package icons

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"appshelf/internal/inventory"
)

func TestPackedProductCode(t *testing.T) {
	if got := packedProductCode("{12345678-1234-ABCD-1234-1234567890AB}"); got != "876543214321DCBA21432143658709BA" {
		t.Fatalf("packed code %s", got)
	}
	for _, s := range []string{"", "../../x", "{12345678_1234-ABCD-1234-1234567890AB}", "{G2345678-1234-ABCD-1234-1234567890AB}"} {
		if packedProductCode(s) != "" {
			t.Errorf("accepted %q", s)
		}
	}
}

func TestManifestLogoNamespaces(t *testing.T) {
	xml := `<Package xmlns:uap="urn:uap"><Properties><Logo>Assets/Store.png</Logo></Properties><Applications><Application><uap:VisualElements Square44x44Logo="Assets/App.png" /></Application></Applications></Package>`
	if got := manifestLogos(strings.NewReader(xml)); !reflect.DeepEqual(got, []string{"Assets/App.png", "Assets/Store.png"}) {
		t.Fatalf("logos: %v", got)
	}
	if got := manifestLogos(strings.NewReader(`<Package><Logo>a.png</Logo>`)); got != nil {
		t.Fatalf("accepted truncated manifest: %v", got)
	}
}

func TestNamedExecutableBeforeHelperAndLargeDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"App.exe", "AppHelper.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("MZ"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		if err := os.WriteFile(filepath.Join(dir, strings.Repeat("x", i+1)+".dll"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := New(Options{Dir: t.TempDir(), FileIcon: func(path string, _ int) ([]byte, error) {
		if filepath.Base(path) != "App.exe" {
			t.Errorf("wrong executable: %s", path)
		}
		return pngData, nil
	}})
	if _, _, err := s.installedFile(inventory.Entry{Name: "App version 1.2.3", Location: dir}); err != nil {
		t.Fatal(err)
	}
}

func TestStartMenuDisplayNameFallback(t *testing.T) {
	s := New(Options{Dir: t.TempDir(), PackagedApps: func() (map[string]string, error) {
		return map[string]string{ApplicationIconKey("App"): "shell:AppsFolder\\App"}, nil
	}, FileIcon: func(string, int) ([]byte, error) { return pngData, nil }})
	s.SetInventory([]inventory.Entry{{Key: "app", Kind: inventory.Local, Name: "App 版本 1.2.3"}})
	if _, _, err := s.Icon(context.Background(), []string{"i", "app"}); err != nil {
		t.Fatal(err)
	}
	if ApplicationIconKey("Other App") == ApplicationIconKey("App") {
		t.Fatal("inexact name collision")
	}
}

func TestRegistryWingetIdentityIsExplicit(t *testing.T) {
	if got := registryWingetID(`reg:hkcu\Gyan.FFmpeg_Microsoft.Winget.Source_8wekyb3d8bbwe`); got != "gyan.ffmpeg" {
		t.Fatal(got)
	}
	for _, key := range []string{`reg:hkcu\Some App`, `reg:hkcu\bad/id_Microsoft.Winget.Source_8wekyb3d8bbwe`} {
		if registryWingetID(key) != "" {
			t.Fatalf("guessed identity: %s", key)
		}
	}
}

func TestPackageLogoResponsesReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	logos, err := PackageLogos(context.Background())
	if err != nil {
		t.Skipf("package repository unavailable: %v", err)
	}
	s := New(Options{Dir: t.TempDir(), PackagedApps: func() (map[string]string, error) { return logos, nil }})
	var entries []inventory.Entry
	for family := range logos {
		entries = append(entries, inventory.Entry{Key: family, Kind: inventory.Store, PackageFamily: family})
	}
	s.SetInventory(entries)
	for _, e := range entries {
		data, ctype, err := s.Icon(context.Background(), []string{"i", e.Key})
		if err != nil || ctype != "image/png" {
			t.Fatalf("package icon response: %s %v", e.Key, err)
		}
		if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
			t.Fatalf("invalid PNG for %s: %v", e.Key, err)
		}
	}
	t.Logf("%d package icon responses decoded as valid PNG", len(entries))
}
