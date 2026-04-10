const API_BASE = process.env.NEXT_PUBLIC_API_BASE || 'http://localhost:8080';

function getToken(): string | null {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem('auth_token');
}

function headers(extra?: Record<string, string>): Record<string, string> {
  const h: Record<string, string> = {
    'Content-Type': 'application/json',
    ...extra,
  };
  const token = getToken();
  if (token) {
    h['Authorization'] = `Bearer ${token}`;
  }
  return h;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}/api/v1${path}`, {
    ...init,
    headers: headers(init?.headers as Record<string, string>),
  });

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const message = body?.error?.message || `HTTP ${res.status}`;
    throw new Error(message);
  }

  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'POST',
      body: body ? JSON.stringify(body) : undefined,
    }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'PUT',
      body: body ? JSON.stringify(body) : undefined,
    }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, {
      method: 'PATCH',
      body: body ? JSON.stringify(body) : undefined,
    }),
  delete: (path: string) => request<void>(path, { method: 'DELETE' }),
};

import type { components } from '@/types/api';

// Re-export generated types with convenient aliases
export type TemplateInfo = components['schemas']['Template'];
export type Project = components['schemas']['Project'];
export type Conversation = components['schemas']['Conversation'];
export type Message = components['schemas']['Message'];
export type Settings = components['schemas']['Settings'];
export type User = components['schemas']['User'];
