'use client';

import { createContext, useContext, useEffect, useRef, useState, ReactNode, useCallback } from 'react';
import { dispatcher } from '@/lib/updateDispatcher';
import { getLatestSeq, setLatestSeq } from '@/store/indexedDB';
import { useAuth } from './AuthProvider';

interface WSContextValue {
  connected: boolean;
  reconnecting: boolean;
  reconnect: () => void;
}

const WSContext = createContext<WSContextValue>({ connected: false, reconnecting: false, reconnect: () => {} });

export function useWS() {
  return useContext(WSContext);
}

const WS_BASE = process.env.NEXT_PUBLIC_WS_BASE || 'ws://localhost:8080';
const API_BASE = process.env.NEXT_PUBLIC_API_BASE || 'http://localhost:8080';

export function WSProvider({ children }: { children: ReactNode }) {
  const { token } = useAuth();
  const tokenRef = useRef(token);
  tokenRef.current = token;
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelayRef = useRef(1000);
  const heartbeatRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const pollBackoffRef = useRef(1000);
  const isPullingRef = useRef(false);

  // HTTP pull for gap recovery — fetches all updates since localMaxSeq
  const pullMissingUpdates = useCallback(async (localMaxSeq: number) => {
    if (isPullingRef.current) return;
    isPullingRef.current = true;
    const currentToken = tokenRef.current;
    if (!currentToken) { isPullingRef.current = false; return; }
    try {
      const res = await fetch(
        `${API_BASE}/api/v1/users/me/updates?last_seq=${localMaxSeq}`,
        { headers: { Authorization: `Bearer ${currentToken}` } }
      );
      if (res.ok) {
        const data = await res.json() as { updates: unknown[]; max_seq?: number };
        if (data.max_seq !== undefined) {
          await setLatestSeq(data.max_seq);
        }
        if (data.updates?.length > 0) {
          dispatcher.applyUpdates(data.updates as never);
        }
      }
    } catch (err) {
      console.error('[WSProvider] gap recovery pull failed', err);
    } finally {
      isPullingRef.current = false;
    }
  }, []);

  // Register gap detection callback on mount
  useEffect(() => {
    dispatcher.setGapCallback((localMaxSeq) => pullMissingUpdates(localMaxSeq));
  }, [pullMissingUpdates]);

  const clearHeartbeat = useCallback(() => {
    if (heartbeatRef.current) {
      clearInterval(heartbeatRef.current);
      heartbeatRef.current = null;
    }
  }, []);

  const clearPolling = useCallback(() => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    pollBackoffRef.current = 5000;
  }, []);

  const startPolling = useCallback(() => {
    clearPolling();
    const poll = async () => {
      const currentToken = tokenRef.current;
      if (!currentToken) return;
      try {
        const lastSeq = await getLatestSeq();
        const res = await fetch(
          `${API_BASE}/api/v1/users/me/updates?last_seq=${lastSeq}`,
          { headers: { Authorization: `Bearer ${currentToken}` } }
        );
        if (res.ok) {
          const data = await res.json();
          if (data.max_seq !== undefined) {
            await setLatestSeq(data.max_seq);
          }
          if (data.updates?.length > 0) {
            dispatcher.applyUpdates(data.updates);
          }
          pollBackoffRef.current = 5000;
        } else {
          pollBackoffRef.current = Math.min(pollBackoffRef.current * 2, 30_000);
        }
      } catch {
        pollBackoffRef.current = Math.min(pollBackoffRef.current * 2, 30_000);
      }
      // Schedule next poll with current backoff value (recursive setTimeout)
      pollTimerRef.current = setTimeout(poll, pollBackoffRef.current);
    };
    pollTimerRef.current = setTimeout(poll, pollBackoffRef.current);
  }, [clearPolling]);

  const startHeartbeat = useCallback((ws: WebSocket) => {
    clearHeartbeat();
    heartbeatRef.current = setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping', payload: {} }));
      } else {
        clearHeartbeat();
      }
    }, 30_000);
  }, [clearHeartbeat]);

  const connect = useCallback(async (authToken: string) => {
    if (!authToken) return;

    // Close existing connection
    if (wsRef.current) {
      wsRef.current.onclose = null;
      wsRef.current.close();
    }
    clearHeartbeat();

    const lastSeq = await getLatestSeq();
    const url = `${WS_BASE}/ws?token=${encodeURIComponent(authToken)}&last_seq=${lastSeq}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setReconnecting(false);
      reconnectDelayRef.current = 1000;
      startHeartbeat(ws);
      clearPolling(); // Stop HTTP polling when WS is connected
    };

    ws.onmessage = (event) => {
      try {
        const frame = JSON.parse(event.data);
        if (frame.type === 'updates') {
          dispatcher.applyUpdates(frame.payload);
        } else if (frame.type === 'connected') {
          // Parse max_seq for gap detection
          const maxSeq = frame.payload?.max_seq as number | undefined;
          if (maxSeq !== undefined) {
            dispatcher.setMaxServerSeq(maxSeq);
          }
        }
      } catch {
        // Ignore parse errors
      }
    };

    ws.onclose = () => {
      setConnected(false);
      setReconnecting(true);
      clearHeartbeat();
      startPolling(); // Start HTTP polling as fallback

      // Exponential backoff
      const delay = reconnectDelayRef.current;
      const currentToken = tokenRef.current;
      reconnectTimerRef.current = setTimeout(() => {
        reconnectDelayRef.current = Math.min(delay * 2, 30_000);
        connect(currentToken || '');
      }, delay);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [startHeartbeat, clearHeartbeat, clearPolling, startPolling]);

  const reconnect = useCallback(() => {
    reconnectDelayRef.current = 1000;
    connect(token || '');
  }, [connect, token]);

  // Connect when token changes
  useEffect(() => {
    if (token) {
      reconnectDelayRef.current = 1000;
      connect(token);
    }
    return () => {
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      clearHeartbeat();
      clearPolling();
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close();
      }
    };
  }, [token, clearHeartbeat, clearPolling, connect]);

  return (
    <WSContext.Provider value={{ connected, reconnecting, reconnect }}>
      {children}
    </WSContext.Provider>
  );
}
