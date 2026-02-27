import { reactive, onUnmounted } from 'vue'

/**
 * Manages thumbnail readiness state for the current folder.
 *
 * - `thumbReadyPaths` — reactive Set of file paths whose thumbnails are ready.
 *   Add paths here either via `seed()` (from the initial list response) or via
 *   the SSE stream (as thumbnails are generated in the background).
 *
 * - `connect(folderPath)` — clears state and opens an SSE connection that adds
 *   to `thumbReadyPaths` as the server finishes generating thumbnails.
 *
 * - `seed(paths)` — pre-populate from `has_thumb: true` entries in the listing.
 */
export function useThumbnails() {
  const thumbReadyPaths = reactive(new Set<string>())
  let eventSource: EventSource | null = null

  function connect(folderPath: string) {
    eventSource?.close()
    thumbReadyPaths.clear()

    const token = localStorage.getItem('token') ?? ''
    const url = `/api/v1/thumbnails/stream?path=${encodeURIComponent(folderPath)}&token=${encodeURIComponent(token)}`
    eventSource = new EventSource(url)

    eventSource.onmessage = (e: MessageEvent) => {
      thumbReadyPaths.add(e.data as string)
    }

    // On error, stop reconnecting — the server isn't sending more events.
    eventSource.onerror = () => {
      eventSource?.close()
      eventSource = null
    }
  }

  function seed(paths: string[]) {
    paths.forEach((p) => thumbReadyPaths.add(p))
  }

  function disconnect() {
    eventSource?.close()
    eventSource = null
  }

  onUnmounted(disconnect)

  return { thumbReadyPaths, connect, seed }
}
