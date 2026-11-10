// Package procs tells whether programs of a directory are running, for
// updates not to close a program the user is working in, and follows the
// processes of an uninstall: their parents, their activity, their windows.
package procs

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Proc is a process as a snapshot lists it.
type Proc struct {
	PID, Parent uint32
	// Name is its executable's file name, as "Au_.exe".
	Name string
}

// Usage is how much a process has worked since it started.
type Usage struct {
	CPU time.Duration // processor time, of all its threads
	IO  uint64        // bytes read, written and otherwise transferred
}

// Window is a visible top-level window of a process.
type Window struct {
	PID    uint32
	Handle uintptr
	Title  string
	// Text is what its labels say, when Windows lets this user read them:
	// not those of a program running as administrator.
	Text []string
}

// ErrDenied reports a process this user may not end: one running as
// administrator, or of another user.
var ErrDenied = errors.New("没有权限结束它（它以管理员身份运行）")

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
