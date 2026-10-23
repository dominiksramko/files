// A layer is something opened over the page and named in the URL: the file
// preview (?preview=), a kanban card's editor (?card=).

// Keys marking a browser history entry a layer added when it opened; the
// router keeps them through replace(), so they survive moving between files
// or renaming a card.
export const PREVIEW_OPENED_STATE = 'previewOpened'
export const CARD_OPENED_STATE = 'cardOpened'

export type LayerHistoryStep = 'push' | 'replace' | 'back' | 'none'

/**
 * How the browser history follows a layer, given what it shows (null:
 * closed), what the URL names, and whether the current history entry was
 * added by the layer opening. Opening adds an entry, so Back (a phone's own
 * button) closes the layer rather than leaving the page under it; showing
 * something else replaces it, so Back doesn't step through every file seen;
 * closing goes back to the entry underneath rather than adding one, so Back
 * after closing doesn't open the layer again.
 */
export function layerHistoryStep(shown: string | null, inUrl: string | null, openedByLayer: boolean): LayerHistoryStep {
  if (shown === inUrl) return 'none'
  if (inUrl === null) return 'push'
  if (shown === null) return openedByLayer ? 'back' : 'replace'
  return 'replace'
}
