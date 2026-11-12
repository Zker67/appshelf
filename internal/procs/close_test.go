package procs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeDesk plays the programs Close closes: procs run, paths are their
// executables, windows their windows; closing a window of a program that
// closes ends it, ending one ends it unless denied.
type fakeDesk struct {
	mu      sync.Mutex
	procs   []Proc
	paths   map[uint32]string
	windows map[uint32]Window
	// closes are the programs that end when their window is asked to
	// close; asks keep their window open, as one asking to save; tray
	// hide their window and keep running.
	closes, asks, tray, denied map[uint32]bool
	closed, ended, fronted     []uint32
}

func newDesk() *fakeDesk {
	return &fakeDesk{paths: map[uint32]string{}, windows: map[uint32]Window{}, closes: map[uint32]bool{}, asks: map[uint32]bool{}, tray: map[uint32]bool{}, denied: map[uint32]bool{}}
}

func (d *fakeDesk) run(pid uint32, name, path string, window bool) {
	d.procs = append(d.procs, Proc{PID: pid, Name: name})
	d.paths[pid] = path
	if window {
		d.windows[pid] = Window{PID: pid, Handle: uintptr(pid) * 10, Title: name}
	}
}

func (d *fakeDesk) gone(pid uint32) {
	d.procs = slices.DeleteFunc(d.procs, func(p Proc) bool { return p.PID == pid })
	delete(d.windows, pid)
}

func (d *fakeDesk) sys() CloseSystem {
	return CloseSystem{
		Snapshot: func() ([]Proc, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			return slices.Clone(d.procs), nil
		},
		Path: func(pid uint32) (string, bool) {
			d.mu.Lock()
			defer d.mu.Unlock()
			p, ok := d.paths[pid]
			return p, ok
		},
		Windows: func(pids map[uint32]bool) []Window {
			d.mu.Lock()
			defer d.mu.Unlock()
			var out []Window
			for _, p := range d.procs {
				if w, ok := d.windows[p.PID]; ok && pids[p.PID] {
					out = append(out, w)
				}
			}
			return out
		},
		CloseWindow: func(h uintptr) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			pid := uint32(h / 10)
			d.closed = append(d.closed, pid)
			switch {
			case d.closes[pid]:
				d.gone(pid)
			case d.tray[pid]:
				delete(d.windows, pid)
			}
			return true
		},
		Front: func(h uintptr) bool {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.fronted = append(d.fronted, uint32(h/10))
			return true
		},
		End: func(pid uint32) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			if d.denied[pid] {
				return ErrDenied
			}
			d.ended = append(d.ended, pid)
			d.gone(pid)
			return nil
		},
	}
}

func fastCloser(d *fakeDesk) *Closer {
	c := NewCloser(d.sys())
	c.Times = CloseTimes{Poll: time.Millisecond, Grace: 30 * time.Millisecond, User: 60 * time.Millisecond, Settle: time.Millisecond}
	return c
}

const appDir = `C:\Users\me\AppData\Local\Programs\Tool`

func TestCloseAsksTheWindowsFirst(t *testing.T) {
	d := newDesk()
	d.run(1, "Tool.exe", appDir+`\Tool.exe`, true)
	d.closes[1] = true
	d.run(2, "Other.exe", `C:\Other\Other.exe`, true)
	d.run(3, "ToolTool.exe", appDir+`Tool\x.exe`, false) // another directory
	var steps []CloseStep
	res, err := fastCloser(d).Close(context.Background(), appDir, func(e CloseEvent) { steps = append(steps, e.Step) })
	if err != nil || len(res.Found) != 1 || len(res.Open) != 0 || len(res.Denied) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if !slices.Equal(d.closed, []uint32{1}) || len(d.ended) != 0 || !slices.Equal(steps, []CloseStep{CloseAsking}) {
		t.Errorf("closed %v ended %v steps %v", d.closed, d.ended, steps)
	}
}

func TestCloseEndsWhatHasNoWindowLeft(t *testing.T) {
	d := newDesk()
	d.run(1, "Tool.exe", appDir+`\Tool.exe`, true)
	d.tray[1] = true // closing hides it in the tray
	d.run(2, "helper.exe", appDir+`\helper.exe`, false)
	var steps []CloseStep
	start := time.Now()
	res, err := fastCloser(d).Close(context.Background(), appDir, func(e CloseEvent) { steps = append(steps, e.Step) })
	if err != nil || len(res.Open) != 0 || time.Since(start) < 30*time.Millisecond {
		t.Fatalf("%+v %v after %v", res, err, time.Since(start))
	}
	slices.Sort(d.ended)
	if !slices.Equal(d.ended, []uint32{1, 2}) || !slices.Equal(steps, []CloseStep{CloseAsking, CloseEnding}) {
		t.Errorf("ended %v steps %v", d.ended, steps)
	}

	// Without any window, nothing is waited for.
	d = newDesk()
	d.run(2, "helper.exe", appDir+`\helper.exe`, false)
	c := fastCloser(d)
	c.Times.Grace = time.Hour
	if _, err := c.Close(context.Background(), appDir, func(CloseEvent) {}); err != nil || !slices.Equal(d.ended, []uint32{2}) {
		t.Errorf("windowless: %v %v", d.ended, err)
	}
}

func TestCloseLeavesAWindowAskingToSave(t *testing.T) {
	d := newDesk()
	d.run(1, "Tool.exe", appDir+`\Tool.exe`, true)
	d.asks[1] = true
	var waited Window
	res, err := fastCloser(d).Close(context.Background(), appDir, func(e CloseEvent) {
		if e.Step == CloseWaiting {
			waited = e.Window
		}
	})
	if err != nil || len(res.Open) != 1 || res.Open[0].PID != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if len(d.ended) != 0 || waited.PID != 1 || !slices.Equal(d.fronted, []uint32{1}) {
		t.Errorf("ended %v waited %+v fronted %v", d.ended, waited, d.fronted)
	}

	// The user closes it while it waits.
	d = newDesk()
	d.run(1, "Tool.exe", appDir+`\Tool.exe`, true)
	c := fastCloser(d)
	c.Times.User = time.Second
	go func() {
		time.Sleep(50 * time.Millisecond)
		d.mu.Lock()
		d.gone(1)
		d.mu.Unlock()
	}()
	if res, err := c.Close(context.Background(), appDir, func(CloseEvent) {}); err != nil || len(res.Open) != 0 {
		t.Errorf("closed by the user: %+v %v", res, err)
	}
}

func TestCloseDeniedAndCanceled(t *testing.T) {
	d := newDesk()
	d.run(1, "service.exe", appDir+`\service.exe`, false)
	d.denied[1] = true
	res, err := fastCloser(d).Close(context.Background(), appDir, func(CloseEvent) {})
	if err != nil || len(res.Denied) != 1 || len(res.Open) != 0 {
		t.Errorf("denied: %+v %v", res, err)
	}

	d = newDesk()
	d.run(1, "Tool.exe", appDir+`\Tool.exe`, true)
	d.asks[1] = true
	c := fastCloser(d)
	c.Times.User = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := c.Close(ctx, appDir, func(CloseEvent) {}); err == nil {
		t.Error("not canceled")
	}
	// AppShelf itself is never closed, nor anything of an empty directory.
	d = newDesk()
	d.run(uint32(os.Getpid()), "AppShelf.exe", appDir+`\AppShelf.exe`, true)
	if res, _ := fastCloser(d).Close(context.Background(), appDir, func(CloseEvent) {}); len(res.Found) != 0 {
		t.Errorf("self: %+v", res)
	}
	if res, _ := fastCloser(d).Close(context.Background(), "", func(CloseEvent) {}); len(res.Found) != 0 {
		t.Errorf("empty dir: %+v", res)
	}
}

// TestCloseARealProgram ends a copy of ping.exe running from a directory,
// as a program's background process, which has no window.
func TestCloseARealProgram(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	dir := t.TempDir()
	ping := filepath.Join(dir, "tool-helper.exe")
	copyFile(t, filepath.Join(os.Getenv("SystemRoot"), "System32", "ping.exe"), ping)
	cmd := exec.Command(ping, "-n", "60", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer cmd.Process.Kill()
	c := NewCloser(LiveClose())
	c.Times.Settle = 0
	res, err := c.Close(context.Background(), dir, func(CloseEvent) {})
	if err != nil || len(res.Found) != 1 || len(res.Open) != 0 || len(res.Denied) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("not ended")
	}
}
