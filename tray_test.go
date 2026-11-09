package main

import (
	"bytes"
	"image/png"
	"testing"

	"appshelf/internal/api"
)

func TestTrayIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("size %v", b)
	}
	// The logo's middle is opaque, its corners transparent.
	if _, _, _, a := img.At(16, 16).RGBA(); a < 0xf000 {
		t.Errorf("middle alpha %x", a)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a > 0x4000 {
		t.Errorf("corner alpha %x", a)
	}
}

func TestTrayBadge(t *testing.T) {
	plain := trayIcon()
	img, err := png.Decode(bytes.NewReader(trayBadge(plain)))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("size %v", b)
	}
	// The dot is opaque orange at the lower right; the middle is the logo's.
	if r, g, b, a := img.At(24, 24).RGBA(); a < 0xf000 || r < 0xe000 || g > 0x9000 || b > 0x4000 {
		t.Errorf("dot %x %x %x %x", r, g, b, a)
	}
	src, _ := png.Decode(bytes.NewReader(plain))
	if img.At(10, 10) != src.At(10, 10) {
		t.Errorf("logo changed: %v, was %v", img.At(10, 10), src.At(10, 10))
	}
}

func TestTrayTexts(t *testing.T) {
	if got := trayToolTip(api.TrayStatus{}); got != "AppShelf 软件图书馆" {
		t.Errorf("idle tooltip %q", got)
	}
	busy := api.TrayStatus{Updates: 3, Runnable: 2, Active: 3, Running: "Visual Studio Code Insiders Edition", Action: api.ActionUpgrade}
	want := "AppShelf 软件图书馆\n3 个软件可更新\n正在更新 Visual Studio Code Insi…，另有 2 项排队"
	if got := trayToolTip(busy); got != want {
		t.Errorf("busy tooltip %q", got)
	}
	if got := trayToolTip(api.TrayStatus{Checking: true, Updates: 3, Active: 1}); got != "AppShelf 软件图书馆\n正在检查更新…\n1 项任务等待中" {
		t.Errorf("checking tooltip %q", got)
	}
	if n := len([]rune(trayToolTip(busy))); n > 127 {
		t.Errorf("tooltip of %d characters", n)
	}
	u, all, check, tasks := trayMenuLabels(busy)
	if u != "查看更新（3）" || all != "全部更新（2 个）" || check != "检查更新" || tasks != "任务（3 项进行中）" {
		t.Errorf("labels %q %q %q %q", u, all, check, tasks)
	}
	u, all, check, tasks = trayMenuLabels(api.TrayStatus{Checking: true})
	if u != "查看更新" || all != "全部更新" || check != "正在检查更新…" || tasks != "任务" {
		t.Errorf("idle labels %q %q %q %q", u, all, check, tasks)
	}
	if got := trayQuitDetail(busy); got != "正在更新「Visual Studio Code Insiders Edition」，退出可能让它中断；排队的任务不会再执行。" {
		t.Errorf("quit detail %q", got)
	}
}
