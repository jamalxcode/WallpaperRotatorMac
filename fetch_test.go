package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type fakeSource string

func (f fakeSource) Name() string                                { return string(f) }
func (f fakeSource) Candidates(context.Context) ([]Image, error) { return nil, nil }

func TestGoodShape(t *testing.T) {
	cases := []struct {
		w, h int
		ok   bool
	}{
		{1920, 1080, true}, {3840, 2160, true}, {2560, 1080, true},
		{1919, 1080, false}, // too narrow
		{2000, 2000, false}, // square
		{3000, 4000, false}, // portrait
		{6000, 1500, false}, // extreme panorama
		{1920, 0, false},
	}
	for _, c := range cases {
		if got := goodShape(c.w, c.h); got != c.ok {
			t.Errorf("goodShape(%d,%d) = %v, want %v", c.w, c.h, got, c.ok)
		}
	}
}

func TestWeightedOrder(t *testing.T) {
	srcs := []Source{fakeSource("A"), fakeSource("B"), fakeSource("C")}
	r := rand.New(rand.NewPCG(1, 2))

	got := weightedOrder(srcs, map[string]int{"A": 1, "B": 0, "C": 5}, r)
	if len(got) != 2 {
		t.Fatalf("weight 0 should be excluded, got %d sources", len(got))
	}
	if len(weightedOrder(srcs, map[string]int{}, r)) != 0 {
		t.Error("no weights should give no sources")
	}

	// C (weight 5) should come first far more often than A (weight 1).
	first := map[string]int{}
	for i := 0; i < 6000; i++ {
		first[weightedOrder(srcs, map[string]int{"A": 1, "C": 5}, r)[0].Name()]++
	}
	if ratio := float64(first["C"]) / float64(first["A"]); ratio < 4 || ratio > 6.5 {
		t.Errorf("C/A first-pick ratio = %.2f, want about 5", ratio)
	}
}

func TestVisionVerdict(t *testing.T) {
	cases := []struct {
		kind string
		r    VisionResult
		ok   bool
	}{
		{KindNature, VisionResult{}, true},
		{KindArt, VisionResult{HumanArea: 0.95}, false},
		{KindSpace, VisionResult{HumanArea: 0.6}, true}, // astronauts are fine
		{KindNature, VisionResult{HumanArea: 0.08}, true},
		{KindSpace, VisionResult{Swimsuit: 0.58}, false},
		{KindPhoto, VisionResult{Skull: 0.7}, false},
	}
	for _, c := range cases {
		err := visionVerdict(c.kind, c.r)
		if (err == nil) != c.ok {
			t.Errorf("visionVerdict(%s, %+v) = %v, want ok=%v", c.kind, c.r, err, c.ok)
		}
		if err != nil && !errors.Is(err, errUnsuitable) {
			t.Errorf("rejections must wrap errUnsuitable, got %v", err)
		}
	}
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProbe(t *testing.T) {
	wide, square := jpegBytes(t, 2400, 1350), jpegBytes(t, 1200, 1200)
	var sawRange bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRange = r.Header.Get("Range") != ""
		data := wide
		if r.URL.Path == "/square.jpg" {
			data = square
		}
		http.ServeContent(w, r, "x.jpg", fileTime, bytes.NewReader(data))
	}))
	defer srv.Close()

	if err := probe(context.Background(), srv.URL+"/wide.jpg"); err != nil {
		t.Errorf("wide image: %v", err)
	}
	if !sawRange {
		t.Error("probe should send a Range header")
	}
	if err := probe(context.Background(), srv.URL+"/square.jpg"); !errors.Is(err, errUnsuitable) {
		t.Errorf("square image: got %v, want errUnsuitable", err)
	}
}

func TestCheckImageFile(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "good.jpg"), filepath.Join(dir, "bad.jpg")
	os.WriteFile(good, jpegBytes(t, 1920, 1080), 0o644)
	os.WriteFile(bad, []byte("not an image"), 0o644)
	if err := checkImageFile(good); err != nil {
		t.Errorf("good image: %v", err)
	}
	if err := checkImageFile(bad); !errors.Is(err, errUnsuitable) {
		t.Errorf("garbage file: got %v, want errUnsuitable", err)
	}
}

func TestOfflinePick(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.jpg"), filepath.Join(dir, "b.jpg")
	os.WriteFile(a, []byte("x"), 0o644)
	os.WriteFile(b, []byte("x"), 0o644)
	saved := filepath.Join(dir, "saved")
	os.Mkdir(saved, 0o755)
	os.WriteFile(filepath.Join(saved, "Fav.png"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(saved, "notes.txt"), []byte("x"), 0o644)
	hist := []Shown{{ID: "1", Path: a}, {ID: "2", Path: b}, {ID: "3", Path: filepath.Join(dir, "gone.jpg")}}

	r := rand.New(rand.NewPCG(3, 4))
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		s, ok := offlinePick(hist, saved, a, r)
		if !ok {
			t.Fatal("expected a pick")
		}
		seen[filepath.Base(s.Path)] = true
	}
	if seen["a.jpg"] || seen["gone.jpg"] || seen["notes.txt"] {
		t.Errorf("picked the current, a missing, or a non-image file: %v", seen)
	}
	if !seen["b.jpg"] || !seen["Fav.png"] {
		t.Errorf("should pick from history and saved pictures, got %v", seen)
	}
	if _, ok := offlinePick(nil, filepath.Join(dir, "none"), "", r); ok {
		t.Error("nothing on disk should give no pick")
	}
}

func TestIsNetworkError(t *testing.T) {
	if !isNetworkError(&net.DNSError{Err: "no such host", Name: "x"}) {
		t.Error("DNS error should count as network error")
	}
	if isNetworkError(errUnsuitable) {
		t.Error("unsuitable image is not a network error")
	}
}
