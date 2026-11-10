package procs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

// TestSnapshotUsageAndEnd reads the real processes: the test finds itself
// and its parent, reads its own activity, and ends a process it started.
func TestSnapshotUsageAndEnd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	list, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	self := uint32(os.Getpid())
	var me *Proc
	for i := range list {
		if list[i].PID == self {
			me = &list[i]
		}
	}
	if me == nil || me.Parent == 0 || !strings.EqualFold(me.Name, filepath.Base(os.Args[0])) {
		t.Fatalf("this process in the snapshot: %+v", me)
	}
	if p, ok := Path(self); !ok || !strings.EqualFold(p, mustExe(t)) {
		t.Errorf("Path = %q, %v", p, ok)
	}
	before, ok := UsageOf(self)
	// Windows counts processor time in ticks of about 16 ms.
	for end := time.Now().Add(200 * time.Millisecond); time.Now().Before(end); {
		spin++
	}
	after, ok2 := UsageOf(self)
	if !ok || !ok2 || after.CPU <= before.CPU {
		t.Errorf("usage %+v → %+v", before, after)
	}
	if Windows(nil) != nil {
		t.Error("windows of no process")
	}
	// The windows of all processes are those of the processes asked.
	all := map[uint32]bool{}
	for _, p := range list {
		if p.PID != self {
			all[p.PID] = true
		}
	}
	wins := Windows(all)
	for _, w := range wins {
		if !all[w.PID] || w.Handle == 0 {
			t.Errorf("window %+v", w)
		}
	}
	t.Logf("%d windows; first: %+v", len(wins), firstOf(wins))

	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe"), "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err := End(uint32(cmd.Process.Pid)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("End did not end the process")
	}
}

func mustExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func firstOf(w []Window) any {
	if len(w) == 0 {
		return nil
	}
	return w[0]
}

// spin is worked on to use the processor.
var spin int
