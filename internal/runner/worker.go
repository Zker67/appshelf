package runner

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// workerLoop is the script a Worker runs: it reads one request a line and
// answers each with one line. Both are base64 of UTF-8 JSON, so the console's
// code page never touches the text; answers carry a marker and the
// request's number, so what the scripts write to the host is skipped.
const workerLoop = hostSetup + `
$script:resultJson = $null
function Write-Result($value) {
  $script:resultJson = ConvertTo-Json -InputObject $value -Depth 8 -Compress
}
function Write-Failure([string]$message) { throw $message }
$stdin = [Console]::In
$stdout = [Console]::Out
while ($true) {
  $line = $stdin.ReadLine()
  if ($null -eq $line) { break }
  $script:resultJson = $null
  $id = ''
  try {
    $req = $utf8.GetString([Convert]::FromBase64String($line)) | ConvertFrom-Json
    $id = [string]$req.Id
    $P = $req.Input
    $null = & ([scriptblock]::Create([string]$req.Body))
    $resp = @{ Ok = $true; Result = $script:resultJson }
  } catch {
    $resp = @{ Ok = $false; Error = [string]$_.Exception.Message }
  }
  $json = ConvertTo-Json -InputObject $resp -Compress
  $stdout.WriteLine('` + workerMarker + `' + $id + ':' + [Convert]::ToBase64String($utf8.GetBytes($json)))
  $stdout.Flush()
}
`

const workerMarker = "@@appshelf:"

// workerGrace is how long a request whose caller gave up may still run
// before the process is ended: a quick one finishes, and the process stays.
const workerGrace = 2 * time.Second

// Worker is a Windows PowerShell kept running to run scripts one at a time,
// without starting PowerShell and importing modules for each. Scripts are
// as for PowerShell, with $P and Write-Result, but must not exit: an error
// they throw comes back as a *ScriptError. The process starts on first use
// and ends after Idle unused; one that fails is started again next time.
type Worker struct {
	// Idle ends the process after this long unused; 0 means 3 minutes.
	Idle time.Duration

	sem   chan struct{} // held while a request runs
	mu    sync.Mutex    // guards proc, timer and next
	next  uint64        // the number of the next request
	proc  *workerProc
	timer *time.Timer
}

type workerProc struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	lines chan string   // the answers, in order
	done  chan struct{} // closed when the process has ended
	quit  chan struct{} // closed when it is being ended
	stop  sync.Once
	dir   string
}

// NewWorker returns a Worker; its process starts on first use.
func NewWorker() *Worker { return &Worker{sem: make(chan struct{}, 1)} }

type workerReply struct {
	Ok     bool
	Result *string
	Error  string
}

// Run runs body in the worker with input as $P, decoding what it passes to
// Write-Result into out when out is not nil. It waits for the requests
// before it. A timeout of 0 waits as long as ctx.
func (w *Worker) Run(ctx context.Context, body string, input, out any, timeout time.Duration) error {
	if input == nil {
		input = struct{}{}
	}
	select {
	case w.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	w.mu.Lock()
	w.next++
	id := strconv.FormatUint(w.next, 10)
	w.mu.Unlock()
	req, err := json.Marshal(struct {
		ID    string `json:"Id"`
		Body  string
		Input any
	}{id, body, input})
	if err != nil {
		<-w.sem
		return err
	}
	p, err := w.process()
	if err != nil {
		<-w.sem
		return err
	}
	if _, err := io.WriteString(p.in, base64.StdEncoding.EncodeToString(req)+"\n"); err != nil {
		w.end(p)
		<-w.sem
		return fmt.Errorf("PowerShell 工作进程无法接收请求：%w", err)
	}
	var deadline <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		deadline = t.C
	}
	reply := make(chan string, 1)
	gone := make(chan struct{})
	go func() {
		defer close(reply)
		for {
			select {
			case line, ok := <-p.lines:
				if !ok {
					return
				}
				if rest, ok := strings.CutPrefix(line, id+":"); ok {
					reply <- rest
					return
				}
			case <-gone:
				return
			}
		}
	}()
	select {
	case line, ok := <-reply:
		if !ok {
			w.end(p)
			<-w.sem
			return errors.New("PowerShell 工作进程意外结束")
		}
		w.release()
		return decodeReply(line, out)
	case <-deadline:
		close(gone)
		w.end(p)
		<-w.sem
		return errors.New("PowerShell 超时未结束")
	case <-ctx.Done():
		// Let a quick request finish and keep the process; end it otherwise.
		go func() {
			select {
			case _, ok := <-reply:
				if !ok {
					w.end(p)
				}
			case <-time.After(workerGrace):
				close(gone)
				w.end(p)
			}
			w.release()
		}()
		return ctx.Err()
	}
}

func decodeReply(line string, out any) error {
	data, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		return fmt.Errorf("PowerShell 工作进程的回复无法解析：%w", err)
	}
	var r workerReply
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("PowerShell 工作进程的回复无法解析：%w", err)
	}
	if !r.Ok {
		return &ScriptError{ExitCode: 1, Message: strings.TrimSpace(r.Error)}
	}
	if out == nil {
		return nil
	}
	if r.Result == nil {
		return errors.New("PowerShell 脚本没有返回结果")
	}
	if err := json.Unmarshal([]byte(*r.Result), out); err != nil {
		return fmt.Errorf("PowerShell 脚本的结果无法解析：%w", err)
	}
	return nil
}

// Warm starts the process, if it is not running, so that the next request
// does not wait for it.
func (w *Worker) Warm(ctx context.Context, body string) error {
	return w.Run(ctx, body, nil, nil, time.Minute)
}

// Close ends the process; the next request starts another.
func (w *Worker) Close() {
	w.mu.Lock()
	p := w.proc
	w.mu.Unlock()
	if p != nil {
		w.end(p)
	}
}

// release frees the worker for the next request and restarts the idle
// timer.
func (w *Worker) release() {
	w.mu.Lock()
	idle := w.Idle
	if idle <= 0 {
		idle = 3 * time.Minute
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(idle, w.closeIdle)
	w.mu.Unlock()
	<-w.sem
}

// closeIdle ends the process unless a request is running.
func (w *Worker) closeIdle() {
	select {
	case w.sem <- struct{}{}:
		w.Close()
		<-w.sem
	default:
	}
}

// process returns the running process, starting one when there is none.
func (w *Worker) process() (*workerProc, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if p := w.proc; p != nil {
		select {
		case <-p.done:
		default:
			return p, nil
		}
	}
	p, err := startWorker()
	if err != nil {
		return nil, err
	}
	w.proc = p
	return p, nil
}

// end kills a process and forgets it.
func (w *Worker) end(p *workerProc) {
	w.mu.Lock()
	if w.proc == p {
		w.proc = nil
	}
	w.mu.Unlock()
	p.stop.Do(func() {
		close(p.quit)
		p.in.Close()
		if p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
	})
	<-p.done
}

func startWorker() (*workerProc, error) {
	dir, err := os.MkdirTemp("", "appshelf-psw-")
	if err != nil {
		return nil, err
	}
	script := filepath.Join(dir, "worker.ps1")
	if err := os.WriteFile(script, append([]byte("\xef\xbb\xbf"), workerLoop...), 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	cmd := exec.Command(PowerShellPath(), "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script)
	hideConsole(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("无法启动 PowerShell：%w", err)
	}
	p := &workerProc{cmd: cmd, in: in, lines: make(chan string, 1), done: make(chan struct{}), quit: make(chan struct{}), dir: dir}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 64<<20)
		for sc.Scan() {
			if line, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), workerMarker); ok {
				select {
				case p.lines <- line:
				case <-p.quit:
				}
			}
		}
		close(p.lines)
		cmd.Wait()
		os.RemoveAll(dir)
		close(p.done)
	}()
	return p, nil
}
