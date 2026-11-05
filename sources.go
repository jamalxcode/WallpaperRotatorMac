package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const userAgent = "WallpaperChanger/1.0 (personal macOS desktop wallpaper app)"

var httpClient = &http.Client{Timeout: 90 * time.Second}

// Image is a candidate wallpaper. URLs are tried in order until one passes checks.
type Image struct {
	ID      string
	Title   string
	Credit  string
	PageURL string
	Source  string
	// urls returns download URLs in order of preference (may need a network call).
	urls func(ctx context.Context) ([]string, error)
}

type Source interface {
	Name() string
	Candidates(ctx context.Context) ([]Image, error)
}

func getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

func stripHTML(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	return strings.Join(strings.Fields(s), " ")
}

// ---------------------------------------------------------------- NASA

type NASA struct{}

func (NASA) Name() string { return "NASA" }

var nasaTopics = []string{
	"nebula", "galaxy", "aurora", "hubble nebula", "webb telescope image", "chandra",
	"earth from space", "earth as art", "landsat", "astronaut photograph earth",
	"saturn rings", "jupiter juno", "cassini", "mars surface", "moon surface", "earthrise",
	"rocket launch", "falcon heavy launch", "artemis launch", "spacewalk",
	"supernova", "star cluster", "hurricane from space", "city lights at night",
	"volcano from space", "sunrise space station", "solar flare", "solar eclipse", "comet",
	"blue marble", "milky way", "lightning from space", "glacier from space", "sahara from space",
}

// Things that make dull wallpapers (people at events, lab hardware, documents).
var nasaBoring = regexp.MustCompile(`(?i)\b(meetings?|town hall|conference|ceremony|briefing|press|interview|speaks?|speech|speaker|visits?|tour|signing|award\w*|portrait|headshot|office|mock-?up|training|class(room)?|students?|employees?|workers?|technicians?|engineers?|inspect\w*|team|group photo|logo|diagram|chart|graph|experiments?|equipment|hardware|assembly|clean ?room|installation|test(ing|s)?|lab(oratory)?|exhibit\w*|museum|reception|panel|workshop|celebrat\w*|officials?|administrator)\b`)

func (NASA) Candidates(ctx context.Context) ([]Image, error) {
	topic := nasaTopics[rand.IntN(len(nasaTopics))]
	page := 1 + rand.IntN(4)
	var out []Image
	for attempt := 0; attempt < 2; attempt++ {
		q := url.Values{"q": {topic}, "media_type": {"image"}, "page": {fmt.Sprint(page)}, "page_size": {"100"}}
		var r struct {
			Collection struct {
				Items []struct {
					Data []struct {
						NasaID      string   `json:"nasa_id"`
						Title       string   `json:"title"`
						Description string   `json:"description"`
						Keywords    []string `json:"keywords"`
						Center      string   `json:"center"`
					} `json:"data"`
				} `json:"items"`
			} `json:"collection"`
		}
		if err := getJSON(ctx, "https://images-api.nasa.gov/search?"+q.Encode(), &r); err != nil {
			return nil, err
		}
		for _, it := range r.Collection.Items {
			if len(it.Data) == 0 {
				continue
			}
			d := it.Data[0]
			kw := strings.Join(d.Keywords, " ")
			if isBlocked(d.Title, d.Description, kw) || nasaBoring.MatchString(d.Title+" "+kw) {
				continue
			}
			id := d.NasaID
			title := d.Title
			if !strings.Contains(strings.TrimSpace(title), " ") { // titles like "KSC-01pp1062"
				desc := stripHTML(d.Description)
				if i := strings.IndexAny(desc, ".\n"); i > 0 {
					desc = desc[:i]
				}
				if desc != "" {
					title = desc
				}
			}
			credit := "NASA"
			if d.Center != "" {
				credit = "NASA " + d.Center
			}
			out = append(out, Image{
				ID:      "nasa:" + id,
				Title:   title,
				Credit:  credit,
				PageURL: "https://images.nasa.gov/details/" + url.PathEscape(id),
				Source:  "NASA",
				urls:    func(ctx context.Context) ([]string, error) { return nasaAssetURLs(ctx, id) },
			})
		}
		if len(out) > 0 {
			break
		}
		page = 1 // random page was past the end; retry first page
	}
	return out, nil
}

func nasaAssetURLs(ctx context.Context, id string) ([]string, error) {
	var r struct {
		Collection struct {
			Items []struct {
				Href string `json:"href"`
			} `json:"items"`
		} `json:"collection"`
	}
	if err := getJSON(ctx, "https://images-api.nasa.gov/asset/"+url.PathEscape(id), &r); err != nil {
		return nil, err
	}
	var orig, large []string
	for _, it := range r.Collection.Items {
		h := strings.Replace(it.Href, "http://", "https://", 1)
		l := strings.ToLower(h)
		if !(strings.HasSuffix(l, ".jpg") || strings.HasSuffix(l, ".jpeg") || strings.HasSuffix(l, ".png")) {
			continue
		}
		switch {
		case strings.Contains(l, "~orig."):
			orig = append(orig, h)
		case strings.Contains(l, "~large."):
			large = append(large, h)
		}
	}
	return append(orig, large...), nil
}

// ---------------------------------------------------------------- Wikimedia Commons

type Wikimedia struct{}

func (Wikimedia) Name() string { return "Wikimedia" }

// Picks a random slice of Commons "Featured pictures" and keeps only
// public-domain / CC0 photos that look like decent landscape wallpapers.
func (Wikimedia) Candidates(ctx context.Context) ([]Image, error) {
	// Only ~1 in 10 featured pictures is public domain AND wallpaper-shaped,
	// so look at a few random slices before giving up.
	var all []Image
	for i := 0; i < 3 && len(all) < 3; i++ {
		imgs, err := wikimediaBatch(ctx)
		if err != nil {
			return all, err
		}
		all = append(all, imgs...)
	}
	return all, nil
}

func wikimediaBatch(ctx context.Context) ([]Image, error) {
	hex := fmt.Sprintf("%06x", rand.IntN(0xFFFFFF))
	q := url.Values{
		"generator":          {"categorymembers"},
		"gcmtitle":           {"Category:Featured_pictures_on_Wikimedia_Commons"},
		"gcmtype":            {"file"},
		"gcmlimit":           {"30"},
		"gcmsort":            {"sortkey"},
		"gcmstarthexsortkey": {hex},
	}
	return commonsQuery(ctx, q, "Wikimedia Commons", func(texts ...string) bool {
		return isBlocked(texts...) || isBlockedArt(texts...)
	})
}

// commonsQuery runs a Commons API generator query (q supplies the generator
// params) and turns the results into vetted, public-domain wallpaper candidates.
// reject is applied to the file name, title and categories.
func commonsQuery(ctx context.Context, q url.Values, source string, reject func(...string) bool) ([]Image, error) {
	for k, v := range map[string]string{
		"action":              "query",
		"format":              "json",
		"prop":                "imageinfo|categories",
		"cllimit":             "max",
		"clshow":              "!hidden", // hidden maintenance categories push past the 500 limit
		"iiprop":              "url|size|mime|extmetadata",
		"iiextmetadatafilter": "LicenseShortName|ObjectName|Artist|ImageDescription",
		"iiurlwidth":          "3840",
	} {
		q.Set(k, v)
	}
	var r struct {
		Query struct {
			Pages map[string]struct {
				Title      string `json:"title"`
				Categories *[]struct {
					Title string `json:"title"`
				} `json:"categories"`
				ImageInfo []struct {
					URL            string `json:"url"`
					ThumbURL       string `json:"thumburl"`
					DescriptionURL string `json:"descriptionurl"`
					Width          int    `json:"width"`
					Height         int    `json:"height"`
					Mime           string `json:"mime"`
					Ext            map[string]struct {
						Value string `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := getJSON(ctx, "https://commons.wikimedia.org/w/api.php?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	var out []Image
	for _, p := range r.Query.Pages {
		if len(p.ImageInfo) == 0 || p.Categories == nil {
			continue // no category data => can't vet it, skip
		}
		ii := p.ImageInfo[0]
		lic := strings.ToLower(ii.Ext["LicenseShortName"].Value)
		if !(strings.Contains(lic, "public domain") || strings.HasPrefix(lic, "pd") || strings.Contains(lic, "cc0")) {
			continue
		}
		if ii.Mime != "image/jpeg" && ii.Mime != "image/png" {
			continue
		}
		if !goodShape(ii.Width, ii.Height) {
			continue
		}
		cats := make([]string, 0, len(*p.Categories))
		for _, c := range *p.Categories {
			cats = append(cats, c.Title)
		}
		title := stripHTML(ii.Ext["ObjectName"].Value)
		if title == "" || strings.Contains(title, "QS:") || len(title) > 120 {
			title = strings.TrimPrefix(p.Title, "File:")
			if i := strings.LastIndex(title, "."); i > 0 {
				title = title[:i]
			}
		}
		desc := stripHTML(ii.Ext["ImageDescription"].Value)
		title = cleanTitle(title, desc)
		if reject(p.Title, title, desc, strings.Join(cats, " | ")) {
			continue
		}
		credit := stripHTML(ii.Ext["Artist"].Value)
		if len(credit) > 60 {
			credit = credit[:60] + "…"
		}
		urls := []string{}
		if ii.ThumbURL != "" {
			urls = append(urls, ii.ThumbURL)
		}
		urls = append(urls, ii.URL)
		out = append(out, Image{
			ID:      "commons:" + p.Title,
			Title:   title,
			Credit:  credit,
			PageURL: ii.DescriptionURL,
			Source:  source,
			urls:    func(context.Context) ([]string, error) { return urls, nil },
		})
	}
	return out, nil
}

var (
	flickrSuffix = regexp.MustCompile(`(?i)\s*(\(\d{8,}\)|-\s*flickr\s*-.*|\d{8,}\s*o)\s*$`)
	codeTitle    = regexp.MustCompile(`^[A-Za-z_-]{0,8}\d[\w-]*$`) // e.g. "Anim1019", "IMG_2041"
)

// cleanTitle strips Flickr/catalogue noise and falls back to the description's
// first sentence when the title is just a code.
func cleanTitle(title, desc string) string {
	t := strings.TrimSpace(flickrSuffix.ReplaceAllString(title, ""))
	if t == "" || codeTitle.MatchString(t) {
		d := desc
		if i := strings.IndexAny(d, ".;\n"); i > 0 {
			d = d[:i]
		}
		d = strings.TrimSpace(d)
		if len(d) > 8 && len(d) <= 140 {
			return d
		}
		if t == "" {
			return title
		}
	}
	return t
}
