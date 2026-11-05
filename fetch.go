package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math/rand/v2"
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
	keepInCache  = 8
	maxDownloads = 10 // per change attempt
)

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Each source's search results are kept and used up one by one, so a source's
// API is only queried when its stock runs out (be polite; stay under rate limits).
// A source that errors is skipped for a while.
var (
	poolMu  sync.Mutex
	pool    = map[string][]Image{}
	backoff = map[string]time.Time{}
)

const (
	errorBackoff     = 15 * time.Minute
	rateLimitBackoff = 2 * time.Hour
)

// resetSource forgets a source's cached results and back-off (e.g. after new keys).
func resetSource(name string) {
	poolMu.Lock()
	defer poolMu.Unlock()
	delete(pool, name)
	delete(backoff, name)
}

// nextCandidate pops an unseen candidate for src, refilling from its API if needed.
func nextCandidate(ctx context.Context, src Source, seen *Seen) (Image, bool, error) {
	name := src.Name()
	poolMu.Lock()
	defer poolMu.Unlock()
	for refilled := false; ; {
		for len(pool[name]) > 0 {
			c := pool[name][0]
			pool[name] = pool[name][1:]
			if !seen.Has(c.ID) {
				return c, true, nil
			}
		}
		if refilled || time.Now().Before(backoff[name]) {
			return Image{}, false, nil
		}
		cands, err := src.Candidates(ctx)
		refilled = true
		if err != nil {
			wait := errorBackoff
			if strings.Contains(err.Error(), "429") {
				wait = rateLimitBackoff
			}
			backoff[name] = time.Now().Add(wait)
			return Image{}, false, fmt.Errorf("%s (pausing it %v): %w", name, wait, err)
		}
		rand.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
		pool[name] = cands
	}
}

// fetchWallpaper finds and downloads one new image from the enabled sources.
func fetchWallpaper(ctx context.Context, sources []Source, seen *Seen) (Image, string, error) {
	if len(sources) == 0 {
		return Image{}, "", errors.New("no sources enabled")
	}
	order := rand.Perm(len(sources))
	downloads := 0
	var lastErr error
	for round := 0; round < 2*len(sources); round++ {
		src := sources[order[round%len(order)]]
		for tries := 0; tries < 4; tries++ {
			c, ok, err := nextCandidate(ctx, src, seen)
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
				downloads++
				path, err := download(ctx, u, c.ID)
				if err != nil {
					lastErr = err
					log.Printf("skip %s: %v", c.ID, err)
					if errors.Is(err, errUnsuitable) {
						break // other sizes of the same image won't help
					}
					continue
				}
				seen.Add(c.ID)
				return c, path, nil
			}
			seen.Add(c.ID) // don't retry broken/unsuitable ones
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no new candidates")
	}
	return Image{}, "", lastErr
}

var errUnsuitable = errors.New("unsuitable image")

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
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownload+1))
	f.Close()
	if err == nil && n > maxDownload {
		err = errors.New("too large")
	}
	if err == nil {
		err = checkImage(tmp)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, os.Rename(tmp, path)
}

func checkImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("%w: not a readable image: %v", errUnsuitable, err)
	}
	ar := float64(cfg.Width) / float64(cfg.Height)
	if cfg.Width < minWidth || ar < minAspect || ar > maxAspect {
		return fmt.Errorf("%w: %dx%d", errUnsuitable, cfg.Width, cfg.Height)
	}
	return nil
}
