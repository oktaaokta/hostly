package domain

import (
	"testing"
	"time"
)

func closedAt(h, m int) time.Time {
	return time.Date(2026, 9, 7, h, m, 0, 0, time.UTC)
}

func TestVenueIsOpenSchedule(t *testing.T) {
	v := Venue{OpenTime: "10:00", CloseTime: "22:00"}
	cases := []struct {
		now  time.Time
		want bool
	}{
		{closedAt(9, 59), false},
		{closedAt(10, 0), true}, // [open, close): opening minute is open
		{closedAt(12, 0), true},
		{closedAt(21, 59), true},
		{closedAt(22, 0), false},
	}
	for _, c := range cases {
		if got := v.IsOpen(c.now); got != c.want {
			t.Errorf("IsOpen(%v) = %v, want %v", c.now, got, c.want)
		}
	}
}

func TestVenueIsOpenOverride(t *testing.T) {
	open := "open"
	closed := "closed"
	v := Venue{OpenTime: "10:00", CloseTime: "22:00", OpenOverride: &open}
	if !v.IsOpen(closedAt(3, 0)) {
		t.Error("override=open should be open at 3am")
	}
	v.OpenOverride = &closed
	if v.IsOpen(closedAt(12, 0)) {
		t.Error("override=closed should be closed at noon")
	}
}

func TestVenueIsValid(t *testing.T) {
	ok := Venue{Slug: "joes-diner", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok"}
	if err := ok.IsValid(); err != nil {
		t.Errorf("valid venue rejected: %v", err)
	}
	bad := ok
	bad.OpenTime = "noon"
	if err := bad.IsValid(); err == nil {
		t.Error("invalid open time accepted")
	}
}
