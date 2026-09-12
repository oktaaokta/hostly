package usecase

import (
	"sort"
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

type WaitingEntry struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Pax   int    `json:"pax"`
	Order int    `json:"order"`
}

type CustomerView struct {
	Venue        *domain.Venue  `json:"venue"`
	IsOpen       bool           `json:"is_open"`
	Waiting      []WaitingEntry `json:"waiting"`
	WaitingCount int            `json:"waiting_count"`
}

func (q *Queue) CustomerView(slug string) (*CustomerView, error) {
	ven, err := q.ven.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	list, err := q.par.ListByVenue(ven.ID)
	if err != nil {
		return nil, err
	}
	out := &CustomerView{Venue: ven, IsOpen: ven.IsOpen(q.now())}
	for _, p := range list {
		if p.Status == domain.PartyWaiting {
			out.Waiting = append(out.Waiting, WaitingEntry{ID: p.ID, Name: p.Name, Pax: p.Pax, Order: p.Order})
			out.WaitingCount++
		}
	}
	sort.Slice(out.Waiting, func(i, j int) bool { return out.Waiting[i].Order < out.Waiting[j].Order })
	return out, nil
}

type Stats struct {
	Waiting     int `json:"waiting"`
	SeatedToday int `json:"seated_today"`
}

type StaffView struct {
	Venue   *domain.Venue   `json:"venue"`
	Parties []*domain.Party `json:"parties"`
	Stats   Stats           `json:"stats"`
}

func (q *Queue) StaffView(slug, token string) (*StaffView, error) {
	ven, err := q.ven.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	if !domain.TokenOK(ven.StaffToken, token) {
		return nil, domain.ErrUnauthorized
	}
	list, err := q.par.ListByVenue(ven.ID)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Order < list[j].Order })
	view := &StaffView{Venue: ven, Parties: []*domain.Party{}}
	today := q.now().Format("2006-01-02")
	for i := range list {
		p := &list[i]
		view.Parties = append(view.Parties, p)
		if p.Status == domain.PartyWaiting {
			view.Stats.Waiting++
		}
		if p.Status == domain.PartySeated && p.CreatedAt.Format("2006-01-02") == today {
			view.Stats.SeatedToday++
		}
	}
	return view, nil
}

func (q *Queue) seatOrLeave(slug, token string, partyID int64, next domain.PartyStatus) error {
	venue, err := q.fetchAuthorized(slug, token)
	if err != nil {
		return err
	}
	p, err := q.par.Get(partyID)
	if err != nil {
		return err
	}
	if p.VenueID != venue.ID {
		return domain.ErrNotFound
	}
	if p.Status != domain.PartyWaiting {
		return domain.ErrNotWaiting
	}
	p.Status = next
	return q.par.Update(p)
}

// Seat marks a waiting party seated.
func (q *Queue) Seat(slug, token string, partyID int64) error {
	return q.seatOrLeave(slug, token, partyID, domain.PartySeated)
}

// Leave marks a waiting party left/removed.
func (q *Queue) Leave(slug, token string, partyID int64) error {
	return q.seatOrLeave(slug, token, partyID, domain.PartyLeft)
}

// Top fast-tracks a waiting party ahead of every party still in the venue.
// The new order is (minimum order over ALL parties) - 1, so it can never
// collide with an existing order (orders are unique per venue) and the topped
// party is strictly before every other party, seated or waiting.
func (q *Queue) Top(slug, token string, partyID int64) error {
	venue, err := q.fetchAuthorized(slug, token)
	if err != nil {
		return err
	}
	p, err := q.par.Get(partyID)
	if err != nil {
		return err
	}
	if p.VenueID != venue.ID {
		return domain.ErrNotFound
	}
	if p.Status != domain.PartyWaiting {
		return domain.ErrNotWaiting
	}
	list, err := q.par.ListByVenue(venue.ID)
	if err != nil {
		return err
	}
	minOrder := 0
	first := true
	for _, x := range list {
		if first || x.Order < minOrder {
			minOrder = x.Order
			first = false
		}
	}
	if first {
		return nil
	}
	p.Order = minOrder - 1
	return q.par.Update(p)
}

// EditParty updates a waiting party's size and note.
func (q *Queue) EditParty(slug, token string, partyID int64, pax int, note string) error {
	venue, err := q.fetchAuthorized(slug, token)
	if err != nil {
		return err
	}
	if pax < 1 || pax > 20 {
		return domain.ErrInvalid
	}
	p, err := q.par.Get(partyID)
	if err != nil {
		return err
	}
	if p.VenueID != venue.ID {
		return domain.ErrNotFound
	}
	p.Pax, p.Note = pax, note
	return q.par.Update(p)
}

// UpdateHours sets a venue's open/close window and optional override
// (nil = follow the schedule).
func (q *Queue) UpdateHours(venueSlug, token, openTime, closeTime string, override *string) error {
	ven, err := q.fetchAuthorized(venueSlug, token)
	if err != nil {
		return err
	}
	ven.OpenTime, ven.CloseTime, ven.OpenOverride = openTime, closeTime, override
	if err := ven.IsValid(); err != nil {
		return err
	}
	return q.ven.Update(ven)
}

func (q *Queue) fetchAuthorized(slug, token string) (*domain.Venue, error) {
	ven, err := q.ven.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	if !domain.TokenOK(ven.StaffToken, token) {
		return nil, domain.ErrUnauthorized
	}
	return ven, nil
}
