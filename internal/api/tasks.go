package api

import (
	"errors"

	"appshelf/internal/queue"
)

// Tasks installs and updates apps of the catalog, one at a time, after a
// preview of what each would do.
type Tasks struct{ a *App }

// Preview says what installing or updating the apps would do, from the
// states known. Nothing runs.
func (t *Tasks) Preview(ids []string, action TaskAction) []PreviewEntry {
	a := t.a
	out := []PreviewEntry{}
	act, ok := action.queue()
	if !ok {
		return out
	}
	a.read(func() {
		for _, it := range a.core.Preview(ids, act) {
			e := PreviewEntry{AppID: it.App.ID, Name: it.App.Name, Icon: iconOf(it.App.Source), Runnable: it.Runnable, Plan: it.Plan, Source: it.App.Source}
			if st, ok := a.core.Status(it.App.ID); ok {
				if st.Asset != "" {
					e.Asset, e.Release = st.Asset, st.Latest
				}
				if it.Runnable {
					e.Note = st.Note
				}
			}
			out = append(out, e)
		}
	})
	if action == ActionUpgrade {
		a.markRunning(out)
	}
	return out
}

// Start queues the apps whose preview, made again now, can run, and
// returns how many. Each is checked again right before it runs.
func (t *Tasks) Start(ids []string, action TaskAction) (int, error) {
	a := t.a
	act, ok := action.queue()
	if !ok {
		return 0, errors.New("这里只能安装或更新清单里的软件")
	}
	n := 0
	a.write([]ChangeKind{ChangedTasks, ChangedShelf}, func() {
		n = a.core.Enqueue(a.core.Preview(ids, act), act)
	})
	if n == 0 {
		return 0, errors.New("没有可以执行的软件")
	}
	return n, nil
}

// List returns the tasks.
func (t *Tasks) List() TaskList {
	a := t.a
	<-a.ready
	q := a.core.Queue
	running, stopping := q.Running()
	out := TaskList{Items: []TaskEntry{}, Running: running, Stopping: stopping}
	for _, it := range q.Items() {
		out.Items = append(out.Items, TaskEntry{
			ID: it.ID, AppID: it.App.ID, Name: it.App.Name, Icon: iconOf(it.App.Source),
			Action: actionOf(it.Action), State: taskStateOf(it.State), Message: it.Message,
			HasLog: it.LogPath != "", Started: it.Started, Ended: it.Ended,
			Stage: it.Stage, Done: it.Progress.Done, Total: it.Progress.Total,
			Interactive: it.State == queue.Failed && interactiveRetry(it),
			Leftovers:   canFindLeftovers(it),
		})
	}
	return out
}

func actionOf(a queue.Action) TaskAction {
	switch a {
	case queue.Upgrade:
		return ActionUpgrade
	case queue.Uninstall:
		return ActionUninstall
	}
	return ActionInstall
}

func taskStateOf(s queue.ItemState) TaskState {
	switch s {
	case queue.Running:
		return TaskRunning
	case queue.Succeeded:
		return TaskSucceeded
	case queue.Skipped:
		return TaskSkipped
	case queue.Failed:
		return TaskFailed
	}
	return TaskWaiting
}

// Stop lets the task running finish and leaves the others waiting.
func (t *Tasks) Stop() {
	<-t.a.ready
	t.a.core.Queue.Stop()
}

// Resume starts the waiting tasks again.
func (t *Tasks) Resume() {
	<-t.a.ready
	t.a.core.Queue.Resume()
}

// Retry queues a failed task again.
func (t *Tasks) Retry(id int) error {
	<-t.a.ready
	if !t.a.core.Queue.Retry(id) {
		return errors.New("这个任务不能重试")
	}
	return nil
}

// RetryInteractive runs a failed removal again with its uninstaller's
// window, for the user to finish it there.
func (t *Tasks) RetryInteractive(id int) error {
	a := t.a
	<-a.ready
	for _, it := range a.core.Queue.Items() {
		if it.ID == id && !interactiveRetry(it) {
			return errors.New("这个任务不能用卸载程序的窗口重试")
		}
	}
	if !a.core.Queue.RetryInteractive(id) {
		return errors.New("这个任务不能重试")
	}
	return nil
}

// ClearDone removes the finished tasks.
func (t *Tasks) ClearDone() {
	<-t.a.ready
	t.a.core.Queue.ClearDone()
}

// Log returns a task's output from line from on, to show in the window.
func (t *Tasks) Log(id int, from int) (TaskLog, error) {
	a := t.a
	<-a.ready
	found, done := false, true
	for _, it := range a.core.Queue.Items() {
		if it.ID == id {
			found, done = true, it.State.Done()
		}
	}
	if !found {
		return TaskLog{}, errors.New("找不到这个任务")
	}
	// A task waiting, or checking before it runs, has no output yet.
	c, _ := a.core.Queue.Log(id, from)
	lines := c.Lines
	if lines == nil {
		lines = []string{}
	}
	return TaskLog{Lines: lines, Start: c.Start, Next: c.Next, Live: c.Live, Done: done}, nil
}

// OpenLog opens a task's log.
func (t *Tasks) OpenLog(id int) error {
	a := t.a
	<-a.ready
	for _, it := range a.core.Queue.Items() {
		if it.ID == id && it.LogPath != "" {
			if a.hooks.OpenPath == nil {
				return errors.New("无法打开日志")
			}
			return a.hooks.OpenPath(it.LogPath)
		}
	}
	return errors.New("这个任务没有日志")
}
