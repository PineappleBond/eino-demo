import { useState, useCallback } from 'react';

export function getLatestTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light';
  return (localStorage.getItem('theme') as 'light' | 'dark') || 'light';
}

export function setTheme(theme: 'light' | 'dark'): void {
  localStorage.setItem('theme', theme);
}

export function useTheme() {
  const [theme, setThemeState] = useState<'light' | 'dark'>(getLatestTheme);
  const changeTheme = useCallback((next: 'light' | 'dark') => {
    setTheme(next);
    setThemeState(next);
  }, []);
  return { theme, setTheme: changeTheme };
}
