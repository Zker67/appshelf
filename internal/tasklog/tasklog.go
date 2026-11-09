// Package tasklog keeps the log of a task: the output as it came, in a
// file, and the same output as clean lines in memory, for the window to
// show while the task runs, with the progress the source reports.
package tasklog

import (
	"io"
	"strings"
	"sync"

	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// MaxLines is how many lines a log keeps in memory; the file keeps all.
const MaxLines = 2000

// maxLine is the longest line kept whole: output without newlines is cut.
const maxLine = 16 << 10

// Log writes a task's output to its file and keeps its last lines. It is
// safe for concurrent use.
type Log struct {
	changed func()

	mu       sync.Mutex
	file     io.WriteCloser
	lines    []string
	start    int    // the number of lines dropped before lines[0]
	cur      []byte // the line being written
	cr       bool   // a carriage return came last: the line redraws, unless a newline follows
	progress source.Progress
	closed   bool
}

// New returns a log writing to file, which may be nil, and calling changed,
// when not nil, after new output or progress.
func New(file io.WriteCloser, changed func()) *Log {
	if changed == nil {
		changed = func() {}
	}
	return &Log{file: file, changed: changed}
}

// Write records output. A failing file does not fail the task: the lines
// are still kept.
func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	if l.file != nil && !l.closed {
		l.file.Write(p)
	}
	for _, c := range p {
		switch c {
		case '\n':
			l.commit()
			l.cr = false
		case '\r':
			l.cr = true
		default:
			if l.cr {
				// The line is drawn again from its start, as progress is.
				l.cur = l.cur[:0]
				l.cr = false
			}
			l.cur = append(l.cur, c)
			if len(l.cur) >= maxLine {
				l.commit()
			}
		}
	}
	l.mu.Unlock()
	l.changed()
	return len(p), nil
}

// commit ends the line being written: noise is dropped, and blank lines
// are kept only one between lines of text.
func (l *Log) commit() {
	line := Clean(string(l.cur))
	l.cur = l.cur[:0]
	switch {
	case line == "":
		if n := len(l.lines); n == 0 || l.lines[n-1] == "" {
			return
		}
	case isNoise(line):
		return
	}
	l.lines = append(l.lines, line)
	if over := len(l.lines) - MaxLines; over > 0 {
		l.lines = append(l.lines[:0], l.lines[over:]...)
		l.start += over
	}
}

// Clean makes a line of output readable: no escape sequences or control
// characters, no trailing spaces.
func Clean(s string) string {
	s = runner.StripANSI(s)
	if strings.IndexFunc(s, isControl) >= 0 {
		s = strings.Map(func(r rune) rune {
			switch {
			case r == '\t':
				return ' '
			case isControl(r):
				return -1
			}
			return r
		}, s)
	}
	return strings.TrimRight(s, " ")
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

// isNoise reports a line of nothing but a spinner or a progress bar.
func isNoise(line string) bool { return strings.Trim(line, `-\|/ █▒░▓`) == "" }

// ReportProgress records how far the task has come.
func (l *Log) ReportProgress(p source.Progress) {
	l.mu.Lock()
	l.progress = p
	l.mu.Unlock()
	l.changed()
}

// Progress returns what was last reported.
func (l *Log) Progress() source.Progress {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.progress
}

// Chunk is part of a log, from a line on.
type Chunk struct {
	// Lines are the lines from Start on; Next is the number to ask from
	// next time.
	Lines []string
	Start int
	Next  int
	// Live is the line being written, as a progress line redrawn, which
	// may still change.
	Live string
}

// Lines returns the lines from from on: those dropped are skipped, which
// Start tells.
func (l *Log) Lines(from int) Chunk {
	l.mu.Lock()
	defer l.mu.Unlock()
	from = max(from, l.start)
	end := l.start + len(l.lines)
	from = min(from, end)
	c := Chunk{Lines: append([]string(nil), l.lines[from-l.start:]...), Start: from, Next: end}
	if live := Clean(string(l.cur)); live != "" && !isNoise(live) {
		c.Live = live
	}
	return c
}

// Close ends the line being written and closes the file.
func (l *Log) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	if len(l.cur) > 0 {
		l.commit()
	}
	l.closed = true
	var err error
	if l.file != nil {
		err = l.file.Close()
	}
	l.mu.Unlock()
	l.changed()
	return err
}
