package api

import (
	"context"
	"errors"

	"appshelf/internal/queue"
)

// TrayStatus is what AppShelf's notification area icon shows of the
// updates and the tasks.
type TrayStatus struct {
	// Updates is how many updates the updates page offers, AppShelf's own
	// included; Runnable how many of the packages' ones UpdateAll queues.
	Updates  int
	Runnable int
	Checking bool
	// Active is how many tasks wait or run; Running names the one
	// running and Action what it does, "" while none runs.
	Active  int
	Running string
	Action  TaskAction
	// Reboot is how many tasks need Windows to restart to be done.
	Reboot int
}

// TrayStatus returns what the notification area icon shows.
func (a *App) TrayStatus() TrayStatus {
	<-a.ready
	list := a.updateList()
	out := TrayStatus{Checking: list.Checking, Runnable: len(runnableKeys(list))}
	for _, it := range list.Items {
		if it.Hold == HoldNone {
			out.Updates++
		}
	}
	if list.Self.Available != "" {
		out.Updates++
	}
	out.Reboot = len(a.reboots())
	for _, it := range a.core.Queue.Items() {
		switch it.State {
		case queue.Running:
			out.Active++
			out.Running, out.Action = it.App.Name, actionOf(it.Action)
		case queue.Waiting:
			out.Active++
		}
	}
	return out
}

// UpdateAll queues the updates offered that are not queued yet, as the
// updates page's 全部更新 does, and returns how many.
func (a *App) UpdateAll(ctx context.Context) (int, error) {
	<-a.ready
	keys := runnableKeys(a.updateList())
	if len(keys) == 0 {
		return 0, errors.New("没有可以更新的软件")
	}
	return a.startUpdates(ctx, keys, nil)
}

// runnableKeys are the keys of the updates offered, neither held back nor
// queued already.
func runnableKeys(list UpdateList) []string {
	var keys []string
	for _, it := range list.Items {
		if it.Hold == HoldNone && !it.Pending {
			keys = append(keys, it.Key)
		}
	}
	return keys
}
