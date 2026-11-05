package main

import "testing"

func TestIsBlocked(t *testing.T) {
	blocked := []string{
		"Reclining Nude", "Study of a naked figure", "Topless sunbather", "Odalisque",
		"Bathers at Asnières", "erotic scene", "Desnudo", "Autopsy room", "Massacre of the innocents",
	}
	for _, s := range blocked {
		if !isBlocked(s) {
			t.Errorf("isBlocked(%q) = false, want true", s)
		}
	}
	fine := []string{
		"Bald eagle over the river", "Saturn's rings", "Sunset at Death Valley", "Nudibranch on a reef",
		"Sussex landscape", "Essex coast", "Middlesex", "The Pacific Ocean", "Breastwork of the old fort",
	}
	for _, s := range fine {
		if isBlocked(s) {
			t.Errorf("isBlocked(%q) = true, want false", s)
		}
	}
}

func TestIsBlockedArt(t *testing.T) {
	if !isBlockedArt("The Birth of Venus") || !isBlockedArt("Nymphs and Satyr") {
		t.Error("art list should catch classical nude subjects")
	}
	if isBlockedArt("View of Lake Pepin") {
		t.Error("art list should not block an ordinary landscape")
	}
}

// Photos of Bath, England used to be skipped because "bath" is on the art list.
// The art list now only applies to art categories.
func TestArtCategoryScopesArtList(t *testing.T) {
	photoCats := "Category:Bath Abbey | Category:Featured pictures of England"
	if artCategory.MatchString(photoCats) {
		t.Errorf("photo categories detected as art: %q", photoCats)
	}
	if !artCategory.MatchString("Category:Oil paintings in the Louvre") {
		t.Error("painting category not detected as art")
	}
}

func TestWildlifeReject(t *testing.T) {
	for _, s := range []string{"Map of the refuge", "Biologists tagging a seal", "Stranded whale", "Visitor center parking"} {
		if !wildlifeReject.MatchString(s) {
			t.Errorf("wildlifeReject should match %q", s)
		}
	}
	for _, s := range []string{"Bald eagle in flight", "Grand Prismatic Spring", "Humpback whale breaching"} {
		if wildlifeReject.MatchString(s) {
			t.Errorf("wildlifeReject should not match %q", s)
		}
	}
}

func TestNasaBoring(t *testing.T) {
	for _, s := range []string{"Map of Mimas", "Town Hall meeting", "Clean room assembly", "Award ceremony"} {
		if !nasaBoring.MatchString(s) {
			t.Errorf("nasaBoring should match %q", s)
		}
	}
	for _, s := range []string{"Pillars of Creation", "Aurora over Canada", "Falcon Heavy launch"} {
		if nasaBoring.MatchString(s) {
			t.Errorf("nasaBoring should not match %q", s)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	cases := []struct{ title, desc, want string }{
		{"Anim1019 - Flickr - NOAA Photo Library", "Humpback whale breaching. Photo by NOAA.", "Humpback whale breaching"},
		{"Cave-like features (50401407963)", "", "Cave-like features"},
		{"Tufted Puffin Bogoslof Island", "whatever", "Tufted Puffin Bogoslof Island"},
		{"IMG_2041", "", "IMG_2041"}, // nothing better available
	}
	for _, c := range cases {
		if got := cleanTitle(c.title, c.desc); got != c.want {
			t.Errorf("cleanTitle(%q, %q) = %q, want %q", c.title, c.desc, got, c.want)
		}
	}
}

func TestAIWords(t *testing.T) {
	for _, s := range []string{"Castle (AI art)", "Midjourney landscape", "stable diffusion test"} {
		if !aiWords.MatchString(s) {
			t.Errorf("aiWords should match %q", s)
		}
	}
	for _, s := range []string{"Mountain Air", "Fairytale forest", "Said the raven"} {
		if aiWords.MatchString(s) {
			t.Errorf("aiWords should not match %q", s)
		}
	}
}
