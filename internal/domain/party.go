package domain

import "time"

type PartyStatus string

const (
	PartyWaiting PartyStatus = "waiting"
	PartySeated  PartyStatus = "seated"
	PartyLeft    PartyStatus = "left"
)

// Party is one group in a venue's queue. Order is the sort key: lower = earlier.
type Party struct {
	ID        int64       `json:"id"`
	VenueID   int64       `json:"venue_id"`
	Name      string      `json:"name"`
	Pax       int         `json:"pax"`
	Note      string      `json:"note"`
	Status    PartyStatus `json:"status"`
	Order     int         `json:"order"`
	CreatedAt time.Time   `json:"created_at"`
}
