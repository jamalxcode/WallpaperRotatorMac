package main

import (
	"context"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strconv"
)

// Wildlife searches Wikimedia Commons for photos taken by US federal agencies.
// Their work is public domain by law and tagged with a PD-USGov-* template,
// so the license is certain. Searches are steered by a fun subject.
type Wildlife struct{}

func (Wildlife) Name() string { return "Wildlife" }

type agency struct {
	template, label string
	subjects        []string
}

var agencies = []agency{
	{"PD-USGov-FWS", "U.S. Fish & Wildlife Service", []string{
		"wolf", "bear", "bald eagle", "owl", "fox", "bison", "elk", "moose", "butterfly", "frog",
		"turtle", "heron", "puffin", "lynx", "hummingbird", "deer", "pelican", "otter", "crane",
		"refuge sunset", "flamingo", "salmon", "mountain goat", "bobcat", "swan",
	}},
	{"PD-USGov-NPS", "National Park Service", []string{
		"canyon", "geyser", "waterfall", "mountain", "sunset", "milky way", "arch", "glacier",
		"lake", "wildflowers", "snow", "cave", "coast", "volcano", "sand dunes", "redwood",
		"night sky", "aurora", "autumn", "hot spring",
	}},
	{"PD-USGov-NOAA", "NOAA", []string{
		"lightning", "aurora", "whale", "dolphin", "coral reef", "jellyfish", "octopus", "iceberg",
		"supercell", "tornado", "hurricane", "sunset", "seal", "shark", "sea turtle", "deep sea",
		"waves", "rainbow", "polar bear", "penguin", "manta", "kelp forest",
	}},
}

// Photos that are public domain but make poor wallpapers, or could be grim.
var wildlifeReject = regexp.MustCompile(`(?i)\b(maps?|diagrams?|charts?|graphs?|logos?|signs?|poster|brochure|documents?|letters?|specimens?|skulls?|skeletons?|carcass(es)?|necropsy|roadkill|dead|killed|trapped|tagging|banding|biologists?|staff|employees?|volunteers?|meetings?|ceremony|award\w*|visitors? center|offices?|headquarters|portrait|headshot|press|radar|satellite image|model|museum|exhibit\w*|aerial survey|helicopter|boat ramp|parking|restroom|construction|dredg\w*|oil spill|debris|marine debris|entangle\w*|stranded|stranding|movie|film|video|preview|webinar|presentation)\b`)

func (Wildlife) Candidates(ctx context.Context) ([]Image, error) {
	ag := agencies[rand.IntN(len(agencies))]
	subject := ag.subjects[rand.IntN(len(ag.subjects))]
	search := "hastemplate:" + ag.template + ` filew:>2400 filemime:image/jpeg "` + subject + `"`
	reject := func(texts ...string) bool {
		if isBlocked(texts...) {
			return true
		}
		for _, t := range texts {
			if wildlifeReject.MatchString(t) {
				return true
			}
		}
		return false
	}
	var out []Image
	for _, offset := range []int{rand.IntN(120), 0} {
		q := url.Values{
			"generator":    {"search"},
			"gsrsearch":    {search},
			"gsrnamespace": {"6"},
			"gsrlimit":     {"40"},
			"gsroffset":    {strconv.Itoa(offset)},
		}
		imgs, err := commonsQuery(ctx, q, ag.label, reject)
		if err != nil {
			return nil, err
		}
		out = append(out, imgs...)
		if len(out) > 0 {
			break
		}
	}
	return out, nil
}
