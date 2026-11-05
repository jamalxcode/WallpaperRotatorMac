package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math/bits"
	"path/filepath"
	"testing"
)

// scene draws a simple "landscape": sky gradient, sun, and a hill whose
// position depends on variant, so different variants are different pictures.
func scene(w, h, variant int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{uint8(40 + 150*y/h), uint8(80 + 100*y/h), 200, 255}
			sx, sy := w*(1+variant%3)/4, h/4
			if (x-sx)*(x-sx)+(y-sy)*(y-sy) < (h/10)*(h/10) {
				c = color.RGBA{255, 220, 80, 255}
			}
			if y > h*2/3-(x*(variant+1)%h)/6 {
				c = color.RGBA{30, 90 + uint8(variant*30), 40, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func recompress(t *testing.T, img image.Image, quality int) image.Image {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDHashSameAndDifferent(t *testing.T) {
	big := dHash(scene(1920, 1080, 0))
	small := dHash(recompress(t, scene(960, 540, 0), 60)) // resized + recompressed copy
	other := dHash(scene(1920, 1080, 1))

	if d := bits.OnesCount64(big ^ small); d > sameImageDistance {
		t.Errorf("same picture at another size differs by %d bits (max %d)", d, sameImageDistance)
	}
	if d := bits.OnesCount64(big ^ other); d <= sameImageDistance {
		t.Errorf("different pictures only differ by %d bits", d)
	}
}

func TestHashSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fp.json")
	s := loadHashSet(path, 2)
	s.Add(0b1111)
	if !s.Near(0b1110) {
		t.Error("1-bit difference should count as the same picture")
	}
	if s.Near(^uint64(0)) {
		t.Error("completely different hash should not match")
	}
	s.Add(0xFFFF_FFFF_0000_0000)
	s.Add(0x00FF_00FF_00FF_00FF) // pushes out 0b1111
	if s.Near(0b1111) {
		t.Error("limit should drop the oldest fingerprint")
	}
	if !loadHashSet(path, 2).Near(0x00FF_00FF_00FF_00FF) {
		t.Error("fingerprints should persist")
	}
}
