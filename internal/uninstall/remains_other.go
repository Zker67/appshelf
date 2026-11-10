//go:build !windows

package uninstall

// pendingRenames is only available on Windows.
func pendingRenames() []string { return nil }
