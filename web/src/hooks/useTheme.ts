export function getLatestTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light';
  return (localStorage.getItem('theme') as 'light' | 'dark') || 'light';
}

export function setTheme(theme: 'light' | 'dark'): void {
  localStorage.setItem('theme', theme);
}
