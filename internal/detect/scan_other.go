//go:build !windows

package detect

import "errors"

// Scan reads the Windows registry, which other systems do not have.
func Scan() ([]Entry, error) {
	return nil, errors.ErrUnsupported
}
