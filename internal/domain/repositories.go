package domain

type VenueRepository interface {
	Create(v *Venue) error
	GetBySlug(slug string) (*Venue, error)
	GetByID(id int64) (*Venue, error)
	Update(v *Venue) error
}

type PartyRepository interface {
	Create(p *Party) error
	Get(id int64) (*Party, error)
	ListByVenue(venueID int64) ([]Party, error)
	Update(p *Party) error
}
