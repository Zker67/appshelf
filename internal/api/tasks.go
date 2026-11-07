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
	a.read(func() {
		for _, it := range a.core.Preview(ids, action.queue()) {
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
	n := 0
	a.write([]ChangeKind{ChangedTasks, ChangedShelf}, func() {
		n = a.core.Enqueue(a.core.Preview(ids, action.queue()), action.queue())
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
		})
	}
	return out
}

func actionOf(a queue.Action) TaskAction {
	if a == queue.Upgrade {
		return ActionUpgrade
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

// ClearDone removes the finished tasks.
func (t *Tasks) ClearDone() {
	<-t.a.ready
	t.a.core.Queue.ClearDone()
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
