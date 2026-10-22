// Where the slide to the next or previous panel starts, in px from the middle
// panel, given where the track is drawn now (mid-slide too) and the panel
// width. The panels move over by one at once, so the picture is first put
// back where it was, then slides to the middle; never more than one panel
// away, as only the neighbours either side are there to show.
export function slideStart(shownOffset: number, direction: 'prev' | 'next', panelWidth: number): number {
  const start = shownOffset + (direction === 'next' ? panelWidth : -panelWidth)
  return Math.max(-panelWidth, Math.min(panelWidth, start))
}

// The horizontal translation of a computed CSS transform ("none", "matrix(…)"
// or "matrix3d(…)"), or null if it can't be read.
export function translateXOf(transform: string): number | null {
  if (transform === 'none') return 0
  const match = /^matrix(3d)?\(([^)]+)\)$/.exec(transform)
  if (!match) return null
  const values = match[2]!.split(',').map(Number)
  const translateX = match[1] ? values[12] : values[4]
  return values.length === (match[1] ? 16 : 6) && Number.isFinite(translateX) ? translateX! : null
}
