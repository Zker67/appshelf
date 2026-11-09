package winget

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"appshelf/internal/source"
)

// recorder is a log that keeps the progress reported.
type recorder struct {
	bytes.Buffer
	mu     sync.Mutex
	stages []string
	last   source.Progress
}

func (r *recorder) ReportProgress(p source.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n := len(r.stages); n == 0 || r.stages[n-1] != p.Stage {
		r.stages = append(r.stages, p.Stage)
	}
	r.last = p
}

func (r *recorder) get() ([]string, source.Progress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stages...), r.last
}

func TestProgressFollowsTheStages(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	p := newProgress(rec, "jqlang.jq", "正在安装")
	p.dir, p.every = dir, 10*time.Millisecond
	// Output of winget 1.12 on a Chinese Windows, sent to a file.
	fmt.Fprint(p, "已找到 jq [jqlang.jq] 版本 1.8.2\r\n此应用程序由其所有者授权给你。\r\n")
	fmt.Fprint(p, "正在下载 https://github.com/jqlang/jq/releases/download/jq-1.8.2/jq-windows-amd64.exe\r\n")

	// The download directory of the package grows; another package's does
	// not count.
	pkg := filepath.Join(dir, "jqlang.jq.1.8.2")
	other := filepath.Join(dir, "jqlang.jqx.1.0")
	for _, d := range []string{pkg, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(pkg, "setup.exe"), make([]byte, 4096), 0o644)
	os.WriteFile(filepath.Join(other, "setup.exe"), make([]byte, 100), 0o644)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, last := rec.get(); last.Done == 4096 {
			break
		}
		if time.Now().After(deadline) {
			_, last := rec.get()
			t.Fatalf("download size not reported: %+v", last)
		}
		time.Sleep(5 * time.Millisecond)
	}

	fmt.Fprint(p, "已成功验证安装程序哈希\r\n正在启动程序包安装...\r\n已成功安装\r\n")
	p.close()
	stages, _ := rec.get()
	want := []string{"正在准备", "正在下载", "正在安装"}
	if fmt.Sprint(stages) != fmt.Sprint(want) {
		t.Fatalf("stages = %q, want %q", stages, want)
	}
	if !bytes.Contains(rec.Bytes(), []byte("已成功安装")) {
		t.Fatal("the output must reach the log")
	}
}
