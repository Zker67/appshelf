//go:build !windows

package usage

import (
	"context"
	"errors"
	"time"
)

// Read reads what Windows records of the user's launches, which other
// systems do not have.
func Read(context.Context) (Record, error) {
	return Record{At: time.Now()}, errors.ErrUnsupported
}
