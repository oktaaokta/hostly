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
	ID        int64
	VenueID   int64
	Name      string
	Pax       int
	Note      string
	Status    PartyStatus
	Order     int
	CreatedAt time.Time
}
