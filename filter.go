package main

import (
	"regexp"
	"strings"
)

// Keyword filter applied to titles, descriptions, keywords and categories.
// Sources are already chosen to be safe (NASA, Commons featured photos);
// this is the second layer.
var blockedWords = regexp.MustCompile(`(?i)\b(` + strings.Join([]string{
	`nudes?`, `nudity`, `naked`, `topless`, `bare[- ]?breasted`, `breasts?`, `nipples?`,
	`genitals?`, `genitalia`, `penis`, `vagina`, `buttocks`, `erotic\w*`, `porn\w*`,
	`sex`, `sexual\w*`, `lingerie`, `bikinis?`, `swimsuits?`, `underwear`, `undress\w*`,
	`nus`, `desnud[oa]s?`, `nackt\w*`, `aktbild\w*`, `nudo`, `nuda`,
	`odalisques?`, `bathers?`, `bathing women`, `harem`, `pin-?up`, `burlesque`,
	`corpses?`, `dead bod(y|ies)`, `autopsy`, `executions?`, `gore`, `lynching`,
	`massacre`, `atrocit\w*`, `victims?`, `injur\w*`, `wounded`, `blood`,
	`surgery`, `surgical`, `medical`, `anatomy`, `anatomical`,
}, "|") + `)\b`)

// Extra terms for art-heavy sources (paintings often show nudes without saying "nude").
var blockedArt = regexp.MustCompile(`(?i)\b(venus|aphrodite|susanna|leda|danaë|danae|eve|three graces|bacchanal\w*|nymphs?|cupid|psyche|judgement of paris|bath(ing)?|toilette)\b`)

func isBlocked(texts ...string) bool {
	for _, t := range texts {
		if blockedWords.MatchString(t) {
			return true
		}
	}
	return false
}

func isBlockedArt(texts ...string) bool {
	for _, t := range texts {
		if blockedArt.MatchString(t) {
			return true
		}
	}
	return false
}
