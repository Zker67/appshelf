package procs

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

// Closer closes the programs of a directory before an update replaces
// their files. Their windows are asked to close, as their close buttons
// do; what still runs without a window after a while is ended. A program
// whose window stays open, as one asking whether to save, is never ended:
// it is brought to the front and left for the user to close.
type Closer struct {
	sys   CloseSystem
	self  uint32
	Times CloseTimes
}

// CloseSystem is what Closer uses of Windows, replaced in tests.
type CloseSystem struct {
	Snapshot    func() ([]Proc, error)
	Path        func(pid uint32) (string, bool)
	Windows     func(pids map[uint32]bool) []Window
	CloseWindow func(handle uintptr) bool
	Front       func(handle uintptr) bool
	End         func(pid uint32) error
}

// LiveClose closes the programs of this machine.
func LiveClose() CloseSystem {
	return CloseSystem{Snapshot: Snapshot, Path: Path, Windows: Windows, CloseWindow: CloseWindow, Front: Front, End: End}
}

// CloseTimes are how Close waits.
type CloseTimes struct {
	Poll time.Duration // how often the processes are looked at
	// Grace is how long the programs have to close once asked, and the
	// processes ended to go.
	Grace time.Duration
	// User is how long a window left open waits for the user.
	User time.Duration
	// Settle is how long, once they are gone, their files have to be let
	// go of.
	Settle time.Duration
}

// DefaultCloseTimes are the times of NewCloser.
var DefaultCloseTimes = CloseTimes{Poll: 250 * time.Millisecond, Grace: 10 * time.Second, User: 2 * time.Minute, Settle: time.Second}

// NewCloser returns a closer that never closes AppShelf itself.
func NewCloser(sys CloseSystem) *Closer {
	return &Closer{sys: sys, self: uint32(os.Getpid()), Times: DefaultCloseTimes}
}

// CloseStep is what Close does.
type CloseStep int

const (
	CloseAsking  CloseStep = iota // it asks the windows of Procs to close
	CloseEnding                   // it ends Procs, which have no window left
	CloseWaiting                  // Window stays open: it waits for the user
)

// CloseEvent is what Close tells as it goes.
type CloseEvent struct {
	Step   CloseStep
	Procs  []Proc
	Window Window // of CloseWaiting
}

// CloseResult is how Close ended. Found are the processes it found running
// under the directory; Denied those it could not end, as an
// administrator's, left for the update to deal with; Open the windows the
// user did not close in time, whose programs still run.
type CloseResult struct {
	Found, Denied []Proc
	Open          []Window
}

// Close closes the programs running under dir, telling report what it
// does. A process it cannot read the path of is left alone.
func (c *Closer) Close(ctx context.Context, dir string, report func(CloseEvent)) (CloseResult, error) {
	var res CloseResult
	found, err := c.under(dir)
	if err != nil || len(found) == 0 {
		return res, err
	}
	res.Found = found
	report(CloseEvent{Step: CloseAsking, Procs: found})
	wins := c.sys.Windows(pidsOf(found))
	for _, w := range wins {
		c.sys.CloseWindow(w.Handle)
	}
	// Without a window to close, nothing is waited for: they are ended.
	grace := time.Duration(0)
	if len(wins) > 0 {
		grace = c.Times.Grace
	}
	alive, err := c.waitGone(ctx, found, grace)
	if err != nil {
		return res, err
	}
	if len(alive) == 0 {
		return res, c.settle(ctx)
	}

	windowed := map[uint32]bool{}
	for _, w := range c.sys.Windows(pidsOf(alive)) {
		windowed[w.PID] = true
	}
	var quiet, open []Proc
	for _, p := range alive {
		if windowed[p.PID] {
			open = append(open, p)
		} else {
			quiet = append(quiet, p)
		}
	}
	if len(quiet) > 0 {
		report(CloseEvent{Step: CloseEnding, Procs: quiet})
		var ended []Proc
		for _, p := range quiet {
			if err := c.sys.End(p.PID); err != nil {
				if errors.Is(err, ErrDenied) {
					res.Denied = append(res.Denied, p)
				}
				continue
			}
			ended = append(ended, p)
		}
		if _, err := c.waitGone(ctx, ended, c.Times.Grace); err != nil {
			return res, err
		}
	}
	if len(open) > 0 {
		wins := c.sys.Windows(pidsOf(open))
		if len(wins) > 0 {
			report(CloseEvent{Step: CloseWaiting, Procs: open, Window: wins[0]})
			c.sys.Front(wins[0].Handle)
		}
		left, err := c.waitGone(ctx, open, c.Times.User)
		if err != nil {
			return res, err
		}
		if len(left) > 0 {
			if res.Open = c.sys.Windows(pidsOf(left)); len(res.Open) == 0 {
				// Its window closed, but it has not ended yet: say so
				// rather than end a program that may be saving.
				res.Open = []Window{{PID: left[0].PID, Title: left[0].Name}}
			}
			return res, nil
		}
	}
	return res, c.settle(ctx)
}

// under returns the processes, but AppShelf, whose executables are in dir.
func (c *Closer) under(dir string) ([]Proc, error) {
	if clean(dir) == "" {
		return nil, nil
	}
	snap, err := c.sys.Snapshot()
	if err != nil {
		return nil, err
	}
	var out []Proc
	for _, p := range snap {
		if p.PID == c.self {
			continue
		}
		if path, ok := c.sys.Path(p.PID); ok && Under([]string{path}, dir) {
			out = append(out, p)
		}
	}
	return out, nil
}

// waitGone waits at most d for list to end, returning those still running.
func (c *Closer) waitGone(ctx context.Context, list []Proc, d time.Duration) ([]Proc, error) {
	deadline := time.Now().Add(d)
	for {
		alive, err := c.alive(list)
		if err != nil || len(alive) == 0 || !time.Now().Before(deadline) {
			return alive, err
		}
		if err := sleepCtx(ctx, c.Times.Poll); err != nil {
			return alive, err
		}
	}
}

// alive returns those of list still running: the same process ID, of the
// same name.
func (c *Closer) alive(list []Proc) ([]Proc, error) {
	if len(list) == 0 {
		return nil, nil
	}
	snap, err := c.sys.Snapshot()
	if err != nil {
		return nil, err
	}
	names := map[uint32]string{}
	for _, p := range snap {
		names[p.PID] = strings.ToLower(p.Name)
	}
	var out []Proc
	for _, p := range list {
		if names[p.PID] == strings.ToLower(p.Name) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (c *Closer) settle(ctx context.Context) error { return sleepCtx(ctx, c.Times.Settle) }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func pidsOf(list []Proc) map[uint32]bool {
	out := make(map[uint32]bool, len(list))
	for _, p := range list {
		out[p.PID] = true
	}
	return out
}
