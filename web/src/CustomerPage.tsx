import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { API } from './api';
import { playChime } from './chime';
import { useVenue } from './useVenue';

const STORAGE_KEY = (slug: string) => `hostly.${slug}`;

export default function CustomerPage({ slug }: { slug: string }) {
  const { view, error } = useVenue(slug);
  const [name, setName] = useState('');
  const [pax, setPax] = useState(2);
  const [formError, setFormError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [joined, setJoined] = useState<{ id: number; name: string } | null>(null);
  const [called, setCalled] = useState(false);
  const prevAhead = useRef<number | null>(null);

  useEffect(() => {
    const raw = sessionStorage.getItem(STORAGE_KEY(slug));
    if (raw) {
      try {
        setJoined(JSON.parse(raw));
      } catch {
        sessionStorage.removeItem(STORAGE_KEY(slug));
      }
    }
  }, [slug]);

  const waiting = view?.waiting ?? [];
  const mine = joined ? waiting.find((w) => w.id === joined.id) : undefined;
  const ahead = mine ? waiting.filter((w) => w.order < mine.order).length : null;

  useEffect(() => {
    if (ahead === null) return;
    const prev = prevAhead.current;
    prevAhead.current = ahead;
    if (prev !== null && prev > 0 && ahead === 0) {
      playChime();
      setCalled(true);
    }
  }, [ahead]);

  const seated = view !== null && joined && mine === undefined;
  const loading = view === null && !error;
  const closed = view !== null && !view.is_open;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setFormError(null);
    try {
      const res = await new API(slug).join(name.trim(), pax, '');
      const info = { id: res.party.id, name: name.trim() };
      setJoined(info);
      sessionStorage.setItem(STORAGE_KEY(slug), JSON.stringify(info));
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Something went wrong');
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="center-page">
      {joined ? (
        <div className="card body-width">
          <p className="venue-name">{view?.venue.name ?? slug}</p>
          <h1 className="hero">{mine ? `${(ahead ?? 0) + 1}` : seated ? 'Enjoy!' : '—'}</h1>
          <p className="sub">
            {mine
              ? ahead === 0
                ? "You're up — head to the front."
                : `${ahead} ${ahead === 1 ? 'party' : 'parties'} ahead of you, ${mine.name}`
              : seated
                ? "You've been seated. Thanks for coming!"
                : 'You have left the line.'}
          </p>
          {mine && (
            <p className="wait-chip">
              {mine.name} · party of {mine.pax}
            </p>
          )}
          <p className="muted">Open {view?.venue.open_time ?? '…'}–{view?.venue.close_time ?? '…'}</p>
        </div>
      ) : (
        <div className="body-width">
          <p className="venue-name">{view?.venue.name ?? slug}</p>
          <h1 className="hero">Waitlist</h1>
          <p className="sub">Scan in and we'll text you… just kidding, watch this screen.</p>

          {error && <div className="notice error">{error}</div>}
          {!error && closed ? (
            <div className="notice closed">We're closed right now (open {view?.venue.open_time}–{view?.venue.close_time}). Come back soon!</div>
          ) : (
            <>
              <div className="ahead-panel">
                <div className="ahead-number">{loading ? '…' : view?.waiting_count ?? '…'}</div>
                <div className="ahead-label">parties waiting</div>
              </div>
              {formError && <div className="notice error">{formError}</div>}
              <form className="card form" onSubmit={submit}>
                <div>
                  <label htmlFor="name">Your name</label>
                  <input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Alex" required maxLength={40} />
                </div>
                <div className="row">
                  <div>
                    <label htmlFor="pax">Party size</label>
                    <select id="pax" value={pax} onChange={(e) => setPax(Number(e.target.value))}>
                      {Array.from({ length: 20 }, (_, i) => i + 1).map((n) => (
                        <option key={n} value={n}>{n}</option>
                      ))}
                    </select>
                  </div>
                </div>
                <button className="primary" type="submit" disabled={busy}>
                  {busy ? 'Joining…' : 'Join the waitlist'}
                </button>
              </form>
            </>
          )}
        </div>
      )}

      {called && (
        <div className="banner" onClick={() => setCalled(false)}>
          <h1>{view?.venue.name}: you're up!</h1>
          <p>Tap anywhere to dismiss</p>
        </div>
      )}
    </main>
  );
}