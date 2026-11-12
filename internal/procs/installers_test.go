package procs

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeSystem plays the processes: procs are those running, paths their
// executables, usage what they have used.
type fakeSystem struct {
	mu    sync.Mutex
	procs []Proc
	paths map[uint32]string
	usage map[uint32]Usage
	err   error
	reads int // the path reads
}

func (f *fakeSystem) sys() System {
	return System{
		Snapshot: func() ([]Proc, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return slices.Clone(f.procs), f.err
		},
		Path: func(pid uint32) (string, bool) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.reads++
			p, ok := f.paths[pid]
			return p, ok
		},
		Usage: func(pid uint32) (Usage, bool) {
			f.mu.Lock()
			defer f.mu.Unlock()
			u, ok := f.usage[pid]
			return u, ok
		},
	}
}

func (f *fakeSystem) start(pid uint32, name, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.procs = append(f.procs, Proc{PID: pid, Name: name})
	f.paths[pid] = path
}

func (f *fakeSystem) end(pid uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.procs = slices.DeleteFunc(f.procs, func(p Proc) bool { return p.PID == pid })
}

func (f *fakeSystem) work(pid uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.usage[pid]
	u.CPU += time.Second
	f.usage[pid] = u
}

func newFakeSystem() *fakeSystem {
	return &fakeSystem{paths: map[uint32]string{}, usage: map[uint32]Usage{}}
}

const temp = `C:\Users\me\AppData\Local\Temp`

func TestInstallersAreToldByWhereTheyRun(t *testing.T) {
	f := newFakeSystem()
	// Running before: never one, even in the temporary directory.
	f.start(1, "old.exe", temp+`\old.exe`)
	w := WatchInstallers(f.sys(), []string{temp})
	f.start(2, "setup.tmp", `C:\Users\me\AppData\Local\Temp\is-AB12.tmp\setup.tmp`)
	f.start(3, "Obsidian.exe", `C:\Users\me\AppData\Local\Programs\Obsidian\Obsidian.exe`)
	f.start(4, "bundle.exe", `C:\ProgramData\Package Cache\.cr\bundle.exe`)
	f.start(5, "Update.exe", `C:\Users\me\AppData\Local\SquirrelTemp\Update.exe`)
	f.start(6, "msedge.exe", temp+`\msedge.exe`)
	f.start(7, "unreadable.exe", "")
	delete(f.paths, 7)
	f.start(8, "x.exe", `C:\Windows\SystemTemp\x.exe`)
	f.start(9, "TEMPest.exe", `C:\Users\me\AppData\Local\Tempest\TEMPest.exe`)
	alive, err := w.Alive()
	if err != nil {
		t.Fatal(err)
	}
	var pids []uint32
	for _, p := range alive {
		pids = append(pids, p.PID)
	}
	// x.exe is in Windows' temporary directory, not given here.
	if want := []uint32{2, 4, 5}; !slices.Equal(pids, want) {
		t.Errorf("installers %v, want %v", pids, want)
	}
	reads := f.reads
	if _, err := w.Alive(); err != nil {
		t.Fatal(err)
	}
	if f.reads != reads {
		t.Errorf("paths read again: %d, then %d", reads, f.reads)
	}
	// A new process under a pid that was another's is looked at again.
	f.end(3)
	f.start(3, "setup.exe", temp+`\WinGet\setup.exe`)
	if alive, _ := w.Alive(); !slices.ContainsFunc(alive, func(p Proc) bool { return p.PID == 3 }) {
		t.Errorf("reused pid: %v", alive)
	}
	if got := Names([]Proc{{Name: "a.exe"}, {Name: "A.EXE"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}); got != "a.exe、b、c、…" {
		t.Errorf("names %q", got)
	}
}

func fast(w *Installers) {
	w.Times = WaitTimes{Poll: 5 * time.Millisecond, Grace: 30 * time.Millisecond, Idle: time.Hour, Max: time.Hour}
}

func TestInstallersWait(t *testing.T) {
	ctx := context.Background()
	t.Run("none", func(t *testing.T) {
		f := newFakeSystem()
		w := WatchInstallers(f.sys(), []string{temp})
		w.Times.Poll = time.Hour // returns without waiting at all
		f.start(2, "Obsidian.exe", `C:\Apps\Obsidian\Obsidian.exe`)
		if end := w.Wait(ctx, func([]Proc) { t.Error("reported") }); end != WaitDone {
			t.Errorf("end %v", end)
		}
	})
	t.Run("until they end, through a chain", func(t *testing.T) {
		f := newFakeSystem()
		w := WatchInstallers(f.sys(), []string{temp})
		fast(w)
		f.start(2, "setup.tmp", temp+`\is-1.tmp\setup.tmp`)
		var mu sync.Mutex
		var reported []string
		go func() {
			time.Sleep(40 * time.Millisecond)
			f.end(2)
			// The next step starts within the grace.
			f.start(3, "next.exe", temp+`\next.exe`)
			time.Sleep(40 * time.Millisecond)
			f.end(3)
		}()
		start := time.Now()
		end := w.Wait(ctx, func(alive []Proc) {
			mu.Lock()
			reported = append(reported, Names(alive))
			mu.Unlock()
		})
		if end != WaitDone || time.Since(start) < 80*time.Millisecond {
			t.Errorf("end %v after %v", end, time.Since(start))
		}
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(reported, []string{"setup.tmp", "next.exe"}) {
			t.Errorf("reported %q", reported)
		}
	})
	t.Run("idle", func(t *testing.T) {
		f := newFakeSystem()
		w := WatchInstallers(f.sys(), []string{temp})
		fast(w)
		w.Times.Idle = 50 * time.Millisecond
		f.start(2, "helper.exe", temp+`\helper.exe`)
		f.usage[2] = Usage{}
		if end := w.Wait(ctx, func([]Proc) {}); end != WaitIdle {
			t.Errorf("end %v", end)
		}
	})
	t.Run("a window is not idle", func(t *testing.T) {
		f := newFakeSystem()
		sys := f.sys()
		sys.Windows = func(pids map[uint32]bool) []Window {
			if pids[2] {
				return []Window{{PID: 2, Title: "安装向导"}}
			}
			return nil
		}
		w := WatchInstallers(sys, []string{temp})
		fast(w)
		w.Times.Idle = 20 * time.Millisecond
		f.start(2, "setup.exe", temp+`\setup.exe`)
		f.usage[2] = Usage{}
		go func() {
			time.Sleep(100 * time.Millisecond)
			f.end(2)
		}()
		if end := w.Wait(ctx, func([]Proc) {}); end != WaitDone {
			t.Errorf("end %v", end)
		}
	})
	t.Run("working until the most", func(t *testing.T) {
		f := newFakeSystem()
		w := WatchInstallers(f.sys(), []string{temp})
		fast(w)
		w.Times.Idle, w.Times.Max = 30*time.Millisecond, 120*time.Millisecond
		f.start(2, "setup.exe", temp+`\setup.exe`)
		f.usage[2] = Usage{}
		stop := make(chan struct{})
		defer close(stop)
		go func() {
			for {
				select {
				case <-stop:
					return
				case <-time.After(5 * time.Millisecond):
					f.work(2)
				}
			}
		}()
		if end := w.Wait(ctx, func([]Proc) {}); end != WaitTimeout {
			t.Errorf("end %v", end)
		}
	})
	t.Run("aborted and unreadable", func(t *testing.T) {
		f := newFakeSystem()
		w := WatchInstallers(f.sys(), []string{temp})
		fast(w)
		f.start(2, "setup.exe", temp+`\setup.exe`)
		c, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		if end := w.Wait(c, func([]Proc) {}); end != WaitAborted {
			t.Errorf("end %v", end)
		}
		f.err = errors.New("no snapshot")
		if end := w.Wait(ctx, func([]Proc) {}); end != WaitDone {
			t.Errorf("unreadable: %v", end)
		}
	})
}

// TestInstallersWaitForARealProcess runs a copy of ping.exe from a
// temporary directory, as an installer's setup runs.
func TestInstallersWaitForARealProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	ping := filepath.Join(dir, "setup-copy.exe")
	copyFile(t, filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe"), ping)
	w := WatchInstallers(Live(), []string{dir})
	w.Times.Poll, w.Times.Grace = 100*time.Millisecond, 300*time.Millisecond
	cmd := exec.Command(ping, "-n", "3", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	alive, err := w.Alive()
	if err != nil || !slices.ContainsFunc(alive, func(p Proc) bool { return p.PID == uint32(cmd.Process.Pid) }) {
		t.Fatalf("alive %v %v", alive, err)
	}
	var reported []Proc
	if end := w.Wait(context.Background(), func(a []Proc) { reported = a }); end != WaitDone {
		t.Errorf("end %v", end)
	}
	if len(reported) == 0 || reported[0].Name != "setup-copy.exe" {
		t.Errorf("reported %v", reported)
	}
	if alive, _ := w.Alive(); len(alive) != 0 {
		t.Errorf("still alive: %v", alive)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}
