package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DeviantArt browses art by tag using the DeviantArt API with OAuth 2.0
// client credentials (no user sign-in). Unlike the other sources this art is
// NOT public domain: it belongs to the artists, so it's off by default and
// always credited. Keys come from the user's own app at
// https://www.deviantart.com/developers and are stored in config.json only.
type DeviantArt struct {
	creds func() (id, secret string, skipAI bool)
}

func (DeviantArt) Name() string { return "DeviantArt" }

var deviantTags = []string{
	"wallpaper", "landscape", "fantasylandscape", "spaceart", "conceptart", "scifi",
	"pixelart", "digitalpainting", "cityscape", "mattepainting", "environment",
	"underwater", "sunset", "mountains", "nightsky", "cyberpunk", "steampunk",
	"isometric", "lowpoly", "scenery", "naturephotography", "landscapephotography",
}

var aiWords = regexp.MustCompile(`(?i)\b(ai|ai[- ]?art|ai[- ]?generated|stable ?diffusion|midjourney|dall-?e|novelai|sdxl)\b`)

var errNoDeviantKeys = errors.New("add your DeviantArt client ID and secret in Settings")

type daToken struct {
	mu      sync.Mutex
	key     string // id+secret the token was issued for
	token   string
	expires time.Time
}

var daAuth daToken

// accessToken returns a cached client-credentials token, fetching a new one
// shortly before the old one expires (they last one hour).
func (t *daToken) accessToken(ctx context.Context, id, secret string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && t.key == id+"\x00"+secret && time.Until(t.expires) > 2*time.Minute {
		return t.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {id}, "client_secret": {secret}}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://www.deviantart.com/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&r)
	if resp.StatusCode != 200 || r.AccessToken == "" {
		return "", fmt.Errorf("DeviantArt sign-in failed (%s): %s %s", resp.Status, r.Error, r.ErrorDesc)
	}
	t.key, t.token = id+"\x00"+secret, r.AccessToken
	t.expires = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	return t.token, nil
}

func (d DeviantArt) Candidates(ctx context.Context) ([]Image, error) {
	id, secret, skipAI := d.creds()
	if id == "" || secret == "" {
		return nil, errNoDeviantKeys
	}
	token, err := daAuth.accessToken(ctx, id, secret)
	if err != nil {
		return nil, err
	}
	tag := deviantTags[rand.IntN(len(deviantTags))]
	var out []Image
	for _, offset := range []int{rand.IntN(4) * 50, 0} {
		q := url.Values{
			"tag":            {tag},
			"limit":          {"50"},
			"offset":         {strconv.Itoa(offset)},
			"mature_content": {"false"},
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://www.deviantart.com/api/v1/oauth2/browse/tags?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", userAgent)
		// Go's client requests gzip and decompresses it automatically (DeviantArt requires compression).
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		var r struct {
			Results []struct {
				DeviationID string `json:"deviationid"`
				URL         string `json:"url"`
				Title       string `json:"title"`
				IsMature    bool   `json:"is_mature"`
				IsDeleted   bool   `json:"is_deleted"`
				Author      struct {
					Username string `json:"username"`
				} `json:"author"`
				Content *struct {
					Src    string `json:"src"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"content"`
			} `json:"results"`
			Error     string `json:"error"`
			ErrorDesc string `json:"error_description"`
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if resp.StatusCode == 401 {
			daAuth.mu.Lock()
			daAuth.token = "" // force a fresh token next time
			daAuth.mu.Unlock()
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("DeviantArt browse %q: %s %s %s", tag, resp.Status, r.Error, r.ErrorDesc)
		}
		if err != nil {
			return nil, err
		}
		for _, dv := range r.Results {
			if dv.IsMature || dv.IsDeleted || dv.Content == nil || dv.Content.Src == "" {
				continue
			}
			if !goodShape(dv.Content.Width, dv.Content.Height) {
				continue
			}
			if !artOK(dv.Title) || (skipAI && aiWords.MatchString(dv.Title)) {
				continue
			}
			src := dv.Content.Src
			out = append(out, Image{
				ID:      "da:" + dv.DeviationID,
				Title:   dv.Title,
				Credit:  "by " + dv.Author.Username,
				PageURL: dv.URL,
				Source:  "DeviantArt",
				Kind:    KindArt,
				urls:    func(context.Context) ([]string, error) { return []string{src}, nil },
			})
		}
		if len(out) > 0 {
			break
		}
	}
	return out, nil
}
