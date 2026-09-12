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
