import { defineStore } from 'pinia'
import { ref } from 'vue'
import { files as filesApi, NotFoundError } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import type { FileEntry } from '@/types'

export const useFilesStore = defineStore('files', () => {
  const currentPath = ref('')
  const entries = ref<FileEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  // Signals the last path removed via the watch stream (for selection cleanup).
  const lastRemovedPath = ref('')
  const { show: showToast } = useToast()

  let watchSource: EventSource | null = null

  function watchDisconnect() {
    watchSource?.close()
    watchSource = null
  }

  function watchConnect(path: string) {
    watchDisconnect()
    const token = localStorage.getItem('token') ?? ''
    const url = `/api/v1/files/watch?path=${encodeURIComponent(path)}&token=${encodeURIComponent(token)}`
    const es = new EventSource(url)
    watchSource = es

    es.addEventListener('add', (e: MessageEvent) => {
      try {
        const entry = JSON.parse(e.data as string) as FileEntry
        const idx = entries.value.findIndex((en) => en.path === entry.path)
        if (idx >= 0) entries.value[idx] = entry
        else entries.value.push(entry)
      } catch { /* ignore */ }
    })

    es.addEventListener('remove', (e: MessageEvent) => {
      try {
        const { path: removedPath } = JSON.parse(e.data as string) as { path: string }
        entries.value = entries.value.filter((en) => en.path !== removedPath)
        lastRemovedPath.value = removedPath
      } catch { /* ignore */ }
    })

    es.addEventListener('update', (e: MessageEvent) => {
      try {
        const entry = JSON.parse(e.data as string) as FileEntry
        const idx = entries.value.findIndex((en) => en.path === entry.path)
        if (idx >= 0) entries.value[idx] = entry
        else entries.value.push(entry)
      } catch { /* ignore */ }
    })

    es.addEventListener('thumb', (e: MessageEvent) => {
      try {
        const { hash } = JSON.parse(e.data as string) as { hash: string }
        for (const entry of entries.value) {
          if (entry.content_hash === hash) {
            entry.has_thumb = true
            entry.thumb_hash = hash
          }
        }
      } catch { /* ignore */ }
    })

    es.onerror = () => {
      es.close()
      if (watchSource === es) watchSource = null
    }
  }

  // Returns the path actually navigated to (may differ from requested path if not found).
  async function navigate(path: string): Promise<string> {
    currentPath.value = path // update immediately so watchers see the new path before entries load
    loading.value = true
    error.value = null
    watchDisconnect()
    try {
      entries.value = await filesApi.list(path)
      watchConnect(path)
      return path
    } catch (e: unknown) {
      if (e instanceof NotFoundError) {
        showToast(e.message, 'error')
        const fallback = e.fallbackPath
        currentPath.value = fallback
        try {
          entries.value = await filesApi.list(fallback)
          watchConnect(fallback)
        } catch {
          entries.value = []
        }
        return fallback
      }
      error.value = e instanceof Error ? e.message : 'Failed to load directory'
      return path
    } finally {
      loading.value = false
    }
  }

  function refresh() {
    return navigate(currentPath.value)
  }

  function reset() {
    watchDisconnect()
    currentPath.value = ''
    entries.value = []
    error.value = null
  }

  return { currentPath, entries, loading, error, navigate, refresh, reset, watchDisconnect, lastRemovedPath }
})
