package runner

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestWorker runs a Worker on Windows PowerShell for real.
func TestWorker(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	w := NewWorker()
	t.Cleanup(w.Close)
	ctx := context.Background()
	pid := func() int {
		t.Helper()
		var n int
		if err := w.Run(ctx, `Write-Result $PID`, nil, &n, 0); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Input and result keep any text; what the script writes to the host is
	// skipped.
	in := map[string]string{"Text": "-Name 微信 \"quoted\" $env:PATH `tick\n@@appshelf:x"}
	var out struct {
		Echo   string
		Length int
	}
	err := w.Run(ctx, `Write-Host "@@appshelf:noise"; Write-Output "pipeline"; Write-Result @{ Echo = $P.Text; Length = $P.Text.Length }`, in, &out, 0)
	if err != nil || out.Echo != in["Text"] || out.Length != len([]rune(in["Text"])) {
		t.Errorf("echo: %+v, %v", out, err)
	}

	// An error comes back with its message; the process stays.
	first := pid()
	err = w.Run(ctx, `throw "出错了：无法继续"`, nil, nil, 0)
	var se *ScriptError
	if !errors.As(err, &se) || !strings.Contains(se.Message, "出错了：无法继续") {
		t.Errorf("throw: %v", err)
	}
	if err := w.Run(ctx, `Get-Item -LiteralPath 'Z:\no\such\file'`, nil, nil, 0); !errors.As(err, &se) {
		t.Errorf("a non-terminating error: %v", err)
	}
	if err := w.Run(ctx, `$x = 1`, nil, &out, 0); err == nil {
		t.Error("no result: no error")
	}
	var empty []string
	if err := w.Run(ctx, `Write-Result @()`, nil, &empty, 0); err != nil || len(empty) != 0 {
		t.Errorf("empty array: %v %v", empty, err)
	}
	if got := pid(); got != first {
		t.Errorf("process changed: %d, then %d", first, got)
	}

	// A caller that gives up on a quick request keeps the process.
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	err = w.Run(short, `Start-Sleep -Milliseconds 400; Write-Result 1`, nil, nil, 0)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("canceled: %v", err)
	}
	if got := pid(); got != first {
		t.Errorf("a quick canceled request ended the process")
	}

	// A long one is ended, and the next request starts another process.
	short, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
	w.Run(short, `Start-Sleep -Seconds 30`, nil, nil, 0)
	cancel()
	start := time.Now()
	second := pid()
	if second == first {
		t.Error("a long canceled request kept running")
	}
	if d := time.Since(start); d > workerGrace+5*time.Second {
		t.Errorf("the next request waited %v", d)
	}

	// So does one past its timeout, and one that exits.
	if err := w.Run(ctx, `Start-Sleep -Seconds 30`, nil, nil, 200*time.Millisecond); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Errorf("timeout: %v", err)
	}
	if err := w.Run(ctx, `exit 3`, nil, nil, 0); err == nil {
		t.Error("exit: no error")
	}
	if got := pid(); got == second {
		t.Error("the process outlived its timeout or exit")
	}

	// An idle process ends.
	w.Idle = 200 * time.Millisecond
	idle := pid()
	time.Sleep(time.Second)
	if got := pid(); got == idle {
		t.Error("an idle process kept running")
	}
}

// TestWorkerFaster checks what the worker is for: a request once warm takes
// far less than starting PowerShell.
func TestWorkerFaster(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	w := NewWorker()
	t.Cleanup(w.Close)
	ctx := context.Background()
	if err := w.Warm(ctx, `$null = 1`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	const n = 5
	for range n {
		if err := w.Run(ctx, `Write-Result 1`, nil, nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	warm := time.Since(start) / n
	start = time.Now()
	if _, err := PowerShell(ctx, `Write-Result 1`, nil, nil, Options{}); err != nil {
		t.Fatal(err)
	}
	cold := time.Since(start)
	t.Logf("warm worker %v a request, a new PowerShell %v", warm, cold)
	if warm*3 > cold {
		t.Errorf("warm worker %v is not much faster than a new PowerShell %v", warm, cold)
	}
}
