import { onScopeDispose, ref, watch, type Ref } from 'vue'
import type { UsageEntry, UsageListing, UsageTotals } from '@/types'
import { ScanNeededError, usage as usageApi } from '@/utils/api'

// The disk usage page's data: the folder at inside the scanned folder root,
// the scan's largest files, and the scan itself. A folder without a scan on
// the server is scanned first (streaming progress), then shown.
export function useDiskUsage(root: Readonly<Ref<string>>, at: Readonly<Ref<string>>) {
  const listing = ref<UsageListing | null>(null)
  const largest = ref<UsageEntry[] | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const scanning = ref(false)
  const queued = ref(false)
  const progress = ref<UsageTotals | null>(null)

  let scanSource: EventSource | null = null
  // Counts requests, so an answer about a folder no longer shown is dropped.
  let loadRequest = 0

  function stopScan() {
    scanSource?.close()
    scanSource = null
    scanning.value = false
    queued.value = false
  }

  async function load() {
    const request = ++loadRequest
    loading.value = true
    try {
      const result = await usageApi.list(root.value, at.value)
      if (request !== loadRequest) return
      listing.value = result
      error.value = null
    } catch (err) {
      if (request !== loadRequest) return
      if (err instanceof ScanNeededError) scan()
      else error.value = (err as Error).message
    } finally {
      if (request === loadRequest) loading.value = false
    }
  }

  // Scans root again (or for the first time), then loads the folder shown.
  function scan() {
    stopScan()
    ++loadRequest
    listing.value = null
    largest.value = null
    error.value = null
    scanning.value = true
    progress.value = { files: 0, dirs: 0, disk: 0 }
    const source = new EventSource(usageApi.scanStreamUrl(root.value))
    scanSource = source
    source.addEventListener('queued', () => { queued.value = true })
    source.addEventListener('progress', (event) => {
      queued.value = false
      progress.value = JSON.parse((event as MessageEvent).data)
    })
    source.addEventListener('done', () => {
      stopScan()
      load()
    })
    source.addEventListener('failed', (event) => {
      stopScan()
      error.value = JSON.parse((event as MessageEvent).data).error
    })
    // The connection broke: don't let EventSource reconnect, which would
    // start the scan over.
    source.onerror = () => {
      if (scanSource !== source) return
      stopScan()
      error.value = 'The scan stopped before it finished.'
    }
  }

  async function loadLargest() {
    try {
      largest.value = await usageApi.largest(root.value)
    } catch (err) {
      if (err instanceof ScanNeededError) scan()
      else error.value = (err as Error).message
    }
  }

  // Deletes paths for good and shows the new sizes; returns what was freed.
  async function remove(paths: string[]): Promise<UsageTotals> {
    const result = await usageApi.delete(root.value, paths)
    if (largest.value) {
      const removed = (path: string) => paths.some((gone) => path === gone || path.startsWith(gone + '/'))
      largest.value = largest.value.filter((entry) => !removed(entry.path))
    }
    await load()
    return result.freed
  }

  watch(root, () => { largest.value = null })
  watch([root, at], load, { immediate: true })
  onScopeDispose(stopScan)

  return { listing, largest, loading, error, scanning, queued, progress, rescan: scan, cancelScan: stopScan, loadLargest, remove }
}
