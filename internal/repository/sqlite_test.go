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
