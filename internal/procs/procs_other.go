//go:build !windows

package procs

import "errors"

var errWindowsOnly = errors.New("只支持 Windows")

// Paths is only available on Windows.
func Paths() ([]string, error) { return nil, errWindowsOnly }

// Snapshot is only available on Windows.
func Snapshot() ([]Proc, error) { return nil, errWindowsOnly }

// Path is only available on Windows.
func Path(uint32) (string, bool) { return "", false }

// UsageOf is only available on Windows.
func UsageOf(uint32) (Usage, bool) { return Usage{}, false }

// Windows is only available on Windows.
func Windows(map[uint32]bool) []Window { return nil }

// Front is only available on Windows.
func Front(uintptr) bool { return false }

// End is only available on Windows.
func End(uint32) error { return errWindowsOnly }
