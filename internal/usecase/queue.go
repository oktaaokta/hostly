package usecase

import (
	"strings"
	"time"

	"github.com/oktaaokta/hostly/internal/domain"
)

type Queue struct {
	ven domain.VenueRepository
	par domain.PartyRepository
	now func() time.Time
}

func NewQueue(ven domain.VenueRepository, par domain.PartyRepository, now func() time.Time) *Queue {
	return &Queue{ven: ven, par: par, now: now}
}

type JoinResult struct {
	Party *domain.Party
	Ahead int
}

// Join validates input, checks the venue is open, guards against duplicate
// names, assigns the next order, and persists the party.
func (q *Queue) Join(slug, name string, pax int, note string) (*JoinResult, error) {
	name = strings.TrimSpace(name)
	if name == "" || pax < 1 || pax > 20 {
		return nil, domain.ErrInvalid
	}
	ven, err := q.ven.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	if !ven.IsOpen(q.now()) {
		return nil, domain.ErrClosed
	}
	list, err := q.par.ListByVenue(ven.ID)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p.Status == domain.PartyWaiting && strings.EqualFold(p.Name, name) {
			return nil, domain.ErrDuplicate
		}
	}
	order := 0
	waiting := 0
	for _, p := range list {
		if p.Order >= order {
			order = p.Order + 1
		}
		if p.Status == domain.PartyWaiting {
			waiting++
		}
	}
	p := &domain.Party{
		VenueID: ven.ID, Name: name, Pax: pax, Note: note,
		Status: domain.PartyWaiting, Order: order, CreatedAt: q.now(),
	}
	if err := q.par.Create(p); err != nil {
		return nil, err
	}
	return &JoinResult{Party: p, Ahead: waiting}, nil
}

// VenueBySlug returns a venue (used by handlers for auth checks).
func (q *Queue) VenueBySlug(slug string) (*domain.Venue, error) {
	return q.ven.GetBySlug(slug)
}
