# hostly

A zero-setup queueing system for a restaurant or café. Customers scan a QR
code (or open a link) to join the waitlist and watch their position update
live. Staff manage the queue from a secret per-venue dashboard.

Single binary: a Go server (chi) that serves a REST + WebSocket API and the
built React SPA from one process. No external services, no cgo.

## How it works

### For customers

1. **Scan the QR code** (or open the link the staff gave you). You land on a
   dark, focused page that shows the venue name, whether it's open, and how
   many parties are currently waiting.

2. **Enter your name and party size**, then tap **Join the waitlist**. That's
   it — you're in.

3. **Watch the number tick down.** Your position (the count of parties ahead)
   updates live over a WebSocket. No pull-to-refresh, no page reloads.

4. **You're up!** When your turn comes, a two-note chime plays and a
   full-screen green banner announces your name. Tap anywhere to dismiss it
   and head to the front.

5. **Seated.** The staff marks you as seated, and your page confirms it.

If you close the tab and come back, your position is still there for the
rest of the session.

### For staff

1. **Open the staff dashboard** using a secret URL with a token in the query
   string — something like:

   ```
   /staff/joes-diner?token=demo-staff-token
   ```

   No login required. Keep this URL private.

2. **See everything at a glance:** waiting parties listed in order, party
   sizes, join times, and a stats bar (waiting / seated today).

3. **Manage the queue:**
   - **Seat** — moves a waiting party to "seated" status
   - **Edit** — change party size or add a note
   - **Fast-track** — move a party to the front of the queue
   - **Leave** — remove a party (no-show, changed plans, etc.)

4. **Adjust hours** — override the schedule on the fly:
   - **Follow schedule** — open/close at the configured times
   - **Force open** — let people join outside hours (late night, special event)
   - **Force closed** — stop new joins during the open window (full house)

Changes push to the customer page instantly via WebSocket.

## Quick start

```bash
make demo              # builds and runs the app, seeding a demo venue
```

Open:
- **Customer:** http://localhost:8080/q/joes-diner
- **Staff:** http://localhost:8080/staff/joes-diner?token=demo-staff-token

`-seed` creates "Joe's Diner" (slug `joes-diner`, staff token `demo-staff-token`,
open 10:00–22:00).

## Development

```bash
go test ./...              # run all tests
make run                   # API on :8080 without seeding
cd web && npm run dev      # Vite dev server (proxies /api -> :8080)
```

Open http://localhost:5173/q/joes-diner — the Vite dev server hot-reloads
the React app while the Go API runs on :8080.

## Configuration

| Variable    | Default      | Purpose                                  |
|-------------|-------------|------------------------------------------|
| `PORT`      | `8080`      | HTTP listen port                         |
| `DB_PATH`   | `hostly.db` | SQLite file path                         |
| `BASE_PATH` | (empty)     | Subpath to serve under, e.g. `/hostly`   |

Frontend builds against `VITE_BASE` (default `/`). For deploy under a subpath:

```bash
VITE_BASE=/hostly/ make build
PORT=8080 BASE_PATH=/hostly ./bin/hostly
```

## Adding a venue

There is no signup UI yet — insert a venue row directly:

```bash
sqlite3 hostly.db \
  "INSERT INTO venues (slug,name,open_time,close_time,open_override,staff_token)
   VALUES ('my-cafe','My Cafe','08:00','20:00',NULL,'put-a-long-random-token-here')"
```

Give customers the link to `/q/my-cafe` (via QR code) and keep
`/staff/my-cafe?token=put-a-long-random-token-here` private.

## Data model

- **venues** — slug, name, open/close window, staff token, optional open
  override (auto / force-open / force-closed)
- **parties** — name, party size, note, status (`waiting | seated | left`),
  order (sort key; lower = earlier), created time

Seated and left parties stay in the database so the "seated today" count and
the earlier list remain accurate across the day.

## Live updates

Both the customer and staff pages open a WebSocket to
`/api/venues/{slug}/ws`. The server pushes `party_joined` /
`venue_updated` events; clients refetch the snapshot over REST. A 5-second
polling fallback keeps things fresh when sockets can't stay open.

## Tech stack

- **Go 1.26** — chi router, gorilla/websocket
- **React 18 + TypeScript + Vite** — SPA, no router dependency
- **SQLite** (modernc.org/sqlite) — pure-Go, no cgo
- **Web Audio** — chime when the customer is called
- **Single binary** — `go:embed` serves the built SPA from the Go process

## License

MIT
