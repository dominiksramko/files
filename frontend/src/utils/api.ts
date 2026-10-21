import type { ArchiveEntry, FileEntry, LoginResponse, RenamePreset, RenamePreviewItem, SubtitlePrefs, UsageEntry, UsageListing, UsageTotals, User } from '@/types'

const BASE = '/api/v1'

// How an operation treats names that are already taken: 'error' refuses with
// ConflictError (the default), 'replace' overwrites, 'rename' keeps both by
// giving the new item a " (n)" name.
export type ConflictMode = 'error' | 'replace' | 'rename'

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

// An archive can't be listed or extracted without a password, or not with the
// one given.
export class PasswordRequiredError extends Error {}

// The disk usage page asked about a folder the user hasn't scanned (or whose
// scan has expired): start a scan.
export class ScanNeededError extends Error {}

// A versioned save was refused because the file changed since the version it
// was based on; carries the file's current content and version for merging.
export class EditConflictError extends Error {
  content: string
  version: string
  constructor(content: string, version: string) {
    super('File was changed elsewhere')
    this.content = content
    this.version = version
  }
}

// A text file's content plus its version (sent back as baseVersion when saving).
export interface VersionedContent {
  content: string
  version: string
}

// Called whenever the server answers 401: the session has ended (expired,
// logged out elsewhere, password changed, user deleted). main.ts sets it.
let sessionEndedHandler: (() => void) | null = null
export function onSessionEnded(handler: () => void) {
  sessionEndedHandler = handler
}
function noteStatus(status: number) {
  if (status === 401) sessionEndedHandler?.()
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

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    signal,
  })

  noteStatus(res.status)
  if (res.status === 409) {
    const body = await res.json().catch(() => ({ conflicts: [] as string[] }))
    throw new ConflictError(body.conflicts ?? [])
  }
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    if (res.status === 404 && 'fallback_path' in err) {
      throw new NotFoundError(err.error ?? res.statusText, err.fallback_path as string)
    }
    if (res.status === 404 && err.scan_needed === true) throw new ScanNeededError(err.error)
    if (err.password_required === true) throw new PasswordRequiredError(err.error)
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

  // Ends the session on the server and clears its cookie.
  logout: () => request<{ message: string }>('POST', '/auth/logout'),

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
function thumbnailUrl(kind: 'get' | 'preview', path: string, hash: string): string {
  return `${BASE}/thumbnails/${kind}?path=${encodeURIComponent(path)}&hash=${encodeURIComponent(hash)}`
}

export const files = {
  list: (path = '') => request<FileEntry[]>('GET', `/files/list${qs({ path })}`),

  stat: (path: string) => request<FileEntry>('GET', `/files/stat${qs({ path })}`),

  searchStreamUrl: (q: string, path = '', showHidden = false): string => {
    const params = new URLSearchParams({ q })
    if (path) params.set('path', path)
    if (showHidden) params.set('hidden', 'true')
    return `${BASE}/files/search?${params}`
  },

  downloadUrl: (path: string, modTime: string) =>
    `${BASE}/files/download?path=${encodeURIComponent(path)}&t=${new Date(modTime).getTime()}`,

  streamUrl: (path: string, modTime: string) =>
    `${BASE}/files/stream?path=${encodeURIComponent(path)}&t=${new Date(modTime).getTime()}`,

  streamUrlByPath: (path: string) =>
    `${BASE}/files/stream?path=${encodeURIComponent(path)}`,

  hlsPlaylistUrl: (path: string) =>
    `${BASE}/files/hls/playlist?path=${encodeURIComponent(path)}`,

  // The square thumbnail for file lists.
  thumbUrl: (path: string, hash: string) => thumbnailUrl('get', path, hash),

  // The whole picture, small: shown while the preview loads, and as a video's poster.
  previewThumbUrl: (path: string, hash: string) => thumbnailUrl('preview', path, hash),

  // POST so the password travels in the body, never in a URL (URLs end up in logs).
  archiveContents: (path: string, password = '') =>
    request<ArchiveEntry[]>('POST', '/files/archive', { path, password }),

  extract: (path: string, dest?: string, password?: string, conflictMode: ConflictMode = 'error') =>
    request<{ message: string }>('POST', '/files/extract', { path, dest, password, conflict_mode: conflictMode }),

  readContent: (path: string, signal?: AbortSignal): Promise<VersionedContent> =>
    fetch(`${BASE}/files/content?path=${encodeURIComponent(path)}`, {
      cache: 'no-store',
      signal,
    }).then(async (res) => {
      noteStatus(res.status)
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(err.error ?? res.statusText)
      }
      return { content: await res.text(), version: (res.headers.get('ETag') ?? '').replace(/"/g, '') }
    }),

  // With baseVersion the save only goes through if the file is still at that
  // version, otherwise it throws EditConflictError; without it, it overwrites.
  // Create new files with fileops.create, which never overwrites.
  writeContent: (path: string, content: string, baseVersion?: string) =>
    fetch(`${BASE}/files/content?path=${encodeURIComponent(path)}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'text/plain; charset=utf-8',
        ...(baseVersion ? { 'If-Match': `"${baseVersion}"` } : {}),
      },
      body: content,
    }).then(async (res) => {
      noteStatus(res.status)
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: res.statusText }))
        if (res.status === 412) throw new EditConflictError(err.content ?? '', err.version ?? '')
        throw new Error(err.error ?? res.statusText)
      }
      return res.json() as Promise<{ version: string; mod_time: string }>
    }),

  // All subtitle tracks (file-based + embedded) for a video file.
  // The server does all discovery; each track has a ready-to-use url.
  subtitles: (path: string) =>
    request<{ label: string; srclang: string; url: string }[]>(
      'GET',
      `/files/subtitles${qs({ path })}`,
    ),

  // SSE URL for streaming recursive size calculation over one or more paths.
  sizeStreamUrl: (paths: string[]): string => {
    const params = new URLSearchParams()
    paths.forEach((p) => params.append('paths[]', p))
    return `${BASE}/files/size?${params}`
  },

  officeToken: (path: string) =>
    request<{ token: string }>('POST', '/files/office-token', { path }),

  screenshot: (path: string, timestamp: number) =>
    request<{ filename: string }>('POST', '/files/screenshot', { path, timestamp }),

  // Image resolution or video duration/resolution/codec for a single file.
  fileMeta: (path: string) =>
    request<{ image?: { width: number; height: number }; video?: { duration: number; width: number; height: number; codec?: string } }>(
      'GET',
      `/files/info${qs({ path })}`,
    ),
}

// --- Disk usage ---
// A scan of the folder root is kept on the server (the user's latest one);
// the other calls browse and change it.
export const usage = {
  // SSE: queued, progress (UsageTotals), done, failed ({ error }).
  scanStreamUrl: (root: string) => `${BASE}/files/usage/scan${qs({ path: root })}`,

  list: (root: string, at: string) => request<UsageListing>('GET', `/files/usage${qs({ path: root, at })}`),

  largest: (root: string) =>
    request<UsageEntry[]>('GET', `/files/usage/largest${qs({ path: root })}`),

  // Deletes for good, like fileops.delete.
  delete: (root: string, paths: string[]) =>
    request<{ freed: UsageTotals; root_total: UsageTotals }>('DELETE', '/files/usage', { path: root, paths }),
}

// --- File operations ---
export const fileops = {
  delete: (paths: string[]) => request<{ message: string }>('DELETE', '/files', { paths }),

  downloadZip: async (paths: string[]): Promise<Blob> => {
    const res = await fetch(`${BASE}/files/download-zip`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ paths }),
    })
    noteStatus(res.status)
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(err.error ?? res.statusText)
    }
    return res.blob()
  },

  copy: (paths: string[], dest: string, conflictMode: ConflictMode = 'error') =>
    request<{ message: string }>('POST', '/files/copy', { paths, dest, conflict_mode: conflictMode }),

  move: (paths: string[], dest: string, conflictMode: ConflictMode = 'error') =>
    request<{ message: string }>('POST', '/files/move', { paths, dest, conflict_mode: conflictMode }),

  mkdir: (path: string) =>
    request<{ message: string }>('POST', '/files/mkdir', { path }),

  // Creates an empty file; throws ConflictError when the name is taken (unless
  // conflictMode says otherwise). Resolves to the name it was created under.
  create: (path: string, conflictMode: ConflictMode = 'error') =>
    request<{ name: string }>('POST', '/files/create', { path, conflict_mode: conflictMode }),

  // Resolves with the name the item ended up with.
  rename: (path: string, name: string, conflictMode: ConflictMode = 'error') =>
    request<{ message: string; name: string }>('POST', '/files/rename', { path, name, conflict_mode: conflictMode }),

  compress: (paths: string[], dest: string, format: string, password?: string, protectFilenames?: boolean, conflictMode: ConflictMode = 'error') =>
    request<{ message: string }>('POST', '/files/compress', { paths, dest, format, password, protect_filenames: protectFilenames, conflict_mode: conflictMode }),

  upload: (
    path: string,
    files: File[],
    onProgress?: (loaded: number, total: number) => void,
    conflictMode: ConflictMode = 'error',
  ): { promise: Promise<{ message: string; count: number }>; abort: () => void } => {
    let xhr: XMLHttpRequest
    const promise = new Promise<{ message: string; count: number }>((resolve, reject) => {
      const form = new FormData()
      files.forEach((f, i) => {
        form.append('files', f)
        form.append(`mtime_${i}`, Math.floor(f.lastModified / 1000).toString())
      })
      xhr = new XMLHttpRequest()
      xhr.open('POST', `/api/v1/files/upload${qs({ path, conflict_mode: conflictMode })}`)
      if (onProgress) {
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) onProgress(e.loaded, e.total)
        }
      }
      xhr.onload = () => {
        noteStatus(xhr.status)
        if (xhr.status >= 200 && xhr.status < 300) {
          try { resolve(JSON.parse(xhr.responseText)) }
          catch { reject(new Error('Invalid response')) }
        } else {
          try {
            const err = JSON.parse(xhr.responseText)
            reject(xhr.status === 409 ? new ConflictError(err.conflicts ?? []) : new Error(err.error ?? xhr.statusText))
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

// --- Advanced renamer ---
export const renamer = {
  preview: (template: string, paths: string[]) =>
    request<{ items: RenamePreviewItem[] }>('POST', '/rename/preview', { template, paths }),

  apply: (
    renames: { from: string; to: string }[],
    folder: string,
    template: string,
    conflictMode: ConflictMode = 'error',
  ) =>
    request<{ message: string }>('POST', '/rename/apply', {
      renames,
      folder,
      template,
      conflict_mode: conflictMode,
    }),

  listPresets: () => request<RenamePreset[]>('GET', '/rename/presets'),

  savePreset: (name: string, template: string) =>
    request<RenamePreset>('POST', '/rename/presets', { name, template }),

  deletePreset: (id: number) => request<{ message: string }>('DELETE', `/rename/presets/${id}`),

  getFolderTemplate: (folder: string) =>
    request<{ template: string }>('GET', `/rename/folder-template${qs({ folder })}`),
}

// --- System ---
export const system = {
  diskSpace: () =>
    request<{ total: number; free: number; used: number; pct: number }>('GET', '/system/disk'),
  version: () =>
    request<{ version: string }>('GET', '/system/version').then(data => data.version),
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
