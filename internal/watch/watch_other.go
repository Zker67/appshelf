//go:build !windows

package watch

import (
	"context"
	"errors"
)

// Watch watches the Windows registry, which other systems do not have.
func Watch(ctx context.Context, keys []Key, dirs []string, changed func()) (int, error) {
	return 0, errors.ErrUnsupported
}
