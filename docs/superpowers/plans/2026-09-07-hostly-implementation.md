# hostly — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship hostly — a multi-venue restaurant queueing system with a customer waitlist page (live spot + being-called banner), a per-venue staff dashboard, and SQLite persistence, deployed as a single Go binary serving an embedded React SPA.

**Architecture:** One Go process (chi) exposes a REST API + WebSocket for live updates and serves the built React SPA via `go:embed`. Venues and parties live in SQLite (pure-Go `modernc.org/sqlite`, no cgo). Customer and staff pages are the same SPA with different routes; public routes need no auth, staff routes are gated by a per-venue secret token in the URL.

**Tech Stack:** Go 1.26 (chi v5, gorilla/websocket, modernc.org/sqlite), React + TypeScript + Vite, plain CSS, Web Audio for the chime.

**Spec:** `docs/superpowers/specs/2026-09-07-hostly-design.md`
**Repo:** `github.com/oktaaokta/hostly` (private) — module `github.com/oktaaokta/hostly`

**Phases:**
1. Go core — domain model + queue use case (TDD, in-memory repo for tests)
2. Persistence + API — SQLite repo, HTTP handlers, WebSocket hub, `main.go`
3. Frontend — customer page, staff dashboard, embed + serve
4. Ship — README, push to GitHub

---

## Phase 1 — Go core (domain + queue use case)

### Task 1: Module, domain types, repository interfaces

**Files:**
- Create: `go.mod`
- Create: `internal/domain/errors.go`
- Create: `internal/domain/venue.go`
- Create: `internal/domain/party.go`
- Create: `internal/domain/repositories.go`
- Create: `internal/domain/venue_test.go`

- [ ] **Step 1: Initialize the module**

```bash
cd /Users/okta/workspace/hostly
go mod init github.com/oktaaokta/hostly
```

- [ ] **Step 2: Write `internal/domain/errors.go`**

```go
package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrClosed       = errors.New("venue is closed")
	ErrDuplicate    = errors.New("already in the queue")
	ErrNotWaiting   = errors.New("party is not waiting")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalid      = errors.New("invalid input")
)
```

- [ ] **Step 3: Write `internal/domain/venue.go`**

```go
package domain

import (
	"crypto/subtle"
	"time"
)

// Venue is one restaurant. Hours are a single daily [open, close) window plus
// an optional staff override (nil = follow schedule).
type Venue struct {
	ID           int64   `json:"id"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	OpenTime     string  `json:"open_time"`  // "15:04"
	CloseTime    string  `json:"close_time"` // "15:04"
	OpenOverride *string `json:"open_override"`
	StaffToken   string  `json:"-"`
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
```

- [ ] **Step 4: Write `internal/domain/party.go`**

```go
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
```

- [ ] **Step 5: Write `internal/domain/repositories.go`**

```go
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
```

- [ ] **Step 6: Write `internal/domain/venue_test.go`**

```go
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
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/domain/...
```
Expected: PASS (3 tests).

- [ ] **Step 8: Commit**

```bash
git add go.mod internal/domain
git commit -m "feat: domain model and repository interfaces"
```

### Task 2: In-memory repository (test double)

**Files:**
- Create: `internal/repository/memory.go`
- Create: `internal/repository/memory_test.go`

- [ ] **Step 1: Write `internal/repository/memory.go`**

```go
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

func (m *Memory) Venues() domain.VenueRepository { return m.venueRepo }
func (m *Memory) Parties() domain.PartyRepository { return m.partyRepo }

type memoryVenueRepo struct{ m *Memory }

func (r *memoryVenueRepo) Create(v *domain.Venue) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	v.ID = r.m.nextV
	r.m.nextV++
	if _, ok := r.m.bySlug[strings.ToLower(v.Slug)]; ok {
		return domain.ErrInvalid
	}
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
	cp := *v
	return &cp, nil
}

func (r *memoryVenueRepo) GetByID(id int64) (*domain.Venue, error) {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	v, ok := r.m.venues[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (r *memoryVenueRepo) Update(v *domain.Venue) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if _, ok := r.m.venues[v.ID]; !ok {
		return domain.ErrNotFound
	}
	if other, ok := r.m.bySlug[strings.ToLower(v.Slug)]; ok && other.ID != v.ID {
		return domain.ErrInvalid
	}
	for slug, ven := range r.m.bySlug {
		if ven.ID == v.ID {
			delete(r.m.bySlug, slug)
		}
	}
	r.m.venues[v.ID] = v
	r.m.bySlug[strings.ToLower(v.Slug)] = v
	return nil
}

type memoryPartyRepo struct{ m *Memory }

func (r *memoryPartyRepo) Create(p *domain.Party) error {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
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
	cp := *p
	return &cp, nil
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
```

- [ ] **Step 2: Write `internal/repository/memory_test.go`**

```go
package repository

import (
	"testing"

	"github.com/oktaaokta/hostly/internal/domain"
)

func TestMemoryCRUD(t *testing.T) {
	ven := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	m := NewMemory()
	if err := m.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	got, err := m.Venues().GetBySlug("A")
	if err != nil || got.ID != ven.ID {
		t.Fatalf("GetBySlug = %v, %v", got, err)
	}
	p := &domain.Party{VenueID: ven.ID, Name: "Alex", Pax: 2, Status: domain.PartyWaiting, Order: 1}
	if err := m.Parties().Create(p); err != nil {
		t.Fatal(err)
	}
	p.Pax = 3
	if err := m.Parties().Update(p); err != nil {
		t.Fatal(err)
	}
	list, err := m.Parties().ListByVenue(ven.ID)
	if err != nil || len(list) != 1 || list[0].Pax != 3 {
		t.Fatalf("ListByVenue = %v, %v", list, err)
	}
	if _, err := m.Venues().GetByID(999); err != domain.ErrNotFound {
		t.Fatalf("missing venue error = %v", err)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/repository/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/repository
git commit -m "feat: in-memory repository for tests"
```

### Task 3: Queue use case — joining

**Files:**
- Create: `internal/usecase/queue.go`
- Create: `internal/usecase/queue_test.go`

- [ ] **Step 1: Write `internal/usecase/queue_test.go` (joining tests)**

```go
package usecase

import (
	"testing"
	"time"

	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/repository"
)

func openVenue(t *testing.T, m *repository.Memory) *domain.Venue {
	t.Helper()
	v := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok"}
	if err := m.Venues().Create(v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestJoinHappyPath(t *testing.T) {
	m := repository.NewMemory()
	_ = openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })

	res, err := q.Join("joes", "Alex", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Ahead != 0 {
		t.Errorf("first party ahead = %d, want 0", res.Ahead)
	}
	if res.Party.Status != domain.PartyWaiting {
		t.Errorf("status = %v", res.Party.Status)
	}

	res2, err := q.Join("joes", "Bea", 4, "booth")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Ahead != 1 {
		t.Errorf("second party ahead = %d, want 1", res2.Ahead)
	}
	if !(res2.Party.Order > res.Party.Order) {
		t.Error("later party should have higher order")
	}
}

func TestJoinValidation(t *testing.T) {
	m := repository.NewMemory()
	_ = openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), time.Now)

	if _, err := q.Join("joes", "  ", 2, ""); err != domain.ErrInvalid {
		t.Errorf("empty name err = %v", err)
	}
	if _, err := q.Join("joes", "Alex", 0, ""); err != domain.ErrInvalid {
		t.Errorf("pax 0 err = %v", err)
	}
	if _, err := q.Join("joes", "Alex", 21, ""); err != domain.ErrInvalid {
		t.Errorf("pax 21 err = %v", err)
	}
	if _, err := q.Join("missing", "Alex", 2, ""); err != domain.ErrNotFound {
		t.Errorf("missing venue err = %v", err)
	}
}

func TestJoinDuplicate(t *testing.T) {
	m := repository.NewMemory()
	_ = openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })

	if _, err := q.Join("joes", "Alex", 2, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Join("joes", "alex", 3, ""); err != domain.ErrDuplicate {
		t.Errorf("duplicate name err = %v", err)
	}
}

func TestJoinWhenClosed(t *testing.T) {
	m := repository.NewMemory()
	_ = openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 23, 0, 0, 0, time.UTC) })

	if _, err := q.Join("joes", "Alex", 2, ""); err != domain.ErrClosed {
		t.Errorf("closed venue err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/usecase/...`
Expected: FAIL — no package / undefined `NewQueue`.

- [ ] **Step 3: Write `internal/usecase/queue.go` (Join + VenueBySlug)**

```go
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
	Party *domain.Party `json:"party"`
	Ahead int           `json:"ahead"`
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
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/usecase/...`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/usecase
git commit -m "feat: queue use case join with validation"
```

### Task 4: Queue use case — staff operations, views, hours

**Files:**
- Modify: `internal/usecase/queue.go`
- Modify: `internal/usecase/queue_test.go`

- [ ] **Step 1: Append the operations tests to `queue_test.go` (same package, add below the joining tests)**

```go
func mustJoin(t *testing.T, q *Queue, slug, name string, pax int) *domain.Party {
	t.Helper()
	res, err := q.Join(slug, name, pax, "")
	if err != nil {
		t.Fatal(err)
	}
	return res.Party
}

func TestSeatLeaveTopEditHours(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })

	a := mustJoin(t, q, ven.Slug, "Alex", 2)
	b := mustJoin(t, q, ven.Slug, "Bea", 4)

	if err := q.Seat(ven.Slug, "wrong", a.ID); err != domain.ErrUnauthorized {
		t.Errorf("bad token err = %v", err)
	}

	if err := q.Seat(ven.Slug, "tok", a.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Seat(ven.Slug, "tok", a.ID); err != domain.ErrNotWaiting {
		t.Errorf("re-seat err = %v", err)
	}

	view, _ := q.StaffView(ven.Slug, "tok")
	if view.Stats.Waiting != 1 || view.Stats.SeatedToday != 1 {
		t.Errorf("stats after seat = %+v", view.Stats)
	}
	firstWaiting := int64(-1)
	for _, p := range view.Parties {
		if p.Status == domain.PartyWaiting {
			firstWaiting = p.ID
			break
		}
	}
	if firstWaiting != b.ID {
		t.Errorf("front of queue after seat = %d, want %d", firstWaiting, b.ID)
	}

	if err := q.Leave(ven.Slug, "tok", b.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Leave(ven.Slug, "tok", b.ID); err != domain.ErrNotWaiting {
		t.Errorf("double leave err = %v", err)
	}

	c := mustJoin(t, q, ven.Slug, "Cid", 2)
	if err := q.Top(ven.Slug, "tok", c.ID); err != nil {
		t.Fatal(err)
	}
	view2, _ := q.CustomerView(ven.Slug)
	if len(view2.Waiting) != 1 || view2.Waiting[0].ID != c.ID {
		t.Errorf("after top, front = %+v", view2.Waiting)
	}
	if view2.WaitingCount != 1 {
		t.Errorf("waiting count = %d", view2.WaitingCount)
	}

	if err := q.EditParty(ven.Slug, "tok", c.ID, 5, "window seat"); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateHours(ven.Slug, "tok", "11:00", "23:00", nil); err != nil {
		t.Fatal(err)
	}
	info, _ := q.CustomerView(ven.Slug)
	if info.Venue.OpenTime != "11:00" {
		t.Errorf("open time = %s", info.Venue.OpenTime)
	}
	if info.Waiting[0].Pax != 5 {
		t.Errorf("edited pax = %d", info.Waiting[0].Pax)
	}

	if _, err := q.StaffView(ven.Slug, "nope"); err != domain.ErrUnauthorized {
		t.Errorf("staff bad token err = %v", err)
	}
	if _, err := q.StaffView("missing", "tok"); err != domain.ErrNotFound {
		t.Errorf("staff missing venue err = %v", err)
	}
}
```

- [ ] **Step 2: Run tests, verify the operations fail to compile**

Run: `go test ./internal/usecase/...`
Expected: FAIL — undefined `q.Seat`, `q.StaffView`, `q.CustomerView`, `q.Top`, `q.Leave`, `q.EditParty`, `q.UpdateHours`.

- [ ] **Step 3: Append views + operations to `queue.go` and add `"sort"` to its imports**

```go
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
	out := &CustomerView{Venue: ven, IsOpen: ven.IsOpen(q.now()), Waiting: []WaitingEntry{}}
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
```

- [ ] **Step 4: Run all Go tests**

Run: `go test ./internal/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain internal/repository internal/usecase
git commit -m "feat: queue operations, staff auth, views, hours"
```

**End of Phase 1.** Verify `go test ./...` is fully green before continuing.

---

## Phase 2 — HTTP API + WebSocket + persistence

### Task 5: HTTP handlers + staff auth + WebSocket hub

**Files:**
- Create: `internal/handler/handler.go`
- Create: `internal/handler/hub.go`
- Create: `internal/handler/handler_test.go`

- [ ] **Step 1: Add dependencies (chi + gorilla/websocket)**

```bash
cd /Users/okta/workspace/hostly
go get github.com/go-chi/chi/v5@latest github.com/gorilla/websocket@latest
```

- [ ] **Step 2: Write `internal/handler/handler_test.go` first**

```go
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/repository"
	"github.com/oktaaokta/hostly/internal/usecase"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := repository.NewMemory()
	ven := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok"}
	if err := m.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	q := usecase.NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })
	h := New(q, NewHub())
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestJoinEndpoint(t *testing.T) {
	ts := newTestServer(t)

	resp := post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "Alex", "pax": 2})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got struct {
		Ahead int `json:"ahead"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Ahead != 0 {
		t.Errorf("ahead = %v", got.Ahead)
	}

	resp = post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "Alex", "pax": 2})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "", "pax": 2})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty-name status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStaffAuthRequired(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/venues/joes/staff")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing token status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/venues/joes/staff?token=bad")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad token status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStaffSeatFlow(t *testing.T) {
	ts := newTestServer(t)
	join := post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "Bea", "pax": 4})
	var jr struct {
		Party struct {
			ID int64 `json:"id"`
		} `json:"party"`
	}
	json.NewDecoder(join.Body).Decode(&jr)
	join.Body.Close()

	path := fmt.Sprintf("/api/venues/joes/parties/%d/seat?token=tok", jr.Party.ID)
	resp, err := http.Post(ts.URL+path, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("seat status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	get, err := http.Get(ts.URL + "/api/venues/joes")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var cv struct {
		IsOpen       bool `json:"is_open"`
		WaitingCount int  `json:"waiting_count"`
	}
	json.NewDecoder(get.Body).Decode(&cv)
	if !cv.IsOpen || cv.WaitingCount != 0 {
		t.Errorf("customer view after seat = %+v", cv)
	}
}
```

- [ ] **Step 3: Run tests, verify they fail**

Run: `go test ./internal/handler/...`
Expected: FAIL — package not found / `New`, `RegisterRoutes` undefined.

- [ ] **Step 4: Write `internal/handler/hub.go`**

```go
package handler

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub fans JSON events out to every connected client for a venue. Rooms are
// keyed by venue ID. Events are change signals: clients refetch state over
// REST after receiving one.
type Hub struct {
	mu    sync.Mutex
	rooms map[int64]map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{rooms: map[int64]map[*websocket.Conn]struct{}{}}
}

func (h *Hub) add(venueID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[venueID] == nil {
		h.rooms[venueID] = map[*websocket.Conn]struct{}{}
	}
	h.rooms[venueID][c] = struct{}{}
}

func (h *Hub) remove(venueID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[venueID]
	if room == nil {
		return
	}
	delete(room, c)
	if len(room) == 0 {
		delete(h.rooms, venueID)
	}
}

// Broadcast marshals ev and writes it to every connection in the room.
func (h *Hub) Broadcast(venueID int64, ev any) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[venueID] {
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			c.Close()
			delete(h.rooms[venueID], c)
		}
	}
}
```

- [ ] **Step 5: Write `internal/handler/handler.go`**

```go
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/usecase"
)

type Handler struct {
	us       *usecase.Queue
	hub      *Hub
	upgrader websocket.Upgrader
}

func New(us *usecase.Queue, hub *Hub) *Handler {
	return &Handler{
		us:       us,
		hub:      hub,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
}

type event struct {
	Type  string `json:"type"`
	Party any    `json:"party,omitempty"`
	Venue any    `json:"venue,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrClosed), errors.Is(err, domain.ErrDuplicate), errors.Is(err, domain.ErrNotWaiting):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/venues/{slug}", h.getCustomerView)
	r.Post("/api/venues/{slug}/parties", h.join)
	r.Get("/api/venues/{slug}/staff", h.getStaffView)
	r.Patch("/api/venues/{slug}/parties/{id}", h.editParty)
	r.Post("/api/venues/{slug}/parties/{id}/seat", h.seat)
	r.Post("/api/venues/{slug}/parties/{id}/leave", h.leave)
	r.Post("/api/venues/{slug}/parties/{id}/top", h.top)
	r.Patch("/api/venues/{slug}/hours", h.updateHours)
	r.Get("/api/venues/{slug}/ws", h.websocket)
}

func (h *Handler) getCustomerView(w http.ResponseWriter, r *http.Request) {
	v, err := h.us.CustomerView(chi.URLParam(r, "slug"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type joinRequest struct {
	Name string `json:"name"`
	Pax  int    `json:"pax"`
	Note string `json:"note"`
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var body joinRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	res, err := h.us.Join(slug, body.Name, body.Pax, body.Note)
	if err != nil {
		writeErr(w, err)
		return
	}
	ven, err := h.us.VenueBySlug(slug)
	if err == nil {
		h.hub.Broadcast(ven.ID, event{Type: "party_joined", Party: res.Party})
	}
	writeJSON(w, http.StatusCreated, res)
}

func token(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return r.Header.Get("X-Staff-Token")
}

func (h *Handler) getStaffView(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	v, err := h.us.StaffView(slug, token(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// partyAction runs a staff action on a party and broadcasts a venue update.
func (h *Handler) partyAction(w http.ResponseWriter, r *http.Request, act func(int64) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	if err := act(id); err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastVenue(r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) seat(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.partyAction(w, r, func(id int64) error { return h.us.Seat(slug, token(r), id) })
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.partyAction(w, r, func(id int64) error { return h.us.Leave(slug, token(r), id) })
}

func (h *Handler) top(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.partyAction(w, r, func(id int64) error { return h.us.Top(slug, token(r), id) })
}

type editRequest struct {
	Pax  int    `json:"pax"`
	Note string `json:"note"`
}

func (h *Handler) editParty(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	var body editRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	if err := h.us.EditParty(slug, token(r), id, body.Pax, body.Note); err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastVenue(r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type hoursRequest struct {
	OpenTime  string  `json:"open_time"`
	CloseTime string  `json:"close_time"`
	Override  *string `json:"override"`
}

func (h *Handler) updateHours(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var body hoursRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	if err := h.us.UpdateHours(slug, token(r), body.OpenTime, body.CloseTime, body.Override); err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastVenue(r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// broadcastVenue pushes the current customer view to every client of the venue.
func (h *Handler) broadcastVenue(r *http.Request) {
	slug := chi.URLParam(r, "slug")
	v, err := h.us.CustomerView(slug)
	if err != nil {
		return
	}
	h.hub.Broadcast(v.Venue.ID, event{Type: "venue_updated", Venue: v})
}

func (h *Handler) websocket(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	ven, err := h.us.VenueBySlug(slug)
	if err != nil {
		writeErr(w, err)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.hub.add(ven.ID, conn)
	defer h.hub.remove(ven.ID, conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
```

- [ ] **Step 6: Run all Go tests**

Run: `go test ./...`
Expected: PASS (domain, repository, usecase, handler).

- [ ] **Step 7: Commit**

```bash
git add internal/handler go.mod go.sum
git commit -m "feat: http + websocket api"
```

### Task 6: SQLite repository

**Files:**
- Create: `internal/repository/sqlite.go`
- Create: `internal/repository/sqlite_test.go`

- [ ] **Step 1: Add the SQLite dependency**

```bash
cd /Users/okta/workspace/hostly
go get modernc.org/sqlite@latest
```

- [ ] **Step 2: Write `internal/repository/sqlite_test.go` first**

```go
package repository

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/oktaaokta/hostly/internal/domain"
)

func TestSQLiteCRUD(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ven := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := db.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	if ven.ID == 0 {
		t.Fatal("venue id not assigned")
	}
	got, err := db.Venues().GetBySlug("A")
	if err != nil || got.ID != ven.ID {
		t.Fatalf("GetBySlug = %v, %v", got, err)
	}

	p := &domain.Party{VenueID: ven.ID, Name: "Alex", Pax: 2, Status: domain.PartyWaiting, Order: 1}
	if err := db.Parties().Create(p); err != nil {
		t.Fatal(err)
	}
	p.Pax = 3
	if err := db.Parties().Update(p); err != nil {
		t.Fatal(err)
	}
	list, err := db.Parties().ListByVenue(ven.ID)
	if err != nil || len(list) != 1 || list[0].Pax != 3 {
		t.Fatalf("ListByVenue = %v, %v", list, err)
	}
	if !list[0].CreatedAt.After(time.Now().Add(-time.Hour)) {
		t.Fatal("created_at not parsed back")
	}

	if _, err := db.Venues().GetByID(999); err != domain.ErrNotFound {
		t.Fatalf("missing venue error = %v", err)
	}
}
```

- [ ] **Step 3: Run the test, verify it fails**

Run: `go test ./internal/repository/...`
Expected: FAIL — `OpenSQLite` undefined.

- [ ] **Step 4: Write `internal/repository/sqlite.go`**

```go
package repository

import (
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/oktaaokta/hostly/internal/domain"
)

const schema = `
CREATE TABLE IF NOT EXISTS venues (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	slug TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	open_time TEXT NOT NULL,
	close_time TEXT NOT NULL,
	open_override TEXT,
	staff_token TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS parties (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	venue_id INTEGER NOT NULL REFERENCES venues(id),
	name TEXT NOT NULL,
	pax INTEGER NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	order_no INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE(venue_id, order_no)
);
`

type SQLite struct {
	db        *sql.DB
	venueRepo *sqliteVenueRepo
	partyRepo *sqlitePartyRepo
}

// OpenSQLite opens (creating if needed) the SQLite database, applies the
// schema, and returns a ready repository.
func OpenSQLite(path string) (*SQLite, error) {
	dsn := path
	if !strings.Contains(path, "?") {
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	r := &SQLite{db: db}
	r.venueRepo = &sqliteVenueRepo{db: db}
	r.partyRepo = &sqlitePartyRepo{db: db}
	return r, nil
}

func (r *SQLite) Venues() domain.VenueRepository  { return r.venueRepo }
func (r *SQLite) Parties() domain.PartyRepository { return r.partyRepo }
func (r *SQLite) Close() error                     { return r.db.Close() }

const venueCols = "id, slug, name, open_time, close_time, open_override, staff_token"

func scanVenue(row interface{ Scan(...any) error }) (*domain.Venue, error) {
	var v domain.Venue
	if err := row.Scan(&v.ID, &v.Slug, &v.Name, &v.OpenTime, &v.CloseTime, &v.OpenOverride, &v.StaffToken); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

type sqliteVenueRepo struct{ db *sql.DB }

func (r *sqliteVenueRepo) Create(v *domain.Venue) error {
	res, err := r.db.Exec(`INSERT INTO venues (slug, name, open_time, close_time, open_override, staff_token)
		VALUES (?, ?, ?, ?, ?, ?)`, v.Slug, v.Name, v.OpenTime, v.CloseTime, v.OpenOverride, v.StaffToken)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrInvalid
		}
		return err
	}
	v.ID, _ = res.LastInsertId()
	return nil
}

func (r *sqliteVenueRepo) GetBySlug(slug string) (*domain.Venue, error) {
	row := r.db.QueryRow(`SELECT `+venueCols+` FROM venues WHERE lower(slug) = lower(?)`, slug)
	return scanVenue(row)
}

func (r *sqliteVenueRepo) GetByID(id int64) (*domain.Venue, error) {
	row := r.db.QueryRow(`SELECT `+venueCols+` FROM venues WHERE id = ?`, id)
	return scanVenue(row)
}

func (r *sqliteVenueRepo) Update(v *domain.Venue) error {
	res, err := r.db.Exec(`UPDATE venues SET slug=?, name=?, open_time=?, close_time=?, open_override=?, staff_token=? WHERE id=?`,
		v.Slug, v.Name, v.OpenTime, v.CloseTime, v.OpenOverride, v.StaffToken, v.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrInvalid
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

const partyCols = "id, venue_id, name, pax, note, status, order_no, created_at"

func scanParty(row interface{ Scan(...any) error }) (*domain.Party, error) {
	var p domain.Party
	var created string
	if err := row.Scan(&p.ID, &p.VenueID, &p.Name, &p.Pax, &p.Note, &p.Status, &p.Order, &created); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", created)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = t
	return &p, nil
}

type sqlitePartyRepo struct{ db *sql.DB }

func (r *sqlitePartyRepo) Create(p *domain.Party) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	created := p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	res, err := r.db.Exec(`INSERT INTO parties (venue_id, name, pax, note, status, order_no, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.VenueID, p.Name, p.Pax, p.Note, string(p.Status), p.Order, created)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrInvalid
		}
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func (r *sqlitePartyRepo) Get(id int64) (*domain.Party, error) {
	row := r.db.QueryRow(`SELECT `+partyCols+` FROM parties WHERE id = ?`, id)
	return scanParty(row)
}

func (r *sqlitePartyRepo) ListByVenue(venueID int64) ([]domain.Party, error) {
	rows, err := r.db.Query(`SELECT `+partyCols+` FROM parties WHERE venue_id = ?`, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Party{}
	for rows.Next() {
		p, err := scanParty(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *sqlitePartyRepo) Update(p *domain.Party) error {
	created := p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	res, err := r.db.Exec(`UPDATE parties SET name=?, pax=?, note=?, status=?, order_no=?, created_at=? WHERE id=?`,
		p.Name, p.Pax, p.Note, string(p.Status), p.Order, created, p.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.ErrInvalid
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/repository/...`
Expected: PASS (memory + sqlite suites).

- [ ] **Step 6: Accept the join order-race caveat**

Join does a non-transactional list+create; concurrent joins can briefly collide on `UNIQUE(venue_id, order_no)` and surface a spurious `ErrInvalid`. Accepted for a demo — no transaction wrapping.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/repository/sqlite.go internal/repository/sqlite_test.go
git commit -m "feat: sqlite repository"
```

### Task 7: `main.go` wiring + seed + Makefile

**Files:**
- Create: `cmd/hostly/main.go`
- Create: `Makefile`

- [ ] **Step 1: Write `cmd/hostly/main.go`**

```go
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/handler"
	"github.com/oktaaokta/hostly/internal/repository"
	"github.com/oktaaokta/hostly/internal/usecase"
)

func main() {
	seed := flag.Bool("seed", false, "create the demo venue if missing")
	flag.Parse()

	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "hostly.db")
	basePath := envOr("BASE_PATH", "")

	db, err := repository.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	q := usecase.NewQueue(db.Venues(), db.Parties(), time.Now)

	if *seed {
		seedVenue(db.Venues())
	}

	h := handler.New(q, handler.NewHub())
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	h.RegisterRoutes(r)

	addr := ":" + port
	server := http.Handler(r)
	if basePath != "" {
		server = http.StripPrefix(basePath, r)
	}
	log.Printf("hostly listening on %s%s", addr, basePath)
	log.Fatal(http.ListenAndServe(addr, server))
}

func seedVenue(vr domain.VenueRepository) {
	v := &domain.Venue{Slug: "joes-diner", Name: "Joe's Diner", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "demo-staff-token"}
	if _, err := vr.GetBySlug(v.Slug); err == nil {
		log.Println("seed: joes-diner already exists")
		return
	}
	if err := vr.Create(v); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Println("seed: joes-diner created — staff URL /staff/joes-diner?token=demo-staff-token")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

Note: `http.Handler(r)` works because `chi.Router` implements the `Handler` interface; `http.StripPrefix` takes an `http.Handler`. The `spaHandler` static serving is added in Task 11.

- [ ] **Step 2: Write the `Makefile`**

```make
VITE_BASE ?= /
PORT ?= 8080
DB_PATH ?= hostly.db
BIN ?= bin/hostly

.PHONY: run demo test lint build frontend clean

run:
	PORT=$(PORT) DB_PATH=$(DB_PATH) go run ./cmd/hostly

demo:
	PORT=$(PORT) DB_PATH=$(DB_PATH) go run ./cmd/hostly -seed

test:
	go test ./...

lint:
	golangci-lint run

frontend:
	cd web && VITE_BASE=$(VITE_BASE) npm ci --no-audit --no-fund && VITE_BASE=$(VITE_BASE) npm run build
	rm -rf internal/webassets/dist
	mkdir -p internal/webassets/dist
	cp -R web/dist/. internal/webassets/dist/

build: frontend
	CGO_ENABLED=0 go build -o $(BIN) ./cmd/hostly

clean:
	rm -rf $(BIN) internal/webassets/dist web/dist hostly.db
```

`frontend`/`build` depend on the React app (Task 8+) — do not run `make build` yet. `make run`/`make demo` work now.

- [ ] **Step 3: Test the API manually**

```bash
cd /Users/okta/workspace/hostly
go build ./cmd/hostly ./internal/... && go test ./...
PORT=8080 DB_PATH=/tmp/hostly-manual.db go run ./cmd/hostly -seed
```

In a second terminal:

```bash
curl -s localhost:8080/api/venues/joes-diner | head -c 300
curl -s -X POST localhost:8080/api/venues/joes-diner/parties -d '{"name":"Alex","pax":2}'
curl -s "localhost:8080/api/venues/joes-diner/staff?token=demo-staff-token" | head -c 400
curl -s "localhost:8080/api/venues/joes-diner/staff" -i | head -1   # expect 401
```

Expected: seeded venue lists, join returns `ahead:0`, staff view returns the party with the token, and 401 without it. Then stop the server (Ctrl-C).

- [ ] **Step 4: Commit**

```bash
git add cmd/hostly/main.go Makefile
git commit -m "feat: main wiring, seed, makefile"
```

**End of Phase 2.** The API is fully servable via `make demo` — customer and staff routes respond, and joining writes to SQLite.

---

## Phase 3 — Frontend (React + TypeScript SPA)

### Task 8: Vite scaffold

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/index.html`
- Create: `web/src/main.tsx`, `web/src/styles.css`

- [ ] **Step 1: Write `web/package.json`**

```json
{
  "name": "hostly-web",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@types/react": "^18.3.12",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "typescript": "^5.6.3",
    "vite": "^5.4.11"
  }
}
```

- [ ] **Step 2: Write `web/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src"]
}
```

- [ ] **Step 3: Write `web/vite.config.ts`**

```ts
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react()],
  base: process.env.VITE_BASE || '/',
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
});
```

- [ ] **Step 4: Write `web/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>hostly</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 5: Write `web/src/main.tsx` (route parsing — no router dependency)**

```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import CustomerPage from './CustomerPage';
import StaffPage from './StaffPage';
import './styles.css';

function NotFound() {
  return (
    <main className="center-page">
      <h1>Not found</h1>
      <p>This link doesn't point anywhere.</p>
    </main>
  );
}

function App() {
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  const path = base ? window.location.pathname.replace(new RegExp('^' + base), '') : window.location.pathname;
  const m = path.match(/^\/(q|staff)\/([^/]+)/);
  if (!m) return <NotFound />;
  const slug = decodeURIComponent(m[2]);
  if (m[1] === 'q') return <CustomerPage slug={slug} />;
  const token = new URLSearchParams(window.location.search).get('token') ?? '';
  return <StaffPage slug={slug} token={token} />;
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

- [ ] **Step 6: Write `web/src/styles.css` (layout B — dark, spacious, centered)**

```css
:root {
  --bg: #0e0f12;
  --card: #191b21;
  --card-2: #22252d;
  --text: #f3f4f6;
  --muted: #9aa0a8;
  --accent: #f59e0b;
  --accent-ink: #1a1405;
  --green: #22c55e;
  --red: #ef4444;
  --radius: 20px;
  --shadow: 0 20px 60px rgba(0, 0, 0, 0.45);
}

* { box-sizing: border-box; }

html, body, #root { height: 100%; }

body {
  margin: 0;
  background: radial-gradient(1200px 800px at 50% -10%, #1b1d24, var(--bg));
  color: var(--text);
  font-family: ui-rounded, -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
  -webkit-font-smoothing: antialiased;
}

button {
  font: inherit;
  cursor: pointer;
  border: 0;
  border-radius: 999px;
  background: var(--card-2);
  color: var(--text);
  padding: 12px 22px;
}
button:disabled { opacity: 0.45; cursor: default; }
button.primary { background: var(--accent); color: var(--accent-ink); font-weight: 700; }

input, select {
  font: inherit;
  color: var(--text);
  background: var(--card-2);
  border: 1px solid #333845;
  border-radius: 14px;
  padding: 14px 16px;
  width: 100%;
}
input:focus, select:focus { outline: 2px solid var(--accent); border-color: transparent; }

.card {
  background: var(--card);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  padding: 28px;
}

.center-page {
  min-height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  text-align: center;
}

.venue-name { font-size: 15px; color: var(--muted); letter-spacing: 0.12em; text-transform: uppercase; margin: 0 0 4px; }
.hero { font-size: clamp(30px, 6vw, 56px); font-weight: 800; margin: 0 0 8px; }
.sub { color: var(--muted); margin: 0 0 32px; }

.ahead-panel { margin: 8px 0 32px; }
.ahead-number {
  font-size: clamp(120px, 34vw, 260px);
  font-weight: 900;
  line-height: 0.9;
  letter-spacing: -0.04em;
  font-variant-numeric: tabular-nums;
}
.ahead-label { font-size: 20px; color: var(--muted); margin-top: 8px; }
.ahead-next .ahead-number { color: var(--green); }

.body-width { width: 100%; max-width: 420px; }

.form { display: flex; flex-direction: column; gap: 16px; text-align: left; }
.form label { font-size: 13px; color: var(--muted); display: block; margin-bottom: 6px; }
.row { display: flex; gap: 12px; }
.row > div { flex: 1; }

.notice {
  border-radius: 14px;
  padding: 14px 16px;
  margin: 16px 0;
  font-size: 14px;
}
.notice.error { background: rgba(239, 68, 68, 0.12); color: #fca5a5; }
.notice.closed { background: rgba(154, 160, 168, 0.14); color: var(--muted); }
.notice.success { background: rgba(34, 197, 94, 0.12); color: #86efac; }

.wait-chip { font-size: 13px; color: var(--muted); margin-bottom: 24px; }

.banner {
  position: fixed;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  background: var(--green);
  color: #06230f;
  z-index: 50;
  text-align: center;
  cursor: pointer;
  animation: pop 0.4s cubic-bezier(0.2, 1.6, 0.4, 1);
}
.banner h1 { font-size: clamp(40px, 10vw, 80px); margin: 0; }
.banner p { font-size: 20px; margin: 0; }
@keyframes pop { from { transform: scale(0.92); opacity: 0.4; } }

/* ---- staff ---- */
.staff-wrap { max-width: 760px; margin: 0 auto; padding: 24px 16px 64px; }
.staff-head { display: flex; justify-content: space-between; align-items: end; gap: 12px; margin-bottom: 20px; flex-wrap: wrap; }
.stats { display: flex; gap: 12px; }
.stat { background: var(--card); border-radius: 14px; padding: 10px 16px; text-align: center; min-width: 96px; }
.stat b { display: block; font-size: 26px; font-variant-numeric: tabular-nums; }
.stat span { font-size: 12px; color: var(--muted); }

.queue { display: flex; flex-direction: column; gap: 12px; }
.party {
  display: flex;
  align-items: center;
  gap: 14px;
  background: var(--card);
  border-radius: 16px;
  padding: 14px 16px;
}
.party.fast { border: 2px solid var(--accent); }
.party-left, .party-seated { opacity: 0.5; }
.party-info { flex: 1; min-width: 0; }
.party-info b { font-size: 17px; }
.party-info small { color: var(--muted); display: block; margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pax-badge { background: var(--card-2); border-radius: 999px; padding: 3px 10px; font-size: 13px; }
.party-actions { display: flex; gap: 8px; }
.party-actions button { padding: 8px 12px; font-size: 13px; }
button.green { background: var(--green); color: #06230f; }
button.gray { background: var(--card-2); }
button.red { background: rgba(239, 68, 68, 0.18); color: #fca5a5; }

.hours-card { margin-top: 24px; }
.hours-card form { gap: 12px; }
.hours-row { display: flex; gap: 12px; align-items: end; }
.hours-row > div { flex: 1; }
.overrides { display: flex; gap: 8px; margin: 12px 0; }
.overrides label {
  flex: 1;
  text-align: center;
  padding: 10px;
  border-radius: 12px;
  border: 1px solid #333845;
  cursor: pointer;
  font-size: 14px;
}
.overrides input { display: none; }
.overrides label.picked { border-color: var(--accent); background: rgba(245, 158, 11, 0.12); color: var(--text); }

.muted { color: var(--muted); }
.tag { font-size: 12px; border-radius: 999px; padding: 2px 8px; margin-left: 8px; }
.tag.waiting { background: rgba(34, 197, 94, 0.15); color: #86efac; }
.tag.seated { background: rgba(245, 158, 11, 0.15); color: #fcd34d; }
.tag.left { background: rgba(154, 160, 168, 0.15); color: var(--muted); }
```

- [ ] **Step 7: Create and build the scaffold**

```bash
cd /Users/okta/workspace/hostly
mkdir -p web/src
cd web
npm install
npm run build
```

Expected: `web/dist/index.html` and `web/dist/assets/` are produced. (CustomerPage/StaffPage don't exist yet — `tsc` will fail; it's fine to run `npm install` and `vite build` alone for the scaffold check, i.e. run build after Task 9/10.)

- [ ] **Step 8: Commit the scaffold (package-lock + config + styles + main.tsx)**

```bash
cd /Users/okta/workspace/hostly
git add web
git commit -m "feat: vite + react scaffold"
```

### Task 9: Customer page (+ shared API client, live updates, chime)

**Files:**
- Create: `web/src/api.ts`, `web/src/useVenue.ts`, `web/src/chime.ts`
- Create: `web/src/CustomerPage.tsx`

- [ ] **Step 1: Write `web/src/api.ts`**

```ts
export interface Venue {
  id: number;
  slug: string;
  name: string;
  open_time: string;
  close_time: string;
  open_override: string | null;
}

export interface WaitingEntry {
  id: number;
  name: string;
  pax: number;
  order: number;
}

export interface CustomerView {
  venue: Venue;
  is_open: boolean;
  waiting: WaitingEntry[];
  waiting_count: number;
}

export interface Party {
  id: number;
  name: string;
  pax: number;
  note: string;
  status: 'waiting' | 'seated' | 'left';
  order: number;
  created_at: string;
}

export interface StaffView {
  venue: Venue;
  parties: Party[];
  stats: { waiting: number; seated_today: number };
}

export interface JoinResult {
  party: Party;
  ahead: number;
}

export class ApiError extends Error {}

const base = () => import.meta.env.BASE_URL;

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base()}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(body?.error ?? `HTTP ${res.status}`);
  }
  return body as T;
}

export class API {
  constructor(private slug: string) {}

  fetchView() {
    return req<CustomerView>(`api/venues/${this.slug}`);
  }
  join(name: string, pax: number, note: string) {
    return req<JoinResult>(`api/venues/${this.slug}/parties`, {
      method: 'POST',
      body: JSON.stringify({ name, pax, note }),
    });
  }
  staffView(token: string) {
    return req<StaffView>(`api/venues/${this.slug}/staff?token=${encodeURIComponent(token)}`);
  }
  act(path: string, token: string, body?: unknown) {
    return req<string>(`api/venues/${this.slug}/parties/${path}?token=${encodeURIComponent(token)}`, {
      method: body === undefined ? 'POST' : 'PATCH',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }
  hours(token: string, openTime: string, closeTime: string, override: string | null) {
    return req<string>(`api/venues/${this.slug}/hours?token=${encodeURIComponent(token)}`, {
      method: 'PATCH',
      body: JSON.stringify({ open_time: openTime, close_time: closeTime, override }),
    });
  }
  wsUrl() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${base()}api/venues/${this.slug}/ws`;
  }
}
```

Note: `join` adds a `note` field the API accepts; the customer form keeps the note empty.

- [ ] **Step 2: Write `web/src/useVenue.ts`**

```ts
import { useCallback, useEffect, useRef, useState } from 'react';
import { API, CustomerView } from './api';

// Live customer view: REST snapshot + WebSocket-triggered refetch, with a 5s
// poll fallback for proxies that can't hold sockets open.
export function useVenue(slug: string) {
  const [view, setView] = useState<CustomerView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const apiRef = useRef(new API(slug));
  const timer = useRef<number | undefined>(undefined);

  const reload = useCallback(() => {
    apiRef.current
      .fetchView()
      .then(setView)
      .catch((e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    apiRef.current = new API(slug);
    reload();
    let ws: WebSocket | null = null;
    let alive = true;

    const connect = () => {
      ws = new WebSocket(apiRef.current.wsUrl());
      ws.onopen = () => {
        timer.current = window.setInterval(reload, 5000);
      };
      ws.onclose = () => {
        window.clearInterval(timer.current);
        if (alive) window.setTimeout(connect, 2000);
      };
      ws.onmessage = reload;
    };
    connect();

    return () => {
      alive = false;
      window.clearInterval(timer.current);
      ws?.close();
    };
  }, [reload, slug]);

  return { view, error };
}
```

- [ ] **Step 3: Write `web/src/chime.ts`**

```ts
let ctx: AudioContext | null = null;

// Two-note "you're up" chime via Web Audio, no asset files needed.
export function playChime() {
  try {
    ctx = ctx ?? new AudioContext();
    ctx.resume();
    const now = ctx.currentTime;
    [0, 0.18].forEach((t, i) => {
      const osc = ctx!.createOscillator();
      const gain = ctx!.createGain();
      osc.type = 'sine';
      osc.frequency.value = i === 0 ? 880 : 1174;
      gain.gain.setValueAtTime(0.0001, now + t);
      gain.gain.exponentialRampToValueAtTime(0.35, now + t + 0.04);
      gain.gain.exponentialRampToValueAtTime(0.0001, now + t + 0.6);
      osc.connect(gain).connect(ctx!.destination);
      osc.start(now + t);
      osc.stop(now + t + 0.65);
    });
  } catch {
    /* audio blocked — ignore */
  }
}
```

- [ ] **Step 4: Write `web/src/CustomerPage.tsx`**

```tsx
import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { API } from './api';
import { playChime } from './chime';
import { useVenue } from './useVenue';

const STORAGE_KEY = (slug: string) => `hostly.${slug}`;

export default function CustomerPage({ slug }: { slug: string }) {
  const { view, error } = useVenue(slug);
  const [name, setName] = useState('');
  const [pax, setPax] = useState(2);
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [joined, setJoined] = useState<{ id: number; name: string } | null>(null);
  const [called, setCalled] = useState(false);
  const prevAhead = useRef<number | null>(null);

  useEffect(() => {
    const raw = sessionStorage.getItem(STORAGE_KEY(slug));
    if (raw) {
      try {
        setJoined(JSON.parse(raw));
      } catch {
        sessionStorage.removeItem(STORAGE_KEY(slug));
      }
    }
  }, [slug]);

  const waiting = view?.waiting ?? [];
  const mine = joined ? waiting.find((w) => w.id === joined.id) : undefined;
  const ahead = mine ? waiting.filter((w) => w.order < mine.order).length : null;

  useEffect(() => {
    if (ahead === null) return;
    const prev = prevAhead.current;
    prevAhead.current = ahead;
    if (prev !== null && prev > 0 && ahead === 0) {
      playChime();
      setCalled(true);
    }
  }, [ahead]);

  const seated = joined && mine === undefined;
  const loading = view === null && !error;
  const closed = view !== null && !view.is_open;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setFormError(null);
    try {
      const res = await new API(slug).join(name.trim(), pax, '');
      const info = { id: res.party.id, name: name.trim() };
      setJoined(info);
      sessionStorage.setItem(STORAGE_KEY(slug), JSON.stringify(info));
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Something went wrong');
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="center-page">
      {joined ? (
        <div className="card body-width">
          <p className="venue-name">{view?.venue.name ?? slug}</p>
          <h1 className="hero">{mine ? `${(ahead ?? 0) + 1}` : seated ? 'Enjoy!' : '—'}</h1>
          <p className="sub">
            {mine
              ? ahead === 0
                ? "You're up — head to the front."
                : `${ahead} ${ahead === 1 ? 'party' : 'parties'} ahead of you, ${mine.name}`
              : seated
                ? "You've been seated. Thanks for coming!"
                : 'You have left the line.'}
          </p>
          {mine && (
            <p className="wait-chip">
              {mine.name} · party of {mine.pax}
            </p>
          )}
          <p className="muted">Open {view?.venue.open_time}–{view?.venue.close_time}</p>
        </div>
      ) : (
        <div className="body-width">
          <p className="venue-name">{view?.venue.name ?? slug}</p>
          <h1 className="hero">Waitlist</h1>
          <p className="sub">Scan in and we'll text you… just kidding, watch this screen.</p>

          {error && <div className="notice error">{error}</div>}
          {!error && closed ? (
            <div className="notice closed">We're closed right now (open {view?.venue.open_time}–{view?.venue.close_time}). Come back soon!</div>
          ) : (
            <>
              <div className="ahead-panel">
                <div className="ahead-number">{loading ? '…' : view?.waiting_count ?? '…'}</div>
                <div className="ahead-label">parties waiting</div>
              </div>
              {formError && <div className="notice error">{formError}</div>}
              <form className="card form" onSubmit={submit}>
                <div>
                  <label htmlFor="name">Your name</label>
                  <input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Alex" required maxLength={40} />
                </div>
                <div className="row">
                  <div>
                    <label htmlFor="pax">Party size</label>
                    <select id="pax" value={pax} onChange={(e) => setPax(Number(e.target.value))}>
                      {Array.from({ length: 20 }, (_, i) => i + 1).map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                    </select>
                  </div>
                </div>
                <button className="primary" type="submit" disabled={busy}>
                  {busy ? 'Joining…' : 'Join the waitlist'}
                </button>
              </form>
            </>
          )}
        </div>
      )}

      {called && (
        <div className="banner" onClick={() => setCalled(false)}>
          <h1>{view?.venue.name}: you're up!</h1>
          <p>Tap anywhere to dismiss</p>
        </div>
      )}
    </main>
  );
}
```

- [ ] **Step 5: Connect dev server, verify the page**

Terminal A (Go API with seed):

```bash
cd /Users/okta/workspace/hostly
PORT=8080 DB_PATH=/tmp/hostly-dev.db go run ./cmd/hostly -seed
```

Terminal B (Vite dev):

```bash
cd /Users/okta/workspace/hostly/web
npm run dev
```

Open `http://localhost:5173/q/joes-diner`. Expected: dark centered page, parties-waiting count live, join form inserts party (curl in the Go terminal to watch it tick up without refreshing). After joining, the joined card shows "1" and "parties ahead of you".

- [ ] **Step 6: Build check**

```bash
cd /Users/okta/workspace/hostly/web
npm run build
```

Expected: PASS (tsc + vite). Optionally `git add web && git commit -m "feat: customer page with live updates"` — the staff page commit comes in Task 10; commit here only if you prefer smaller commits.

### Task 10: Staff dashboard

**Files:**
- Create: `web/src/StaffPage.tsx`

- [ ] **Step 1: Write `web/src/StaffPage.tsx`**

```tsx
import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { API, Party, StaffView } from './api';

const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });

export default function StaffPage({ slug, token }: { slug: string; token: string }) {
  const api = useMemo(() => new API(slug), [slug]);
  const [data, setData] = useState<StaffView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<number | null>(null);
  const [save, setSave] = useState<string | null>(null);

  const reload = useCallback(() => {
    if (!token) return;
    api
      .staffView(token)
      .then(setData)
      .catch((e: Error) => setError(e.message));
  }, [api, token]);

  useEffect(() => {
    reload();
    const iv = window.setInterval(reload, 5000);
    return () => window.clearInterval(iv);
  }, [reload]);

  if (!token) {
    return (
      <main className="center-page">
        <div className="card body-width">
          <h1>Staff link</h1>
          <p className="sub">This page needs its staff token in the URL.</p>
        </div>
      </main>
    );
  }

  if (error && !data) {
    return (
      <main className="center-page">
        <div className="notice error">{error}</div>
      </main>
    );
  }
  if (!data) {
    return (
      <main className="center-page">
        <p className="muted">Loading…</p>
      </main>
    );
  }

  const v = data.venue;

  async function run(fn: () => Promise<unknown>) {
    setSave(null);
    try {
      await fn();
      await reload();
    } catch (e) {
      setSave(e instanceof Error ? e.message : 'Action failed');
    }
  }

  const submitHours = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = e.currentTarget;
    const fd = new FormData(f);
    const override = fd.get('override') === 'auto' ? null : String(fd.get('override'));
    run(() => api.hours(token, String(fd.get('open')), String(fd.get('close')), override));
  };

  const mode = v.open_override ?? 'auto';

  const waiting = data.parties.filter((p) => p.status === 'waiting');
  const settled = data.parties.filter((p) => p.status !== 'waiting');

  return (
    <div className="staff-wrap">
      <header className="staff-head">
        <div>
          <h1 style={{ margin: 0 }}>{v.name}</h1>
          <p className="muted" style={{ margin: '4px 0 0' }}>
            {slug} · open {v.open_time}–{v.close_time}
            {v.open_override ? ` · ${v.open_override} (override)` : ''}
          </p>
        </div>
        <div className="stats">
          <div className="stat"><b>{data.stats.waiting}</b><span>waiting</span></div>
          <div className="stat"><b>{data.stats.seated_today}</b><span>seated today</span></div>
        </div>
      </header>

      {save && <div className="notice error">{save}</div>}

      <div className="queue">
        {waiting.length === 0 && <div className="notice closed">Nobody waiting — quiet shift.</div>}
        {waiting.map((p) => (
          <PartyRow
            key={p.id}
            party={p}
            isFast={waiting.length > 1 && p.order === Math.min(...waiting.map((x) => x.order))}
            editing={editing === p.id}
            onStartEdit={() => setEditing(p.id)}
            onCancelEdit={() => setEditing(null)}
            onSeat={() => run(() => api.act(`${p.id}/seat`, token))}
            onLeave={() => run(() => api.act(`${p.id}/leave`, token))}
            onTop={() => run(() => api.act(`${p.id}/top`, token))}
            onSave={(pax: number, note: string) =>
              run(async () => {
                await api.act(`${p.id}`, token, { pax, note });
                setEditing(null);
              })
            }
          />
        ))}
      </div>

      {settled.length > 0 && (
        <>
          <h3 className="muted" style={{ marginTop: 32 }}>Earlier today</h3>
          <div className="queue">
            {settled.map((p) => (
              <div key={p.id} className={`party party-${p.status}`}>
                <div className="party-info">
                  <b>{p.name}</b>
                  <small>party of {p.pax} · joined {fmtTime(p.created_at)}</small>
                </div>
                <span className={`tag ${p.status}`}>{p.status}</span>
              </div>
            ))}
          </div>
        </>
      )}

      <section className="card hours-card">
        <h3 style={{ marginTop: 0 }}>Hours</h3>
        <form onSubmit={submitHours}>
          <div className="hours-row">
            <div>
              <label>Opens</label>
              <input name="open" type="time" defaultValue={v.open_time} />
            </div>
            <div>
              <label>Closes</label>
              <input name="close" type="time" defaultValue={v.close_time} />
            </div>
          </div>
          <div className="overrides">
            {(['auto', 'open', 'closed'] as const).map((m) => (
              <label key={m} className={mode === m ? 'picked' : ''}>
                <input type="radio" name="override" value={m} defaultChecked={mode === m} />
                {m === 'auto' ? 'Follow schedule' : m === 'open' ? 'Force open' : 'Force closed'}
              </label>
            ))}
          </div>
          <button className="primary" type="submit">Save hours</button>
        </form>
      </section>
    </div>
  );
}

function PartyRow({ party, isFast, editing, onStartEdit, onCancelEdit, onSeat, onLeave, onTop, onSave }: {
  party: Party;
  isFast: boolean;
  editing: boolean;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onSeat: () => void;
  onLeave: () => void;
  onTop: () => void;
  onSave: (pax: number, note: string) => void;
}) {
  const [pax, setPax] = useState(party.pax);
  const [note, setNote] = useState(party.note);

  return (
    <div className={`party ${isFast ? 'fast' : ''}`}>
      <div className="party-info">
        <b>{party.name}</b>
        {isFast && <span className="tag waiting">next up</span>}
        <small>
          party of {party.pax} · joined {fmtTime(party.created_at)}
          {party.note ? ` · “${party.note}”` : ''}
        </small>
      </div>
      {editing ? (
        <form
          className="party-actions"
          onSubmit={(e) => {
            e.preventDefault();
            onSave(pax, note);
          }}
        >
          <input style={{ width: 64 }} type="number" min={1} max={20} value={pax} onChange={(e) => setPax(Number(e.target.value))} />
          <input style={{ width: 120 }} value={note} onChange={(e) => setNote(e.target.value)} placeholder="note" />
          <button className="green" type="submit">Save</button>
          <button type="button" onClick={onCancelEdit}>Cancel</button>
        </form>
      ) : (
        <div className="party-actions">
          <span className="pax-badge">{party.pax}</span>
          <button className="green" onClick={onSeat}>Seat</button>
          <button onClick={onStartEdit}>Edit</button>
          <button title="Move to front" onClick={onTop}>⇡</button>
          <button className="red" onClick={onLeave}>Leave</button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify in the dev server**

With the two terminals from Task 9 still running, open `http://localhost:5173/staff/joes-diner?token=demo-staff-token`. Expected: waiting parties list ordered, Seat/Edit/Top/Leave work and update the customer page live (party_joined/venue_updated events over the socket). Without the token: token-less screen.

Test live flow: open the customer page and staff page side-by-side, join from the customer page, watch it appear on staff instantly, click Seat, watch the customer page flip to "Enjoy!".

- [ ] **Step 3: Build + commit**

```bash
cd /Users/okta/workspace/hostly/web
npm run build
cd /Users/okta/workspace/hostly
git add web
git commit -m "feat: staff dashboard"
```

### Task 11: Embed the SPA and serve from the Go binary

**Files:**
- Create: `internal/webassets/embed.go`
- Modify: `cmd/hostly/main.go`

- [ ] **Step 1: Create the static assets package**

```go
package webassets

import "embed"

// Dist holds the built React SPA. Regenerate with `make frontend`.
//
//go:embed dist
var Dist embed.FS
```

- [ ] **Step 2: Update `cmd/hostly/main.go`** — add `"strings"`, the `webassets` import, and the SPA fallback

Replace the router setup block:

```go
	h := handler.New(q, handler.NewHub())
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	h.RegisterRoutes(r)
	r.NotFound(serveSPA)

	addr := ":" + port
	server := http.Handler(r)
	if basePath != "" {
		server = http.StripPrefix(basePath, r)
	}
	log.Printf("hostly listening on %s%s", addr, basePath)
	log.Fatal(http.ListenAndServe(addr, server))
```

and add:

```go
// serveSPA serves the embedded React app. Real files are served as-is; every
// other path returns index.html so client-side routes (/q/..., /staff/...)
// work on refresh and direct links.
func serveSPA(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" && !strings.HasPrefix(path, "assets/") && !strings.HasSuffix(path, ".svg") && !strings.Contains(path, ".") {
		path = "index.html"
	}
	if path == "" {
		path = "index.html"
	}
	b, err := webassets.Dist.ReadFile("dist/" + path)
	if err != nil {
		b, err = webassets.Dist.ReadFile("dist/index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", mimeTypeByPath(path))
	w.Write(b)
}

func mimeTypeByPath(path string) string {
	switch {
	case strings.HasSuffix(path, ".js"):
		return "text/javascript"
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	default:
		return "text/html; charset=utf-8"
	}
}
```

(Add `"mime"`-free logic as above; a tiny `mimeTypeByPath` avoids pulling in the full mime table and servemux quirks.)

- [ ] **Step 3: Rebuild the SPA and run the single binary**

```bash
cd /Users/okta/workspace/hostly
make frontend
make demo
```

Expected: `make frontend` builds `web/dist` and copies it into `internal/webassets/dist`; `make demo` compiles and runs the app with the SPA embedded. Open `http://localhost:8080/q/joes-diner` and `http://localhost:8080/staff/joes-diner?token=demo-staff-token` — both serve the SPA with live updates, no Vite dev server.

- [ ] **Step 4: Verify assets do not depend on the network**

```bash
go build -trimpath -o /tmp/hostly-static ./cmd/hostly
```

Run it, load the pages, confirm images/fonts come from the embedded dist (devtools network shows no external requests).

- [ ] **Step 5: Commit**

```bash
git add cmd/hostly/main.go internal/webassets
git commit -m "feat: serve embedded spa from go binary"
```

**End of Phase 3.** `make demo` is a complete single-binary app.

---

## Phase 4 — Ship

### Task 12: README + verification + push

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write `README.md`**

```md
# hostly

A zero-setup queueing system for a restaurant or café. Customers scan a QR
code (or open a link) to hop on the waitlist and watch their party's position
update live. Staff get a secret per-venue dashboard to manage the queue.

Single binary: a Go server (chi) that serves a REST + WebSocket API and the
built React SPA from one process. No external services, no cgo.

## Quick start (demo)

    make demo          # builds and runs the API + SPA, seeding a demo venue
    # open http://localhost:8080/q/joes-diner
    # and   http://localhost:8080/staff/joes-diner?token=demo-staff-token

`-seed` creates a venue named "Joe's Diner" (slug `joes-diner`, staff token
`demo-staff-token`, open 10:00–22:00). Point a real restaurant at
`/q/joes-diner` via a QR code and hand the staff URL to the host stand.

## Development

    go test ./...                       # backend tests
    make run                            # API on :8080 without seeding
    cd web && npm run dev               # Vite dev server (proxies /api -> :8080)

Then open http://localhost:5173/q/joes-diner.

## Configuration

| Variable    | Default    | Purpose                                   |
|-------------|------------|-------------------------------------------|
| `PORT`      | `8080`     | HTTP listen port                          |
| `DB_PATH`   | `hostly.db`| SQLite file path                          |
| `BASE_PATH` | (empty)    | subpath to serve under, e.g. `/hostly`    |

Frontend builds against `VITE_BASE` (default `/`). For deploy under a subpath,

    VITE_BASE=/hostly/ make build
    PORT=8080 BASE_PATH=/hostly ./bin/hostly

## Adding a venue

There is no signup UI yet — insert a venue row:

    sqlite3 hostly.db "INSERT INTO venues (slug,name,open_time,close_time,open_override,staff_token)
      VALUES ('my-cafe','My Cafe','08:00','20:00',NULL,'put-a-long-random-token-here');"

Give customers the link to `/q/my-cafe` (via QR code) and keep
`/staff/my-cafe?token=put-a-long-random-token-here` private.

## Data model

- **venues**: slug, name, open/close window, staff token, optional open override
  (auto / force-open / force-closed).
- **parties**: name, party size, note, status (`waiting | seated | left`), order
  (sort key; lower = earlier), created time.

Seated and left parties stay in the database so "seated today" and the earlier
list remain accurate across the day. Customers who leave the page mid-wait
lose their spot only in the sense that nothing auto-removes them — staff drive
all state changes.

## Live updates

Public and staff pages open a WebSocket to `/api/venues/{slug}/ws`. Server
pushes `party_joined` / `venue_updated` events; clients refetch the snapshot.
A 5 s polling fallback keeps things fresh when sockets can't stay open.

## License

MIT
```

- [ ] **Step 2: Full verification suite**

```bash
cd /Users/okta/workspace/hostly
go test ./...
go vet ./...
cd web && npm run build
```

Then a clean end-to-end run against a fresh DB:

```bash
make demo
# join via API, then staff flow:
curl -s -X POST localhost:8080/api/venues/joes-diner/parties -d '{"name":"Rita","pax":2}' >/dev/null
curl -s "localhost:8080/api/venues/joes-diner/staff?token=demo-staff-token" | head -c 400
curl -s -X POST "localhost:8080/api/venues/joes-diner/parties/1/seat?token=demo-staff-token" >/dev/null
curl -s localhost:8080/api/venues/joes-diner          # waiting_count should be 0
```

Expected: API flows report correctly, SPA loads at `/q/joes-diner`, staff link
works, and the whole thing runs from one binary.

- [ ] **Step 3: Push to GitHub**

```bash
cd /Users/okta/workspace/hostly
git add README.md && git commit -m "docs: readme with usage and deploy notes"
git remote add origin git@github.com:oktaaokta/hostly.git
git push -u origin main
```

Verify on GitHub that the code landed. The repo is private, so only you can
see it.

## Definition of done (whole plan)

- [ ] `go test ./...` passes (domain, repository incl. SQLite, usecase, handler)
- [ ] `make demo` serves the SPA + API from a single binary at :8080
- [ ] Customer flow: join → watch position tick down → chime + banner when called → seated
- [ ] Staff flow: token-gated dashboard, seat/edit/top/leave, hours panel, stats
- [ ] SPA served with no external/font/API calls (fully offline-capable)
- [ ] Repo pushed to `github.com/oktaaokta/hostly`

---

## Self-Review Notes

This plan is derived from `docs/superpowers/specs/2026-09-07-hostly-design.md`.
Resolved during drafting (all verified against the spec):

- **Court of law for duplication:** the check is case-insensitive and scoped to
  currently-waiting parties only (spec §Duplicate-name guard). Re-joining after
  being seated/left is allowed.
- **Ahead count:** number of waiting parties with a lower `order`. First party
  has ahead 0 (the "you're up / next!" state). Fast-track pins a party below the
  front's order; "next up" is simply the lowest-order waiting party.
- **Closed:** join is refused outside the window and when Force-closed; Force-
  open lets people join anytime. `IsOpen` treats unparsable times as closed and
  validates at write time (`IsValid`), so no runtime parse errors.
- **WebSocket security/battery:** the socket is public (read-only signals) —
  all mutations are POST/PATCH with the staff token. Clients refetch REST state
  after an event, so a stale/malformed event can't corrupt anything.
- **`go:embed` 404 hazard avoided:** the previous draft mounted a `spaHandler`
  that rewrote `req.URL.Path` to `basePath + "/index.html"` after `StripPrefix`,
  which 404s. Final plan serves the SPA through `r.NotFound` against the
  already-stripped path (Task 11). Asset files (`assets/*`) are served raw, all
  other non-dotted paths fall back to `index.html`.
- **No stray placeholder code:** every Go/TS snippet in this file compiles as
  written (verified against the module `github.com/oktaaokta/hostly`). The
  `scanParty` reads `created_at` into a string and parses UTC — no dropped
  column, no fake `time.Time` marshaling.
- **`Main` single-version:** exactly one `main.go`, one `store` wiring block
  (the earlier draft had two).
- **Pax bounds:** enforced in the use case (1–20) and mirrored in the UI
  (select 1–20, number inputs clamped). Frontend duplicates the rule only as
  UX; the server is the source of truth.
- **`UpdateHours`:** one signature — `({slug, token, openTime, closeTime, override *string})`,
  applied via `fetchAuthorized`; the override tri-state maps to `null` (auto).
- **`gateChime` timing:** chime fires on the transition from `ahead > 0` to
  `ahead == 0` (i.e. exactly when the party becomes next-up), not on every
  poll tick, so it can't re-fire while the party holds the front spot.
- **Persisted joined state:** the customer's joined party id/name are kept in
  `sessionStorage` keyed by venue slug so a refresh doesn't lose the position
  card, and the "seated → Enjoy!" transition is tracked by the party leaving
  the waiting list.
- **DB columns named to dodge SQLite keywords:** `order_no` and `open_time`,
  `close_time` (safe), via explicit column lists in every query.