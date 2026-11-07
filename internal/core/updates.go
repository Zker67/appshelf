package core

import (
	"context"

	"appshelf/internal/catalog"
	"appshelf/internal/queue"
	"appshelf/internal/source"
)

// PreviewUpdates reads exact installed targets without adding them to the
// catalog. It runs off the core loop; only immutable sources and the locked
// availability/queue state are used. Every queue item rechecks again at run time.
func (c *Core) PreviewUpdates(ctx context.Context, apps []catalog.App) []PreviewItem {
	valid := make([]catalog.App, 0, len(apps))
	rejected := map[string]source.Status{}
	for _, app := range apps {
		note := ""
		if app.Source.Type != catalog.Winget && app.Source.Type != catalog.Scoop {
			note = "本机直接更新仅支持 winget 和 scoop"
		} else if err := catalog.ValidateApp(app); err != nil {
			note = err.Error()
		}
		if note != "" {
			rejected[app.ID] = source.Status{State: source.Unavailable, Note: note}
		} else {
			valid = append(valid, app)
		}
	}
	states := c.readAllContext(ctx, valid, false)
	out := make([]PreviewItem, 0, len(apps))
	for _, app := range apps {
		st, ok := rejected[app.ID]
		if !ok {
			st = states[app.ID]
		}
		out = append(out, c.preview(app, st, true, queue.Upgrade))
	}
	return out
}
