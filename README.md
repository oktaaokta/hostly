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