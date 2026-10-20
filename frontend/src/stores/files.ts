import { defineStore } from 'pinia'
import { ref } from 'vue'
import { auth as authApi, files as filesApi, NotFoundError } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import { debounce } from '@/utils/debounce'
import type { FileEntry } from '@/types'

// The watch stream pings every 25 s; one silent for longer is treated as dead
// (proxies, NAT and mobile networks can drop it without the browser noticing).
const WATCH_SILENCE_LIMIT_MS = 60_000
const RECONNECT_BASE_DELAY_MS = 1_000
const RECONNECT_MAX_DELAY_MS = 30_000

export const useFilesStore = defineStore('files', () => {
  const currentPath = ref('')
  const entries = ref<FileEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const { show: showToast } = useToast()

  // ── Live updates ───────────────────────────────────────────────────────────
  // The watch stream starts with a full listing ("snapshot") taken after the
  // server began watching, and sends a new one whenever it may have missed
  // events, so every (re)connect brings the listing fully up to date.
  let watchSource: EventSource | null = null
  let watchedPath: string | null = null // folder the stream follows; null when stopped
  let rejectPendingSnapshot: (() => void) | null = null
  let reconnectAttempts = 0
  let lastWatchEventAt = 0
  let pendingReconnect: ReturnType<typeof debounce> | null = null

  const reconnectIfSilent = debounce(reconnectNow, WATCH_SILENCE_LIMIT_MS)

  function closeWatchSource() {
    watchSource?.close()
    watchSource = null
    rejectPendingSnapshot?.()
    rejectPendingSnapshot = null
    reconnectIfSilent.cancel()
    pendingReconnect?.cancel()
  }

  function watchDisconnect() {
    closeWatchSource()
    watchedPath = null
  }

  function reconnectNow() {
    if (watchedPath !== null) openWatch(watchedPath).catch(() => { /* onerror schedules the retry */ })
  }

  function scheduleReconnect() {
    const delay = Math.min(RECONNECT_BASE_DELAY_MS * 2 ** reconnectAttempts, RECONNECT_MAX_DELAY_MS)
    reconnectAttempts++
    pendingReconnect = debounce(reconnectNow, delay)
    pendingReconnect()
  }

  function upsertEntry(entry: FileEntry) {
    const index = entries.value.findIndex((existing) => existing.path === entry.path)
    if (index >= 0) entries.value[index] = entry
    else entries.value.push(entry)
  }

  // Opens the stream for path; resolves once its first snapshot is applied and
  // rejects if the stream fails (or is replaced) before that.
  function openWatch(path: string): Promise<void> {
    closeWatchSource()
    watchedPath = path
    lastWatchEventAt = Date.now()
    reconnectIfSilent()

    const source = new EventSource(`/api/v1/files/watch?path=${encodeURIComponent(path)}`)
    watchSource = source

    return new Promise((resolve, reject) => {
      rejectPendingSnapshot = () => reject(new Error('Live updates unavailable'))

      function listen<T>(eventName: string, apply: (payload: T) => void) {
        source.addEventListener(eventName, (event: MessageEvent) => {
          if (watchSource !== source) return
          lastWatchEventAt = Date.now()
          reconnectIfSilent()
          let payload: T
          try { payload = JSON.parse(event.data as string) as T } catch { return }
          apply(payload)
        })
      }

      listen<FileEntry[]>('snapshot', (snapshot) => {
        entries.value = snapshot
        error.value = null
        reconnectAttempts = 0
        rejectPendingSnapshot = null
        resolve()
      })
      listen<FileEntry>('add', upsertEntry)
      listen<FileEntry>('update', upsertEntry)
      listen<{ path: string }>('remove', ({ path: removedPath }) => {
        entries.value = entries.value.filter((entry) => entry.path !== removedPath)
      })
      listen<{ hash: string }>('thumb', ({ hash }) => {
        for (const entry of entries.value) {
          if (entry.content_hash === hash) {
            entry.has_thumb = true
            entry.thumb_hash = hash
          }
        }
      })
      listen('ping', () => {})

      // EventSource's own retry would resume without backoff;
      // reconnect ourselves instead (the new stream starts with a snapshot).
      source.onerror = () => {
        if (watchSource !== source) return
        closeWatchSource()
        scheduleReconnect()
        // A stream can't tell why it failed. If the session ended, asking the
        // server about it brings up the login page (which also stops this).
        authApi.me().catch(() => { /* answered via onSessionEnded, or offline */ })
      }
    })
  }

  // Background tabs throttle or freeze timers, so the silence check may not have
  // run; catch up as soon as the page is visible or the network is back.
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible' || watchedPath === null) return
    if (!watchSource || Date.now() - lastWatchEventAt > WATCH_SILENCE_LIMIT_MS / 2) {
      reconnectAttempts = 0
      reconnectNow()
    }
  })
  window.addEventListener('online', () => {
    reconnectAttempts = 0
    reconnectNow()
  })

  // ── Navigation ─────────────────────────────────────────────────────────────
  // Bumped by every navigate(); an older navigation that finishes late must not
  // overwrite the listing of the folder navigated to since.
  let navigationId = 0

  // Returns the path actually navigated to (may differ from requested path if not found).
  async function navigate(path: string): Promise<string> {
    const thisNavigation = ++navigationId
    const isStale = () => thisNavigation !== navigationId
    currentPath.value = path // update immediately so watchers see the new path before entries load
    loading.value = true
    error.value = null
    try {
      await openWatch(path)
      return path
    } catch {
      if (isStale()) return currentPath.value
      // The stream failed before its snapshot (it keeps retrying on its own);
      // the list endpoint says why, and where to go if the folder is gone.
      try {
        const listing = await filesApi.list(path)
        if (isStale()) return currentPath.value
        entries.value = listing
        return path
      } catch (e: unknown) {
        if (isStale()) return currentPath.value
        if (e instanceof NotFoundError && e.fallbackPath !== path) {
          showToast(e.message, 'error')
          return navigate(e.fallbackPath)
        }
        error.value = e instanceof Error ? e.message : 'Failed to load directory'
        return path
      }
    } finally {
      if (!isStale()) loading.value = false
    }
  }

  function refresh() {
    return navigate(currentPath.value)
  }

  // Back to nothing loaded: closes the live-update stream (no reconnects) and
  // drops the listing, e.g. when the user logs out.
  function reset() {
    navigationId++
    watchDisconnect()
    currentPath.value = ''
    entries.value = []
    loading.value = false
    error.value = null
  }

  return { currentPath, entries, loading, error, navigate, refresh, reset, watchDisconnect }
})
