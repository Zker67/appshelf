package api

// AppShelf's own updates, as Vibe Atlas has them (Zker67/vibe-atlas
// updates.go): checked in the background shortly after launch, then as the
// user chose; a newer version shows at the bottom right of the window; 去更新
// downloads it in place, its progress in the same toast, and AppShelf starts
// again in it. There is no update window.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// SelfPhase is where updating AppShelf itself is.
type SelfPhase string

const (
	SelfIdle        SelfPhase = "idle"        // up to date, or not checked yet
	SelfChecking    SelfPhase = "checking"    // asking GitHub for a newer version
	SelfAvailable   SelfPhase = "available"   // a newer version is out
	SelfDownloading SelfPhase = "downloading" // downloading and installing it
	SelfRestarting  SelfPhase = "restarting"  // installed; AppShelf starts again
	SelfFailed      SelfPhase = "failed"      // installing it failed
)

// SelfState is what the pages show of updating AppShelf itself.
type SelfState struct {
	// Version is the version running.
	Version string `json:"version"`
	// Enabled means this copy can update itself: a build with updates, in
	// a folder it can write. Development builds cannot.
	Enabled bool      `json:"enabled"`
	Phase   SelfPhase `json:"phase"`
	// Available is the newer version found, or "".
	Available string `json:"available,omitempty"`
	// Progress is how much of it downloaded, 0 to 1.
	Progress float64 `json:"progress"`
	// Error says why installing it failed, in the phase failed.
	Error string `json:"error,omitempty"`
	// CheckedAt is when a check last ended without an error.
	CheckedAt time.Time `json:"checkedAt,omitzero"`
}

// SelfRelease is a newer AppShelf found: its version, and how to put it in
// place of the one running, telling how much downloaded.
type SelfRelease struct {
	Version string
	Install func(ctx context.Context, progress func(downloaded, total int64)) error
}

// SelfUpdateChanged is broadcast to the pages as a check for a newer
// AppShelf and its installation go on.
var SelfUpdateChanged = mygo.NewEvent[SelfState]("selfUpdate")

// SelfCheckMode is when AppShelf checks for a newer version of itself.
type SelfCheckMode string

const (
	SelfCheckOff      SelfCheckMode = "off"      // 不自动检查
	SelfCheckLaunch   SelfCheckMode = "launch"   // 启动时检查
	SelfCheckInterval SelfCheckMode = "interval" // 定时检查：启动时，之后每隔几小时
)

// selfIntervals are the hours between checks the user can choose;
// defaultSelfEvery the one out of the box.
var selfIntervals = []int{1, 6, 24}

const defaultSelfEvery = 24

// selfCheckDelay is how long after launch the first check waits, clear of
// the start of the app; selfCheckTimeout how long a check may take, the
// feed's own retries included. Tests shorten them.
var (
	selfCheckDelay   = 3 * time.Second
	selfCheckTimeout = 60 * time.Second
	// selfRelaunchDelay lets the page show that AppShelf starts again.
	selfRelaunchDelay = 400 * time.Millisecond
)

var (
	errSelfDisabled  = errors.New("这个版本不能自动更新：开发版本，或者程序所在的文件夹不能写入。可以到 GitHub 下载安装包")
	errSelfNotFound  = errors.New("还没有找到新版本，先检查更新")
	errSelfUpdating  = errors.New("正在更新")
	errSelfUnchecked = errors.New("正在检查更新")
)

// selfUpdater is the state of updating AppShelf itself.
type selfUpdater struct {
	mu    sync.Mutex // guards the fields below; never held across hooks
	state SelfState
	found *SelfRelease
	// checkM lets one check run at a time.
	checkM sync.Mutex
	// wake tells watch that the schedule changed.
	wake chan struct{}
}

func newSelfUpdater() *selfUpdater {
	return &selfUpdater{state: SelfState{Phase: SelfIdle}, wake: make(chan struct{}, 1)}
}

// busy reports a phase a check leaves be.
func (p SelfPhase) busy() bool { return p == SelfDownloading || p == SelfRestarting }

// offers reports a phase with a newer version to show.
func (p SelfPhase) offers() bool {
	return p == SelfAvailable || p == SelfDownloading || p == SelfRestarting || p == SelfFailed
}

func (a *App) selfState() SelfState {
	a.self.mu.Lock()
	defer a.self.mu.Unlock()
	s := a.self.state
	s.Version, s.Enabled = a.cfg.Version, a.canUpdate()
	return s
}

// setSelf changes the state and tells the pages; change reports whether it
// changed anything. The list of updates hears of a version found or gone.
func (a *App) setSelf(change func(*SelfState) bool) bool {
	a.self.mu.Lock()
	before := a.self.state
	changed := change(&a.self.state)
	after := a.self.state
	a.self.mu.Unlock()
	if !changed {
		return false
	}
	s := a.selfState()
	if a.hooks.EmitSelf != nil {
		a.hooks.EmitSelf(s)
	} else {
		_ = SelfUpdateChanged.Broadcast(s)
	}
	if before.Phase.offers() != after.Phase.offers() || before.Available != after.Available {
		a.note.mark(ChangedUpdates)
	}
	return true
}

// selfSchedule is when AppShelf checks by itself, and how often with
// 定时检查.
func (a *App) selfSchedule() (SelfCheckMode, time.Duration) {
	a.mu.Lock()
	c := a.settings.Updates
	a.mu.Unlock()
	return c.selfCheck(), time.Duration(c.selfEvery()) * time.Hour
}

// rescheduleSelf makes watchSelf follow a new schedule at once.
func (a *App) rescheduleSelf() {
	select {
	case a.self.wake <- struct{}{}:
	default:
	}
}

// watchSelf checks for a newer AppShelf as the user chose: shortly after
// launch, then every few hours with 定时检查, or never. A change of the
// schedule takes effect at once; switching to 定时检查 checks when the last
// check is as old as the interval.
func (a *App) watchSelf(ctx context.Context) {
	if !a.canUpdate() {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(selfCheckDelay):
	}
	var last time.Time
	launched := false
	for {
		mode, every := a.selfSchedule()
		due := false
		switch mode {
		case SelfCheckLaunch:
			due = !launched
		case SelfCheckInterval:
			due = !launched || time.Since(last) >= every
		}
		launched = true
		if due {
			last = time.Now()
			if err := a.checkSelf(ctx); err != nil {
				log.Println("appshelf: checking for a newer AppShelf:", err)
			}
		}
		var next <-chan time.Time
		if mode == SelfCheckInterval {
			next = time.After(time.Until(last.Add(every)))
		}
		select {
		case <-ctx.Done():
			return
		case <-next:
		case <-a.self.wake:
		}
	}
}

// checkSelf asks for a newer version. While one is being installed, it
// leaves it be.
func (a *App) checkSelf(ctx context.Context) error {
	if !a.canUpdate() {
		return errSelfDisabled
	}
	a.self.checkM.Lock()
	defer a.self.checkM.Unlock()
	var prev SelfPhase
	if !a.setSelf(func(s *SelfState) bool {
		if s.Phase.busy() {
			return false
		}
		prev, s.Phase = s.Phase, SelfChecking
		return true
	}) {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, selfCheckTimeout)
	found, err := a.hooks.SelfCheck(cctx)
	cancel()
	a.setSelf(func(s *SelfState) bool {
		switch {
		case err != nil:
			s.Phase = prev
		case found == nil:
			s.Phase, s.Available, s.Error, s.Progress = SelfIdle, "", "", 0
			s.CheckedAt = time.Now()
			a.self.found = nil
		default:
			s.Phase, s.Available, s.Error, s.Progress = SelfAvailable, found.Version, "", 0
			s.CheckedAt = time.Now()
			a.self.found = found
		}
		return true
	})
	return err
}

// installSelf downloads the version found and puts it in place of this one,
// telling the pages how far it got. AppShelf runs on as it is; the new
// version runs once it starts again.
func (a *App) installSelf(ctx context.Context) error {
	var found *SelfRelease
	if !a.setSelf(func(s *SelfState) bool {
		found = a.self.found
		if found == nil || s.Phase.busy() || s.Phase == SelfChecking {
			return false
		}
		s.Phase, s.Progress, s.Error = SelfDownloading, 0, ""
		return true
	}) {
		return nil
	}
	var last time.Time
	err := found.Install(ctx, func(downloaded, total int64) {
		if total <= 0 || (time.Since(last) < 100*time.Millisecond && downloaded < total) {
			return
		}
		last = time.Now()
		p := float64(downloaded) / float64(total)
		a.setSelf(func(s *SelfState) bool { s.Progress = p; return true })
	})
	if err != nil {
		log.Println("appshelf: installing the update:", err)
		a.setSelf(func(s *SelfState) bool {
			s.Phase, s.Error = SelfFailed, "没有更新成功，原来的版本没有改动："+selfProblem(err)+"。"
			return true
		})
		return err
	}
	a.setSelf(func(s *SelfState) bool { s.Phase, s.Progress = SelfRestarting, 1; return true })
	return nil
}

// SelfUpdate returns what there is to show of updating AppShelf itself.
func (s *About) SelfUpdate() SelfState {
	<-s.a.ready
	return s.a.selfState()
}

// CheckForUpdates checks for a newer AppShelf now, as the user asked. A
// version found shows at the bottom right of the window.
func (s *About) CheckForUpdates(ctx context.Context) (SelfState, error) {
	a := s.a
	<-a.ready
	if err := a.checkSelf(ctx); err != nil {
		if errors.Is(err, errSelfDisabled) {
			return a.selfState(), err
		}
		return a.selfState(), errors.New("检查更新失败：" + selfProblem(err))
	}
	return a.selfState(), nil
}

// InstallUpdate downloads the newer AppShelf found and starts AppShelf
// again in it. It returns at once; the selfUpdate event tells how it goes.
func (s *About) InstallUpdate() error {
	a := s.a
	<-a.ready
	st := a.selfState()
	a.self.mu.Lock()
	found := a.self.found != nil
	a.self.mu.Unlock()
	switch {
	case !st.Enabled:
		return errSelfDisabled
	case st.Phase == SelfChecking:
		return errSelfUnchecked
	case st.Phase.busy():
		return errSelfUpdating
	case !found:
		return errSelfNotFound
	}
	go func() {
		if a.installSelf(context.Background()) != nil {
			return
		}
		if a.selfState().Phase != SelfRestarting {
			return
		}
		time.Sleep(selfRelaunchDelay)
		if a.hooks.Relaunch != nil {
			a.hooks.Relaunch()
		}
	}()
	return nil
}

// httpStatus finds the status of a failed request in the updater's errors,
// "GET <url>: 504 Gateway Time-out".
var httpStatus = regexp.MustCompile(`GET \S+: (\d{3}) `)

// selfProblem says in words why reaching the updates failed.
func selfProblem(err error) string {
	n := 0
	if m := httpStatus.FindStringSubmatch(err.Error()); m != nil {
		n, _ = strconv.Atoi(m[1])
	}
	var ne net.Error
	switch {
	case n == 404:
		return "GitHub 上找不到更新文件，稍后再试"
	case n == 403 || n == 429:
		return fmt.Sprintf("GitHub 限制了访问频率（%d），过一会儿再试", n)
	case n >= 500:
		return fmt.Sprintf("GitHub 暂时没有响应（%d），稍后再试；开着代理时，确认代理能访问 GitHub", n)
	case n != 0:
		return fmt.Sprintf("GitHub 拒绝了请求（%d），稍后再试", n)
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		return "连接 GitHub 超时，检查网络后再试；开着代理时，确认代理能访问 GitHub"
	case errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF):
		return "连不上 GitHub，检查网络后再试；开着代理时，确认代理能访问 GitHub"
	}
	return "更新文件不对（内容或签名没有通过校验），或者写不进程序所在的文件夹"
}
