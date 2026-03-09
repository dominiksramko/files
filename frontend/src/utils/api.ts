import type { ArchiveEntry, FileEntry, LoginResponse, SubtitlePrefs, User } from '@/types'

const BASE = '/api/v1'

export class ConflictError extends Error {
  conflicts: string[]
  constructor(conflicts: string[]) {
    super('conflict')
    this.conflicts = conflicts
  }
}

export class NotFoundError extends Error {
  fallbackPath: string
  constructor(message: string, fallbackPath: string) {
    super(message)
    this.fallbackPath = fallbackPath
  }
}

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

  if (res.status === 409) {
    const body = await res.json().catch(() => ({ conflicts: [] as string[] }))
    throw new ConflictError(body.conflicts ?? [])
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    if (res.status === 404 && 'fallback_path' in err) {
      throw new NotFoundError(err.error ?? res.statusText, err.fallback_path as string)
    }
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

  updateProfile: (display_name: string) =>
    request<User>('PUT', '/auth/profile', { display_name }),

  updateSubtitlePrefs: (prefs: SubtitlePrefs) =>
    request<SubtitlePrefs>('PUT', '/auth/subtitle-prefs', prefs),
}

// --- Files ---
export const files = {
  list: (path = '') => request<FileEntry[]>('GET', `/files/list${qs({ path })}`),

  searchStreamUrl: (q: string, path = '', showHidden = false): string => {
    const params = new URLSearchParams({ q, token: token() })
    if (path) params.set('path', path)
    if (showHidden) params.set('hidden', 'true')
    return `${BASE}/files/search?${params}`
  },

  downloadUrl: (path: string, modTime: string) =>
    `${BASE}/files/download?path=${encodeURIComponent(path)}&t=${new Date(modTime).getTime()}&token=${token()}`,

  streamUrl: (path: string, modTime: string) =>
    `${BASE}/files/stream?path=${encodeURIComponent(path)}&t=${new Date(modTime).getTime()}&token=${token()}`,

  hlsPlaylistUrl: (path: string) =>
    `${BASE}/files/hls/playlist?path=${encodeURIComponent(path)}&token=${token()}`,

  thumbUrl: (path: string, hash: string) =>
    `${BASE}/thumbnails/get?path=${encodeURIComponent(path)}&hash=${encodeURIComponent(hash)}&token=${token()}`,

  archiveContents: (path: string, password = '') =>
    request<ArchiveEntry[]>(
      'GET',
      `/files/archive${qs({ path, ...(password ? { password } : {}) })}`,
    ),

  extract: (path: string, dest?: string, password?: string, conflictMode: 'error' | 'replace' | 'rename' = 'error') =>
    request<{ message: string }>('POST', '/files/extract', { path, dest, password, conflict_mode: conflictMode }),

  writeContent: (path: string, content: string) =>
    fetch(`${BASE}/files/content?path=${encodeURIComponent(path)}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'text/plain; charset=utf-8',
        Authorization: `Bearer ${token()}`,
      },
      body: content,
    }).then(async (res) => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error ?? res.statusText)
      }
      return res.json() as Promise<{ message: string }>
    }),

  // All subtitle tracks (file-based + embedded) for a video file.
  // The server does all discovery; each track has a ready-made url (without token).
  subtitles: (path: string) =>
    request<{ label: string; srclang: string; url: string }[]>(
      'GET',
      `/files/subtitles${qs({ path })}`,
    ),

  // Append the auth token to a subtitle url returned by the server.
  subtitleUrl: (url: string) => `${url}&token=${token()}`,

  // SSE URL for streaming recursive size calculation over one or more paths.
  // Use with EventSource — auth via ?token= since EventSource has no headers.
  sizeStreamUrl: (paths: string[]): string => {
    const params = new URLSearchParams()
    paths.forEach((p) => params.append('paths[]', p))
    params.set('token', token())
    return `${BASE}/files/size?${params}`
  },

  officeToken: (path: string) =>
    request<{ token: string }>('POST', '/files/office-token', { path }),

  // Image resolution or video duration/resolution/codec for a single file.
  fileMeta: (path: string) =>
    request<{ image?: { width: number; height: number }; video?: { duration: number; width: number; height: number; codec?: string } }>(
      'GET',
      `/files/info${qs({ path })}`,
    ),
}

// --- File operations ---
export const fileops = {
  delete: (paths: string[]) => request<{ message: string }>('DELETE', '/files', { paths }),

  downloadZip: async (paths: string[]): Promise<Blob> => {
    const res = await fetch(`${BASE}/files/download-zip`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${localStorage.getItem('token') ?? ''}`,
      },
      body: JSON.stringify({ paths }),
    })
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(err.error ?? res.statusText)
    }
    return res.blob()
  },

  copy: (paths: string[], dest: string, conflictMode: 'error' | 'replace' | 'rename' = 'error') =>
    request<{ message: string }>('POST', '/files/copy', { paths, dest, conflict_mode: conflictMode }),

  move: (paths: string[], dest: string, conflictMode: 'error' | 'replace' | 'rename' = 'error') =>
    request<{ message: string }>('POST', '/files/move', { paths, dest, conflict_mode: conflictMode }),

  mkdir: (path: string) =>
    request<{ message: string }>('POST', '/files/mkdir', { path }),

  rename: (path: string, name: string) =>
    request<{ message: string }>('POST', '/files/rename', { path, name }),

  compress: (paths: string[], dest: string, format: string, password?: string, protectFilenames?: boolean, conflictMode: 'error' | 'replace' | 'rename' = 'error') =>
    request<{ message: string }>('POST', '/files/compress', { paths, dest, format, password, protect_filenames: protectFilenames, conflict_mode: conflictMode }),

  upload: (
    path: string,
    files: File[],
    onProgress?: (loaded: number, total: number) => void,
  ): { promise: Promise<{ message: string; count: number }>; abort: () => void } => {
    let xhr: XMLHttpRequest
    const promise = new Promise<{ message: string; count: number }>((resolve, reject) => {
      const form = new FormData()
      files.forEach((f, i) => {
        form.append('files', f)
        form.append(`mtime_${i}`, Math.floor(f.lastModified / 1000).toString())
      })
      xhr = new XMLHttpRequest()
      xhr.open('POST', `/api/v1/files/upload?path=${encodeURIComponent(path)}`)
      xhr.setRequestHeader('Authorization', `Bearer ${localStorage.getItem('token') ?? ''}`)
      if (onProgress) {
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) onProgress(e.loaded, e.total)
        }
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try { resolve(JSON.parse(xhr.responseText)) }
          catch { reject(new Error('Invalid response')) }
        } else {
          try {
            const err = JSON.parse(xhr.responseText)
            reject(new Error(err.error ?? xhr.statusText))
          } catch { reject(new Error(xhr.statusText)) }
        }
      }
      xhr.onerror = () => reject(new Error('Upload failed'))
      xhr.onabort = () => reject(new Error('cancelled'))
      xhr.send(form)
    })
    return { promise, abort: () => xhr?.abort() }
  },
}

// --- Folders ---
export const folders = {
  getPref: (path: string) =>
    request<{ view_mode: 'grid' | 'list' }>('GET', `/folders/pref${qs({ path })}`),

  setPref: (path: string, view_mode: 'grid' | 'list') =>
    request<{ view_mode: 'grid' | 'list' }>('PUT', '/folders/pref', { path, view_mode }),
}

// --- Watch progress ---
export const progress = {
  get: (path: string) =>
    request<{ position: number; duration: number }>('GET', `/progress${qs({ path })}`),

  save: (path: string, position: number, duration: number) =>
    request<{ message: string }>('PUT', '/progress', { path, position, duration }),

  delete: (path: string) =>
    request<{ message: string }>('DELETE', `/progress${qs({ path })}`),
}

// --- System ---
export const system = {
  diskSpace: () =>
    request<{ total: number; free: number; used: number; pct: number }>('GET', '/system/disk'),
}

// --- Admin cache ---
export const adminCache = {
  clearAll: () => request<{ message: string }>('DELETE', '/cache'),
}

// --- Users ---
export const users = {
  list: () => request<User[]>('GET', '/users'),

  create: (data: { username: string; password: string; display_name?: string; root_dir?: string; is_admin?: boolean }) =>
    request<User>('POST', '/users', data),

  update: (id: number, data: { password?: string; display_name?: string; root_dir?: string; is_admin?: boolean }) =>
    request<User>('PUT', `/users/${id}`, data),

  delete: (id: number) => request<{ message: string }>('DELETE', `/users/${id}`),
}
