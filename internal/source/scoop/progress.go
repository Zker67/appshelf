package scoop

import (
	"io"
	"strings"
	"sync"

	"appshelf/internal/runner"
	"appshelf/internal/source"
)

// stages are the lines scoop starts its steps with, always in English, and
// the stage each one starts. "" keeps the stage it is in.
var stages = []struct{ prefix, stage string }{
	{"checking repo", "正在添加 bucket"},
	{"updating '", "正在更新"},
	{"installing '", "install"},
	{"uninstalling '", "uninstall"},
	{"downloading", "正在下载"},
	{"starting download", "正在下载"},
	{"checking hash", "正在校验"},
	{"extracting", "正在解压"},
	{"running installer", "正在运行安装程序"},
	{"running pre_install", "正在运行安装脚本"},
	{"running post_install", "正在运行安装脚本"},
	{"linking", "正在完成"},
	{"creating shim", "正在完成"},
	{"creating shortcut", "正在完成"},
	{"persisting", "正在完成"},
}

// progress follows scoop's output for the stage of its work.
type progress struct {
	log  io.Writer
	verb string // "install", "update" or "uninstall"

	mu   sync.Mutex
	line []byte
}

func newProgress(log io.Writer, verb string) *progress {
	source.ReportProgress(log, source.Progress{Stage: "正在准备"})
	return &progress{log: log, verb: verb}
}

func (p *progress) Write(b []byte) (int, error) {
	n, err := p.log.Write(b)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range b {
		if c == '\n' || c == '\r' {
			p.lineDone(strings.ToLower(strings.TrimSpace(runner.StripANSI(string(p.line)))))
			p.line = p.line[:0]
			continue
		}
		p.line = append(p.line, c)
	}
	return n, err
}

func (p *progress) lineDone(line string) {
	for _, s := range stages {
		if !strings.HasPrefix(line, s.prefix) {
			continue
		}
		stage := s.stage
		switch {
		case stage == "install" && p.verb == "update":
			stage = "正在安装新版本"
		case stage == "install":
			stage = "正在安装"
		case stage == "uninstall" && p.verb == "update":
			stage = "正在移除旧版本"
		case stage == "uninstall":
			stage = "正在卸载"
		}
		source.ReportProgress(p.log, source.Progress{Stage: stage})
		return
	}
}
