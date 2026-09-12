# Shiro Cafe: contact capture, notify placeholder, dynamic daily QR

Created: 2026-09-12

## Context

_hostly_ is a demo single-binary queueing app (Go + React SPA via `go:embed`,
SQLite) completed over eleven tasks. A real neighborhood cafe is using the
pattern: customers scan a static QR, join the queue from home in the morning,
and the queue is swarmed the instant it opens. This iteration makes hostly's
own demo product useful for that cafe:

1. Rebrand the seeded demo venue **Joe's Diner (`joes-diner`)** to
   **Shiro Cafe (`shiro-cafe`)**.
2. Collect **email and/or phone** at join so customers can be contacted when
   their table is ready.
3. Emit a **placeholder notification** (logged, not truly sent) when staff
   seats a party — the seam where real email/WhatsApp senders go later.
4. Replace the static QR with a **token-gated daily QR**: the printed link
   only works on its own day, so yesterday's QR dies and people can't pre-arm
   the queue from home.

## Goals

- Rename demo venue everywhere (`seed`, README, docs, tests).
- Join form collects optional `email` + `phone`; **at least one required**.
- Light format validation for both.
- Staff "Seat" action logs a placeholder notify line per provided channel and
  stamps the party `notified_at`.
- Staff dashboard shows each waiting party's contact and a `Notified HH:MM`
  chip once stamped.
- Staff dashboard shows **today's** QR with copyable link and a "Regenerate"
  button.
- Join is gated by date + daily key; stale/foreign links get a clear error.
- Keep it a single binary with no external services; SQLite migrations are
  idempotent so existing `hostly.db` files keep working.
- Full TDD; every behavior above covered by tests, then a fresh-DB E2E.

## Non-goals

- Actually sending email or WhatsApp messages (placeholder log only).
- Notifying on events other than seat (no "added to queue" / "about to be
  seated" notifications).
- Staff ability to pick a different printed QR date.
- Per-venue timezone configuration (the "day" is the server's local calendar
  date; documented for deployment).
- Restricting queue *viewing* on the public page (join is gated, viewing is
  open — the count stays visible without a key).

## Data model changes

### `domain.Venue`

| Field | Type | JSON | Notes |
|---|---|---|---|
| `DailySecret` | `string` | `"-"` | 32-byte random hex; created at venue creation/seed; regenerable via rotate. Never serialized. |

### `domain.Party`

| Field | Type | JSON | Notes |
|---|---|---|---|
| `Email` | `string` | `email` | Empty when not provided. |
| `Phone` | `string` | `phone` | Empty when not provided. |
| `NotifiedAt` | `*time.Time` | `notified_at` | `null` until seat-time placeholder notify is logged. |

### New domain error

`ErrStale` — message "this link only works on the day it was printed". Maps to
HTTP **410 Gone** for the join endpoint.

## Daily key scheme

- Per-venue random `DailySecret` (hex).
- Daily key for calendar date `D`:
  `key = hex(sha256(DailySecret + ":" + D))[:16]`
- Deterministic per (venue, secret, date): stable across restarts, changes
  automatically when the date flips, no per-day storage.
- Customer queue link (app-internal form): `{base}/q/{slug}?d={D}&k={key}`
  (served under `BASE_PATH`).
- **QR encodes an ABSOLUTE URL** built at render time from the request
  `Host`/`X-Forwarded-Host` (+ `X-Forwarded-Proto`, defaulting to `https` on
  forwarded requests, else `http`) plus `BASE_PATH` plus the internal link —
  e.g. `https://shiro.example.com/q/shiro-cafe?d=2026-09-12&k=3f9a…`. A phone
  scanning the QR must land on a working web address, not a relative path.
- The `qrcode.link` field in staff view stays RELATIVE (the SPA makes it
  absolute when copying); only the QR payload is absolute.
- "Regenerate" replaces `DailySecret` with a fresh random value: the current
  day's key changes and any leaked current link dies immediately; future days
  derive from the new secret automatically.

## Join flow (use case `Queue.Join`)

New signature (kept positional, per repo style):

```
Join(slug, name string, pax int, note, email, phone, key, date string) (*JoinResult, error)
```

Validation order → first failure wins:

1. `name` / `pax` rules unchanged (`ErrInvalid`).
2. Contact: `email != "" || phone != ""`, else `ErrInvalid` ("provide an email
   or phone so we can reach you when your table's ready").
3. Email format (only if non-empty): contains `@`, no whitespace →
   `ErrInvalid`.
4. Phone format (only if non-empty): strip `[ -()]`, must match
   `^\+?[0-9]{6,15}$` → `ErrInvalid`.
5. Daily gate: `date` string must equal today (server-local `YYYY-MM-DD`)
   **and** `key` must equal the venue's derived key → `ErrStale`. Checked
   before open-hours; a stale link on a closed day reports stale (clearer).
6. Venue open check unchanged (`ErrClosed`).
7. Duplicate check unchanged (`ErrDuplicate`).

`JoinResult` unchanged in shape; the returned party carries `email`, `phone`,
`notified_at: null`.

## Notify on seat (use case `Queue.Seat`)

On a `waiting → seated` transition:

- If the party has `email` or `phone`, call the notify package, then set
  `NotifiedAt = now` (persisted atomically with the status change).
- `internal/notify` (new): a package-level `var Logger *log.Logger` defaulting
  to `log.Default()` (swappable in tests), and:

```
func Send(p domain.Party)
```

  builds the channel list (`email rita@x.com`, `whatsapp +6281...`) and writes
  one log line:

  `notify: party 5 (Rita) table ready — via email rita@x.com, whatsapp +6281 — (messaging implementation put here)`

  This function is the documented seam for real senders later.

Staff view JSON for each party includes `email`, `phone`, `notified_at`.

## Staff view additions

`GET /api/venues/{slug}/staff?token=...` response gains a top-level object:

```
"qrcode": {
  "date": "2026-09-12",
  "link": "/q/shiro-cafe?d=2026-09-12&k=3f9a...",
  "qr_url": "/api/venues/shiro-cafe/qr.png?d=2026-09-12&k=3f9a..."
}
```

## New/changed HTTP endpoints

| Method + Path | Auth | Behavior |
|---|---|---|
| `POST /api/venues/{slug}/parties?d=&k=` | none (key in query) | Join. Body `{name,pax,note,email,phone}`. `ErrInvalid`→400, `ErrStale`→410, `ErrClosed`→409, `ErrDuplicate`→409, ok→201 + party. |
| `GET /api/venues/{slug}/qr.png?d=&k=` | none (key in query) | Validates `d`/`k` for the venue; returns `image/png` of that day's QR (payload = the link). Invalid→400. |
| `POST /api/venues/{slug}/staff/rotate-qr` | staff token | Replaces `DailySecret`; returns the fresh `qrcode` object. 401 without/with-bad token, 404 unknown venue. |

`qr.png` is public by design: possession of a valid `d`+`k` is the ticket, and
the printed QR is the point of distribution.

## QR rendering

New dependency `github.com/skip2/go-qrcode` (pure Go, no cgo) — the only new
dependency. Server renders the PNG so the key can't be fabricated client-side
and printing works from any device. QR encodes the full customer link; a
mid-size (e.g. 256px) fixed output is fine for print and screen.

## Frontend (React SPA, Vite)

### Customer page

- Reads `d` and `k` from `location.search`. If either is missing → reuse the
  existing "token-less"/bad-link screen with a message to scan the current
  QR. Appends `?d=&k=` to all API calls.
- On a 410 response: show the stale-link message with the text "this link
  only works on the day it was printed — scan the cafe's QR for today".
- Join form: add "Email" (`type=email`) and "Phone" (`type=tel`) inputs,
  both optional; client-side enforce "at least one" with an inline message
  mirroring the server error.

### Staff page

- New top panel: today's date, the generated QR image (`qr_url`), the full
  customer link as copyable text, and a "Regenerate code" button
  (busy-guarded like the existing actions) that calls `rotate-qr` and
  re-renders the panel.
- Waiting/seated party cards: show `email` / `phone` when present; once
  `notified_at` is set show a `Notified HH:MM` chip (existing chip styling).

## SQLite (migrations + repo)

OpenSQLite, after table creation, runs an idempotent column check:

```
ensureColumn(db, table, column, ddl)   // PRAGMA table_info → ALTER TABLE ADD COLUMN if missing
```

- `parties`: add `email TEXT NOT NULL DEFAULT ''`,
  `phone TEXT NOT NULL DEFAULT ''`, `notified_at TEXT`.
- `venues`: add `daily_secret TEXT NOT NULL DEFAULT ''`; then backfill any
  venue with an empty secret with a fresh one (so legacy rows get a working
  daily QR).

Repos gain: `email`/`phone`/`notified_at` on create+update; `DailySecret` on
venue load/create; new `RotateDailySecret(venueID)` persist.

## Testing matrix

- **domain**: daily key is deterministic, differs across dates, differs when
  secret changes.
- **usecase**: join ok with today's key; `ErrStale` for yesterday's date,
  tampered key, tomorrow's date; `ErrInvalid` for missing both contacts, bad
  email, bad phone; seat with contact sets `NotifiedAt` and the notify log
  line is captured (buffer logger) listing both channels; seat without
  contact logs nothing / leaves `NotifiedAt` nil.
- **repository**: round-trip new columns; migration upgrades a constructed
  old-schema DB (joins work after); rotate persists and changes the derived
  key.
- **handler**: `qr.png` 200 image/png for valid d+k, 400 for invalid; join
  410 for stale; rotate-qr 401 without token, 200 with, returned key changed
  and old key now stale.
- **frontend**: `npm run build` clean; manual E2E (fresh DB): customer join
  via QR link, staff sees contact + notified chip after seat, QR panel
  renders, regenerate breaks the old link.
- **E2E** (fresh DB, single binary): join → seat → waiting_count 0, notify
  log line present in stdout, `shiro-cafe` seed shown, `joes-diner` gone.

## Deployment notes

- The "day" is the server's **local calendar date**. For a café, run the
  binary on a host set to the venue's timezone (README documents
  `TZ=Asia/Jakarta`-style at deploy time).
- Join requires `d`+`k`; QRs must be printed daily so yesterday's link dies
  naturally. The Regenerate button handles mid-day leaks.

## Docs updates

- README: `shiro-cafe` links, daily-QR explanation, join fields
  (email/phone, one required), timezone note, `qr.png`/rotate endpoints.
- Implementation plan doc: sync replaced `joes-diner` strings and new
  behavior blocks for future reference.

## Out of scope for this iteration

- Real SMTP / WhatsApp sending.
- Multi-day tickets / reservations.
- Timezone-per-venue config.
- Restricting public queue viewing without a key.