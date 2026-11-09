package tasklog

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"appshelf/internal/source"
)

type buffer struct {
	bytes.Buffer
	closed bool
}

func (b *buffer) Close() error { b.closed = true; return nil }

func TestLinesAreCleaned(t *testing.T) {
	f := &buffer{}
	var calls atomic.Int32
	l := New(f, func() { calls.Add(1) })
	raw := "first\r\n\x1b[32mgreen\x1b[0m\n\n\n  -\r  \\\r  |\n" +
		"download  10%\rdownload  50%\rdownload 100%\n" +
		"██████▒▒▒▒\n" +
		"tab\there   \n" +
		"partial"
	fmt.Fprint(l, raw)
	c := l.Lines(0)
	want := []string{"first", "green", "", "download 100%", "tab here"}
	if strings.Join(c.Lines, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", c.Lines, want)
	}
	if c.Live != "partial" || c.Start != 0 || c.Next != len(want) {
		t.Fatalf("chunk = %+v", c)
	}
	if f.String() != raw {
		t.Fatal("the file must keep the output as it came")
	}
	if calls.Load() == 0 {
		t.Fatal("changed was not called")
	}
	if err := l.Close(); err != nil || !f.closed {
		t.Fatal("Close must close the file")
	}
	if c := l.Lines(len(want)); len(c.Lines) != 1 || c.Lines[0] != "partial" || c.Live != "" {
		t.Fatalf("after Close: %+v", c)
	}
	// Writes after Close keep the lines but leave the file alone.
	fmt.Fprintln(l, "late")
	if strings.Contains(f.String(), "late") {
		t.Fatal("wrote to a closed file")
	}
}

func TestCarriageReturnRedrawsInSeparateWrites(t *testing.T) {
	l := New(nil, nil)
	fmt.Fprint(l, "a 1%")
	fmt.Fprint(l, "\r")
	if c := l.Lines(0); c.Live != "a 1%" {
		t.Fatalf("live before redraw = %q", c.Live)
	}
	fmt.Fprint(l, "a 2%")
	if c := l.Lines(0); c.Live != "a 2%" || len(c.Lines) != 0 {
		t.Fatalf("after redraw = %+v", c)
	}
	fmt.Fprint(l, "\r")
	fmt.Fprint(l, "\n")
	if c := l.Lines(0); len(c.Lines) != 1 || c.Lines[0] != "a 2%" {
		t.Fatalf("CR then LF must end the line: %+v", c)
	}
}

func TestKeepsTheLastLines(t *testing.T) {
	l := New(nil, nil)
	for i := range MaxLines + 10 {
		fmt.Fprintf(l, "line %d\n", i)
	}
	c := l.Lines(0)
	if c.Start != 10 || len(c.Lines) != MaxLines || c.Lines[0] != "line 10" || c.Next != MaxLines+10 {
		t.Fatalf("start %d, %d lines, first %q, next %d", c.Start, len(c.Lines), c.Lines[0], c.Next)
	}
	// Reading on from a cursor returns only the new lines.
	fmt.Fprintln(l, "new")
	c = l.Lines(c.Next)
	if len(c.Lines) != 1 || c.Lines[0] != "new" || c.Start != MaxLines+10 {
		t.Fatalf("from cursor: %+v", c)
	}
	// A cursor past the end returns nothing.
	if c := l.Lines(1 << 30); len(c.Lines) != 0 || c.Next != MaxLines+11 {
		t.Fatalf("past the end: %+v", c)
	}
}

func TestLongLinesAreCut(t *testing.T) {
	l := New(nil, nil)
	fmt.Fprint(l, strings.Repeat("x", maxLine+5))
	c := l.Lines(0)
	if len(c.Lines) != 1 || len(c.Lines[0]) != maxLine || c.Live != "xxxxx" {
		t.Fatalf("lines %d, live %q", len(c.Lines), c.Live)
	}
}

func TestProgress(t *testing.T) {
	l := New(nil, nil)
	source.ReportProgress(l, source.Progress{Stage: "正在下载", Done: 5, Total: 10})
	if p := l.Progress(); p.Stage != "正在下载" || p.Done != 5 || p.Total != 10 {
		t.Fatalf("progress = %+v", p)
	}
	// Other writers ignore progress.
	source.ReportProgress(&bytes.Buffer{}, source.Progress{Stage: "x"})
}
