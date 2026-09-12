# Shiro Cafe + Contact + Daily QR Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebrand the demo venue to Shiro Cafe, collect email/phone (at least one) at join, log a placeholder notification when staff seats a party, and gate the queue behind a token-gated daily QR shown on the staff dashboard.

**Architecture:** Extends the existing single-binary Go app (chi + SQLite + `go:embed` React SPA). Daily key = `sha256(venue.DailySecret + ":" + date)[:16]`; join requires today's key; seat-time notify logs via a swappable `notify.Logger`; QR PNG rendered server-side with `github.com/skip2/go-qrcode` (the only new dependency, pure Go). The "day" is the server's local calendar date.

**Tech Stack:** Go 1.26, chi v5, gorilla/websocket, modernc.org/sqlite, skip2/go-qrcode, React 18 + Vite + TS.

**Style rule (user directive):** No gratuitous comments. Only doc comments needed for exported identifiers and the bare minimum orientation. No step-by-step narration in code.

**Working tree:** Run inside the isolated worktree /Users/okta/workspace/hostly/.worktrees/daily-qr (branch `daily-qr`, created from `main` at `02c9242`).

---

### Task 1: Domain layer — venue daily secret, party contact fields, validation, ErrStale

**Files:**
- Create: `internal/domain/daily.go`
- Create: `internal/domain/contact.go`
- Create: `internal/domain/daily_test.go`
- Create: `internal/domain/contact_test.go`
- Modify: `internal/domain/errors.go`
- Modify: `internal/domain/venue.go`
- Modify: `internal/domain/party.go`

- [ ] **Step 1: Write the failing tests**

`internal/domain/daily_test.go`:
```go
package domain

import "testing"

func TestDailyKeyDeterministic(t *testing.T) {
	a := DailyKey("secret", "2026-09-12")
	if a != DailyKey("secret", "2026-09-12") {
		t.Fatal("daily key must be deterministic")
	}
	if a == DailyKey("secret", "2026-09-13") {
		t.Error("key must differ across dates")
	}
	if a == DailyKey("other", "2026-09-12") {
		t.Error("key must differ across secrets")
	}
	if len(a) != 16 {
		t.Errorf("key len = %d, want 16", len(a))
	}
}

func TestGenerateSecret(t *testing.T) {
	a, b := GenerateSecret(), GenerateSecret()
	if a == "" || a == b {
		t.Fatalf("secrets must be unique and non-empty: %q", a)
	}
	if len(a) != 64 {
		t.Errorf("secret len = %d, want 64", len(a))
	}
}
```

`internal/domain/contact_test.go`:
```go
package domain

import "testing"

func TestValidateContact(t *testing.T) {
	cases := []struct {
		email, phone string
		wantErr      bool
	}{
		{"rita@x.com", "+62812345678", false},
		{"rita@x.com", "", false},
		{"", "+62812345678", false},
		{"", "", true},
		{"not-an-email", "", true},
		{"a b@x.com", "", true},
		{"", "notaphone", true},
		{"", "12", true},
		{"", "+62 812-3456 7890", false},
		{"", "+6281234567890123456", true},
	}
	for _, c := range cases {
		err := ValidateContact(c.email, c.phone)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateContact(%q, %q) err = %v, wantErr %v", c.email, c.phone, err, c.wantErr)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/`
Expected: FAIL (`undefined: DailyKey`, `undefined: GenerateSecret`, `undefined: ValidateContact`)

- [ ] **Step 3: Implement**

`internal/domain/daily.go`:
```go
package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

func DailyKey(secret, date string) string {
	sum := sha256.Sum256([]byte(secret + ":" + date))
	return hex.EncodeToString(sum[:])[:16]
}

func GenerateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
```

`internal/domain/contact.go`:
```go
package domain

import "strings"

func ValidateContact(email, phone string) error {
	if email == "" && phone == "" {
		return ErrInvalid
	}
	if email != "" && (strings.ContainsAny(email, " \t\r\n") || !strings.Contains(email, "@")) {
		return ErrInvalid
	}
	if phone != "" && !validPhone(phone) {
		return ErrInvalid
	}
	return nil
}

func validPhone(phone string) bool {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' {
			return -1
		}
		return r
	}, phone)
	if clean == "" {
		return false
	}
	if clean[0] == '+' {
		clean = clean[1:]
	}
	if len(clean) < 6 || len(clean) > 15 {
		return false
	}
	for _, c := range clean {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
```

`internal/domain/errors.go` — add:
```go
	ErrStale = errors.New("this link only works on the day it was printed")
```

`internal/domain/venue.go` — add field to `Venue`:
```go
	DailySecret string  `json:"-"`
```

`internal/domain/party.go` — add fields to `Party`:
```go
	Email      string     `json:"email"`
	Phone      string     `json:"phone"`
	NotifiedAt *time.Time `json:"notified_at"`
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/`
Expected: PASS

(If the stray `if true` placeholder in contact.go worries you, delete it — it was a plan artifact. Do not ship it.)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/
git commit -m "feat: domain daily key, contact validation, party contact fields"
```

---

### Task 2: Repositories — persist new fields, migrate existing DBs, backfill secrets

**Files:**
- Modify: `internal/repository/memory.go`
- Modify: `internal/repository/sqlite.go`
- Modify: `internal/repository/sqlite_test.go`
- Modify: `internal/repository/memory_test.go` (create if absent)

- [ ] **Step 1: Write the failing tests**

`internal/repository/memory_test.go`:
```go
package repository

import (
	"testing"

	"github.com/oktaaokta/hostly/internal/domain"
)

func TestMemoryAutoSecret(t *testing.T) {
	m := NewMemory()
	v := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := m.Venues().Create(v); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Venues().GetBySlug("a")
	if len(got.DailySecret) != 64 {
		t.Errorf("auto secret len = %d, want 64", len(got.DailySecret))
	}
}
```

`internal/repository/sqlite_test.go` — append:
```go
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
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/repository/`
Expected: FAIL (no `DailySecret`, missing columns in INSERT, etc.)

- [ ] **Step 3: Implement**

`internal/repository/memory.go` — in `memoryVenueRepo.Create`, before assigning ID:
```go
	if v.DailySecret == "" {
		v.DailySecret = domain.GenerateSecret()
	}
```

`internal/repository/sqlite.go`:

1. Schema adds columns:
```go
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
```

2. After `db.Exec(schema)`, migrate old databases:
```go
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
```
Add to sqlite.go:
```go
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
```
(sqlite.go imports add `fmt`.)

3. Venue repo:
```go
const venueCols = "id, slug, name, open_time, close_time, open_override, staff_token, daily_secret"
```
scanVenue adds `&v.DailySecret`. `Create`:
```go
func (r *sqliteVenueRepo) Create(v *domain.Venue) error {
	if v.DailySecret == "" {
		v.DailySecret = domain.GenerateSecret()
	}
	res, err := r.db.Exec(`INSERT INTO venues (slug, name, open_time, close_time, open_override, staff_token, daily_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		v.Slug, v.Name, v.OpenTime, v.CloseTime, v.OpenOverride, v.StaffToken, v.DailySecret)
	...
}
```
`Update` SQL adds `daily_secret=?` with `v.DailySecret` after `staff_token`.

4. Party repo:
```go
const partyCols = "id, venue_id, name, pax, note, status, order_no, email, phone, notified_at, created_at"
```
`scanParty`:
```go
	var p domain.Party
	var created string
	var noted sql.NullString
	if err := row.Scan(&p.ID, &p.VenueID, &p.Name, &p.Pax, &p.Note, &p.Status, &p.Order, &p.Email, &p.Phone, &noted, &created); err != nil {
		...
	}
	...
	if noted.Valid {
		t, err := time.Parse(time.RFC3339, noted.String)
		if err != nil {
			return nil, err
		}
		p.NotifiedAt = &t
	}
```
`Create`:
```go
	noted := sql.NullString{}
	if p.NotifiedAt != nil {
		noted = sql.NullString{String: p.NotifiedAt.UTC().Format(time.RFC3339), Valid: true}
	}
	res, err := r.db.Exec(`INSERT INTO parties (venue_id, name, pax, note, status, order_no, email, phone, notified_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.VenueID, p.Name, p.Pax, p.Note, string(p.Status), p.Order, p.Email, p.Phone, noted, created)
```
`Update` SQL adds email, phone, notified_at before created_at; params `p.Email, p.Phone, noted`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/repository/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/repository/
git commit -m "feat: persist party contacts and venue daily secret with migration"
```

---

### Task 3: notify package — placeholder logger seam

**Files:**
- Create: `internal/notify/notify.go`
- Create: `internal/notify/notify_test.go`

- [ ] **Step 1: Write the failing test**

`internal/notify/notify_test.go`:
```go
package notify

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/oktaaokta/hostly/internal/domain"
)

func TestSendLogsChannels(t *testing.T) {
	var buf bytes.Buffer
	Logger = log.New(&buf, "", 0)
	p := &domain.Party{ID: 5, Name: "Rita", Email: "rita@x.com", Phone: "+62812345678"}
	Send(p)
	out := buf.String()
	for _, want := range []string{"party 5", "Rita", "email rita@x.com", "whatsapp +62812345678", "(messaging implementation put here)"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q: %s", want, out)
		}
	}
}

func TestSendNoContactSilent(t *testing.T) {
	var buf bytes.Buffer
	Logger = log.New(&buf, "", 0)
	Send(&domain.Party{ID: 1, Name: "Nobody"})
	if buf.Len() != 0 {
		t.Errorf("expected no log, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/notify/`
Expected: FAIL (`undefined: Send`)

- [ ] **Step 3: Implement**

`internal/notify/notify.go`:
```go
package notify

import (
	"log"
	"strings"

	"github.com/oktaaokta/hostly/internal/domain"
)

var Logger = log.Default()

// Send logs the seat-time notification. Real email/WhatsApp senders replace
// this body later.
func Send(p *domain.Party) {
	var channels []string
	if p.Email != "" {
		channels = append(channels, "email "+p.Email)
	}
	if p.Phone != "" {
		channels = append(channels, "whatsapp "+p.Phone)
	}
	if len(channels) == 0 {
		return
	}
	Logger.Printf("notify: party %d (%s) table ready — via %s — (messaging implementation put here)",
		p.ID, p.Name, strings.Join(channels, ", "))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/notify/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/notify/
git commit -m "feat: notify package logging placeholder on seat"
```

---

### Task 4: Use case — gated join, seat notify, daily QR info, rotate

**Files:**
- Modify: `internal/usecase/queue.go`
- Modify: `internal/usecase/queue_test.go`

- [ ] **Step 1: Write/update tests (TDD — update call sites first, then new behavior tests)**

Add the new dependency for the QR render in this task:
```bash
go get github.com/skip2/go-qrcode@latest
```

`internal/usecase/queue_test.go`:

(a) `openVenue` sets a secret:
```go
func openVenue(t *testing.T, m *repository.Memory) *domain.Venue {
	t.Helper()
	v := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok", DailySecret: domain.GenerateSecret()}
	if err := m.Venues().Create(v); err != nil {
		t.Fatal(err)
	}
	return v
}
```

(b) Fix the clock: every `NewQueue` call uses a fixed `now` UTC `2026-09-07 12:00`. Add a helper:
```go
var testNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func testKey(ven *domain.Venue) string {
	return domain.DailyKey(ven.DailySecret, testNow.Format("2006-01-02"))
}
```

(c) Replace every `q.Join(...)` call with the new signature `q.Join(slug, name, pax, note, email, phone, key, date)` and add contact + key/date. New signature:
```go
func (q *Queue) Join(slug, name string, pax int, note, email, phone, key, date string) (*JoinResult, error)
```
Updated examples:
- `mustJoin` helper:
```go
func mustJoin(t *testing.T, q *Queue, ven *domain.Venue, name string, pax int) *domain.Party {
	t.Helper()
	res, err := q.Join(ven.Slug, name, pax, "", "x@x.com", "", testKey(ven), testNow.Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	return res.Party
}
```
- All bare `Join(...)` calls get `, "x@x.com", "", testKey(ven), testNow.Format("2006-01-02")` (email ensures the at-least-one-contact rule passes). For `TestJoinValidation`, use key/date too so validation errors hit early (contact validation runs AFTER name/pax so give valid contact and rely on key ordering: name/pax checks run before contact; e.g. `q.Join("joes", "  ", 2, "", "x@x.com", "", testKey(qVen), testDate)`).

(d) New behavior tests — append:
```go
func TestJoinDailyGate(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	date := testNow.Format("2006-01-02")

	if _, err := q.Join(ven.Slug, "Alex", 2, "", "a@x.com", "", testKey(ven), "2026-09-06"); err != domain.ErrStale {
		t.Errorf("yesterday date err = %v", err)
	}
	if _, err := q.Join(ven.Slug, "Alex", 2, "", "a@x.com", "", "deadbeefdeadbeef", date); err != domain.ErrStale {
		t.Errorf("tampered key err = %v", err)
	}
	if _, err := q.Join(ven.Slug, "Alex", 2, "", "a@x.com", "", testKey(ven), date); err != nil {
		t.Errorf("valid link err = %v", err)
	}
}

func TestJoinContactValidation(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	call := func(email, phone string) error {
		_, err := q.Join(ven.Slug, "Alex", 2, "", email, phone, testKey(ven), testNow.Format("2006-01-02"))
		return err
	}
	if err := call("", ""); err != domain.ErrInvalid {
		t.Errorf("no contact err = %v", err)
	}
	if err := call("bad", ""); err != domain.ErrInvalid {
		t.Errorf("bad email err = %v", err)
	}
	if err := call("", "nope"); err != domain.ErrInvalid {
		t.Errorf("bad phone err = %v", err)
	}
	if err := call("a@x.com", ""); err != nil {
		t.Errorf("email only err = %v", err)
	}
	if err := call("", "+62812345678"); err != nil {
		t.Errorf("phone only err = %v", err)
	}
}

func TestSeatNotifiesAndStamps(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })

	withContact := mustJoin(t, q, ven, "Rita", 2)
	if err := q.Seat(ven.Slug, "tok", withContact.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Parties().Get(withContact.ID)
	if got.NotifiedAt == nil || !got.NotifiedAt.Equal(testNow) {
		t.Errorf("notified_at = %v", got.NotifiedAt)
	}

	noContact := mustJoin(t, q, ven, "Bob", 3)
	noContact.Email, noContact.Phone = "", ""
	m.Parties().Update(noContact)
	if err := q.Seat(ven.Slug, "tok", noContact.ID); err != nil {
		t.Fatal(err)
	}
	got2, _ := m.Parties().Get(noContact.ID)
	if got2.NotifiedAt != nil {
		t.Errorf("no-contact notified_at = %v", got2.NotifiedAt)
	}
}

func TestRotateSecretChangesKey(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	date := testNow.Format("2006-01-02")

	if _, err := q.RotateSecret(ven.Slug, "nope"); err != domain.ErrUnauthorized {
		t.Errorf("rotate bad token err = %v", err)
	}
	info, err := q.RotateSecret(ven.Slug, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if info.Date != date || !strings.Contains(info.Link, "/q/joes?d="+date+"&k=") {
		t.Errorf("qrcode info = %+v", info)
	}
	if _, err := q.Join(ven.Slug, "Alex", 2, "", "a@x.com", "", testKey(ven), date); err != domain.ErrStale {
		t.Errorf("old key after rotate err = %v", err)
	}
}

func TestStaffViewHasQR(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	mustJoin(t, q, ven, "Alex", 2)

	view, err := q.StaffView(ven.Slug, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if view.QRCode.Date != testNow.Format("2006-01-02") {
		t.Errorf("qrcode date = %q", view.QRCode.Date)
	}
	if !strings.HasPrefix(view.QRCode.QrURL, "/api/venues/joes/qr.png?") {
		t.Errorf("qrcode url = %q", view.QRCode.QrURL)
	}
}
```
(Add `"strings"` to queue_test.go imports.)

(e) Existing `TestJSONContractSnakeCaseRedactsToken` — extend the venue-secret check:
```go
	if _, ok := venueJSON["daily_secret"]; ok {
		t.Errorf("venue leaks daily_secret: %s", cb)
	}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usecase/`
Expected: FAIL (compile errors on old `Join` signature until implemented; then behavior failures)

- [ ] **Step 3: Implement**

`internal/usecase/queue.go`:

1. Imports add `"strings"`-unnecessary? (already there) and `"github.com/skip2/go-qrcode"`. New type after `JoinResult`:
```go
type QRInfo struct {
	Date  string `json:"date"`
	Link  string `json:"link"`
	QrURL string `json:"qr_url"`
}
```

2. `StaffView` adds a field:
```go
type StaffView struct {
	Venue   *domain.Venue   `json:"venue"`
	Parties []*domain.Party `json:"parties"`
	Stats   Stats           `json:"stats"`
	QRCode  QRInfo          `json:"qrcode"`
}
```

3. `Join` new body:
```go
func (q *Queue) Join(slug, name string, pax int, note, email, phone, key, date string) (*JoinResult, error) {
	name = strings.TrimSpace(name)
	if name == "" || pax < 1 || pax > 20 {
		return nil, domain.ErrInvalid
	}
	if err := domain.ValidateContact(email, phone); err != nil {
		return nil, err
	}
	ven, err := q.ven.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	today := q.now().Format("2006-01-02")
	if date != today || key != domain.DailyKey(ven.DailySecret, today) {
		return nil, domain.ErrStale
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
		VenueID: ven.ID, Name: name, Pax: pax, Note: note, Email: email, Phone: phone,
		Status: domain.PartyWaiting, Order: order, CreatedAt: q.now(),
	}
	if err := q.par.Create(p); err != nil {
		return nil, err
	}
	return &JoinResult{Party: p, Ahead: waiting}, nil
}
```

4. `seatOrLeave` stays for `Leave`. New `Seat`:
```go
// Seat marks a waiting party seated and logs the notification placeholder.
func (q *Queue) Seat(slug, token string, partyID int64) error {
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
	p.Status = domain.PartySeated
	if p.Email != "" || p.Phone != "" {
		now := q.now()
		p.NotifiedAt = &now
	}
	if err := q.par.Update(p); err != nil {
		return err
	}
	if p.NotifiedAt != nil {
		notify.Send(p)
	}
	return nil
}
```
Imports add `"github.com/oktaaokta/hostly/internal/notify"`.

5. QR helpers + rotate:
```go
func (q *Queue) qrInfo(v *domain.Venue) QRInfo {
	date := q.now().Format("2006-01-02")
	key := domain.DailyKey(v.DailySecret, date)
	return QRInfo{
		Date:  date,
		Link:  "/q/" + v.Slug + "?d=" + date + "&k=" + key,
		QrURL: "/api/venues/" + v.Slug + "/qr.png?d=" + date + "&k=" + key,
	}
}

func (q *Queue) CurrentQR(slug, token string) (*QRInfo, error) {
	v, err := q.fetchAuthorized(slug, token)
	if err != nil {
		return nil, err
	}
	info := q.qrInfo(v)
	return &info, nil
}

// RotateSecret replaces the venue's daily secret, invalidating today's link.
func (q *Queue) RotateSecret(slug, token string) (*QRInfo, error) {
	v, err := q.fetchAuthorized(slug, token)
	if err != nil {
		return nil, err
	}
	v.DailySecret = domain.GenerateSecret()
	if err := q.ven.Update(v); err != nil {
		return nil, err
	}
	info := q.qrInfo(v)
	return &info, nil
}
```

6. `StaffView` sets the QR block (after computing `today` which already exists):
```go
	view := &StaffView{Venue: ven, Parties: []*domain.Party{}, QRCode: q.qrInfo(ven)}
```

7. QR PNG (used by the handler; validates against the same clock as Join):
```go
// QRPNG renders today's join QR as a PNG. origin is the absolute base
// (scheme + host) the printed code must point at.
func (q *Queue) QRPNG(slug, date, key, origin, basePath string) ([]byte, error) {
	v, err := q.ven.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	today := q.now().Format("2006-01-02")
	if date != today || key != domain.DailyKey(v.DailySecret, today) {
		return nil, domain.ErrInvalid
	}
	link := origin + basePath + "/q/" + slug + "?d=" + date + "&k=" + key
	return qrcode.Encode(link, qrcode.Medium, 256)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usecase/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/ go.mod go.sum
git commit -m "feat: gated join, seat notify, daily qr info and rotation"
```

---

### Task 5: Handler — join contact fields, 410 stale, QR + rotate endpoints

**Files:**
- Modify: `internal/handler/handler.go`
- Modify: `internal/handler/handler_test.go`

- [ ] **Step 1: Write/update tests**

`internal/handler/handler_test.go`:

(a) `newTestServer` — set secret, pass basePath:
```go
	ven := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok", DailySecret: domain.GenerateSecret()}
	...
	h := New(q, NewHub(), "")
```
Add helpers with the fixed clock:
```go
var hNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func joinQuery(ven *domain.Venue) string {
	return "?d=" + hNow.Format("2006-01-02") + "&k=" + domain.DailyKey(ven.DailySecret, hNow.Format("2006-01-02"))
}
```
But `newTestServer` doesn't return the venue. Change it to also stash the venue on `httptest.Server`? Simplest: return `(*httptest.Server, *domain.Venue)`:
```go
func newTestServer(t *testing.T) (*httptest.Server, *domain.Venue) {
	t.Helper()
	m := repository.NewMemory()
	ven := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok", DailySecret: domain.GenerateSecret()}
	if err := m.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	q := usecase.NewQueue(m.Venues(), m.Parties(), func() time.Time { return hNow })
	h := New(q, NewHub(), "")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts, ven
}
```
Update all existing callers: `ts := newTestServer(t)` → `ts, ven := newTestServer(t)`.

(b) Every join POST adds the daily query + a contact field:
```go
	resp := post(t, ts, "/api/venues/joes/parties"+joinQuery(ven), map[string]any{"name": "Alex", "pax": 2, "email": "alex@x.com"})
```

(c) New tests — append:
```go
func TestJoinRequiresContact(t *testing.T) {
	ts, ven := newTestServer(t)
	resp := post(t, ts, "/api/venues/joes/parties"+joinQuery(ven), map[string]any{"name": "Alex", "pax": 2})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("no-contact status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestJoinStaleLinkGone(t *testing.T) {
	ts, ven := newTestServer(t)
	path := "/api/venues/joes/parties?d=2026-09-06&k=" + domain.DailyKey(ven.DailySecret, "2026-09-06")
	resp := post(t, ts, path, map[string]any{"name": "Alex", "pax": 2, "email": "a@x.com"})
	if resp.StatusCode != http.StatusGone {
		t.Errorf("stale status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStaffViewIncludesQR(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/venues/joes/staff?token=tok")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sv struct {
		QRCode struct {
			Date string `json:"date"`
			Link string `json:"link"`
		} `json:"qrcode"`
	}
	json.NewDecoder(resp.Body).Decode(&sv)
	if sv.QRCode.Date != "2026-09-07" || sv.QRCode.Link == "" {
		t.Errorf("staff qrcode = %+v", sv.QRCode)
	}
}

func TestQRPNGEndpoint(t *testing.T) {
	ts, ven := newTestServer(t)

	bad := http.Get(ts.URL + "/api/venues/joes/qr.png?d=2026-09-07&k=badbadbadbadbad")
	resp, err := bad
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad key status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/venues/joes/qr.png" + joinQuery(ven))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("qr status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestRotateQRNeedsToken(t *testing.T) {
	ts, ven := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/venues/joes/staff/rotate-qr", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-token rotate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Post(ts.URL+"/api/venues/joes/staff/rotate-qr?token=tok", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d", resp.StatusCode)
	}
	var info struct {
		Link string `json:"link"`
	}
	json.NewDecoder(resp.Body).Decode(&info)
	if info.Link == "" {
		t.Error("rotate returned no link")
	}
}
```

(d) `TestWebSocketBroadcast` and `TestStaffSeatFlow` join posts also need `joinQuery(ven)` + email.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/handler/`
Expected: FAIL until implemented

- [ ] **Step 3: Implement**

`internal/handler/handler.go`:

1. `Handler` struct + New:
```go
type Handler struct {
	us       *usecase.Queue
	hub      *Hub
	basePath string
	upgrader websocket.Upgrader
}

func New(us *usecase.Queue, hub *Hub, basePath string) *Handler {
	return &Handler{
		us:       us,
		hub:      hub,
		basePath: basePath,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
}
```

2. `writeErr` add:
```go
	case errors.Is(err, domain.ErrStale):
		writeJSON(w, http.StatusGone, map[string]string{"error": err.Error()})
```

3. `joinRequest` + `join`:
```go
type joinRequest struct {
	Name  string `json:"name"`
	Pax   int    `json:"pax"`
	Note  string `json:"note"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var body joinRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	res, err := h.us.Join(slug, body.Name, body.Pax, body.Note, body.Email, body.Phone,
		r.URL.Query().Get("k"), r.URL.Query().Get("d"))
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
```

4. Routes:
```go
	r.Get("/api/venues/{slug}/qr.png", h.qrPNG)
	r.Post("/api/venues/{slug}/staff/rotate-qr", h.rotateQR)
```

5. New handlers + origin helper:
```go
func origin(r *http.Request) string {
	proto := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		proto = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return proto + "://" + host
}

func (h *Handler) qrPNG(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	png, err := h.us.QRPNG(slug, r.URL.Query().Get("d"), r.URL.Query().Get("k"), origin(r), h.basePath)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

func (h *Handler) rotateQR(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	info, err := h.us.RotateSecret(slug, token(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/handler/ ./internal/usecase/ ./internal/repository/ ./internal/notify/ ./internal/domain/`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handler/
git commit -m "feat: join contact and daily key via http, qr png, rotate endpoint"
```

---

### Task 6: Seed rename, wiring, docs, test-string cleanup

**Files:**
- Modify: `cmd/hostly/main.go`
- Modify: `internal/domain/venue_test.go`
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-09-07-hostly-design.md`
- Modify: `docs/superpowers/plans/2026-09-07-hostly-implementation.md`

- [ ] **Step 1: main.go wiring + seed rename**

- `seedVenue`:
```go
func seedVenue(vr domain.VenueRepository) {
	v := &domain.Venue{Slug: "shiro-cafe", Name: "Shiro Cafe", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "demo-staff-token"}
	if _, err := vr.GetBySlug(v.Slug); err == nil {
		log.Println("seed: shiro-cafe already exists")
		return
	}
	if err := vr.Create(v); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Println("seed: shiro-cafe created — no QR configured")
}
```
(No staff URL in the seed log — the daily QR panel on the staff page is the distribution point. Keep it factual and short.)
- `handler.New(q, handler.NewHub(), basePath)` (add the basePath arg).
- The `"strings"` import in main.go stays (used by serveSPA/mime). `log.Printf` at startup unchanged.

- [ ] **Step 2: Test string cleanup**

`internal/domain/venue_test.go` line ~45: change `Slug: "joes-diner", Name: "Joe's"` to `Slug: "shiro-cafe", Name: "Shiro"`.

- [ ] **Step 3: Docs**

`README.md`:
- Replace every `joes-diner` with `shiro-cafe` and "Joe's Diner" with "Shiro Cafe".
- Quick start block:
```
    make demo          # builds and runs the API + SPA, seeding a demo venue
    # open http://localhost:8080/q/shiro-cafe?d=2026-09-12&k=<today's key>
```
- After the `-seed` paragraph, replace the "hand the staff URL" sentence with a **Daily QR** paragraph:
```
Each day the venue gets a fresh join link: `sha256(venue secret + date)`,
shown as a QR on the staff dashboard. A link only works on the day it was
printed — yesterday's QR dies at midnight, so people can't pre-arm the queue
from home. Print/publish today's QR each morning, and hit "Regenerate code"
on the staff page if a link leaks mid-day.
```
- Add the join-contact fields to the customer-facing description (write it in, matching the form):
```
Customers join with their name and are asked for at least an email or phone
so staff's seat action can reach them (the notify step is currently a log
line — real email/WhatsApp sending lands later).
```
- Add a config/timezone line:
```
The daily QR's "day" is the server's local calendar date — run the binary in
the cafe's timezone.
```

`docs/superpowers/specs/2026-09-07-hostly-design.md` line 49: `joes-diner` → `shiro-cafe`.

`docs/superpowers/plans/2026-09-07-hostly-implementation.md`: replace every `joes-diner` with `shiro-cafe` and every "Joe's Diner" with "Shiro Cafe" (reader-facing strings only; leave the `cock/booth` example parties untouched).

- [ ] **Step 4: Verify**

Run: `go test ./... && go vet ./...`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/hostly internal/domain/venue_test.go README.md docs/
git commit -m "feat: seed shiro cafe, wire base path, document daily qr"
```

---

### Task 7: Frontend — join contact fields, stale-link screen, staff QR panel

**Files:**
- Modify: `web/src/api.ts`
- Modify: `web/src/CustomerPage.tsx`
- Modify: `web/src/StaffPage.tsx`
- Modify: `web/src/styles.css`

- [ ] **Step 1: api.ts**

```ts
export interface Party {
  id: number;
  name: string;
  pax: number;
  note: string;
  status: 'waiting' | 'seated' | 'left';
  order: number;
  email: string;
  phone: string;
  notified_at: string | null;
  created_at: string;
}

export interface QRInfo {
  date: string;
  link: string;
  qr_url: string;
}

export interface StaffView {
  venue: Venue;
  parties: Party[];
  stats: { waiting: number; seated_today: number };
  qrcode: QRInfo;
}
```

`API.join` gets contact + daily query:
```ts
  join(name: string, pax: number, note: string, email: string, phone: string, q: { d: string; k: string }) {
    return req<JoinResult>(
      `api/venues/${this.slug}/parties?d=${encodeURIComponent(q.d)}&k=${encodeURIComponent(q.k)}`,
      {
        method: 'POST',
        body: JSON.stringify({ name, pax, note, email, phone }),
      },
    );
  }
```

`API.rotateQR`:
```ts
  rotateQR(token: string) {
    return req<QRInfo>(`api/venues/${this.slug}/staff/rotate-qr?token=${encodeURIComponent(token)}`, {
      method: 'POST',
    });
  }
```

- [ ] **Step 2: CustomerPage.tsx**

- Read the daily query once:
```ts
  const q = useMemo(() => {
    const u = new URLSearchParams(window.location.search);
    return { d: u.get('d') ?? '', k: u.get('k') ?? '' };
  }, []);
```
- New state + bad-link screen before the `joined` branch:
```ts
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
```
```tsx
      {!q.d || !q.k ? (
        <div className="body-width">
          <p className="venue-name">{view?.venue.name ?? slug}</p>
          <h1 className="hero">Waiting list</h1>
          <div className="notice closed">Scan the cafe's QR for today to join — each day's queue opens with a fresh code.</div>
        </div>
      ) : joined ? (
        ...
      ) : (
```
(Insert the join-form contact fields into the `form`; and pass through `q`.)
- Submit guard + join call:
```ts
    if (!email.trim() && !phone.trim()) {
      setFormError('Give an email or phone so we can reach you when your table's ready.');
      return;
    }
    ...
      const res = await new API(slug).join(name.trim(), pax, '', email.trim(), phone.trim(), q);
```
- Form fields after the pax row:
```tsx
                <div>
                  <label htmlFor="email">Email (optional)</label>
                  <input id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" maxLength={120} />
                </div>
                <div>
                  <label htmlFor="phone">Phone (optional, WhatsApp)</label>
                  <input id="phone" type="tel" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+62 812 3456 7890" />
                </div>
                <p className="muted" style={{ margin: 0, fontSize: 13 }}>
                  At least an email or phone — we'll ping you when your table's ready.
                </p>
```
- stale-link import stays: the server 410 message ("this link only works on the day it was printed") surfaces in `formError` automatically.
- The `useMemo` import needs adding to the React import line.

- [ ] **Step 3: StaffPage.tsx**

- Copy helper + QR panel. Add after the header block, before the `{save && ...}` line:
```tsx
      <section className="card qr-card">
        <div className="qr-row">
          <img className="qr-img" src={data.qrcode.qr_url} alt="today's queue QR" />
          <div>
            <h3 style={{ margin: '0 0 4px' }}>Today's QR — {data.qrcode.date}</h3>
            <p className="muted" style={{ margin: 0, fontSize: 13 }}>
              Print or show this code each morning; yesterday's stops working at midnight.
            </p>
            <div className="qr-copy">
              <input readOnly value={data.qrcode.link} onClick={(e) => e.currentTarget.select()} />
              <button className="gray" onClick={() => navigator.clipboard.writeText(window.location.origin + data.qrcode.link)}>
                Copy
              </button>
            </div>
            <button className="primary" disabled={busy} onClick={() => run(() => api.rotateQR(token))}>
              Regenerate code
            </button>
          </div>
        </div>
      </section>
```
(`data.qrcode.link` is relative — prefix `window.location.origin` for a usable link.)

- Contact + notified chip in `PartyRow`:
```tsx
        {party.notified_at && <span className="tag seated">Notified {fmtTime(party.notified_at)}</span>}
```
after the `next up` tag, and enrich the `<small>`:
```tsx
          party of {party.pax} · joined {fmtTime(party.created_at)}
          {party.status === 'waiting' && (party.email || party.phone) ? ` · ${party.email || party.phone}` : ''}
          {party.note ? ` · “${party.note}”` : ''}
```
- Settled row: add the same notified chip:
```tsx
                  {p.notified_at && <span className="tag seated">Notified {fmtTime(p.notified_at)}</span>}
```

- [ ] **Step 4: styles.css**

Add near the hours-card styles:
```css
.qr-card { margin-top: 24px; }
.qr-row { display: flex; gap: 18px; align-items: center; flex-wrap: wrap; }
.qr-img { width: 190px; height: 190px; background: #fff; border-radius: 12px; padding: 10px; }
.qr-copy { display: flex; gap: 8px; margin: 12px 0; }
.qr-copy input { flex: 1; min-width: 0; font-size: 12px; padding: 10px 12px; }
```

- [ ] **Step 5: Build**

Run: `cd web && npm run build`
Expected: `tsc` clean + vite build succeeds (35+ modules, dist written)

Run: `cd /Users/okta/workspace/hostly/.worktrees/daily-qr && make frontend`
Expected: web/dist copied into internal/webassets/dist

- [ ] **Step 6: End-to-end smoke (fresh DB, single binary)**

```bash
cd /Users/okta/workspace/hostly/.worktrees/daily-qr
make build
rm -f /tmp/hostly-dailyqr.db
PORT=8080 DB_PATH=/tmp/hostly-dailyqr.db ./bin/hostly -seed &
sleep 4
today=$(date +%F)
secret=$(sqlite3 /tmp/hostly-dailyqr.db "SELECT daily_secret FROM venues WHERE slug='shiro-cafe'")
key=$(printf "%s:%s" "$secret" "$today" | shasum -a 256 | head -c 16)
curl -s "localhost:8080/api/venues/shiro-cafe" | head -c 120
curl -s -X POST "localhost:8080/api/venues/shiro-cafe/parties?d=$today&k=$key" \
  -d '{"name":"Rita","pax":2,"email":"rita@x.com"}' # expect 201 + party.email
curl -s -X POST "localhost:8080/api/venues/shiro-cafe/parties?d=$today&k=$key" \
  -d '{"name":"Rita2","pax":2}' # expect 400 (no contact)
curl -s -o /dev/null -w "%{http_code}\n" "localhost:8080/api/venues/shiro-cafe/parties?d=2000-01-01&k=$key" # expect 410
curl -s "localhost:8080/api/venues/shiro-cafe/staff?token=demo-staff-token" -o /dev/null -w "%{http_code}\n" # 200
curl -s "localhost:8080/api/venues/shiro-cafe/qr.png?d=$today&k=$key" -o /tmp/qr.png -w "%{http_code} %{content_type}\n" # 200 image/png
# open /tmp/qr.png to eyeball it, then kill the server
```
(sqlite3 CLI is macOS-built-in.)

- [ ] **Step 7: Commit**

```bash
git add web/
git commit -m "feat: contact fields in join, stale-link screen, staff qr panel"
```

---

### Task 8: Plan-sync + full verification + feature branch push

**Files:**
- Modify: `docs/superpowers/plans/2026-09-12-shiro-cafe-daily-qr-design.md` (this plan — add an Implementation Notes section at the bottom noting: QR payload absolute via origin/basePath; `notify` on seat only; migration approach; `ErrStale`→410)

- [ ] **Step 1: Sync deviations**

Append to the design doc an "Implementation notes" section listing:
- `QRPNG` validates date/key with the same injected clock as `Join` (handlers can't see `Queue.now()`).
- The QR payload is absolute (`origin + basePath`), built in `usecase` so it's unit-testable; `qrcode.link` stays relative.
- `notify.Send` is called only after a successful seat persist.
- Database continuity: `migrate()` ALTERs missing columns and backfills `daily_secret`.

- [ ] **Step 2: Full verify**

Run: `go test ./... && go vet ./... && cd web && npm run build`
Expected: ALL PASS

Run: `gofmt -l .`
Expected: clean (no files listed)

- [ ] **Step 3: Commit + push**

```bash
git add docs/
git commit -m "docs: note implementation decisions in daily-qr design"
git push -u origin daily-qr
```

---

## Self-review notes (for the plan author)

- Spec coverage: rename (T6), contact fields + gating (T1/4/5/7), notify+stamp (T3/4), staff QR/rotate/absolute payload (T4/5/7), migrations (T2), docs (T6), verify+publish (T8). All spec sections have a task.
- Type consistency: `QRInfo{date,link,qr_url}`, `QRCode` field on StaffView, `notify.Send(*domain.Party)` — consistent across tasks. `New(us, hub, basePath)` ripples to main.go (T6) and handler_test (T5).