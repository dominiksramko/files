// The key marking a browser history entry the preview added when it opened;
// the router keeps it through replace(), so it survives moving between files.
export const PREVIEW_OPENED_STATE = 'previewOpened'

export type PreviewHistoryStep = 'push' | 'replace' | 'back' | 'none'

/**
 * How the browser history follows the preview, given the file shown (null:
 * closed), the file the URL names, and whether the current history entry was
 * added by the preview opening. Opening adds an entry, so Back (a phone's own
 * button) closes the preview rather than leaving the folder; moving to
 * another file replaces it, so Back doesn't step through every file seen;
 * closing goes back to the folder's entry rather than adding one, so Back
 * after closing doesn't open the preview again.
 */
export function previewHistoryStep(shown: string | null, inUrl: string | null, openedByPreview: boolean): PreviewHistoryStep {
  if (shown === inUrl) return 'none'
  if (inUrl === null) return 'push'
  if (shown === null) return openedByPreview ? 'back' : 'replace'
  return 'replace'
}
