package icons

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"appshelf/internal/inventory"
	"appshelf/internal/jsonfile"
)

type localImage struct {
	image
	at time.Time
}

type localCall struct {
	done chan struct{}
	img  image
}

// localIcon coalesces shell work, which MyGo dispatches to the UI thread.
// Only successful extractions survive a restart; a temporary failure is retried.
func (s *Service) localIcon(key string, draw func() image) ([]byte, string, error) {
	s.mu.Lock()
	if img, ok := s.local[key]; ok && time.Since(img.at) < time.Minute*10 && (img.data != nil || time.Since(img.at) < 30*time.Second) {
		s.mu.Unlock()
		return localResult(img.image)
	}
	if c := s.localBusy[key]; c != nil {
		s.mu.Unlock()
		<-c.done
		return localResult(c.img)
	}
	c := &localCall{done: make(chan struct{})}
	s.localBusy[key] = c
	s.mu.Unlock()
	cache := filepath.Join(s.opt.Dir, "local", fmt.Sprintf("%x.png", sha256.Sum256([]byte(key))))
	if fi, err := os.Stat(cache); err == nil && fi.Size() <= maxIconSize && time.Since(fi.ModTime()) < 24*time.Hour {
		if data, err := os.ReadFile(cache); err == nil && imageType("", data) == "image/png" {
			c.img = image{data, "image/png"}
		}
	}
	if c.img.data == nil {
		c.img = draw()
		if len(c.img.data) > maxIconSize {
			c.img = image{}
		}
		if c.img.data != nil {
			_ = jsonfile.WriteBytesAtomic(cache, c.img.data)
		}
	}
	s.mu.Lock()
	if len(s.local) >= 2000 {
		s.local = map[string]localImage{}
	}
	s.local[key] = localImage{c.img, time.Now()}
	delete(s.localBusy, key)
	close(c.done)
	s.mu.Unlock()
	return localResult(c.img)
}

func localResult(img image) ([]byte, string, error) {
	if len(img.data) == 0 {
		return nil, "", ErrNoIcon
	}
	return img.data, img.ctype, nil
}

func (s *Service) fromFile(displayIcon string) ([]byte, string, error) {
	path := IconFile(displayIcon)
	if path == "" {
		return nil, "", ErrNoIcon
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return nil, "", ErrNoIcon
	}
	key := fmt.Sprintf("%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano())
	return s.localIcon(key, func() image {
		if strings.EqualFold(filepath.Ext(path), ".png") {
			if fi.Size() <= maxIconSize {
				if data, err := os.ReadFile(path); err == nil && imageType("", data) == "image/png" {
					return image{data, "image/png"}
				}
			}
			return image{}
		}
		if s.opt.FileIcon != nil {
			if data, err := s.opt.FileIcon(path, localSize); err == nil && len(data) > 0 {
				return image{data, "image/png"}
			}
		}
		return image{}
	})
}

func (s *Service) fromItem(item string) ([]byte, string, error) {
	if filepath.IsAbs(item) {
		return s.fromFile(item)
	}
	return s.localIcon("shell:"+item, func() image {
		if s.opt.FileIcon != nil {
			if data, err := s.opt.FileIcon(item, localSize); err == nil && len(data) > 0 {
				return image{data, "image/png"}
			}
		}
		return image{}
	})
}

// Prefer an explicit DisplayIcon. If absent or stale, only consider named
// logos or an unambiguous executable directly in the installation directory.
func (s *Service) installedFile(e inventory.Entry) ([]byte, string, error) {
	if data, ctype, err := s.fromFile(e.Icon); err == nil {
		return data, ctype, nil
	}
	if data, ctype, err := s.fromFile(installerIcon(e.Key)); err == nil {
		return data, ctype, nil
	}
	if !filepath.IsAbs(e.Location) {
		return nil, "", ErrNoIcon
	}
	// A named executable is unambiguous even when runtimes and helpers live
	// beside it, or the directory contains too many files to scan cheaply.
	for _, name := range []string{programName(e.Name), filepath.Base(filepath.Clean(e.Location))} {
		if name == "" || strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
			continue
		}
		if data, ctype, err := s.fromFile(filepath.Join(e.Location, name+".exe")); err == nil {
			return data, ctype, nil
		}
	}
	for _, name := range []string{"app.ico", "icon.ico", "logo.ico", "app.png", "icon.png", "logo.png"} {
		if data, ctype, err := s.fromFile(filepath.Join(e.Location, name)); err == nil {
			return data, ctype, nil
		}
	}
	dir, err := os.Open(e.Location)
	if err != nil {
		return nil, "", ErrNoIcon
	}
	defer dir.Close()
	entries, err := dir.ReadDir(65)
	if len(entries) > 64 || (err != nil && len(entries) == 0) {
		return nil, "", ErrNoIcon
	}
	var exe string
	for _, f := range entries {
		name := strings.ToLower(f.Name())
		if f.IsDir() || !strings.HasSuffix(name, ".exe") || strings.Contains(name, "unins") || strings.Contains(name, "setup") || strings.Contains(name, "update") || strings.HasSuffix(name, "_console.exe") {
			continue
		}
		if exe != "" {
			return nil, "", ErrNoIcon
		}
		exe = filepath.Join(e.Location, f.Name())
	}
	return s.fromFile(exe)
}

var programVersion = regexp.MustCompile(`(?i)\s+(?:(?:version|版本)\s*|v?\d)[\s\S]*$`)

func programName(name string) string {
	return strings.TrimSpace(programVersion.ReplaceAllString(name, ""))
}

// ApplicationIconKey matches display names only, never installation identity.
// Ambiguous Start menu names are excluded by the caller building the index.
func ApplicationIconKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(programName(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return "name:" + b.String()
}

// One package enumeration serves all concurrent icon requests, including
// failures. Refreshing inventory alone must not cause another shell scan.
func (s *Service) packaged(ctx context.Context, family string) ([]byte, string, error) {
	if family == "" || s.opt.PackagedApps == nil {
		return nil, "", ErrNoIcon
	}
	s.mu.Lock()
	if s.packages == nil || time.Since(s.pkgAt) > 10*time.Minute {
		if s.pkgBusy == nil {
			s.pkgBusy = make(chan struct{})
			go func() {
				pkgs, err := s.opt.PackagedApps()
				lower := map[string]string{}
				for k, v := range pkgs {
					lower[strings.ToLower(k)] = v
				}
				s.mu.Lock()
				s.packages, s.pkgAt = lower, time.Now()
				if err != nil {
					s.pkgAt = s.pkgAt.Add(-10*time.Minute + 30*time.Second)
				}
				close(s.pkgBusy)
				s.pkgBusy = nil
				s.mu.Unlock()
			}()
		}
		wait := s.pkgBusy
		s.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		s.mu.Lock()
	}
	item := s.packages[strings.ToLower(family)]
	s.mu.Unlock()
	if item == "" {
		return nil, "", ErrNoIcon
	}
	return s.fromItem(item)
}
