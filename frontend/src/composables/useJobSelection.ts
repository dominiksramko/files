import { watch, type Ref } from 'vue'
import type { FileEntry } from '@/types'

// Keeps the selection in step with the listing and with background jobs (zip
// download, compress). Paths that leave the listing (removed via the watch
// stream, or missing from a fresh snapshot after a reconnect) are dropped from
// the selection. A finished job clears the selection only if it was the last
// one started and the user hasn't changed the selection since — dropping
// paths that left the listing doesn't count as a change.
export function useJobSelection(selectedPaths: Ref<string[]>, entries: () => FileEntry[]) {
  let lastJobId = -1
  let changedSinceLastJob = false
  let pruning = false

  // flush 'sync' runs it during the assignment, while pruning is still set.
  watch(selectedPaths, () => {
    if (!pruning) changedSinceLastJob = true
  }, { flush: 'sync' })

  watch(entries, (current) => {
    if (!selectedPaths.value.length) return
    const presentPaths = new Set(current.map((entry) => entry.path))
    if (selectedPaths.value.every((path) => presentPaths.has(path))) return
    pruning = true
    selectedPaths.value = selectedPaths.value.filter((path) => presentPaths.has(path))
    pruning = false
  })

  function trackJob(id: number) {
    lastJobId = id
    changedSinceLastJob = false
  }

  function clearSelectionAfterJob(id: number) {
    if (id === lastJobId && !changedSinceLastJob) selectedPaths.value = []
  }

  return { trackJob, clearSelectionAfterJob }
}
