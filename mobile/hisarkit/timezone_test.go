package hisarkit

import (
	"testing"
	"time"
)

func TestSetTimeZoneMovesLocalTime(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })

	if err := SetTimeZone("Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
	if zone := time.Now().Location().String(); zone != "Asia/Tokyo" {
		t.Errorf("local zone = %s", zone)
	}
	newYearsEve := time.Date(2025, time.December, 31, 23, 30, 0, 0, time.UTC)
	if day := newYearsEve.Local().Format(time.DateOnly); day != "2026-01-01" {
		t.Errorf("23:30 UTC on New Year's Eve in Tokyo = %s, want New Year's Day", day)
	}

	if err := SetTimeZone("Mars/Olympus_Mons"); err == nil {
		t.Error("an unknown zone was accepted")
	}
	if zone := time.Now().Location().String(); zone != "Asia/Tokyo" {
		t.Errorf("an unknown zone moved local time to %s", zone)
	}
}
