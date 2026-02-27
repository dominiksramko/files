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

  thumbUrl: (path: string) =>
    `${BASE}/thumbnails/get?path=${encodeURIComponent(path)}&token=${token()}`,

  archiveContents: (path: string, password = '') =>
    request<ArchiveEntry[]>(
      'GET',
      `/files/archive${qs({ path, ...(password ? { password } : {}) })}`,
    ),

  extract: (path: string, dest?: string, password?: string) =>
    request<{ message: string }>('POST', '/files/extract', { path, dest, password }),

  // All subtitle tracks (file-based + embedded) for a video file.
  // The server does all discovery; each track has a ready-made url (without token).
  subtitles: (path: string) =>
    request<{ label: string; srclang: string; url: string }[]>(
      'GET',
      `/files/subtitles${qs({ path })}`,
    ),

  // Append the auth token to a subtitle url returned by the server.
  subtitleUrl: (url: string) => `${url}&token=${token()}`,
}

// --- File operations ---
export const fileops = {
  delete: (paths: string[]) => request<{ message: string }>('DELETE', '/files', { paths }),

  copy: (paths: string[], dest: string) =>
    request<{ message: string }>('POST', '/files/copy', { paths, dest }),

  move: (paths: string[], dest: string) =>
    request<{ message: string }>('POST', '/files/move', { paths, dest }),

  mkdir: (path: string) =>
    request<{ message: string }>('POST', '/files/mkdir', { path }),

  rename: (path: string, name: string) =>
    request<{ message: string }>('POST', '/files/rename', { path, name }),

  upload: (path: string, files: File[]) => {
    const form = new FormData()
    files.forEach((f, i) => {
      form.append('files', f)
      form.append(`mtime_${i}`, Math.floor(f.lastModified / 1000).toString())
    })
    return fetch(`/api/v1/files/upload?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${localStorage.getItem('token') ?? ''}` },
      body: form,
    }).then(async (res) => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error ?? res.statusText)
      }
      return res.json() as Promise<{ message: string; count: number }>
    })
  },
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
