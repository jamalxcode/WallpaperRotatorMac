package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The info box is a panel in the middle of the main display that sits on the
// wallpaper (below windows and desktop icons): the local time and weather,
// world clocks, the month and market moves from heat.sala.company. The clocks
// and calendar are drawn by the window itself (settings_darwin.m); this file
// supplies the weather, prices and the US market line.

const heatURL = "https://heat.sala.company/"

// Shown in this order. Crypto comes from heat's data.json, funds from etfs.json.
var infoSymbols = []struct{ Sym, File string }{
	{"BTC", "data.json"},
	{"XAUT", "data.json"},
	{"SPY", "etfs.json"},
	{"QQQ", "etfs.json"},
	{"DIA", "etfs.json"},
	{"XSD", "etfs.json"},
	{"AIQ", "etfs.json"},
}

const (
	infoPriceEvery   = 10 * time.Minute // heat rebuilds its data every 10 minutes
	infoWeatherEvery = 30 * time.Minute
)

type infoMarket struct {
	Sym   string  `json:"sym"`
	Price string  `json:"price"`
	Chg   float64 `json:"chg"`
	OK    bool    `json:"ok"`
}

// infoData is what the window shows, sent to it as JSON.
type infoData struct {
	City    string       `json:"city"`
	Temp    string       `json:"temp"`
	Range   string       `json:"range"`
	Icon    string       `json:"icon"` // SF Symbol name
	Status  string       `json:"status"`
	Picture string       `json:"picture"` // title of the wallpaper on the desktop now
	Credit  string       `json:"credit"`
	Updated string       `json:"updated"`
	Markets []infoMarket `json:"markets"`
}

type infoBox struct {
	mu      sync.Mutex
	on      bool
	stop    chan struct{}
	data    infoData
	lat     float64
	lon     float64
	located string // the city lat/lon were found for
}

var theInfoBox = newInfoBox()

func newInfoBox() *infoBox {
	b := &infoBox{}
	b.data.Markets, _ = pickMarkets(nil) // symbols with no figures yet
	return b
}

// set shows or hides the box, starting or stopping its refresh loop.
func (b *infoBox) set(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if on == b.on {
		b.pushLocked()
		return
	}
	b.on = on
	if on {
		b.stop = make(chan struct{})
		go b.loop(b.stop)
	} else {
		close(b.stop)
	}
	b.pushLocked()
}

// setPicture shows the current wallpaper's title and credit in the box.
func (b *infoBox) setPicture(title, credit string) {
	title, credit = oneLine(title), oneLine(credit)
	b.mu.Lock()
	defer b.mu.Unlock()
	if title == b.data.Picture && credit == b.data.Credit {
		return
	}
	b.data.Picture, b.data.Credit = title, credit
	if b.on {
		b.pushLocked()
	}
}

// oneLine collapses line breaks and runs of spaces (some titles span lines).
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func (b *infoBox) pushLocked() {
	b.data.City = strings.ToUpper(localCity())
	b.data.Status = usMarketStatus(time.Now())
	j, _ := json.Marshal(b.data)
	infoBoxSet(b.on, string(j))
}

func (b *infoBox) loop(stop chan struct{}) {
	var lastPrices, lastWeather time.Time
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		now := time.Now()
		if infoOnHold() {
			// Locked, display asleep, Low Power Mode or a hotspot: no downloads;
			// catch up when that ends. The clocks keep going.
		} else if now.Sub(lastPrices) >= infoPriceEvery {
			if b.refreshPrices() {
				lastPrices = now
			} else {
				lastPrices = now.Add(-infoPriceEvery + 2*time.Minute) // try again soon
			}
		}
		if !infoOnHold() && now.Sub(lastWeather) >= infoWeatherEvery {
			if b.refreshWeather() {
				lastWeather = now
			} else {
				lastWeather = now.Add(-infoWeatherEvery + 5*time.Minute)
			}
		}
		b.mu.Lock()
		if b.on {
			b.pushLocked() // the US market line changes on the minute
		}
		b.mu.Unlock()
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}

// infoOnHold is true while the app holds off downloads (see autoPauseReason).
// Pausing wallpaper changes by hand doesn't stop the box.
func infoOnHold() bool {
	a := theApp
	if a == nil {
		return false
	}
	r := a.autoPauseReason()
	return r != "" && r != "Paused"
}

// heatFile is the part of heat's data files the box needs.
type heatFile struct {
	Generated int64 `json:"generated"` // ms since 1970
	Markets   []struct {
		Symbol string   `json:"symbol"`
		Price  *float64 `json:"current_price"`
		Chg    *float64 `json:"price_change_percentage_24h_in_currency"`
	} `json:"markets"`
}

func (b *infoBox) refreshPrices() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	files := map[string]*heatFile{}
	for _, s := range infoSymbols {
		if _, done := files[s.File]; done {
			continue
		}
		var f heatFile
		if err := getJSON(ctx, heatURL+s.File, &f); err != nil {
			log.Print("info box prices: ", err)
			files[s.File] = nil
			continue
		}
		files[s.File] = &f
	}
	markets, ok := pickMarkets(files)
	var built int64 // the older of the files, so the time never flatters stale data
	for _, f := range files {
		if f != nil && f.Generated > 0 && (built == 0 || f.Generated < built) {
			built = f.Generated
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if ok == 0 {
		return false
	}
	// keep the last good figure for anything missing this time
	for i := range markets {
		if !markets[i].OK && i < len(b.data.Markets) && b.data.Markets[i].OK {
			markets[i] = b.data.Markets[i]
		}
	}
	b.data.Markets = markets
	if built > 0 {
		b.data.Updated = time.UnixMilli(built).Format("15:04")
	} else {
		b.data.Updated = time.Now().Format("15:04")
	}
	return ok == len(infoSymbols)
}

func pickMarkets(files map[string]*heatFile) ([]infoMarket, int) {
	out := make([]infoMarket, len(infoSymbols))
	ok := 0
	for i, s := range infoSymbols {
		out[i] = infoMarket{Sym: s.Sym}
		f := files[s.File]
		if f == nil {
			continue
		}
		for _, m := range f.Markets {
			if strings.EqualFold(m.Symbol, s.Sym) && m.Price != nil && m.Chg != nil {
				out[i] = infoMarket{Sym: s.Sym, Price: formatPrice(*m.Price), Chg: *m.Chg, OK: true}
				ok++
				break
			}
		}
	}
	return out, ok
}

func formatPrice(p float64) string {
	switch {
	case p >= 10000:
		return commas(fmt.Sprintf("%.0f", p))
	case p >= 1:
		return commas(fmt.Sprintf("%.2f", p))
	default:
		return fmt.Sprintf("%.4g", p)
	}
}

// commas puts thousands separators in a plain decimal number.
func commas(s string) string {
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteString("." + frac)
	}
	return b.String()
}

// localTZ is the Mac's time zone name, e.g. "Asia/Kuwait".
func localTZ() string {
	if p, err := os.Readlink("/etc/localtime"); err == nil {
		if _, after, ok := strings.Cut(p, "zoneinfo/"); ok {
			return after
		}
	}
	return ""
}

// hasWeatherCity is false for zones like Etc/UTC that aren't a place.
func hasWeatherCity(tz string) bool {
	return tz != "" && !strings.HasPrefix(tz, "Etc/") && strings.Contains(tz, "/")
}

// localCity is the city part of the time zone, e.g. "Kuwait" or "New York".
func localCity() string {
	tz := localTZ()
	if tz == "" {
		return "Local time"
	}
	return strings.ReplaceAll(filepath.Base(tz), "_", " ")
}

func (b *infoBox) refreshWeather() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b.mu.Lock()
	located, lat, lon := b.located, b.lat, b.lon
	b.mu.Unlock()
	city := localCity()
	if !hasWeatherCity(localTZ()) {
		b.mu.Lock()
		b.data.Temp, b.data.Range, b.data.Icon = "", "", ""
		b.mu.Unlock()
		return true
	}
	if located != city {
		var g struct {
			Results []struct {
				Latitude    float64 `json:"latitude"`
				Longitude   float64 `json:"longitude"`
				FeatureCode string  `json:"feature_code"`
			} `json:"results"`
		}
		if err := getJSON(ctx, "https://geocoding-api.open-meteo.com/v1/search?count=5&name="+url.QueryEscape(city), &g); err != nil || len(g.Results) == 0 {
			log.Printf("info box: finding %q for the weather: %v", city, err)
			return false
		}
		r := g.Results[0]
		for _, c := range g.Results { // prefer a city over a country's centre
			if strings.HasPrefix(c.FeatureCode, "PPL") {
				r = c
				break
			}
		}
		lat, lon = r.Latitude, r.Longitude
		b.mu.Lock()
		b.lat, b.lon, b.located = lat, lon, city
		b.mu.Unlock()
	}
	var w struct {
		Current struct {
			Temp       float64 `json:"temperature_2m"`
			Code       int     `json:"weather_code"`
			IsDay      int     `json:"is_day"`
			Visibility float64 `json:"visibility"`
		} `json:"current"`
		Daily struct {
			Max []float64 `json:"temperature_2m_max"`
			Min []float64 `json:"temperature_2m_min"`
		} `json:"daily"`
	}
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.3f&longitude=%.3f"+
		"&current=temperature_2m,weather_code,is_day,visibility&daily=temperature_2m_max,temperature_2m_min"+
		"&timezone=auto&forecast_days=1", lat, lon)
	if err := getJSON(ctx, u, &w); err != nil {
		log.Print("info box weather: ", err)
		return false
	}
	text, icon := weatherText(w.Current.Code, w.Current.IsDay != 0, w.Current.Visibility)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data.Temp = fmt.Sprintf("%.0f°C", deg(w.Current.Temp))
	b.data.Range = text
	if len(w.Daily.Max) > 0 && len(w.Daily.Min) > 0 {
		b.data.Range = fmt.Sprintf("%.0f° / %.0f° · %s", deg(w.Daily.Min[0]), deg(w.Daily.Max[0]), text)
	}
	b.data.Icon = icon
	return true
}

// deg rounds a temperature, never giving "-0".
func deg(t float64) float64 {
	r := math.Round(t)
	if r == 0 {
		return 0
	}
	return r
}

// weatherText turns a WMO weather code into words and an SF Symbol.
// Clear or cloudy skies with under 5 km visibility are called hazy.
func weatherText(code int, day bool, visibility float64) (string, string) {
	pick := func(d, n string) string {
		if day {
			return d
		}
		return n
	}
	if code <= 3 && visibility > 0 && visibility < 5000 {
		return "Hazy", pick("sun.haze", "moon.haze")
	}
	switch {
	case code == 0:
		return "Clear", pick("sun.max", "moon.stars")
	case code == 1:
		return "Mostly clear", pick("sun.max", "moon")
	case code == 2:
		return "Partly cloudy", pick("cloud.sun", "cloud.moon")
	case code == 3:
		return "Cloudy", "cloud"
	case code == 45 || code == 48:
		return "Fog", "cloud.fog"
	case code >= 51 && code <= 57:
		return "Drizzle", "cloud.drizzle"
	case code == 65 || code == 67:
		return "Heavy rain", "cloud.heavyrain"
	case code >= 61 && code <= 67:
		return "Rain", "cloud.rain"
	case code >= 71 && code <= 77, code == 85, code == 86:
		return "Snow", "cloud.snow"
	case code >= 80 && code <= 82:
		return "Showers", pick("cloud.sun.rain", "cloud.moon.rain")
	case code >= 95:
		return "Thunderstorms", "cloud.bolt.rain"
	}
	return "", pick("sun.max", "moon")
}

// NYSE holidays and 1 pm early closes.
var (
	nyseHolidays = map[string]bool{
		"2026-01-01": true, "2026-01-19": true, "2026-02-16": true, "2026-04-03": true, "2026-05-25": true,
		"2026-06-19": true, "2026-07-03": true, "2026-09-07": true, "2026-11-26": true, "2026-12-25": true,
		"2027-01-01": true, "2027-01-18": true, "2027-02-15": true, "2027-03-26": true, "2027-05-31": true,
		"2027-06-18": true, "2027-07-05": true, "2027-09-06": true, "2027-11-25": true, "2027-12-24": true,
	}
	nyseEarlyClose = map[string]bool{"2026-11-27": true, "2026-12-24": true, "2027-11-26": true}
)

func usMarketStatus(now time.Time) string {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		return ""
	}
	t := now.In(ny)
	day := t.Format("2006-01-02")
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday || nyseHolidays[day] {
		return "US market closed"
	}
	mins := t.Hour()*60 + t.Minute()
	closeAt := 16 * 60
	if nyseEarlyClose[day] {
		closeAt = 13 * 60
	}
	switch {
	case mins < 9*60+30:
		return fmt.Sprintf("US market opens in %s", untilText(9*60+30-mins))
	case mins < closeAt:
		return fmt.Sprintf("US market open · closes in %s", untilText(closeAt-mins))
	}
	return "US market closed"
}

func untilText(m int) string {
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}
