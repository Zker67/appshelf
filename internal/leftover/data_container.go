package leftover

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"appshelf/internal/detect"
)

// These describe directory roles, never application identities. Unknown siblings
// (including backups and old binaries) are not evidence of application data.
func dataDirectoryName(name string) bool {
	switch strings.ReplaceAll(normName(name), " ", "") {
	case "userdata", "data", "cache", "caches", "config", "configuration", "settings", "logs", "profiles", "storage":
		return true
	}
	return false
}

var programVersionDir = regexp.MustCompile(`(?i)^(app[-_])?v?\d+(\.\d+)+(?:[-+_][a-z0-9.-]+)?$`)

func programDirectoryName(name string) bool {
	switch strings.ToLower(name) {
	case "apps", "app", "application", "applications", "bin", "binaries", "current", "versions":
		return true
	}
	return programVersionDir.MatchString(name)
}

// dataContainers uses an installed path as structural evidence of ownership.
// Only cross program-layout segments, never arbitrary ancestors. Every segment
// must be a real directory within the data roots; links cannot establish ownership.
func dataContainers(ctx context.Context, cfg Config, p DataProgram, others []Other) []string {
	if !filepath.IsAbs(p.Location) {
		return nil
	}
	realDir := func(path string) bool {
		fi, err := os.Lstat(path)
		return err == nil && fi.IsDir() && fi.Mode()&os.ModeSymlink == 0
	}
	insideRoots := func(path string) bool {
		for _, root := range cfg.DirRoots {
			if filepath.IsAbs(root) && within(cleanPath(path), cleanPath(root)) {
				for at := path; ; at = filepath.Dir(at) {
					if !realDir(at) {
						return false
					}
					if cleanPath(at) == cleanPath(root) {
						return true
					}
				}
			}
		}
		return false
	}
	dir := filepath.Clean(p.Location)
	var out []string
	for range 4 {
		if ctx.Err() != nil || !insideRoots(dir) || !realDir(dir) || !programDirectoryName(filepath.Base(dir)) {
			break
		}
		parent := filepath.Dir(dir)
		if !insideRoots(parent) || !realDir(parent) || !detect.OwnDirectory(parent) {
			break
		}
		name := normName(filepath.Base(parent))
		// A vendor directory is a shared namespace, even if only one product
		// is currently installed. Generic names also cannot establish ownership.
		if genericNames[name] || dataDirectoryName(name) || dataPublisher(filepath.Base(parent)) == dataPublisher(p.Publisher) || programDirectoryName(filepath.Base(parent)) {
			dir = parent
			continue
		}
		blocked := false
		for _, path := range cfg.Protected {
			a, b := cleanPath(parent), cleanPath(path)
			if b != "" && (a == b || within(a, b) || within(b, a)) {
				blocked = true
			}
		}
		for _, other := range others {
			// Malformed registry locations at shared roots own no descendants,
			// consistently with FindData's existing candidate protection.
			shared := !detect.OwnDirectory(other.Location)
			for _, root := range cfg.DirRoots {
				shared = shared || cleanPath(other.Location) == cleanPath(root)
			}
			if shared {
				continue
			}
			a, b := cleanPath(parent), cleanPath(other.Location)
			if b != "" && (a == b || within(a, b) || within(b, a)) {
				blocked = true
			}
		}
		if !blocked {
			out = append(out, parent)
		}
		// The first application-specific ancestor is the ownership boundary.
		break
	}
	return out
}
