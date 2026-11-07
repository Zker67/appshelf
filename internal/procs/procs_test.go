package procs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnder(t *testing.T) {
	paths := []string{`C:\Program Files\Git\bin\git.exe`, `c:/users/me/scoop/apps/7zip/current/7zFM.exe`}
	cases := []struct {
		dir  string
		want bool
	}{
		{`C:\Program Files\Git`, true},
		{`c:\program files\git\`, true},
		{`"C:\Program Files\Git"`, true},
		{`C:\Users\me\scoop\apps\7zip`, true},
		{`C:\Program Files\Gi`, false}, // a prefix of the name is another directory
		{`C:\Program Files\Git\cmd`, false},
		{``, false},
		{`Git`, false}, // relative
	}
	for _, c := range cases {
		if got := Under(paths, c.dir); got != c.want {
			t.Errorf("Under(%q) = %v", c.dir, got)
		}
	}
}

// TestPathsFindsItself reads the real processes: the test binary runs.
func TestPathsFindsItself(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 {
		t.Fatalf("only %d processes", len(paths))
	}
	if !Under(paths, filepath.Dir(exe)) {
		t.Errorf("the test's own directory %s has no process among %d", filepath.Dir(exe), len(paths))
	}
	if Under(paths, filepath.Join(t.TempDir(), "nothing-runs-here")) {
		t.Error("an empty directory has a process")
	}
}
