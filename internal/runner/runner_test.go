package runner

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestTail(t *testing.T) {
	out := []byte("first\r\n  \r\n\x1b[32mgreen\x1b[0m\r\nprogress 10%\rprogress 100%\n  -  \n██▒▒\nlast\n")
	if got := Tail(out, 3); got != "green\nprogress 100%\nlast" {
		t.Errorf("got %q", got)
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	b := &tailBuffer{max: 4}
	b.Write([]byte("abc"))
	b.Write([]byte("defg"))
	if got := string(b.Bytes()); got != "defg" {
		t.Errorf("got %q", got)
	}
}

// TestPowerShell runs Windows PowerShell for real: the input arrives
// whatever it holds, the result comes back as UTF-8, and an error comes
// back with its message.
func TestPowerShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell")
	}
	ctx := context.Background()
	in := map[string]string{"Text": "-Name 微信 \"quoted\" $env:PATH `tick"}
	var out struct {
		Echo   string
		Length int
		List   []int
	}
	_, err := PowerShell(ctx, `Write-Result @{ Echo = $P.Text; Length = $P.Text.Length; List = @(1) }`, in, &out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Echo != in["Text"] || out.Length != len([]rune(in["Text"])) || len(out.List) != 1 {
		t.Errorf("got %+v", out)
	}

	_, err = PowerShell(ctx, `throw "出错了：无法继续"`, nil, nil, Options{})
	var se *ScriptError
	if !errors.As(err, &se) || se.ExitCode != 1 || !strings.Contains(se.Message, "出错了：无法继续") {
		t.Errorf("error: %v", err)
	}

	var empty []string
	if _, err := PowerShell(ctx, `Write-Result @()`, nil, &empty, Options{}); err != nil || len(empty) != 0 {
		t.Errorf("empty array: %v %v", empty, err)
	}

	res, err := PowerShell(ctx, `Write-Host "to the log"; exit 7`, nil, nil, Options{})
	if !errors.As(err, &se) || se.ExitCode != 7 || !strings.Contains(string(res.Output), "to the log") {
		t.Errorf("exit 7: %v, output %q", err, res.Output)
	}
}

func TestRunMissingProgram(t *testing.T) {
	if _, err := Run(context.Background(), "appshelf-no-such-program", nil, Options{}); err == nil {
		t.Error("a missing program ran")
	}
}
