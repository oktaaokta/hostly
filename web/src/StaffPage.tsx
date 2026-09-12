import { useCallback, useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { API, Party, StaffView } from './api';

const fmtTime = (iso: string) =>
  new Date(iso).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });

export default function StaffPage({ slug, token }: { slug: string; token: string }) {
  const api = useMemo(() => new API(slug), [slug]);
  const [data, setData] = useState<StaffView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<number | null>(null);
  const [save, setSave] = useState<string | null>(null);

  const reload = useCallback(() => {
    if (!token) return;
    api
      .staffView(token)
      .then(setData)
      .catch((e: Error) => setError(e.message));
  }, [api, token]);

  useEffect(() => {
    reload();
    const iv = window.setInterval(reload, 5000);
    return () => window.clearInterval(iv);
  }, [reload]);

  if (!token) {
    return (
      <main className="center-page">
        <div className="card body-width">
          <h1>Staff link</h1>
          <p className="sub">This page needs its staff token in the URL.</p>
        </div>
      </main>
    );
  }

  if (error && !data) {
    return (
      <main className="center-page">
        <div className="notice error">{error}</div>
      </main>
    );
  }
  if (!data) {
    return (
      <main className="center-page">
        <p className="muted">Loading…</p>
      </main>
    );
  }

  const v = data.venue;

  async function run(fn: () => Promise<unknown>) {
    setSave(null);
    try {
      await fn();
      await reload();
    } catch (e) {
      setSave(e instanceof Error ? e.message : 'Action failed');
    }
  }

  const submitHours = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = e.currentTarget;
    const fd = new FormData(f);
    const override = fd.get('override') === 'auto' ? null : String(fd.get('override'));
    run(() => api.hours(token, String(fd.get('open')), String(fd.get('close')), override));
  };

  const mode = v.open_override ?? 'auto';

  const waiting = data.parties.filter((p) => p.status === 'waiting');
  const settled = data.parties.filter((p) => p.status !== 'waiting');

  return (
    <div className="staff-wrap">
      <header className="staff-head">
        <div>
          <h1 style={{ margin: 0 }}>{v.name}</h1>
          <p className="muted" style={{ margin: '4px 0 0' }}>
            {slug} · open {v.open_time}–{v.close_time}
            {v.open_override ? ` · ${v.open_override} (override)` : ''}
          </p>
        </div>
        <div className="stats">
          <div className="stat"><b>{data.stats.waiting}</b><span>waiting</span></div>
          <div className="stat"><b>{data.stats.seated_today}</b><span>seated today</span></div>
        </div>
      </header>

      {save && <div className="notice error">{save}</div>}

      <div className="queue">
        {waiting.length === 0 && <div className="notice closed">Nobody waiting — quiet shift.</div>}
        {waiting.map((p) => (
          <PartyRow
            key={p.id}
            party={p}
            isFast={waiting.length > 1 && p.order === Math.min(...waiting.map((x) => x.order))}
            editing={editing === p.id}
            onStartEdit={() => setEditing(p.id)}
            onCancelEdit={() => setEditing(null)}
            onSeat={() => run(() => api.act(`${p.id}/seat`, token))}
            onLeave={() => run(() => api.act(`${p.id}/leave`, token))}
            onTop={() => run(() => api.act(`${p.id}/top`, token))}
            onSave={(pax: number, note: string) =>
              run(async () => {
                await api.act(`${p.id}`, token, { pax, note });
                setEditing(null);
              })
            }
          />
        ))}
      </div>

      {settled.length > 0 && (
        <>
          <h3 className="muted" style={{ marginTop: 32 }}>Earlier today</h3>
          <div className="queue">
            {settled.map((p) => (
              <div key={p.id} className={`party party-${p.status}`}>
                <div className="party-info">
                  <b>{p.name}</b>
                  <small>party of {p.pax} · joined {fmtTime(p.created_at)}</small>
                </div>
                <span className={`tag ${p.status}`}>{p.status}</span>
              </div>
            ))}
          </div>
        </>
      )}

      <section className="card hours-card">
        <h3 style={{ marginTop: 0 }}>Hours</h3>
        <form onSubmit={submitHours}>
          <div className="hours-row">
            <div>
              <label>Opens</label>
              <input name="open" type="time" defaultValue={v.open_time} />
            </div>
            <div>
              <label>Closes</label>
              <input name="close" type="time" defaultValue={v.close_time} />
            </div>
          </div>
          <div className="overrides">
            {(['auto', 'open', 'closed'] as const).map((m) => (
              <label key={m} className={mode === m ? 'picked' : ''}>
                <input type="radio" name="override" value={m} defaultChecked={mode === m} />
                {m === 'auto' ? 'Follow schedule' : m === 'open' ? 'Force open' : 'Force closed'}
              </label>
            ))}
          </div>
          <button className="primary" type="submit">Save hours</button>
        </form>
      </section>
    </div>
  );
}

function PartyRow({ party, isFast, editing, onStartEdit, onCancelEdit, onSeat, onLeave, onTop, onSave }: {
  party: Party;
  isFast: boolean;
  editing: boolean;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onSeat: () => void;
  onLeave: () => void;
  onTop: () => void;
  onSave: (pax: number, note: string) => void;
}) {
  const [pax, setPax] = useState(party.pax);
  const [note, setNote] = useState(party.note);

  return (
    <div className={`party ${isFast ? 'fast' : ''}`}>
      <div className="party-info">
        <b>{party.name}</b>
        {isFast && <span className="tag waiting">next up</span>}
        <small>
          party of {party.pax} · joined {fmtTime(party.created_at)}
          {party.note ? ` · “${party.note}”` : ''}
        </small>
      </div>
      {editing ? (
        <form
          className="party-actions"
          onSubmit={(e) => {
            e.preventDefault();
            onSave(pax, note);
          }}
        >
          <input style={{ width: 64 }} type="number" min={1} max={20} value={pax} onChange={(e) => setPax(Number(e.target.value))} />
          <input style={{ width: 120 }} value={note} onChange={(e) => setNote(e.target.value)} placeholder="note" />
          <button className="green" type="submit">Save</button>
          <button type="button" onClick={onCancelEdit}>Cancel</button>
        </form>
      ) : (
        <div className="party-actions">
          <span className="pax-badge">{party.pax}</span>
          <button className="green" onClick={onSeat}>Seat</button>
          <button onClick={onStartEdit}>Edit</button>
          <button title="Move to front" onClick={onTop}>⇡</button>
          <button className="red" onClick={onLeave}>Leave</button>
        </div>
      )}
    </div>
  );
}