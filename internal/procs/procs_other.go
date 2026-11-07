//go:build !windows

package procs

import "errors"

// Paths is only available on Windows.
func Paths() ([]string, error) { return nil, errors.New("只支持 Windows") }
