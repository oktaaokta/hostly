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

func TestVenueUpdateSlug(t *testing.T) {
	m := NewMemory()
	a := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := m.Venues().Create(a); err != nil {
		t.Fatal(err)
	}
	a.Slug = "b"
	if err := m.Venues().Update(a); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Venues().GetBySlug("b"); err != nil {
		t.Fatalf("GetBySlug(b) after rename = %v", err)
	}
	if _, err := m.Venues().GetBySlug("A"); err != domain.ErrNotFound {
		t.Fatalf("GetBySlug(A) after rename = %v, want ErrNotFound", err)
	}
	dup := &domain.Venue{Slug: "b", Name: "B", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := m.Venues().Create(dup); err != domain.ErrInvalid {
		t.Fatalf("Create(duplicate slug) = %v, want ErrInvalid", err)
	}
}
