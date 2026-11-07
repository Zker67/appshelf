package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"log"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
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

// tray is AppShelf's notification area icon, while the user wants AppShelf
// to stay there when its window closes.
type tray struct {
	open  func() // shows the window
	check func() // checks for updates

	// on is read on the main thread as the window closes, apart from mu,
	// which set holds while the main thread makes the icon.
	on   atomic.Bool
	mu   sync.Mutex
	icon *mygo.Tray
}

// enabled reports whether closing the window hides it.
func (t *tray) enabled() bool { return t.on.Load() }

// set shows or removes the icon; the app must be ready.
func (t *tray) set(on bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.on.Store(on)
	switch {
	case on && t.icon == nil:
		icon, err := mygo.NewTray(mygo.TrayOptions{ID: "appshelf", Icon: trayIcon(), ToolTip: "AppShelf 软件图书馆"})
		if err != nil {
			log.Println("tray:", err)
			t.on.Store(false)
			return
		}
		menu := mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开软件图书馆", Click: func(*mygo.MenuItem, *mygo.Window) { t.open() }},
			{Label: "检查更新", Click: func(*mygo.MenuItem, *mygo.Window) { go t.check() }},
			mygo.Separator(),
			{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		})
		// A click opens the window; the menu shows on a right-click.
		icon.OnClick(t.open)
		icon.OnRightClick(func() {
			icon.SetMenu(menu)
			icon.PopUpMenu()
			icon.SetMenu(nil)
		})
		t.icon = icon
	case !on && t.icon != nil:
		t.icon.Destroy()
		t.icon = nil
	}
}
