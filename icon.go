package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// menuIcon draws the menu-bar icon: a rounded tile with a sunset over purple
// mountains and a lake, in the app icon's colours. macOS shows it at 16pt, so
// it's drawn at 64px (sharp on Retina) with 4×4 supersampling for smooth edges.
func menuIcon() []byte {
	const S, ss = 64, 4
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	for py := 0; py < S; py++ {
		for px := 0; px < S; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / S
					y := (float64(py) + (float64(sy)+0.5)/ss) / S
					if c, ok := iconPixel(x, y); ok {
						r, g, b, a = r+c[0], g+c[1], b+c[2], a+1
					}
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				uint8(r / a * 255), uint8(g / a * 255), uint8(b / a * 255), uint8(a / (ss * ss) * 255),
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

type iconRGB [3]float64

func iconHex(v uint32) iconRGB {
	return iconRGB{float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255}
}

func iconMix(a, b iconRGB, t float64) iconRGB {
	t = min(max(t, 0), 1)
	return iconRGB{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

// iconGradient picks a colour at t along stops at positions pos.
func iconGradient(t float64, pos []float64, cols []iconRGB) iconRGB {
	if t <= pos[0] {
		return cols[0]
	}
	for i := 1; i < len(pos); i++ {
		if t <= pos[i] {
			return iconMix(cols[i-1], cols[i], (t-pos[i-1])/(pos[i]-pos[i-1]))
		}
	}
	return cols[len(cols)-1]
}

// underRidge reports whether (x, y) is below the ridge running from (x0, y0)
// up to the peak (px, py) and down to (x1, y1).
func underRidge(x, y, x0, y0, px, py, x1, y1 float64) bool {
	switch {
	case x < x0 || x > x1:
		return false
	case x <= px:
		return y >= y0+(py-y0)*(x-x0)/(px-x0)
	default:
		return y >= py+(y1-py)*(x-px)/(x1-px)
	}
}

// iconPixel returns the colour at (x, y) in 0..1 coordinates, or false outside
// the tile.
func iconPixel(x, y float64) (iconRGB, bool) {
	// rounded square tile
	const m, rad, rim = 0.02, 0.24, 0.035
	if x < m || x > 1-m || y < m || y > 1-m {
		return iconRGB{}, false
	}
	dx := math.Max(math.Max(m+rad-x, x-(1-m-rad)), 0)
	dy := math.Max(math.Max(m+rad-y, y-(1-m-rad)), 0)
	inside := rad - math.Hypot(dx, dy) // distance in from the rounded corner
	if inside < 0 {
		return iconRGB{}, false
	}

	const horizon = 0.72
	c := iconGradient(y, []float64{0.06, 0.32, 0.50, 0.64},
		[]iconRGB{iconHex(0x4F6BFF), iconHex(0xB455F0), iconHex(0xFF6FA0), iconHex(0xFFB15C)})
	if math.Hypot(x-0.62, y-0.50) <= 0.15 {
		c = iconMix(iconHex(0xFFF27A), iconHex(0xFFC93D), (y-0.35)/0.3) // sun
	}
	switch {
	case y >= horizon:
		// lake, with a soft reflection of the sun fading downwards
		c = iconGradient(y, []float64{horizon, 1}, []iconRGB{iconHex(0x6A4BD8), iconHex(0x2B2A8C)})
		if w := 0.16 * (1 - (y-horizon)/0.26); math.Abs(x-0.62) < w {
			c = iconMix(c, iconHex(0xFFB15C), 0.7*(1-math.Abs(x-0.62)/w))
		}
	case underRidge(x, y, 0.48, horizon, 0.80, 0.52, 1.1, horizon):
		c = iconMix(iconHex(0x5A3FC0), iconHex(0x3E2F9E), (y-0.52)/0.2) // front peak
	case underRidge(x, y, -0.1, horizon, 0.28, 0.40, 0.66, horizon):
		c = iconMix(iconHex(0x9A5BE6), iconHex(0x6E48D2), (y-0.40)/0.32) // back peak
	}
	// thin light rim so the tile stands out on dark and light menu bars
	if inside < rim || x < m+rim || x > 1-m-rim || y < m+rim || y > 1-m-rim {
		c = iconMix(c, iconRGB{1, 1, 1}, 0.35)
	}
	return c, true
}
