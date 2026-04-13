'use client';

import { createContext, useContext, useState, useEffect, ReactNode, useCallback } from 'react';

const STORAGE_KEY = 'auth_token';

interface AuthContextValue {
  token: string | null;
  isConnected: boolean;
  setToken: (token: string) => void;
  disconnect: () => void;
}

const AuthContext = createContext<AuthContextValue>({
  token: null,
  isConnected: false,
  setToken: () => {},
  disconnect: () => {},
});

export function useAuth() {
  return useContext(AuthContext);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setTokenState] = useState<string | null>(null);
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setTokenState(stored);
        setIsConnected(true);
      }
    }
  }, []);

  const setToken = useCallback((newToken: string) => {
    localStorage.setItem(STORAGE_KEY, newToken);
    setTokenState(newToken);
    setIsConnected(true);
  }, []);

  const disconnect = useCallback(() => {
    localStorage.removeItem(STORAGE_KEY);
    setTokenState(null);
    setIsConnected(false);
  }, []);

  return (
    <AuthContext.Provider value={{ token, isConnected, setToken, disconnect }}>
      {children}
    </AuthContext.Provider>
  );
}
