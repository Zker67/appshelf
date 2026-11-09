//go:build !windows

package detect

import "errors"

// Scan reads the Windows registry, which other systems do not have.
func Scan() ([]Entry, error) {
	return nil, errors.ErrUnsupported
}

// Lookup reads the Windows registry, which other systems do not have.
func Lookup(scope, arch, name string) (Entry, bool, error) {
	return Entry{}, false, errors.ErrUnsupported
}
