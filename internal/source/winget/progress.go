package winget

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"appshelf/internal/source"
)

// progress follows winget's output for the stage of its work. Its output
// sent to a file has no progress bars, only lines in the language of
// Windows: the line with the installer's URL starts the download, the line
// after it the install. While it downloads, the size of the package's
// directory under %TEMP%\WinGet tells how far it is.
type progress struct {
	log   io.Writer
	id    string
	work  string // the stage after the download, as "正在安装"
	start time.Time
	dir   string // where winget downloads, %TEMP%\WinGet

	mu          sync.Mutex
	line        []byte
	downloading bool
	downloaded  bool
	stop        chan struct{}
	every       time.Duration
}

func newProgress(log io.Writer, id, work string) *progress {
	p := &progress{log: log, id: id, work: work, start: time.Now(), dir: filepath.Join(os.TempDir(), "WinGet"), every: 500 * time.Millisecond}
	source.ReportProgress(log, source.Progress{Stage: "正在准备"})
	return p
}

func (p *progress) Write(b []byte) (int, error) {
	n, err := p.log.Write(b)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		if c == '\n' || c == '\r' {
			p.lineDone(strings.TrimSpace(stripControl(string(p.line))))
			p.line = p.line[:0]
			continue
		}
		p.line = append(p.line, c)
	}
	return n, err
}

func (p *progress) lineDone(line string) {
	switch {
	case line == "":
	case !p.downloading && !p.downloaded && strings.Contains(line, "://"):
		p.downloading = true
		source.ReportProgress(p.log, source.Progress{Stage: "正在下载"})
		p.stop = make(chan struct{})
		go p.watch(p.stop)
	case p.downloading:
		p.downloading, p.downloaded = false, true
		close(p.stop)
		source.ReportProgress(p.log, source.Progress{Stage: p.work})
	}
}

// watch reports the size of the download until stop closes.
func (p *progress) watch(stop chan struct{}) {
	t := time.NewTicker(p.every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		size := p.downloadSize()
		p.mu.Lock()
		if p.downloading && size > 0 {
			source.ReportProgress(p.log, source.Progress{Stage: "正在下载", Done: size})
		}
		p.mu.Unlock()
	}
}

// downloadSize adds up the files written since the work started in the
// directories of the package: <id>.<version>.
func (p *progress) downloadSize() int64 {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return 0
	}
	prefix := strings.ToLower(p.id) + "."
	var total int64
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if !e.IsDir() || !strings.HasPrefix(name, prefix) || len(name) == len(prefix) || name[len(prefix)] < '0' || name[len(prefix)] > '9' {
			continue
		}
		filepath.WalkDir(filepath.Join(p.dir, e.Name()), func(_ string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if fi, err := d.Info(); err == nil && !fi.ModTime().Before(p.start.Add(-time.Minute)) {
				total += fi.Size()
			}
			return nil
		})
	}
	return total
}

// close stops watching the download.
func (p *progress) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.downloading {
		p.downloading = false
		close(p.stop)
	}
}
