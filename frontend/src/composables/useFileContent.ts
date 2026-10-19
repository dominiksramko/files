import { computed, nextTick, onBeforeUnmount, onMounted, ref, toValue, watch, type MaybeRefOrGetter, type Ref } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { useFilesStore } from '@/stores/files'
import { useToast } from '@/composables/useToast'
import { debounce } from '@/utils/debounce'

// Files larger than this aren't loaded into the browser for viewing or editing.
export const FILE_CONTENT_SIZE_LIMIT = 2 * 1024 * 1024 // 2 MB

// Loads a file's text content on mount (aborted on unmount) and hands it to
// onLoaded. Pair with <EditableFileState> to render the loading/error/too-large states.
export function useFileContent(
  entry: MaybeRefOrGetter<FileEntry>,
  onLoaded: (content: string) => void,
) {
  const loading   = ref(false)
  const loadError = ref(false)
  const tooLarge  = ref(false)
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
      const response = await fetch(filesApi.streamUrl(file.path, file.mod_time), {
        signal: fetchController.signal,
      })
      if (!response.ok) throw new Error()
      onLoaded(await response.text())
    } catch (err) {
      if ((err as Error).name !== 'AbortError') loadError.value = true
    } finally {
      loading.value = false
    }
  })

  onBeforeUnmount(() => fetchController?.abort())

  return { loading, loadError, tooLarge }
}

interface EditableFileOptions<T> {
  // Editor state; deep-watched, and every change after the initial load is auto-saved.
  state: Ref<T>
  parse: (content: string) => T
  serialize: (state: T) => string
  // Runs right after the initial content is parsed into `state` — changes made
  // here (e.g. seeding an empty file) don't trigger a save.
  onLoaded?: () => void
}

// Structured editor over a text file: parses the content into `state` on load,
// then auto-saves the serialized state 800 ms after each change (and flushes a
// pending save on unmount).
export function useEditableFile<T>(entry: MaybeRefOrGetter<FileEntry>, options: EditableFileOptions<T>) {
  const filesStore = useFilesStore()
  const { show: showToast } = useToast()

  const ready     = ref(false) // gates auto-save until the initial content is loaded
  const saveState = ref<'idle' | 'saving' | 'saved'>('idle')
  const saveLabel = computed(() =>
    saveState.value === 'saving' ? 'Saving…' : saveState.value === 'saved' ? 'Saved' : '',
  )

  const { loading, loadError, tooLarge } = useFileContent(entry, (content) => {
    options.state.value = options.parse(content)
    options.onLoaded?.()
    nextTick(() => { ready.value = true })
  })

  let pendingSave = false

  async function doSave() {
    pendingSave = false
    saveState.value = 'saving'
    try {
      await filesApi.writeContent(toValue(entry).path, options.serialize(options.state.value))
      saveState.value = 'saved'
      filesStore.refresh()
    } catch (err) {
      saveState.value = 'idle'
      showToast((err as Error).message, 'error')
    }
  }
  const debouncedSave = debounce(doSave, 800)

  watch(
    options.state,
    () => {
      if (!ready.value) return
      pendingSave = true
      saveState.value = 'saving'
      debouncedSave()
    },
    { deep: true },
  )

  onBeforeUnmount(() => {
    debouncedSave.cancel()
    if (pendingSave) doSave()
  })

  return { loading, loadError, tooLarge, saveLabel }
}
