import { reactive, onUnmounted } from 'vue'

/**
 * Manages thumbnail readiness state for the current folder.
 *
 * - `thumbHashes` — reactive Map of file path → content hash for files whose
 *   thumbnails are ready. Populated via `seed()` (from the initial listing) or
 *   via the SSE stream (as thumbnails are generated in the background).
 *
 * - `connect(folderPath)` — clears state and opens an SSE connection that adds
 *   to `thumbHashes` as the server finishes generating thumbnails.
 *
 * - `seed(entries)` — pre-populate from `has_thumb: true` entries in the listing.
 */
export function useThumbnails() {
  const thumbHashes = reactive(new Map<string, string>())
  let eventSource: EventSource | null = null

  function connect(folderPath: string) {
    eventSource?.close()
    thumbHashes.clear()

    const token = localStorage.getItem('token') ?? ''
    const url = `/api/v1/thumbnails/stream?path=${encodeURIComponent(folderPath)}&token=${encodeURIComponent(token)}`
    eventSource = new EventSource(url)

    eventSource.onmessage = (e: MessageEvent) => {
      try {
        const { path, hash } = JSON.parse(e.data as string) as { path: string; hash: string }
        thumbHashes.set(path, hash)
      } catch {
        // ignore malformed events
      }
    }

    // On error, stop reconnecting — the server isn't sending more events.
    eventSource.onerror = () => {
      eventSource?.close()
      eventSource = null
    }
  }

  function seed(entries: Array<{ path: string; hash: string }>) {
    entries.forEach(({ path, hash }) => thumbHashes.set(path, hash))
  }

  function disconnect() {
    eventSource?.close()
    eventSource = null
  }

  onUnmounted(disconnect)

  return { thumbHashes, connect, seed }
}
