package main

import (
	"testing"
	"time"
)

func TestFormatPrice(t *testing.T) {
	for in, want := range map[float64]string{59391: "59,391", 3654.49: "3,654.49", 728.22: "728.22", 1234567.4: "1,234,567", 0.01234: "0.01234"} {
		if got := formatPrice(in); got != want {
			t.Errorf("formatPrice(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestUSMarketStatus(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	cases := map[string]string{
		"2026-11-28 10:00": "US market closed",               // Saturday
		"2026-11-26 10:00": "US market closed",               // Thanksgiving
		"2026-11-27 12:00": "US market open · closes in 1 h", // early close
		"2026-11-27 13:30": "US market closed",
		"2026-11-30 09:00": "US market opens in 30 min",
		"2026-11-30 14:15": "US market open · closes in 1 h 45 min",
		"2026-11-30 16:00": "US market closed",
	}
	for at, want := range cases {
		tm, _ := time.ParseInLocation("2006-01-02 15:04", at, ny)
		if got := usMarketStatus(tm); got != want {
			t.Errorf("%s: got %q, want %q", at, got, want)
		}
	}
}

func TestPickMarkets(t *testing.T) {
	p, c := 59391.0, -3.6
	f := &heatFile{}
	f.Markets = append(f.Markets, struct {
		Symbol string   `json:"symbol"`
		Price  *float64 `json:"current_price"`
		Chg    *float64 `json:"price_change_percentage_24h_in_currency"`
	}{"btc", &p, &c})
	got, ok := pickMarkets(map[string]*heatFile{"data.json": f})
	if ok != 1 || !got[0].OK || got[0].Price != "59,391" || got[1].OK || got[2].Sym != "SPY" {
		t.Fatalf("got %+v (%d ok)", got, ok)
	}
}

func TestWeatherText(t *testing.T) {
	if s, i := weatherText(0, true, 3000); s != "Hazy" || i != "sun.haze" {
		t.Errorf("hazy: %s %s", s, i)
	}
	if s, i := weatherText(2, false, 20000); s != "Partly cloudy" || i != "cloud.moon" {
		t.Errorf("night: %s %s", s, i)
	}
}
