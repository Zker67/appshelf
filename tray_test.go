package main

import (
	"bytes"
	"image/png"
	"testing"
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
