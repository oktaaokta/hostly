package repository

import (
	"database/sql"
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

func TestSQLitePartyContactColumns(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ven := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := db.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	if len(ven.DailySecret) != 64 {
		t.Fatalf("auto secret len = %d, want 64", len(ven.DailySecret))
	}

	now := time.Now().UTC()
	p := &domain.Party{VenueID: ven.ID, Name: "Alex", Pax: 2, Status: domain.PartyWaiting, Order: 1,
		Email: "alex@x.com", Phone: "+62812345678", NotifiedAt: &now}
	if err := db.Parties().Create(p); err != nil {
		t.Fatal(err)
	}
	list, err := db.Parties().ListByVenue(ven.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByVenue = %v, %v", list, err)
	}
	if list[0].Email != "alex@x.com" || list[0].Phone != "+62812345678" {
		t.Errorf("contact round trip = %+v", list[0])
	}
	if list[0].NotifiedAt == nil || !list[0].NotifiedAt.Equal(now) {
		t.Errorf("notified_at round trip = %v", list[0].NotifiedAt)
	}

	p.NotifiedAt = nil
	p.Email = ""
	if err := db.Parties().Update(p); err != nil {
		t.Fatal(err)
	}
	list, _ = db.Parties().ListByVenue(ven.ID)
	if list[0].NotifiedAt != nil || list[0].Email != "" {
		t.Errorf("update cleared contact = %+v", list[0])
	}
}

func TestSQLiteMigrationAddsColumnsAndBackfills(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old := `CREATE TABLE venues (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		slug TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		open_time TEXT NOT NULL,
		close_time TEXT NOT NULL,
		open_override TEXT,
		staff_token TEXT NOT NULL
	);
	CREATE TABLE parties (
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
	INSERT INTO venues (slug, name, open_time, close_time, open_override, staff_token)
		VALUES ('a', 'A', '10:00', '22:00', NULL, 't');`
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(old); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ven, err := db.Venues().GetBySlug("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(ven.DailySecret) != 64 {
		t.Fatalf("backfilled secret len = %d, want 64", len(ven.DailySecret))
	}

	p := &domain.Party{VenueID: ven.ID, Name: "Rita", Pax: 2, Status: domain.PartyWaiting, Order: 1, Email: "r@x.com"}
	if err := db.Parties().Create(p); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Parties().Get(p.ID)
	if got.Email != "r@x.com" {
		t.Errorf("party insert after migration = %+v", got)
	}
	secret := ven.DailySecret
	db.Close()

	db2, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	ven2, err := db2.Venues().GetBySlug("a")
	if err != nil {
		t.Fatal(err)
	}
	if ven2.DailySecret != secret {
		t.Fatalf("reopen regenerated secret: %q != %q", ven2.DailySecret, secret)
	}

	p2 := &domain.Party{VenueID: ven.ID, Name: "Sam", Pax: 1, Status: domain.PartyWaiting, Order: 2, Email: "s@x.com"}
	if err := db2.Parties().Create(p2); err != nil {
		t.Fatal(err)
	}
	got2, _ := db2.Parties().Get(p2.ID)
	if got2.Email != "s@x.com" {
		t.Errorf("party insert after reopen = %+v", got2)
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
