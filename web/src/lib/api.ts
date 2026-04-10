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

export type TemplateInfo = {
  id: string;
  name: string;
  description: string;
  tags: string[];
  difficulty: 'beginner' | 'intermediate' | 'advanced';
};

export type Project = {
  id: string;
  user_id: string;
  template_id: string;
  name: string;
  config: Record<string, unknown>;
  created_at: string;
};

export type Conversation = {
  id: string;
  project_id: string;
  user_id: string;
  title: string;
  summary: string;
  status: string;
  last_preview: string;
  message_count: number;
  latest_seq: number;
  member_count: number;
  token_prompt: number;
  token_completion: number;
  created_at: string;
  updated_at: string;
};

export type Message = {
  id: string;
  conversation_id: string;
  seq: number;
  sender_role: 'user' | 'assistant' | 'system' | 'tool';
  sender_id: string;
  content: string;
  reason_content: string;
  metadata: Record<string, unknown>;
  finish_reason: string | null;
  error_message: string | null;
  duration_ms: number | null;
  token_prompt: number;
  token_completion: number;
  created_at: string;
  reply_to_seq?: number;
  mentioned_members?: string[];
};

export type Settings = {
  model_tier: 'haiku' | 'sonnet' | 'opus';
  locale: 'en' | 'zh';
  theme: 'light' | 'dark';
  updated_at: string;
};
