'use client';

import { createContext, useContext, useEffect, useRef, useState, ReactNode, useCallback } from 'react';
import { dispatcher } from '@/lib/updateDispatcher';
import { getLatestSeq } from '@/store/indexedDB';

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

function getToken(): string {
  return localStorage.getItem('auth_token') || '';
}

export function WSProvider({ children }: { children: ReactNode }) {
  const [connected, setConnected] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelayRef = useRef(1000);

  const connect = async () => {
    const token = getToken();
    if (!token) return;

    // Close existing connection
    if (wsRef.current) {
      wsRef.current.onclose = null; // Don't trigger reconnect on manual close
      wsRef.current.close();
    }

    const lastSeq = await getLatestSeq();
    const url = `${WS_BASE}/ws?token=${encodeURIComponent(token)}&last_seq=${lastSeq}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      setReconnecting(false);
      reconnectDelayRef.current = 1000;

      // Heartbeat every 30s
      const heartbeat = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ping', payload: {} }));
        } else {
          clearInterval(heartbeat);
        }
      }, 30_000);
    };

    ws.onmessage = (event) => {
      try {
        const frame = JSON.parse(event.data);
        if (frame.type === 'updates') {
          dispatcher.applyUpdates(frame.payload);
        }
      } catch {
        // Ignore parse errors
      }
    };

    ws.onclose = () => {
      setConnected(false);
      setReconnecting(true);

      // Exponential backoff
      const delay = reconnectDelayRef.current;
      reconnectTimerRef.current = setTimeout(() => {
        reconnectDelayRef.current = Math.min(delay * 2, 30_000);
        connect();
      }, delay);
    };

    ws.onerror = () => {
      ws.close();
    };
  };

  const reconnect = useCallback(() => {
    reconnectDelayRef.current = 1000;
    connect();
  }, []);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      wsRef.current?.close();
    };
  }, []);

  return (
    <WSContext.Provider value={{ connected, reconnecting, reconnect }}>
      {children}
    </WSContext.Provider>
  );
}
