import type { ArchiveEntry, FileEntry, LoginResponse, User } from '@/types'

const BASE = '/api/v1'

function token(): string {
  return localStorage.getItem('token') ?? ''
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  if (token()) {
    headers['Authorization'] = `Bearer ${token()}`
  }

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    signal,
  })

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }

  return res.json() as Promise<T>
}

function qs(params: Record<string, string>): string {
  const p = new URLSearchParams(params)
  return p.toString() ? '?' + p.toString() : ''
}

// --- Auth ---
export const auth = {
  login: (username: string, password: string) =>
    request<LoginResponse>('POST', '/auth/login', { username, password }),

  me: () => request<User>('GET', '/auth/me'),

  changePassword: (current_password: string, new_password: string) =>
    request<{ message: string }>('PUT', '/auth/password', {
      current_password,
      new_password,
    }),
}

// --- Files ---
export const files = {
  list: (path = '') => request<FileEntry[]>('GET', `/files/list${qs({ path })}`),

  search: (q: string, path = '') =>
    request<FileEntry[]>('GET', `/files/search${qs({ q, ...(path ? { path } : {}) })}`),

  downloadUrl: (path: string) =>
    `${BASE}/files/download?path=${encodeURIComponent(path)}&token=${token()}`,

  streamUrl: (path: string) =>
    `${BASE}/files/stream?path=${encodeURIComponent(path)}&token=${token()}`,

  archiveContents: (path: string, password = '') =>
    request<ArchiveEntry[]>(
      'GET',
      `/files/archive${qs({ path, ...(password ? { password } : {}) })}`,
    ),

  extract: (path: string, dest?: string, password?: string) =>
    request<{ message: string }>('POST', '/files/extract', { path, dest, password }),
}

// --- Folders ---
export const folders = {
  getPref: (path: string) =>
    request<{ view_mode: 'grid' | 'list' }>('GET', `/folders/pref${qs({ path })}`),

  setPref: (path: string, view_mode: 'grid' | 'list') =>
    request<{ view_mode: 'grid' | 'list' }>('PUT', '/folders/pref', { path, view_mode }),
}

// --- Users ---
export const users = {
  list: () => request<User[]>('GET', '/users'),

  create: (data: { username: string; password: string; root_dir?: string; is_admin?: boolean }) =>
    request<User>('POST', '/users', data),

  update: (id: number, data: { password?: string; root_dir?: string; is_admin?: boolean }) =>
    request<User>('PUT', `/users/${id}`, data),

  delete: (id: number) => request<{ message: string }>('DELETE', `/users/${id}`),
}
