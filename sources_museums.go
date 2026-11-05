package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strconv"
	"strings"
)

// Subjects that make good wallpapers. Used to steer museum searches away from
// portraits and figure studies (which is also where nudity tends to be).
var artSubjects = []string{
	"landscape", "seascape", "mountains", "waterfall", "river", "sunset", "harbor",
	"lake", "forest", "canyon", "coast", "moonlight", "storm", "snow", "ships",
	"valley", "garden", "city view", "volcano", "desert", "clouds", "autumn",
}

// artOK applies both blocklists to everything we know about an artwork.
func artOK(texts ...string) bool {
	return !isBlocked(texts...) && !isBlockedArt(texts...)
}

func goodShape(w, h int) bool {
	if w < minWidth || h <= 0 {
		return false
	}
	ar := float64(w) / float64(h)
	return ar >= minAspect && ar <= maxAspect
}

// ---------------------------------------------------------------- Cleveland Museum of Art

// ClevelandArt uses the Cleveland Museum of Art Open Access API (CC0, no key).
type ClevelandArt struct{}

func (ClevelandArt) Name() string { return "Cleveland" }

func (ClevelandArt) Candidates(ctx context.Context) ([]Image, error) {
	subject := artSubjects[rand.IntN(len(artSubjects))]
	type artwork struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Creators    []struct {
			Description string `json:"description"`
		} `json:"creators"`
		Images struct {
			Print *struct {
				URL    string `json:"url"`
				Width  string `json:"width"`
				Height string `json:"height"`
			} `json:"print"`
		} `json:"images"`
	}
	// The API accepts one type per query; mostly paintings, sometimes prints/photos.
	types := []string{"Painting", "Painting", "Painting", "Print", "Photograph", "Drawing"}
	artType := types[rand.IntN(len(types))]
	var out []Image
	for _, skip := range []int{rand.IntN(200), 0} {
		q := url.Values{
			"q": {subject}, "cc0": {"1"}, "has_image": {"1"},
			"type":  {artType},
			"limit": {"100"}, "skip": {strconv.Itoa(skip)},
		}
		var r struct {
			Data []artwork `json:"data"`
		}
		if err := getJSON(ctx, "https://openaccess-api.clevelandart.org/api/artworks/?"+q.Encode(), &r); err != nil {
			return nil, err
		}
		for _, a := range r.Data {
			p := a.Images.Print
			if p == nil || p.URL == "" {
				continue
			}
			w, _ := strconv.Atoi(p.Width)
			h, _ := strconv.Atoi(p.Height)
			if !goodShape(w, h) {
				continue
			}
			artist := ""
			if len(a.Creators) > 0 {
				artist = a.Creators[0].Description
				if i := strings.Index(artist, " ("); i > 0 {
					artist = artist[:i]
				}
			}
			if !artOK(a.Title, a.Description) {
				continue
			}
			u := p.URL
			out = append(out, Image{
				ID:      fmt.Sprintf("cma:%d", a.ID),
				Title:   a.Title,
				Credit:  artist,
				PageURL: a.URL,
				Source:  "Cleveland Museum of Art",
				urls:    func(context.Context) ([]string, error) { return []string{u}, nil },
			})
		}
		if len(out) > 0 {
			break
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- Smithsonian

// Smithsonian uses the Smithsonian Open Access API, limited to the
// American Art Museum (Hudson River School, Western landscapes…), CC0 only.
// DEMO_KEY allows ~50 requests/day; set "smithsonian_api_key" in config.json
// (free from api.data.gov) for more.
type Smithsonian struct{ Key string }

func (Smithsonian) Name() string { return "Smithsonian" }

func (s Smithsonian) Candidates(ctx context.Context) ([]Image, error) {
	key := s.Key
	if key == "" {
		key = "DEMO_KEY"
	}
	subject := artSubjects[rand.IntN(len(artSubjects))]
	var r struct {
		Response struct {
			Rows []struct {
				Title   string `json:"title"`
				Content struct {
					DNR struct {
						RecordLink  string `json:"record_link"`
						OnlineMedia struct {
							Media []struct {
								Usage struct {
									Access string `json:"access"`
								} `json:"usage"`
								Resources []struct {
									Label  string `json:"label"`
									URL    string `json:"url"`
									Width  int    `json:"width"`
									Height int    `json:"height"`
								} `json:"resources"`
							} `json:"media"`
						} `json:"online_media"`
					} `json:"descriptiveNonRepeating"`
					Freetext struct {
						Name []struct {
							Label   string `json:"label"`
							Content string `json:"content"`
						} `json:"name"`
						Notes []struct {
							Content string `json:"content"`
						} `json:"notes"`
					} `json:"freetext"`
					Indexed struct {
						Topic []string `json:"topic"`
					} `json:"indexedStructured"`
				} `json:"content"`
			} `json:"rows"`
		} `json:"response"`
	}
	q := url.Values{
		"q":       {subject + " AND unit_code:SAAM AND online_media_type:Images"},
		"rows":    {"100"},
		"start":   {strconv.Itoa(rand.IntN(3) * 100)},
		"api_key": {key},
	}
	if err := getJSON(ctx, "https://api.si.edu/openaccess/api/v1.0/search?"+q.Encode(), &r); err != nil {
		if strings.Contains(err.Error(), "429") {
			return nil, fmt.Errorf("daily DEMO_KEY limit reached: %w", err)
		}
		return nil, err
	}
	var out []Image
	for _, row := range r.Response.Rows {
		c := row.Content
		if len(c.DNR.OnlineMedia.Media) == 0 {
			continue
		}
		m := c.DNR.OnlineMedia.Media[0]
		if m.Usage.Access != "CC0" {
			continue
		}
		var jpg string
		var w, h int
		for _, res := range m.Resources {
			if res.Label == "High-resolution JPEG" {
				jpg, w, h = res.URL, res.Width, res.Height
			}
		}
		if jpg == "" || (w > 0 && !goodShape(w, h)) {
			continue
		}
		notes := make([]string, 0, len(c.Freetext.Notes))
		for _, n := range c.Freetext.Notes {
			notes = append(notes, n.Content)
		}
		if !artOK(row.Title, strings.Join(c.Indexed.Topic, " | "), strings.Join(notes, " ")) {
			continue
		}
		artist := ""
		for _, n := range c.Freetext.Name {
			if n.Label == "Artist" {
				artist = n.Content
				if i := strings.Index(artist, ", born"); i > 0 {
					artist = artist[:i]
				}
				break
			}
		}
		out = append(out, Image{
			ID:      "si:" + jpg,
			Title:   row.Title,
			Credit:  artist,
			PageURL: c.DNR.RecordLink,
			Source:  "Smithsonian American Art Museum",
			urls:    func(context.Context) ([]string, error) { return []string{jpg}, nil },
		})
	}
	return out, nil
}
