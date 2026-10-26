import { reactive, toValue, watch, type MaybeRefOrGetter } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { nameOf, parentOf } from '@/utils/paths'

// The details of files anywhere in the user's folder (a board's attachments),
// read by listing each folder they're in once rather than asking per file.
// A path whose folder holds no such name (just uploaded, say) has its folder
// listed again; one that's still missing then is reported as missing.
export function useFileEntries(paths: MaybeRefOrGetter<string[]>) {
  // Each listed folder's entries by name; an empty map when it can't be listed.
  const listings = reactive(new Map<string, Map<string, FileEntry>>())
  const listing = new Set<string>() // folders being listed right now

  async function list(folder: string) {
    if (listing.has(folder)) return
    listing.add(folder)
    try {
      const entries = await filesApi.list(folder)
      listings.set(folder, new Map(entries.map((entry) => [entry.name, entry])))
    } catch {
      listings.set(folder, new Map())
    } finally {
      listing.delete(folder)
    }
  }

  // A path's details: undefined until its folder has been listed, null when it isn't there.
  function entryFor(path: string): FileEntry | null | undefined {
    const entries = listings.get(parentOf(path))
    return entries === undefined ? undefined : (entries.get(nameOf(path)) ?? null)
  }

  // Lists the folders of paths never listed or not found in their listing yet.
  // Watching the paths as one string: only a different set of paths matters.
  let checked = new Set<string>()
  watch(
    () => [...new Set(toValue(paths))].sort().join('\n'),
    (joined) => {
      const wanted = joined ? joined.split('\n') : []
      const folders = new Set<string>()
      for (const path of wanted) {
        if (entryFor(path) === undefined || (entryFor(path) === null && !checked.has(path))) folders.add(parentOf(path))
      }
      checked = new Set(wanted)
      folders.forEach(list)
    },
    { immediate: true },
  )

  // Reads every listed folder again (files may have changed since).
  function refresh() {
    [...listings.keys()].forEach(list)
  }

  return { entryFor, refresh }
}
