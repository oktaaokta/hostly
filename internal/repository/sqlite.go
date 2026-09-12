package repository

import (
	"database/sql"
	"fmt"
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
	staff_token TEXT NOT NULL,
	daily_secret TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS parties (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	venue_id INTEGER NOT NULL REFERENCES venues(id),
	name TEXT NOT NULL,
	pax INTEGER NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	order_no INTEGER NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	notified_at TEXT,
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
	if err := migrate(db); err != nil {
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
func (r *SQLite) Close() error                    { return r.db.Close() }

func migrate(db *sql.DB) error {
	cols := []struct{ table, name, ddl string }{
		{"venues", "daily_secret", "daily_secret TEXT NOT NULL DEFAULT ''"},
		{"parties", "email", "email TEXT NOT NULL DEFAULT ''"},
		{"parties", "phone", "phone TEXT NOT NULL DEFAULT ''"},
		{"parties", "notified_at", "notified_at TEXT"},
	}
	for _, c := range cols {
		found, err := hasColumn(db, c.table, c.name)
		if err != nil {
			return err
		}
		if !found {
			if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.ddl); err != nil {
				return err
			}
		}
	}
	rows, err := db.Query(`SELECT id, daily_secret FROM venues`)
	if err != nil {
		return err
	}
	type vsec struct {
		id, sec string
	}
	var need []vsec
	for rows.Next() {
		var id int64
		var sec string
		if err := rows.Scan(&id, &sec); err != nil {
			rows.Close()
			return err
		}
		if sec == "" {
			need = append(need, vsec{fmt.Sprint(id), domain.GenerateSecret()})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, n := range need {
		if _, err := db.Exec(`UPDATE venues SET daily_secret = ? WHERE id = ?`, n.sec, n.id); err != nil {
			return err
		}
	}
	return nil
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

const venueCols = "id, slug, name, open_time, close_time, open_override, staff_token, daily_secret"

func scanVenue(row interface{ Scan(...any) error }) (*domain.Venue, error) {
	var v domain.Venue
	if err := row.Scan(&v.ID, &v.Slug, &v.Name, &v.OpenTime, &v.CloseTime, &v.OpenOverride, &v.StaffToken, &v.DailySecret); err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &v, nil
}

type sqliteVenueRepo struct{ db *sql.DB }

func (r *sqliteVenueRepo) Create(v *domain.Venue) error {
	if v.DailySecret == "" {
		v.DailySecret = domain.GenerateSecret()
	}
	res, err := r.db.Exec(`INSERT INTO venues (slug, name, open_time, close_time, open_override, staff_token, daily_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.Slug, v.Name, v.OpenTime, v.CloseTime, v.OpenOverride, v.StaffToken, v.DailySecret)
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
	res, err := r.db.Exec(`UPDATE venues SET slug=?, name=?, open_time=?, close_time=?, open_override=?, staff_token=?, daily_secret=? WHERE id=?`,
		v.Slug, v.Name, v.OpenTime, v.CloseTime, v.OpenOverride, v.StaffToken, v.DailySecret, v.ID)
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

const partyCols = "id, venue_id, name, pax, note, status, order_no, email, phone, notified_at, created_at"

func scanParty(row interface{ Scan(...any) error }) (*domain.Party, error) {
	var p domain.Party
	var created string
	var noted sql.NullString
	if err := row.Scan(&p.ID, &p.VenueID, &p.Name, &p.Pax, &p.Note, &p.Status, &p.Order, &p.Email, &p.Phone, &noted, &created); err != nil {
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
	if noted.Valid {
		t, err := time.Parse(time.RFC3339Nano, noted.String)
		if err != nil {
			return nil, err
		}
		p.NotifiedAt = &t
	}
	return &p, nil
}

type sqlitePartyRepo struct{ db *sql.DB }

func (r *sqlitePartyRepo) Create(p *domain.Party) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	created := p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	noted := sql.NullString{}
	if p.NotifiedAt != nil {
		noted = sql.NullString{String: p.NotifiedAt.UTC().Format(time.RFC3339Nano), Valid: true}
	}
	res, err := r.db.Exec(`INSERT INTO parties (venue_id, name, pax, note, status, order_no, email, phone, notified_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.VenueID, p.Name, p.Pax, p.Note, string(p.Status), p.Order, p.Email, p.Phone, noted, created)
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
	noted := sql.NullString{}
	if p.NotifiedAt != nil {
		noted = sql.NullString{String: p.NotifiedAt.UTC().Format(time.RFC3339Nano), Valid: true}
	}
	res, err := r.db.Exec(`UPDATE parties SET name=?, pax=?, note=?, status=?, order_no=?, email=?, phone=?, notified_at=?, created_at=? WHERE id=?`,
		p.Name, p.Pax, p.Note, string(p.Status), p.Order, p.Email, p.Phone, noted, created, p.ID)
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
