package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// selfFake plays GitHub's releases of AppShelf and its relaunch.
type selfFake struct {
	mu        sync.Mutex
	checks    int
	found     string // the version a check finds, or ""
	checkErr  error
	installs  int
	installEr error
	relaunch  atomic.Int32
	states    []SelfState
}

// withSelf makes e's AppShelf one that can update itself, on f.
func withSelf(e *env, f *selfFake) {
	h := &e.app.hooks
	h.CanUpdate = func() bool { return true }
	h.SelfCheck = func(ctx context.Context) (*SelfRelease, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.checks++
		if f.checkErr != nil || f.found == "" {
			return nil, f.checkErr
		}
		return &SelfRelease{Version: f.found, Install: func(ctx context.Context, progress func(int64, int64)) error {
			f.mu.Lock()
			f.installs++
			err := f.installEr
			f.mu.Unlock()
			if err != nil {
				return err
			}
			progress(50, 100)
			progress(100, 100)
			return nil
		}}, nil
	}
	h.Relaunch = func() { f.relaunch.Add(1) }
	h.EmitSelf = func(s SelfState) {
		f.mu.Lock()
		f.states = append(f.states, s)
		f.mu.Unlock()
	}
}

func (f *selfFake) set(found string, checkErr, installErr error) {
	f.mu.Lock()
	f.found, f.checkErr, f.installEr = found, checkErr, installErr
	f.mu.Unlock()
}

func (f *selfFake) phases() []SelfPhase {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []SelfPhase
	for _, s := range f.states {
		if len(out) == 0 || out[len(out)-1] != s.Phase {
			out = append(out, s.Phase)
		}
	}
	return out
}

func TestSelfUpdateChecksInstallsAndRelaunches(t *testing.T) {
	old := selfRelaunchDelay
	selfRelaunchDelay = time.Millisecond
	t.Cleanup(func() { selfRelaunchDelay = old })
	e := newEnv(t)
	f := &selfFake{}
	withSelf(e, f)
	ctx := context.Background()

	// Up to date.
	st, err := e.about.CheckForUpdates(ctx)
	if err != nil || st.Phase != SelfIdle || st.Available != "" || st.CheckedAt.IsZero() || !st.Enabled || st.Version != "0.2.0" {
		t.Fatalf("up to date: %+v, %v", st, err)
	}
	if err := e.about.InstallUpdate(); err == nil {
		t.Error("installed with nothing found")
	}

	// A newer version: offered in the list of updates and the tray.
	f.set("0.6.0", nil, nil)
	st, err = e.about.CheckForUpdates(ctx)
	if err != nil || st.Phase != SelfAvailable || st.Available != "0.6.0" {
		t.Fatalf("found: %+v, %v", st, err)
	}
	if l := e.updates.List(); l.Self.Available != "0.6.0" || !l.Self.CanUpdate {
		t.Errorf("list self = %+v", l.Self)
	}

	// Installing fails: the reason in words, offered still, tried again.
	f.set("0.6.0", nil, errors.New("GET https://github.com/x: 504 Gateway Time-out "))
	if err := e.about.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return e.about.SelfUpdate().Phase == SelfFailed })
	st = e.about.SelfUpdate()
	if !strings.Contains(st.Error, "暂时没有响应（504）") || st.Available != "0.6.0" || f.relaunch.Load() != 0 {
		t.Errorf("failed: %+v", st)
	}
	if l := e.updates.List(); l.Self.Available != "0.6.0" {
		t.Errorf("failed update not offered: %+v", l.Self)
	}

	f.set("0.6.0", nil, nil)
	if err := e.about.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	e.until(func() bool { return f.relaunch.Load() == 1 })
	st = e.about.SelfUpdate()
	if st.Phase != SelfRestarting || st.Progress != 1 || st.Error != "" {
		t.Errorf("installed: %+v", st)
	}
	got := f.phases()
	want := []SelfPhase{SelfChecking, SelfIdle, SelfChecking, SelfAvailable, SelfDownloading, SelfFailed, SelfDownloading, SelfRestarting}
	if strings.Join(phaseNames(got), ",") != strings.Join(phaseNames(want), ",") {
		t.Errorf("phases = %v, want %v", got, want)
	}
	// A check while restarting leaves it be; a second install is refused.
	if st, _ := e.about.CheckForUpdates(ctx); st.Phase != SelfRestarting {
		t.Errorf("check while restarting: %+v", st)
	}
	if err := e.about.InstallUpdate(); err == nil {
		t.Error("installed twice")
	}
}

func phaseNames(ps []SelfPhase) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}

func TestSelfUpdateCheckFails(t *testing.T) {
	e := newEnv(t)
	f := &selfFake{}
	withSelf(e, f)
	f.set("0.6.0", nil, nil)
	if _, err := e.about.CheckForUpdates(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A failed check keeps what was found.
	f.set("", context.DeadlineExceeded, nil)
	st, err := e.about.CheckForUpdates(context.Background())
	if err == nil || !strings.Contains(err.Error(), "超时") || st.Phase != SelfAvailable || st.Available != "0.6.0" {
		t.Errorf("failed check: %+v, %v", st, err)
	}
}

func TestSelfUpdateDisabled(t *testing.T) {
	e := newEnv(t)
	st, err := e.about.CheckForUpdates(context.Background())
	if err == nil || st.Enabled || !strings.Contains(err.Error(), "不能自动更新") {
		t.Errorf("disabled: %+v, %v", st, err)
	}
	if err := e.about.InstallUpdate(); err == nil {
		t.Error("a copy that cannot update itself installed")
	}
	if s := e.updates.Settings(); s.CanSelfUpdate || s.SelfCheck != SelfCheckLaunch || s.SelfEvery != 24 {
		t.Errorf("settings = %+v", s)
	}
}

func TestSelfCheckSettings(t *testing.T) {
	e := newEnv(t)
	s := e.updates.Settings()
	s.SelfCheck, s.SelfEvery = SelfCheckInterval, 6
	if err := e.updates.SetSettings(s); err != nil {
		t.Fatal(err)
	}
	if got := e.updates.Settings(); got.SelfCheck != SelfCheckInterval || got.SelfEvery != 6 {
		t.Errorf("kept = %+v", got)
	}
	if c := loadSettings(e.app.cfg.Settings).Updates; c.selfCheck() != SelfCheckInterval || c.selfEvery() != 6 {
		t.Errorf("file = %+v", c)
	}
	for _, bad := range []UpdateSettings{
		{CheckEvery: Check6h, SelfCheck: "sometimes", SelfEvery: 6},
		{CheckEvery: Check6h, SelfCheck: SelfCheckInterval, SelfEvery: 5},
	} {
		if err := e.updates.SetSettings(bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

// At launch AppShelf checks once, shortly after starting; not at all when
// the user chose so.
func TestWatchSelf(t *testing.T) {
	old := selfCheckDelay
	selfCheckDelay = 10 * time.Millisecond
	t.Cleanup(func() { selfCheckDelay = old })
	for _, tc := range []struct {
		mode SelfCheckMode
		want int
	}{{SelfCheckLaunch, 1}, {SelfCheckOff, 0}, {SelfCheckInterval, 1}} {
		t.Run(string(tc.mode), func(t *testing.T) {
			e := newEnv(t)
			f := &selfFake{}
			withSelf(e, f)
			e.app.mu.Lock()
			e.app.settings.Updates.SelfCheck = tc.mode
			e.app.mu.Unlock()
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { e.app.watchSelf(ctx); close(done) }()
			time.Sleep(200 * time.Millisecond)
			cancel()
			<-done
			f.mu.Lock()
			n := f.checks
			f.mu.Unlock()
			if n != tc.want {
				t.Errorf("checked %d times, want %d", n, tc.want)
			}
		})
	}
}
