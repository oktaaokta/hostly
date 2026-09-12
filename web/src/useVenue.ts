import { useCallback, useEffect, useRef, useState } from 'react';
import { API, CustomerView } from './api';

// Live customer view: REST snapshot + WebSocket-triggered refetch, with a 5s
// poll fallback for proxies that can't hold sockets open.
export function useVenue(slug: string) {
  const [view, setView] = useState<CustomerView | null>(null);
  const [error, setError] = useState<string | null>(null);
  const apiRef = useRef(new API(slug));
  const timer = useRef<number | undefined>(undefined);
  const reconnectTimer = useRef<number | undefined>(undefined);

  const reload = useCallback(() => {
    apiRef.current
      .fetchView()
      .then((v) => {
        setView(v);
        setError(null);
      })
      .catch((e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    apiRef.current = new API(slug);
    reload();
    timer.current = window.setInterval(reload, 5000);
    let ws: WebSocket | null = null;
    let alive = true;

    const connect = () => {
      ws = new WebSocket(apiRef.current.wsUrl());
      ws.onclose = () => {
        if (alive) reconnectTimer.current = window.setTimeout(connect, 2000);
      };
      ws.onmessage = reload;
    };
    connect();

    return () => {
      alive = false;
      window.clearInterval(timer.current);
      window.clearTimeout(reconnectTimer.current);
      ws?.close();
    };
  }, [reload, slug]);

  return { view, error };
}