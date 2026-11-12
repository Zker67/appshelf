package winget

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"appshelf/internal/source"
)

func TestComProgress(t *testing.T) {
	cases := []struct {
		status  string
		percent int
		stage   string
		want    float64
	}{
		{"Downloading", 0, "正在下载", 0},
		{"Downloading", 42, "正在下载", 42},
		{"Downloading", 100, "正在下载", 0},
		{"Installing", -1, "正在更新", 0},
		{"Installing", 30, "正在更新", 30},
		{"Uninstalling", 0, "正在卸载", 0},
		{"PostInstall", 0, "正在完成", 0},
		{"Queued", 0, "正在准备", 0},
		{"", 0, "正在准备", 0},
		{"Repairing", 0, "Repairing", 0},
	}
	for _, tc := range cases {
		p := comProgress(tc.status, tc.percent, "正在更新")
		if p.Stage != tc.stage || p.Percent != tc.want || p.Done != 0 || p.Total != 0 {
			t.Errorf("%q %d = %+v, want %s %v", tc.status, tc.percent, p, tc.stage, tc.want)
		}
	}
}

// The script's output, cut anywhere: progress lines are reported, not
// logged; the log gets the other lines and a line per stage.
func TestComLogReportsProgress(t *testing.T) {
	rec := &recorder{}
	c := &comLog{log: rec, work: "正在安装"}
	out := "WARNING: something\r\n" +
		progressMark + "Downloading\t0\r\n" +
		progressMark + "Downloading\t37\r\n" +
		progressMark + "Installing\t0\r\n" +
		"tail without newline"
	for i := 0; i < len(out); i += 7 {
		fmt.Fprint(c, out[i:min(i+7, len(out))])
	}
	c.flush()
	stages, last := rec.get()
	if strings.Join(stages, ",") != "正在下载,正在安装" {
		t.Errorf("stages = %q", stages)
	}
	if last != (source.Progress{Stage: "正在安装"}) {
		t.Errorf("last = %+v", last)
	}
	log := rec.String()
	for _, want := range []string{"WARNING: something\n", "[winget] 正在下载\n", "[winget] 正在安装\n", "tail without newline\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, progressMark) || strings.Count(log, "正在下载") != 1 {
		t.Errorf("progress lines logged:\n%s", log)
	}
}

func TestComLogKeepsThePercent(t *testing.T) {
	rec := &recorder{}
	c := &comLog{log: rec, work: "正在安装"}
	fmt.Fprint(c, progressMark+"Downloading\t57\n")
	// A download can be canceled.
	if _, last := rec.get(); last != (source.Progress{Stage: "正在下载", Percent: 57, Cancelable: true}) {
		t.Errorf("last = %+v", last)
	}
}

func TestInterpretCOM(t *testing.T) {
	ok := func(r comResult, verb string, want source.Result) {
		t.Helper()
		got, err := interpretCOM(r, verb)
		if err != nil || got != want {
			t.Errorf("%+v: %+v %v, want %+v", r, got, err, want)
		}
	}
	fails := func(r comResult, verb, text string) {
		t.Helper()
		_, err := interpretCOM(r, verb)
		if err == nil || !strings.Contains(err.Error(), text) {
			t.Errorf("%+v: %v, want %q", r, err, text)
		}
	}
	ok(comResult{Status: "Ok"}, "install", source.Result{})
	ok(comResult{Status: "Ok", RebootRequired: true}, "upgrade", source.Result{RebootRequired: true})
	ok(comResult{Status: "AlreadyInstalled"}, "install", source.Result{AlreadyInstalled: true})
	ok(comResult{Status: "NotInstalled"}, "uninstall", source.Result{})
	// HRESULTs read as signed 32-bit numbers in PowerShell.
	hr := func(c uint32) int64 { return int64(int32(c)) }
	fails(comResult{Status: "InstallError", Code: hr(0x8A150105)}, "install", "磁盘空间不足")
	fails(comResult{Status: "InstallError", Code: hr(codeCanceledByUser)}, "uninstall", "卸载被取消")
	fails(comResult{Status: "InstallError", Code: hr(codeCanceledByUser)}, "install", "安装被取消")
	fails(comResult{Status: "InstallError", Code: 0x80070005, Message: "Access is denied."}, "install", "Access is denied.")
	fails(comResult{Status: "InstallError", InstallerCode: 1603}, "install", "安装程序退出码 1603")
	fails(comResult{Status: "UninstallError", InstallerCode: 2}, "uninstall", "卸载程序退出码 2")
	fails(comResult{Status: "NoApplicableUpgrade"}, "upgrade", "没有适用的更新")
	fails(comResult{Status: "DownloadError"}, "install", "下载安装程序失败")
	fails(comResult{Status: "Weird"}, "install", "Weird")
	// An installer that cannot update in place: removing it and installing
	// it again is offered.
	if _, err := interpretCOM(comResult{Status: "InstallError", Code: hr(codeUpgradeNotSupported), InstallerCode: 1}, "upgrade"); !errors.Is(err, source.ErrNoDirectUpgrade) {
		t.Errorf("upgrade not supported: %v", err)
	}
}

// TestCOMOfThisMachine goes through the module's cmdlets for packages that
// are not there: an update fails, a removal finds nothing to remove.
// Nothing on the machine changes.
func TestCOMOfThisMachine(t *testing.T) {
	s := available(t)
	ctx := context.Background()
	const id = "AppShelf.Test.NoSuchPackage"
	rec := &recorder{}
	_, err := s.Upgrade(ctx, app(id), rec)
	if err == nil || errors.Is(err, errUnavailable) {
		t.Errorf("update of a package not there: %v", err)
	}
	if !strings.Contains(rec.String(), "Update-WinGetPackage") || strings.Contains(rec.String(), "> winget upgrade") {
		t.Errorf("not through the module:\n%s", rec.String())
	}
	t.Logf("update: %v", err)
	rec = &recorder{}
	if res, err := s.Uninstall(ctx, id, "winget", false, rec); err != nil || res != (source.Result{}) {
		t.Errorf("removal of a package not there: %+v %v\n%s", res, err, rec.String())
	}
}
