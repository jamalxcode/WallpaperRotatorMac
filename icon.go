package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// menuIcon draws a small "picture frame with mountains and sun" template icon.
// Template icons are black + alpha; macOS recolors them for light/dark menu bars.
func menuIcon() []byte {
	const W, H = 44, 36
	img := image.NewNRGBA(image.Rect(0, 0, W, H))
	ink := color.NRGBA{0, 0, 0, 255}
	set := func(x, y int) {
		if x >= 0 && y >= 0 && x < W && y < H {
			img.Set(x, y, ink)
		}
	}
	// frame, 3px stroke
	for x := 2; x < W-2; x++ {
		for t := 0; t < 3; t++ {
			set(x, 2+t)
			set(x, H-3-t)
		}
	}
	for y := 2; y < H-2; y++ {
		for t := 0; t < 3; t++ {
			set(2+t, y)
			set(W-3-t, y)
		}
	}
	// sun
	cx, cy, r := 31.0, 11.0, 3.5
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if math.Hypot(float64(x)-cx, float64(y)-cy) <= r {
				set(x, y)
			}
		}
	}
	// two mountains (filled triangles) sitting on the bottom edge
	mountain := func(peakX, peakY, halfW int) {
		base := H - 5
		for y := peakY; y <= base; y++ {
			w := (y - peakY) * halfW / (base - peakY)
			for x := peakX - w; x <= peakX+w; x++ {
				if x > 4 && x < W-5 {
					set(x, y)
				}
			}
		}
	}
	mountain(15, 12, 13)
	mountain(29, 18, 11)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
