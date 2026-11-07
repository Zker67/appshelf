// Package runner runs the external programs AppShelf drives: winget,
// PowerShell scripts for the WinGet module and scoop, and installers. Every
// command gets its arguments as a list, never a line for a shell to split.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Result is what a command did.
type Result struct {
	ExitCode int    // -1 when it did not run to its end
	Output   []byte // its last output, stdout and stderr as they came
}

// Code returns the exit code as the 32-bit value Windows gives it, as
// winget's 0x8A150061.
func (r Result) Code() uint32 { return uint32(r.ExitCode) }

// Tail returns the last non-empty lines of the output, for messages.
func (r Result) Tail(n int) string { return Tail(r.Output, n) }

// Options adjusts how a command runs.
type Options struct {
	// Log receives a copy of the output as it comes.
	Log io.Writer
	// Timeout ends the command after the duration; 0 waits as long as ctx.
	Timeout time.Duration
}

// keep is how much output a Result keeps in memory.
const keep = 256 << 10

// Run runs name with args and waits for it, without a console window. A
// non-zero exit code is no error: read Result.ExitCode. The error reports
// a command that could not start, or that ctx or the timeout ended.
func Run(ctx context.Context, name string, args []string, opt Options) (Result, error) {
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	hideConsole(cmd)
	buf := &tailBuffer{max: keep}
	var w io.Writer = buf
	if opt.Log != nil {
		w = io.MultiWriter(buf, opt.Log)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.WaitDelay = 5 * time.Second
	err := cmd.Run()
	res := Result{ExitCode: -1, Output: buf.Bytes()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return res, fmt.Errorf("%s 超时未结束", name)
		}
		return res, ctx.Err()
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return res, fmt.Errorf("无法运行 %s：%w", name, err)
	}
	return res, nil
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if over := len(t.b) - t.max; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) Bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return bytes.Clone(t.b)
}

// Tail returns the last n non-empty lines of output, cleaned of the
// carriage returns and spinners of progress output.
func Tail(output []byte, n int) string {
	text := strings.ReplaceAll(string(output), "\r\n", "\n")
	var lines []string
	for line := range strings.SplitSeq(text, "\n") {
		// A progress line redraws itself after carriage returns: keep what
		// it ended as.
		if i := strings.LastIndexByte(line, '\r'); i >= 0 {
			line = line[i+1:]
		}
		line = strings.TrimSpace(stripANSI(line))
		if line == "" || strings.Trim(line, `-\|/ █▒░▓`) == "" {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
