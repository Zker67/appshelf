package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"appshelf/internal/api"
)

//go:embed frontend/src/assets/logo.png
var logoPNG []byte

// trayIcon is the logo at 32×32, the size of a notification area icon on
// a high-resolution display, averaged from the 128×128 logo.
func trayIcon() []byte {
	src, err := png.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return logoPNG
	}
	const size = 32
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			x0, x1 := b.Min.X+x*b.Dx()/size, b.Min.X+(x+1)*b.Dx()/size
			y0, y1 := b.Min.Y+y*b.Dy()/size, b.Min.Y+(y+1)*b.Dy()/size
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := color.NRGBA64Model.Convert(src.At(sx, sy)).(color.NRGBA64)
					// Weigh the colors by their opacity, so that transparent
					// edges do not darken the icon.
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					bl += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					n++
				}
			}
			if n == 0 || a == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r / a >> 8), G: uint8(g / a >> 8), B: uint8(bl / a >> 8), A: uint8(a / n >> 8)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return logoPNG
	}
	return buf.Bytes()
}

// trayBadge is the icon with a dot at its lower right, which says updates
// are offered. A clear ring around the dot sets it apart from the logo.
func trayBadge(icon []byte) []byte {
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return icon
	}
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	size := float64(b.Dx())
	r := size * 0.2       // the dot
	ring := r + size*0.06 // the clear ring around it
	cx, cy := size-r-size*0.02, size-r-size*0.02
	dot := color.NRGBA{R: 0xf9, G: 0x73, B: 0x16, A: 0xff}
	for y := range b.Dy() {
		for x := range b.Dx() {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			// Coverage of the pixel by a disc, over a pixel's width.
			in := func(radius float64) float64 { return min(max(radius-d+0.5, 0), 1) }
			c := dst.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			c.A = uint8(float64(c.A) * (1 - in(ring)))
			if a := in(r); a > 0 {
				c = blend(c, dot, a)
			}
			dst.SetNRGBA(b.Min.X+x, b.Min.Y+y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return icon
	}
	return buf.Bytes()
}

// blend lays over c the opaque color top with coverage a.
func blend(c, top color.NRGBA, a float64) color.NRGBA {
	ca := float64(c.A) / 255
	oa := a + ca*(1-a)
	mix := func(t, u uint8) uint8 { return uint8((float64(t)*a + float64(u)*ca*(1-a)) / oa) }
	return color.NRGBA{R: mix(top.R, c.R), G: mix(top.G, c.G), B: mix(top.B, c.B), A: uint8(oa * 255)}
}

// tray is AppShelf's notification area icon, shown while AppShelf runs: a
// click shows the window; its menu shows the updates and the tasks, and its
// tooltip and a dot on it say what there is.
type tray struct {
	open      func()            // shows the window
	show      func(page string) // shows the window on a page, or "tasks"
	status    func() api.TrayStatus
	check     func() // checks for updates
	updateAll func() // queues the updates offered

	// stay is read on the main thread as the window closes.
	stay atomic.Bool
	// wake asks the icon to show the status again.
	wake chan struct{}

	mu     sync.Mutex
	icon   *mygo.Tray
	last   api.TrayStatus
	plain  []byte // the icon without and with its dot
	badged []byte
	dotted bool
}

// enabled reports whether closing the window hides it.
func (t *tray) enabled() bool { return t.stay.Load() }

// setStay makes closing the window hide it, or quit AppShelf.
func (t *tray) setStay(on bool) { t.stay.Store(on) }

// start shows the icon; the app must be ready. changed does nothing before.
func (t *tray) start() {
	t.plain = trayIcon()
	t.badged = trayBadge(t.plain)
	icon, err := mygo.NewTray(mygo.TrayOptions{ID: "appshelf", Icon: t.plain, ToolTip: trayToolTip(api.TrayStatus{})})
	if err != nil {
		log.Println("tray:", err)
		return
	}
	// A click opens the window; the menu shows on a right-click, made of
	// the status last read, as reading it takes a moment.
	icon.OnClick(t.open)
	icon.OnRightClick(func() {
		t.mu.Lock()
		st := t.last
		t.mu.Unlock()
		icon.SetMenu(t.menu(st))
		icon.PopUpMenu()
		icon.SetMenu(nil)
	})
	wake := make(chan struct{}, 1)
	t.mu.Lock()
	t.icon, t.wake = icon, wake
	t.mu.Unlock()
	go t.follow(icon, wake)
	t.changed()
}

// changed asks the icon to read the status again, as the pages are told
// something changed.
func (t *tray) changed() {
	t.mu.Lock()
	wake := t.wake
	t.mu.Unlock()
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

// follow shows the status each time it may have changed, at most twice a
// second: tasks report their progress far more often.
func (t *tray) follow(icon *mygo.Tray, wake <-chan struct{}) {
	for range wake {
		st := t.status()
		t.mu.Lock()
		t.last = st
		dot := st.Updates > 0
		redraw := dot != t.dotted
		t.dotted = dot
		t.mu.Unlock()
		icon.SetToolTip(trayToolTip(st))
		if redraw {
			img := t.plain
			if dot {
				img = t.badged
			}
			if err := icon.SetIcon(img, false); err != nil {
				log.Println("tray icon:", err)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// menu is the icon's menu for the status st.
func (t *tray) menu(st api.TrayStatus) *mygo.Menu {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}
	updates, all, check, tasks := trayMenuLabels(st)
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "打开软件图书馆", Click: click(t.open)},
		mygo.Separator(),
		{Label: updates, Click: click(func() { t.show("updates") })},
		{Label: all, Disabled: st.Runnable == 0, Click: click(func() { go t.updateAll() })},
		{Label: check, Disabled: st.Checking, Click: click(func() { go t.check() })},
		{Label: tasks, Click: click(func() { t.show("tasks") })},
		mygo.Separator(),
		{Label: "退出", Click: click(func() { go t.quit() })},
	})
}

// quit quits AppShelf, once the user agrees when tasks are not done.
func (t *tray) quit() {
	st := t.status()
	if st.Active > 0 {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Type:          mygo.MessageWarning,
			Title:         "AppShelf 软件图书馆",
			Message:       fmt.Sprintf("还有 %d 项任务没有完成，确定退出吗？", st.Active),
			Detail:        trayQuitDetail(st),
			Buttons:       []string{"退出", "取消"},
			DefaultButton: 1,
			CancelButton:  1,
		})
		if err != nil || res.Button != 0 {
			return
		}
	}
	mygo.App.Quit()
}

// trayMenuLabels are the labels of the menu's items that show the status:
// the updates page, updating all, checking and the tasks.
func trayMenuLabels(st api.TrayStatus) (updates, all, check, tasks string) {
	updates, all, check, tasks = "查看更新", "全部更新", "检查更新", "任务"
	if st.Updates > 0 {
		updates = fmt.Sprintf("查看更新（%d）", st.Updates)
	}
	if st.Runnable > 0 {
		all = fmt.Sprintf("全部更新（%d 个）", st.Runnable)
	}
	if st.Checking {
		check = "正在检查更新…"
	}
	if st.Active > 0 {
		tasks = fmt.Sprintf("任务（%d 项进行中）", st.Active)
	}
	return updates, all, check, tasks
}

// trayToolTip is the icon's hover text for the status st. Windows shows
// 127 characters of it at most.
func trayToolTip(st api.TrayStatus) string {
	lines := []string{"AppShelf 软件图书馆"}
	switch {
	case st.Checking:
		lines = append(lines, "正在检查更新…")
	case st.Updates > 0:
		lines = append(lines, fmt.Sprintf("%d 个软件可更新", st.Updates))
	}
	switch {
	case st.Running != "":
		line := "正在" + actionVerb(st.Action) + " " + shorten(st.Running, 24)
		if st.Active > 1 {
			line += fmt.Sprintf("，另有 %d 项排队", st.Active-1)
		}
		lines = append(lines, line)
	case st.Active > 0:
		lines = append(lines, fmt.Sprintf("%d 项任务等待中", st.Active))
	}
	if st.Reboot > 0 {
		lines = append(lines, fmt.Sprintf("%d 个软件需要重启电脑后生效", st.Reboot))
	}
	return strings.Join(lines, "\n")
}

// trayQuitDetail says what quitting with tasks not done does.
func trayQuitDetail(st api.TrayStatus) string {
	if st.Running == "" {
		return "排队的任务不会再执行。"
	}
	return fmt.Sprintf("正在%s「%s」，退出可能让它中断；排队的任务不会再执行。", actionVerb(st.Action), shorten(st.Running, 40))
}

func actionVerb(a api.TaskAction) string {
	switch a {
	case api.ActionUpgrade:
		return "更新"
	case api.ActionUninstall:
		return "卸载"
	}
	return "安装"
}

// shorten cuts s to n characters, an ellipsis last.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
