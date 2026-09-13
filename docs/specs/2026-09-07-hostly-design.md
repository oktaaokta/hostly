# hostly — Restaurant Queueing System

## Overview

A web-based queueing system for restaurants. Customers scan a QR code pointing at a per-venue page, enter their name and party size, and claim a spot in the waitlist. The page shows how many parties are ahead, the venue's opening hours, and flips to a "your table is ready" banner with a chime when they're called. Restaurant staff use a per-venue dashboard to run the queue: seat parties, mark them left, edit entries, fast-track, and adjust opening hours.

Multiple venues are supported. Each venue has its own queue, its own hours, and its own staff URL. Customer-facing pages need no login.

## Technology Stack

- **Backend:** Go (chi router), single binary
- **Frontend:** React + TypeScript + Vite (single SPA)
- **Realtime:** WebSocket for live updates, HTTP polling as fallback
- **Storage:** SQLite (single file, no server)
- **Deployment:** One Go binary serving REST + WebSocket + the embedded React build, mounted at a subpath behind the user's reverse proxy

## Project Structure

```
hostly/
├── cmd/hostly/main.go          # entrypoint, config, DI
├── internal/
│   ├── domain/                 # entities + repository interfaces
│   ├── usecase/                # queue / venue business logic
│   ├── handler/                # HTTP + WebSocket handlers (chi)
│   └── repository/             # SQLite implementation
├── web/                        # React + TypeScript + Vite SPA
├── docs/
├── Makefile
├── go.mod
└── .gitignore
```

## Architecture & Data Flow

```
React SPA (customer page / staff dashboard)
   │  REST (join, staff actions) + WebSocket (live updates)
   ▼
Go API (chi) ──► SQLite
```

One process serves everything. The built React app is embedded via `go:embed` and served from the URL prefix assigned by the reverse proxy.

### Domain Model

**Venue**
- `id` (int)
- `slug` (string, from the QR URL, e.g. `joes-diner`)
- `name` (string)
- `open_time` / `close_time` (single daily pair, `HH:MM`)
- `open_override` (nullable: `open` | `closed` | null-for-auto). Override wins over the schedule.
- `staff_token` (secret appended to the staff URL)

**Party**
- `id` (int)
- `venue_id`
- `name` (string)
- `pax` (int)
- `status` (`waiting` | `seated` | `left`)
- `note` (string, optional)
- `order` (int, monotonic sort key unique within the venue; lower = earlier in line)
- `created_at`

### Position & Fast-track

- Queue order = `order` ascending among `waiting` parties. New parties get the next `order` value for the venue.
- `parties ahead` for a party = count of `waiting` parties with a lower `order`.
- Fast-track (move to top) = set the party's `order` to (min `order` among `waiting` − 1); all other parties keep their relative order.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/venues/{slug}` | Venue info: name, hours, open/closed now, waiting count, parties waiting (names+pax only) |
| POST | `/api/venues/{slug}/parties` | Join the waitlist (name, pax, optional note) |
| GET | `/api/venues/{slug}/staff?token=…` | Staff data: full party list with statuses, contact info, hours, override |
| PATCH | `/api/venues/{slug}/parties/{id}` | Edit pax / note (staff) |
| POST | `/api/venues/{slug}/parties/{id}/seat` | Mark seated (staff) |
| POST | `/api/venues/{slug}/parties/{id}/leave` | Mark left / removed (staff) |
| POST | `/api/venues/{slug}/parties/{id}/top` | Fast-track to front (staff) |
| PATCH | `/api/venues/{slug}/hours` | Set open/close time and override (staff) |
| WS   | `/api/venues/{slug}/ws` | Live events: party joined, seated, left, edited, reordered, hours changed |

All staff endpoints require the venue's `staff_token` (query param or `Authorization` header). Customer endpoints require nothing.

## Client UI (single React SPA)

Routes:
- `/q/{slug}` — customer page (dark theme)
- `/staff/{slug}?token=…` — staff dashboard (light theme)

### Customer page — three states

1. **Join** — venue name, "Open · 10am–10pm" pill, a large "parties ahead of you" panel showing the current queue count, name field, party-size field, "Claim waitlist" pill button. When closed, the form is disabled and shows "Opens at 10am" / "Closed today".
2. **Waiting** — the count panel becomes your live number: "4 parties ahead of you · Alex, party of 2 — you're #5". Updates pushed over WebSocket.
3. **Being called** — full-screen banner "Alex, party of 2 — your table is ready!" plus a chime. Dismissable.

### Staff dashboard (light theme)

- Queue list: each waiting party shows name, pax, note, time waiting, position. Row actions: Seat, Mark left/removed, Edit pax/note, Move to top.
- Stats bar: parties waiting now, seated so far today.
- Hours panel: edit open/close time; override switch (Auto / Force Open / Force Closed).

## Rules & Validation

- Joining blocked outside [open_time, close_time) unless the venue override is `open`. Return the next opening context to display.
- Name required (trimmed, ≤ 80 chars). Pax integer 1–20.
- Duplicate guard: a `waiting` party with the same normalized name from the same venue is rejected to stop accidental double-taps.
- Seat/leave/top on a non-`waiting` party returns 409.

## Error Handling & Resilience

- WebSocket events carry a monotonically increasing sequence; on reconnect the client refetches state via REST, so a dropped connection never loses a spot.
- If the WebSocket fails entirely, the customer page falls back to a 5-second HTTP poll of `/api/venues/{slug}`.
- JSON error responses: `{"error": "..."}` with appropriate status codes (400/404/409/500).

## Testing

- Unit tests: position math, fast-track reorder, open/close determination (schedule + override), duplicate guard.
- Handler tests: join → seat → leave lifecycle over an in-memory repository.
- Frontend: kept thin; no component test suite in v1 (the core logic lives in Go).

## Deployment

- `make build` produces one binary with the embedded React build.
- Run the binary; it serves REST + WebSocket + static SPA on one port.
- Reverse proxy mounts it at the chosen subpath; SQLite file lives next to the binary (volume-mounted).

## Deliberate Cuts (ponytail)

- **SMS notify (v2):** deferred. Same WebSocket event can later trigger a Twilio/SNS hook.
- **Customer "cancel my spot":** staff handles removals; the page is a thin surface.
- **Per-day-of-week hours:** single daily pair + override for now; a richer schedule fits behind the same check.
- **Auth system:** staff pages use unguessable `token` URLs, no login.