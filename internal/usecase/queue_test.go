package usecase

import (
	"encoding/json"
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

func TestJSONContractSnakeCaseRedactsToken(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })

	res, err := q.Join(ven.Slug, "Alex", 2, "")
	if err != nil {
		t.Fatal(err)
	}

	asMap := func(t *testing.T, b []byte) map[string]any {
		t.Helper()
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v\n%s", err, b)
		}
		return out
	}

	jr, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	jm := asMap(t, jr)
	if jm["ahead"] == nil || jm["party"] == nil {
		t.Errorf("JoinResult keys = %v", jm)
	}

	cv, err := q.CustomerView(ven.Slug)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := json.Marshal(cv)
	if err != nil {
		t.Fatal(err)
	}
	cm := asMap(t, cb)
	venueJSON, _ := cm["venue"].(map[string]any)
	if venueJSON["open_time"] != "10:00" {
		t.Errorf("venue open_time = %v (%s)", venueJSON["open_time"], cb)
	}
	if _, ok := venueJSON["staff_token"]; ok {
		t.Errorf("venue leaks staff_token: %s", cb)
	}
	if _, ok := venueJSON["StaffToken"]; ok {
		t.Errorf("venue leaks StaffToken: %s", cb)
	}

	sv, err := q.StaffView(ven.Slug, "tok")
	if err != nil {
		t.Fatal(err)
	}
	sb, err := json.Marshal(sv)
	if err != nil {
		t.Fatal(err)
	}
	sm := asMap(t, sb)
	if _, ok := sm["staff_token"]; ok {
		t.Errorf("staff view leaks staff_token: %s", sb)
	}
	parties, _ := sm["parties"].([]any)
	if len(parties) != 1 {
		t.Fatalf("staff parties = %v", parties)
	}
	pjson, _ := parties[0].(map[string]any)
	if pjson["created_at"] == nil || pjson["venue_id"] == nil {
		t.Errorf("party JSON not snake_case: %s", sb)
	}
}
