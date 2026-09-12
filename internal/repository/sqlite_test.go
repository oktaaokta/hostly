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

func TestSQLiteUniqueConstraints(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ven := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := db.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	if err := db.Venues().Create(&domain.Venue{Slug: "a", Name: "A2", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}); err != domain.ErrInvalid {
		t.Fatalf("duplicate slug create err = %v", err)
	}

	b := &domain.Venue{Slug: "b", Name: "B", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := db.Venues().Create(b); err != nil {
		t.Fatal(err)
	}
	b.Slug = "a"
	if err := db.Venues().Update(b); err != domain.ErrInvalid {
		t.Fatalf("rename to taken slug err = %v", err)
	}

	p := &domain.Party{VenueID: ven.ID, Name: "Alex", Pax: 2, Status: domain.PartyWaiting, Order: 1}
	if err := db.Parties().Create(p); err != nil {
		t.Fatal(err)
	}
	dup := &domain.Party{VenueID: ven.ID, Name: "Bea", Pax: 3, Status: domain.PartyWaiting, Order: 1}
	if err := db.Parties().Create(dup); err != domain.ErrInvalid {
		t.Fatalf("duplicate (venue_id, order_no) err = %v", err)
	}
}

func TestSQLitePartyNotFound(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Parties().Get(999); err != domain.ErrNotFound {
		t.Fatalf("missing party get err = %v", err)
	}
	if err := db.Parties().Update(&domain.Party{ID: 999, Name: "X", Pax: 1, Status: domain.PartyWaiting, Order: 1}); err != domain.ErrNotFound {
		t.Fatalf("update missing party err = %v", err)
	}
}
