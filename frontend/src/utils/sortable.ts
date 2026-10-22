// Helpers for lists dragged with vue-draggable-plus (SortableJS underneath).

interface DragOverEvent {
  clientX: number
  clientY: number
  target: Element
  rootEl: Element
}

interface SortableInstance {
  _onDragOver(event: DragOverEvent): unknown
}

// SortableJS keeps its instance on the list element under a key named "Sortable" plus a
// timestamp; vue-draggable-plus bundles it without exporting Sortable.get(). _onDragOver is
// private too — recheck both on every vue-draggable-plus upgrade (CLAUDE.md, Drag and drop).
export function sortableOf(list: Element): SortableInstance | null {
  const properties = list as unknown as Record<string, unknown>
  const key = Object.keys(properties).find((name) => name.startsWith('Sortable'))
  const instance = key ? (properties[key] as Partial<SortableInstance> | undefined) : undefined
  return typeof instance?._onDragOver === 'function' ? (instance as SortableInstance) : null
}

export interface HorizontalSpan {
  left: number
  right: number
}

// Index of the span horizontally nearest to x (any span containing it is at distance 0,
// the first wins a tie); -1 when there are none.
export function nearestSpanIndex(spans: HorizontalSpan[], x: number): number {
  let nearestIndex = -1
  let nearestDistance = Infinity
  spans.forEach((span, index) => {
    const distance = x < span.left ? span.left - x : x > span.right ? x - span.right : 0
    if (distance < nearestDistance) {
      nearestIndex = index
      nearestDistance = distance
    }
  })
  return nearestIndex
}

// With force-fallback, SortableJS only notices a list while the pointer is over it. Called
// on each pointer move of a drag, this lets the whole height above and below the lists
// count too: a pointer over none of them is handed to the horizontally nearest one, as if
// it were on that list's centre line at the same height (so above its items puts the
// dragged item first, below them last).
export function dragOverNearestList(lists: HTMLElement[], clientX: number, clientY: number) {
  const hit = document.elementFromPoint(clientX, clientY)
  if (hit && lists.some((list) => list.contains(hit))) return

  const rects = lists.map((list) => list.getBoundingClientRect())
  const index = nearestSpanIndex(rects, clientX)
  if (index < 0) return
  const list = lists[index]!
  const centreX = rects[index]!.left + rects[index]!.width / 2
  const hitOnCentre = document.elementFromPoint(centreX, clientY)
  sortableOf(list)?._onDragOver({
    clientX: centreX,
    clientY,
    target: hitOnCentre && list.contains(hitOnCentre) ? hitOnCentre : list,
    rootEl: list,
  })
}
