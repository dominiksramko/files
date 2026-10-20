import { files as filesApi } from '@/utils/api'
import { useConflictDialog } from '@/composables/useConflictDialog'
import { usePasswordPrompt } from '@/composables/usePasswordPrompt'

// Render <PasswordDialog :prompt="passwordPrompt" … /> where extraction is
// started (inside a modal that starts it), for the password asked for.
export function useArchiveExtract() {
  const { runWithConflictPrompt } = useConflictDialog()
  const passwordPrompt = usePasswordPrompt()

  // Extracts the archive next to itself. Asks for a password when the server
  // needs one — before trying, when the listing showed encrypted files — and
  // asks first (shared conflict dialog) if files would be overwritten; the
  // password that worked is used again for that. Resolves null when the user
  // cancels; other errors are thrown.
  function extractArchive(path: string, { password, encrypted = false }: { password?: string; encrypted?: boolean } = {}) {
    let accepted = password
    return runWithConflictPrompt('Extraction conflicts', (conflictMode) =>
      passwordPrompt.runWithPasswordPrompt(
        (tried) => {
          accepted = tried
          return filesApi.extract(path, undefined, tried, conflictMode)
        },
        { password: accepted, askFirst: encrypted },
      ),
    )
  }

  return { extractArchive, passwordPrompt }
}
