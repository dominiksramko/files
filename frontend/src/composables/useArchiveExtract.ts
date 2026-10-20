import { files as filesApi } from '@/utils/api'
import { useConflictDialog } from '@/composables/useConflictDialog'

export function useArchiveExtract() {
  const { runWithConflictPrompt } = useConflictDialog()

  // Extracts the archive next to itself, asking first (shared conflict dialog)
  // if files would be overwritten. Resolves null when the user cancels; other
  // errors (e.g. a wrong password) are thrown.
  function extractArchive(path: string, password?: string) {
    return runWithConflictPrompt('Extraction conflicts', (conflictMode) =>
      filesApi.extract(path, undefined, password, conflictMode),
    )
  }

  return { extractArchive }
}
