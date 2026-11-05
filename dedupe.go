package main

import (
	"encoding/json"
	"fmt"
	"image"
	"math/bits"
	"os"
	"sync"
)

// The same picture is often published more than once (NASA re-posts Hubble
// images under several IDs; artists re-upload; Commons has crops). ID checks
// can't catch that, so each shown picture also gets a perceptual fingerprint.

// dHash is a 64-bit "difference hash": shrink to 9x8 grey cells and record
// whether each cell is brighter than its right neighbour. Resizing,
// recompression and small edits barely change it.
func dHash(img image.Image) uint64 {
	b := img.Bounds()
	const cols, rows, samples = 9, 8, 4 // each cell = average of samples×samples points
	var cell [rows][cols]float64
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			var sum float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := b.Min.X + (c*samples+sx)*b.Dx()/(cols*samples)
					y := b.Min.Y + (r*samples+sy)*b.Dy()/(rows*samples)
					rr, gg, bb, _ := img.At(x, y).RGBA()
					sum += 0.299*float64(rr) + 0.587*float64(gg) + 0.114*float64(bb)
				}
			}
			cell[r][c] = sum
		}
	}
	var h uint64
	for r := 0; r < rows; r++ {
		for c := 0; c < cols-1; c++ {
			h <<= 1
			if cell[r][c] > cell[r][c+1] {
				h |= 1
			}
		}
	}
	return h
}

func fileHash(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return 0, err
	}
	return dHash(img), nil
}

// sameImageDistance: fingerprints this close (out of 64 bits) are the same picture.
const sameImageDistance = 6

// HashSet remembers fingerprints of recently shown pictures.
type HashSet struct {
	mu     sync.Mutex
	path   string
	limit  int
	hashes []uint64
}

func loadHashSet(path string, limit int) *HashSet {
	s := &HashSet{path: path, limit: limit}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.hashes)
	}
	return s
}

// Near reports whether a fingerprint within sameImageDistance was seen.
func (s *HashSet) Near(h uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.hashes {
		if bits.OnesCount64(x^h) <= sameImageDistance {
			return true
		}
	}
	return false
}

func (s *HashSet) Add(h uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes = append(s.hashes, h)
	if len(s.hashes) > s.limit {
		s.hashes = append([]uint64(nil), s.hashes[len(s.hashes)-s.limit:]...)
	}
	if s.path != "" {
		b, _ := json.Marshal(s.hashes)
		_ = os.WriteFile(s.path, b, 0o644)
	}
}

// duplicateCheck returns errUnsuitable if the file looks like a recently shown picture.
func duplicateCheck(seen *HashSet, path string) (uint64, error) {
	h, err := fileHash(path)
	if err != nil {
		return 0, nil // can't fingerprint it: don't block on that
	}
	if seen.Near(h) {
		return h, fmt.Errorf("%w: same picture as one shown recently", errUnsuitable)
	}
	return h, nil
}
