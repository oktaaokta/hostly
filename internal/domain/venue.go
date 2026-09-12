package domain

import (
	"crypto/subtle"
	"time"
)

// Venue is one restaurant. Hours are a single daily [open, close) window plus
// an optional staff override (nil = follow schedule).
type Venue struct {
	ID           int64
	Slug         string
	Name         string
	OpenTime     string // "15:04"
	CloseTime    string // "15:04"
	OpenOverride *string
	StaffToken   string
}

// TokenOK compares tokens in constant time to avoid timing leaks on the
// staff secret in the URL.
func TokenOK(have, want string) bool {
	if len(have) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(have), []byte(want)) == 1
}

// Hours are validated at write time (IsValid); unparsable times count as
// closed. Cross-midnight hours (close < open) are not supported; the window
// is same-day.
func (v Venue) IsOpen(now time.Time) bool {
	if v.OpenOverride != nil {
		return *v.OpenOverride == "open"
	}
	open, err1 := time.Parse("15:04", v.OpenTime)
	close, err2 := time.Parse("15:04", v.CloseTime)
	cur, err3 := time.Parse("15:04", now.Format("15:04"))
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	return !cur.Before(open) && cur.Before(close)
}

func (v Venue) IsValid() error {
	if v.Name == "" || v.Slug == "" || v.StaffToken == "" {
		return ErrInvalid
	}
	if _, err := time.Parse("15:04", v.OpenTime); err != nil {
		return ErrInvalid
	}
	if _, err := time.Parse("15:04", v.CloseTime); err != nil {
		return ErrInvalid
	}
	return nil
}
