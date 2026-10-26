import { onUnmounted, reactive } from 'vue'
import type { FileEntry } from '@/types'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { useToast } from '@/composables/useToast'
import { fileops } from '@/utils/api'

interface AttachmentUploadOptions {
  // Where the files go (made by the first upload).
  folder: () => string
  // What's known of a file there; a name known to be taken is asked about before sending.
  entryFor: (path: string) => FileEntry | null | undefined
  // Attaches the saved files to the card, wherever it is by then.
  attach: (cardId: number, paths: string[]) => void
}

// Uploads files for kanban cards, several cards at once: asks about names
// already taken (Replace / Keep all), then attaches the files under the names
// they were saved as. Uploads still running stop when the board is closed.
export function useAttachmentUploads({ folder, entryFor, attach }: AttachmentUploadOptions) {
  const { runWithConflictPrompt } = useConflictDialog()
  const { show: showToast } = useToast()

  const percents = reactive(new Map<number, number>()) // card id → percent sent
  const aborts = new Set<() => void>()

  async function upload(cardId: number, files: File[]) {
    const target = folder()
    const taken = files.map((file) => file.name).filter((name) => entryFor(`${target}/${name}`))
    try {
      const uploaded = await runWithConflictPrompt('Upload conflicts', (conflictMode) => {
        percents.set(cardId, 0)
        const sending = fileops.upload(target, files, (loaded, total) => percents.set(cardId, Math.round((loaded / total) * 100)), conflictMode)
        aborts.add(sending.abort)
        return sending.promise.finally(() => aborts.delete(sending.abort))
      }, taken)
      if (uploaded) attach(cardId, uploaded.names.map((name) => `${target}/${name}`))
    } catch (err) {
      if ((err as Error).message !== 'cancelled') showToast((err as Error).message, 'error')
    } finally {
      percents.delete(cardId)
    }
  }

  onUnmounted(() => aborts.forEach((abort) => abort()))

  return { percents, upload }
}
