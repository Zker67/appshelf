//go:build !windows

package inventory

// PackagePath finds no app package off Windows.
func PackagePath(string) string { return "" }
