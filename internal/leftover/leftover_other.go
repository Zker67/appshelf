//go:build !windows

package leftover

import (
	"context"
	"errors"
)

// Outcome is what cleaning one find did.
type Outcome struct {
	ID     string
	OK     bool
	Error  string
	Backup string
}

func (f *finder) registry()                     {}
func (f *finder) run()                          {}
func (f *finder) shortcuts(ctx context.Context) {}

func readLinks(context.Context, []string) map[string]string { return nil }

// Clean cleans Windows' locations, which other systems do not have.
func Clean(ctx context.Context, cfg Config, items []Item, others []Other, own string, backupDir string) []Outcome {
	out := make([]Outcome, len(items))
	for i, it := range items {
		out[i] = Outcome{ID: it.ID, Error: errors.ErrUnsupported.Error()}
	}
	return out
}
