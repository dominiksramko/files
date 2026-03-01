import { onUnmounted } from 'vue'
import { useFilesStore } from '@/stores/files'

/**
 * Manages the SSE connection for background thumbnail generation.
 *
 * - `connect(folderPath)` — opens an SSE connection for the given folder.
 *   As thumbnails are generated, the server emits events and this composable
 *   updates the matching entry in filesStore directly (has_thumb + thumb_hash).
 *   Components read thumbnail state straight from FileEntry props — no separate
 *   map or seeding needed.
 */
export function useThumbnails() {
  const filesStore = useFilesStore()
  let eventSource: EventSource | null = null

  function connect(folderPath: string) {
    eventSource?.close()

    const token = localStorage.getItem('token') ?? ''
    const url = `/api/v1/thumbnails/stream?path=${encodeURIComponent(folderPath)}&token=${encodeURIComponent(token)}`
    eventSource = new EventSource(url)

    eventSource.onmessage = (e: MessageEvent) => {
      try {
        const { path, hash } = JSON.parse(e.data as string) as { path: string; hash: string }
        const entry = filesStore.entries.find((en) => en.path === path)
        if (entry) {
          entry.has_thumb = true
          entry.thumb_hash = hash
        }
      } catch {
        // ignore malformed events
      }
    }

    eventSource.onerror = () => {
      eventSource?.close()
      eventSource = null
    }
  }

  onUnmounted(() => {
    eventSource?.close()
    eventSource = null
  })

  return { connect }
}
