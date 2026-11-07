//go:build !windows

package icons

import "context"

func installerIcon(string) string                         { return "" }
func packageRoots(context.Context) ([]packageRoot, error) { return nil, nil }
