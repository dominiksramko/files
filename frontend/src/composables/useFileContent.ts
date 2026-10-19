import { computed, nextTick, onBeforeUnmount, onMounted, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import type { FileEntry } from '@/types'
import { EditConflictError, files as filesApi, type VersionedContent } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import { debounce } from '@/utils/debounce'
import { mergeText } from '@/utils/merge'

// Files larger than this aren't loaded into the browser for viewing or editing.
export const FILE_CONTENT_SIZE_LIMIT = 2 * 1024 * 1024 // 2 MB

// Loads a file's text content and version on mount (aborted on unmount) and
// hands it to onLoaded. When the file's mod_time changes afterwards (reported by
// the folder watch stream) and the change isn't one of ours (see noteWritten),
// onChangedElsewhere runs; fetchContent gets the new content.
// Pair with <EditableFileState> to render the loading/error/too-large states.
export function useFileContent(
  entry: MaybeRefOrGetter<FileEntry>,
  onLoaded: (file: VersionedContent) => void,
  onChangedElsewhere?: () => void,
) {
  const loading   = ref(false)
  const loadError = ref(false)
  const tooLarge  = ref(false)
  let loaded = false
  let knownModTime = toValue(entry).mod_time
  let fetchController: AbortController | null = null

  onMounted(async () => {
    const file = toValue(entry)
    if (file.size > FILE_CONTENT_SIZE_LIMIT) {
      tooLarge.value = true
      return
    }
    loading.value = true
    fetchController = new AbortController()
    try {
      onLoaded(await filesApi.readContent(file.path, fetchController.signal))
      loaded = true
    } catch (err) {
      if ((err as Error).name !== 'AbortError') loadError.value = true
    } finally {
      loading.value = false
    }
  })

  onBeforeUnmount(() => fetchController?.abort())

  watch(() => toValue(entry).mod_time, (modTime) => {
    if (modTime === knownModTime) return
    knownModTime = modTime
    if (loaded) onChangedElsewhere?.()
  })

  function fetchContent(): Promise<VersionedContent> {
    return filesApi.readContent(toValue(entry).path)
  }

  // Records the mod_time of our own save, so its echo from the watch stream
  // isn't mistaken for a change made elsewhere.
  function noteWritten(modTime: string) {
    knownModTime = modTime
  }

  return { loading, loadError, tooLarge, fetchContent, noteWritten }
}

interface EditableFileOptions<T> {
  // Editor state; deep-watched, and every change after the initial load is auto-saved.
  state: Ref<T>
  parse: (content: string) => T
  serialize: (state: T) => string
  // Runs right after content is parsed into `state` (on load, and on reload after
  // a change made elsewhere) — changes made here (e.g. seeding an empty file)
  // don't count as edits and aren't saved on their own.
  onLoaded?: () => void
  // Gives entries of a freshly parsed state the ids of the same entries in the
  // previous state (see adoptIds in utils/merge.ts), so a reload or merge
  // doesn't close dialogs or move focus.
  carryOverIds?: (previous: T, next: T) => void
}

// Structured editor over a text file: parses the content into `state` on load,
// then auto-saves the serialized state 800 ms after each change (and flushes a
// pending save on unmount).
//
// Several people (or devices) can edit the same file: each save is based on the
// version last seen, and changes saved elsewhere — reported by the watch stream,
// or by a refused save — are merged into the editor line by line. Where both
// sides changed the same lines, the local version wins.
export function useEditableFile<T>(entry: MaybeRefOrGetter<FileEntry>, options: EditableFileOptions<T>) {
  const { show: showToast } = useToast()

  const ready     = ref(false) // gates auto-save until the initial content is loaded
  const saveState = ref<'idle' | 'saving' | 'saved'>('idle')
  const saveLabel = computed(() =>
    saveState.value === 'saving' ? 'Saving…' : saveState.value === 'saved' ? 'Saved' : '',
  )

  // The version on disk the editor is based on, and the editor's serialization
  // of it: the merge base, and what "no local changes" is measured against.
  let syncedVersion = ''
  let syncedSerialized = ''

  const serializeState = () => options.serialize(options.state.value)
  const hasLocalChanges = () => serializeState() !== syncedSerialized

  function showContent(content: string) {
    const next = options.parse(content)
    options.carryOverIds?.(options.state.value, next)
    options.state.value = next
  }

  function load(file: VersionedContent) {
    showContent(file.content)
    options.onLoaded?.()
    syncedVersion = file.version
    syncedSerialized = serializeState()
  }

  // Moves the editor onto `remote`, merging in unsaved local edits (the deep
  // watcher then saves the merged result).
  function applyRemote(remote: VersionedContent) {
    if (remote.version === syncedVersion) return
    if (!hasLocalChanges()) {
      load(remote)
      return
    }
    const theirs = options.serialize(options.parse(remote.content))
    const merged = mergeText(syncedSerialized, serializeState(), theirs, 'mine')
    showContent(merged.text)
    syncedVersion = remote.version
    syncedSerialized = theirs
    if (merged.conflicts) showToast('Merged with changes made elsewhere — where you both edited the same item, yours was kept', 'info')
  }

  const { loading, loadError, tooLarge, fetchContent, noteWritten } = useFileContent(
    entry,
    (file) => {
      load(file)
      nextTick(() => { ready.value = true })
    },
    () => enqueue(async () => applyRemote(await fetchContent())),
  )

  // Saves and pulls run one at a time, so a pull never lands between a save's
  // request and its bookkeeping.
  let syncQueue = Promise.resolve()
  function enqueue(task: () => Promise<void>) {
    syncQueue = syncQueue.then(task).catch((err) => {
      showToast((err as Error).message, 'error')
    })
  }

  let pendingSave = false

  async function saveNow() {
    pendingSave = false
    if (!hasLocalChanges()) return
    saveState.value = 'saving'
    try {
      // A refused save is merged with the newer version and retried.
      for (let attempt = 0; attempt < 3 && hasLocalChanges(); attempt++) {
        const content = serializeState()
        try {
          const saved = await filesApi.writeContent(toValue(entry).path, content, syncedVersion)
          syncedVersion = saved.version
          syncedSerialized = content
          noteWritten(saved.mod_time)
          break
        } catch (err) {
          if (!(err instanceof EditConflictError)) throw err
          applyRemote(err)
        }
      }
      saveState.value = 'saved'
    } catch (err) {
      saveState.value = 'idle'
      throw err
    }
  }
  const debouncedSave = debounce(() => enqueue(saveNow), 800)

  watch(
    options.state,
    () => {
      if (!ready.value) return
      pendingSave = true
      debouncedSave()
    },
    { deep: true },
  )

  onBeforeUnmount(() => {
    debouncedSave.cancel()
    if (pendingSave) enqueue(saveNow)
  })

  return { loading, loadError, tooLarge, saveLabel }
}
