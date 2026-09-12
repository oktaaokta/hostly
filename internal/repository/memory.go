package repository

import (
	"strings"
	"sync"

	"github.com/oktaaokta/hostly/internal/domain"
)

// Memory is a goroutine-safe in-memory repository. Used in tests.
type Memory struct {
	mu        sync.Mutex
	venues    map[int64]*domain.Venue
	parties   map[int64]*domain.Party
	bySlug    map[string]*domain.Venue
	nextV     int64
	nextP     int64
	venueRepo *memoryVenueRepo
	partyRepo *memoryPartyRepo
}

func NewMemory() *Memory {
	m := &Memory{
		venues:  map[int64]*domain.Venue{},
		parties: map[int64]*domain.Party{},
		bySlug:  map[string]*domain.Venue{},
	}
	m.venueRepo = &memoryVenueRepo{m: m}
	m.partyRepo = &memoryPartyRepo{m: m}
	return m
}

func (m *Memory) Venues() domain.VenueRepository  { return m.venueRepo }
func (m *Memory) Parties() domain.PartyRepository { return m.partyRepo }

type memoryVenueRepo struct{ m *Memory }

func (r *memoryVenueRepo) Create(v *domain.Venue) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	v.ID = r.m.nextV
	r.m.nextV++
	r.m.venues[v.ID] = v
	r.m.bySlug[strings.ToLower(v.Slug)] = v
	return nil
}

func (r *memoryVenueRepo) GetBySlug(slug string) (*domain.Venue, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	v, ok := r.m.bySlug[strings.ToLower(slug)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return v, nil
}

func (r *memoryVenueRepo) GetByID(id int64) (*domain.Venue, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	v, ok := r.m.venues[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return v, nil
}

func (r *memoryVenueRepo) Update(v *domain.Venue) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.venues[v.ID]; !ok {
		return domain.ErrNotFound
	}
	if old, ok := r.m.bySlug[strings.ToLower(v.Slug)]; ok && old.ID != v.ID {
		return domain.ErrInvalid
	}
	delete(r.m.bySlug, strings.ToLower(r.m.venues[v.ID].Slug))
	r.m.venues[v.ID] = v
	r.m.bySlug[strings.ToLower(v.Slug)] = v
	return nil
}

type memoryPartyRepo struct{ m *Memory }

func (r *memoryPartyRepo) Create(p *domain.Party) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p.ID = r.m.nextP
	r.m.nextP++
	r.m.parties[p.ID] = p
	return nil
}

func (r *memoryPartyRepo) Get(id int64) (*domain.Party, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p, ok := r.m.parties[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return p, nil
}

func (r *memoryPartyRepo) ListByVenue(venueID int64) ([]domain.Party, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	out := []domain.Party{}
	for _, p := range r.m.parties {
		if p.VenueID == venueID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *memoryPartyRepo) Update(p *domain.Party) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	old, ok := r.m.parties[p.ID]
	if !ok {
		return domain.ErrNotFound
	}
	p.VenueID = old.VenueID
	r.m.parties[p.ID] = p
	return nil
}
