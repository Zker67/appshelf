// Package procs tells whether programs of a directory are running, for
// updates not to close a program the user is working in.
package procs

import (
	"path/filepath"
	"strings"
)

// Under reports whether one of paths, the executables of running
// processes, is in dir or below it. An empty dir has none.
func Under(paths []string, dir string) bool {
	dir = clean(dir)
	if dir == "" {
		return false
	}
	prefix := dir
	if !strings.HasSuffix(prefix, `\`) {
		prefix += `\`
	}
	for _, p := range paths {
		if strings.HasPrefix(clean(p), prefix) {
			return true
		}
	}
	return false
}

// clean makes a Windows path comparable: absolute, backslashes, lower case.
func clean(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(filepath.Clean(strings.ReplaceAll(p, "/", `\`)), "/", `\`)
	if len(p) < 3 || p[1] != ':' {
		// Not a drive path: a relative path finds nothing.
		if !strings.HasPrefix(p, `\`) {
			return ""
		}
	}
	return strings.ToLower(p)
}
