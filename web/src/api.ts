export interface Venue {
  id: number;
  slug: string;
  name: string;
  open_time: string;
  close_time: string;
  open_override: string | null;
}

export interface WaitingEntry {
  id: number;
  name: string;
  pax: number;
  order: number;
}

export interface CustomerView {
  venue: Venue;
  is_open: boolean;
  waiting: WaitingEntry[];
  waiting_count: number;
}

export interface Party {
  id: number;
  name: string;
  pax: number;
  note: string;
  status: 'waiting' | 'seated' | 'left';
  order: number;
  created_at: string;
}

export interface StaffView {
  venue: Venue;
  parties: Party[];
  stats: { waiting: number; seated_today: number };
}

export interface JoinResult {
  party: Party;
  ahead: number;
}

export interface ActionResult {
  status: string;
}

export class ApiError extends Error {}

const base = () => import.meta.env.BASE_URL;

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base()}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(body?.error ?? `HTTP ${res.status}`);
  }
  return body as T;
}

export class API {
  constructor(private slug: string) {}

  fetchView() {
    return req<CustomerView>(`api/venues/${this.slug}`);
  }
  join(name: string, pax: number, note: string) {
    return req<JoinResult>(`api/venues/${this.slug}/parties`, {
      method: 'POST',
      body: JSON.stringify({ name, pax, note }),
    });
  }
  staffView(token: string) {
    return req<StaffView>(`api/venues/${this.slug}/staff?token=${encodeURIComponent(token)}`);
  }
  act(path: string, token: string, body?: unknown) {
    return req<ActionResult>(`api/venues/${this.slug}/parties/${path}?token=${encodeURIComponent(token)}`, {
      method: body === undefined ? 'POST' : 'PATCH',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }
  hours(token: string, openTime: string, closeTime: string, override: string | null) {
    return req<ActionResult>(`api/venues/${this.slug}/hours?token=${encodeURIComponent(token)}`, {
      method: 'PATCH',
      body: JSON.stringify({ open_time: openTime, close_time: closeTime, override }),
    });
  }
  wsUrl() {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${base()}api/venues/${this.slug}/ws`;
  }
}