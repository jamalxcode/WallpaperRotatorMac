package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	minWidth     = 1920
	minAspect    = 1.25 // landscape only
	maxAspect    = 2.6  // skip extreme panoramas
	maxDownload  = 60 << 20
	probeBytes   = 512 << 10 // enough to read the size from almost any JPEG/PNG header
	maxDownloads = 10        // full downloads per change attempt

	errorBackoff     = 15 * time.Minute
	rateLimitBackoff = 2 * time.Hour
)

var (
	unsafeChars   = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	errUnsuitable = errors.New("unsuitable image")
	errNoSources  = errors.New("no sources enabled")
)

// Fetcher picks and downloads new wallpapers. Each source's search results are
// cached and used up one at a time, so APIs are queried rarely; a source that
// errors is paused for a while.
type Fetcher struct {
	mu      sync.Mutex
	pool    map[string][]Image
	backoff map[string]time.Time

	seen   *IDSet // recently shown, don't repeat
	banned *IDSet // "Never show this again", forever
	// check runs on the downloaded file (on-device image check); nil = skip.
	check func(path string, img Image) error
}

func NewFetcher(seen, banned *IDSet) *Fetcher {
	return &Fetcher{pool: map[string][]Image{}, backoff: map[string]time.Time{}, seen: seen, banned: banned}
}

// Reset forgets a source's cached results and back-off (e.g. after new keys).
func (f *Fetcher) Reset(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.pool, name)
	delete(f.backoff, name)
}

func (f *Fetcher) usable(id string) bool { return !f.seen.Has(id) && !f.banned.Has(id) }

// next pops an unused candidate for src, refilling from its API if needed.
func (f *Fetcher) next(ctx context.Context, src Source) (Image, bool, error) {
	name := src.Name()
	f.mu.Lock()
	defer f.mu.Unlock()
	for refilled := false; ; {
		for len(f.pool[name]) > 0 {
			c := f.pool[name][0]
			f.pool[name] = f.pool[name][1:]
			if f.usable(c.ID) {
				return c, true, nil
			}
		}
		if refilled || time.Now().Before(f.backoff[name]) {
			return Image{}, false, nil
		}
		cands, err := src.Candidates(ctx)
		refilled = true
		if err != nil {
			wait := errorBackoff
			if strings.Contains(err.Error(), "429") {
				wait = rateLimitBackoff
			}
			f.backoff[name] = time.Now().Add(wait)
			return Image{}, false, fmt.Errorf("%s (pausing it %v): %w", name, wait, err)
		}
		rand.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
		f.pool[name] = cands
	}
}

// weightedOrder returns sources with weight > 0, ordered by a weighted random
// draw without replacement (heavier sources tend to come first).
func weightedOrder(sources []Source, weights map[string]int, r *rand.Rand) []Source {
	type ws struct {
		s Source
		w int
	}
	var left []ws
	total := 0
	for _, s := range sources {
		if w := weights[s.Name()]; w > 0 {
			left = append(left, ws{s, w})
			total += w
		}
	}
	var out []Source
	for len(left) > 0 {
		n := r.IntN(total)
		for i, x := range left {
			if n < x.w {
				out = append(out, x.s)
				total -= x.w
				left = append(left[:i], left[i+1:]...)
				break
			}
			n -= x.w
		}
	}
	return out
}

// Fetch finds and downloads one new wallpaper.
func (f *Fetcher) Fetch(ctx context.Context, sources []Source, weights map[string]int) (Image, string, error) {
	order := weightedOrder(sources, weights, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	if len(order) == 0 {
		return Image{}, "", errNoSources
	}
	downloads := 0
	var lastErr error
	for round := 0; round < 2*len(order); round++ {
		src := order[round%len(order)]
		for tries := 0; tries < 4; tries++ {
			c, ok, err := f.next(ctx, src)
			if err != nil {
				lastErr = err
				log.Print(err)
			}
			if !ok {
				break
			}
			if downloads >= maxDownloads {
				return Image{}, "", fmt.Errorf("no suitable image after %d downloads (last: %v)", downloads, lastErr)
			}
			urls, err := c.urls(ctx)
			if err != nil {
				lastErr = err
				continue
			}
			for _, u := range urls {
				if c.Probe {
					if err := probe(ctx, u); errors.Is(err, errUnsuitable) {
						lastErr = err
						log.Printf("skip %s: %v (probed)", c.ID, err)
						break
					}
				}
				downloads++
				path, err := download(ctx, u, c.ID)
				if err == nil && f.check != nil {
					if err = f.check(path, c); err != nil {
						os.Remove(path)
					}
				}
				if err != nil {
					lastErr = err
					log.Printf("skip %s: %v", c.ID, err)
					if errors.Is(err, errUnsuitable) {
						break // other sizes of the same image won't help
					}
					continue
				}
				f.seen.Add(c.ID)
				return c, path, nil
			}
			f.seen.Add(c.ID) // don't retry broken/unsuitable ones
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no new candidates")
	}
	return Image{}, "", lastErr
}

// isNetworkError reports whether err means "no internet" (as opposed to a bad image).
func isNetworkError(err error) bool {
	var ne net.Error
	var de *net.DNSError
	var oe *net.OpError
	return errors.As(err, &de) || errors.As(err, &oe) || (errors.As(err, &ne) && ne.Timeout())
}

// probe reads just the start of an image to check its size before a full
// download. A nil result means "looks fine or couldn't tell"; download anyway.
func probe(ctx context.Context, u string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", probeBytes-1))
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() // also stops servers that ignore Range
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("%s: %s", u, resp.Status)
	}
	head, _ := io.ReadAll(io.LimitReader(resp.Body, probeBytes))
	cfg, _, err := image.DecodeConfig(bytes.NewReader(head))
	if err != nil {
		return nil // header didn't fit; let the full download decide
	}
	return shapeErr(cfg.Width, cfg.Height)
}

func shapeErr(w, h int) error {
	if !goodShape(w, h) {
		return fmt.Errorf("%w: %dx%d", errUnsuitable, w, h)
	}
	return nil
}

func goodShape(w, h int) bool {
	if w < minWidth || h <= 0 {
		return false
	}
	ar := float64(w) / float64(h)
	return ar >= minAspect && ar <= maxAspect
}

func download(ctx context.Context, u, id string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s: %s", u, resp.Status)
	}
	if resp.ContentLength > maxDownload {
		return "", fmt.Errorf("too large (%d bytes)", resp.ContentLength)
	}
	ext := ".jpg"
	if strings.Contains(resp.Header.Get("Content-Type"), "png") || strings.HasSuffix(strings.ToLower(u), ".png") {
		ext = ".png"
	}
	base := strings.TrimSuffix(id, filepath.Ext(id))
	name := fmt.Sprintf("%d-%s", time.Now().UnixNano(), unsafeChars.ReplaceAllString(base, "_"))
	if len(name) > 120 {
		name = name[:120]
	}
	path := filepath.Join(cacheDir, name+ext)
	tmp := path + ".part"
	fh, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(fh, io.LimitReader(resp.Body, maxDownload+1))
	fh.Close()
	if err == nil && n > maxDownload {
		err = errors.New("too large")
	}
	if err == nil {
		err = checkImageFile(tmp)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, os.Rename(tmp, path)
}

func checkImageFile(path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	cfg, _, err := image.DecodeConfig(fh)
	if err != nil {
		return fmt.Errorf("%w: not a readable image: %v", errUnsuitable, err)
	}
	return shapeErr(cfg.Width, cfg.Height)
}

// VisionResult is what the on-device check (Apple Vision) found in an image.
type VisionResult struct {
	HumanArea float64 // fraction of the image covered by detected people (0..1)
	Swimsuit  float64 // classifier confidence
	Skull     float64
}

// visionVerdict decides whether an image is OK for a family-friendly scenery
// wallpaper. People-dominated pictures are skipped (that's where nudity and
// portraits live); space photos may show astronauts.
func visionVerdict(kind string, r VisionResult) error {
	switch {
	case r.Swimsuit >= 0.3:
		return fmt.Errorf("%w: swimwear (%.2f)", errUnsuitable, r.Swimsuit)
	case r.Skull >= 0.5:
		return fmt.Errorf("%w: skull (%.2f)", errUnsuitable, r.Skull)
	case kind != KindSpace && r.HumanArea >= 0.12:
		return fmt.Errorf("%w: people fill %.0f%% of the picture", errUnsuitable, r.HumanArea*100)
	}
	return nil
}

// offlinePick chooses a picture already on disk (history and saved favourites),
// avoiding the current one. Used when there's no internet.
func offlinePick(history []Shown, savedDir, current string, r *rand.Rand) (Shown, bool) {
	var opts []Shown
	seen := map[string]bool{current: true}
	for _, h := range history {
		if !seen[h.Path] {
			if _, err := os.Stat(h.Path); err == nil {
				opts = append(opts, h)
				seen[h.Path] = true
			}
		}
	}
	if entries, err := os.ReadDir(savedDir); err == nil {
		for _, e := range entries {
			p := filepath.Join(savedDir, e.Name())
			ext := strings.ToLower(filepath.Ext(p))
			if e.IsDir() || seen[p] || (ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".heic") {
				continue
			}
			opts = append(opts, Shown{ID: "saved:" + e.Name(), Title: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), Source: "Saved pictures", Path: p})
			seen[p] = true
		}
	}
	if len(opts) == 0 {
		return Shown{}, false
	}
	return opts[r.IntN(len(opts))], true
}
