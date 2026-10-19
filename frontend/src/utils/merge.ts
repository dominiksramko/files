import { diffIndices } from 'node-diff3'

// Three-way line merge for files edited in several places at once: `base` is the
// version both edits started from, `mine` the local edit, `theirs` the version
// saved elsewhere in the meantime.
//
// Unlike a classic diff3 merge, changes to neighbouring lines don't conflict —
// two people editing adjacent cards or tasks is the normal case. Only changes
// to the same base lines do; both sides inserting at the same spot keeps both.

// How an overlapping change is resolved: 'mine' keeps the local version of that
// region (structured editors that auto-save), 'markers' keeps both between
// conflict markers for the user to sort out (text editing).
export type ConflictStrategy = 'mine' | 'markers'

export interface MergeResult {
  text: string
  conflicts: number // regions changed differently on both sides
}

export const CONFLICT_MARKERS = {
  mine: '<<<<<<< your changes',
  separator: '=======',
  theirs: '>>>>>>> changes made elsewhere',
}

// One change against base: base lines [baseStart, baseEnd) replaced by `lines`.
interface Hunk {
  side: 'mine' | 'theirs'
  baseStart: number
  baseEnd: number
  lines: string[]
}

function hunksBetween(baseLines: string[], sideLines: string[], side: Hunk['side']): Hunk[] {
  return diffIndices(baseLines, sideLines).map((change) => ({
    side,
    baseStart: change.buffer1[0],
    baseEnd: change.buffer1[0] + change.buffer1[1],
    lines: sideLines.slice(change.buffer2[0], change.buffer2[0] + change.buffer2[1]),
  }))
}

// Base lines [start, end) with one side's hunks (all inside the range) applied.
function applyHunks(baseLines: string[], start: number, end: number, hunks: Hunk[]): string[] {
  const result: string[] = []
  let cursor = start
  for (const hunk of hunks) {
    result.push(...baseLines.slice(cursor, hunk.baseStart), ...hunk.lines)
    cursor = hunk.baseEnd
  }
  result.push(...baseLines.slice(cursor, end))
  return result
}

export function mergeText(base: string, mine: string, theirs: string, strategy: ConflictStrategy): MergeResult {
  if (mine === theirs || theirs === base) return { text: mine, conflicts: 0 }
  if (mine === base) return { text: theirs, conflicts: 0 }

  const baseLines = base.split('\n')
  const isInsertion = (hunk: Hunk) => hunk.baseStart === hunk.baseEnd
  // By position; at the same position insertions go first (they apply before a
  // change starting there), and theirs before mine (theirs was saved first).
  const hunks = [
    ...hunksBetween(baseLines, theirs.split('\n'), 'theirs'),
    ...hunksBetween(baseLines, mine.split('\n'), 'mine'),
  ].sort((first, second) =>
    first.baseStart - second.baseStart
    || Number(isInsertion(second)) - Number(isInsertion(first))
    || (first.side === 'theirs' ? -1 : 1),
  )

  // Group hunks touching the same base lines; each group is resolved on its own.
  const groups: Hunk[][] = []
  let groupEnd = -1
  for (const hunk of hunks) {
    const group = groups[groups.length - 1]
    const overlapsGroup = group !== undefined && (
      hunk.baseStart < groupEnd
      || (isInsertion(hunk) && group.every(isInsertion) && hunk.baseStart === group[0]!.baseStart)
    )
    if (overlapsGroup) {
      group.push(hunk)
      groupEnd = Math.max(groupEnd, hunk.baseEnd)
    } else {
      groups.push([hunk])
      groupEnd = hunk.baseEnd
    }
  }

  const merged: string[] = []
  let conflicts = 0
  let cursor = 0
  for (const group of groups) {
    const start = group[0]!.baseStart
    const end = Math.max(...group.map((hunk) => hunk.baseEnd))
    merged.push(...baseLines.slice(cursor, start))
    cursor = end

    const mineHunks = group.filter((hunk) => hunk.side === 'mine')
    const theirHunks = group.filter((hunk) => hunk.side === 'theirs')
    if (!mineHunks.length || !theirHunks.length) {
      merged.push(...applyHunks(baseLines, start, end, group))
      continue
    }
    const mineLines = applyHunks(baseLines, start, end, mineHunks)
    const theirLines = applyHunks(baseLines, start, end, theirHunks)
    if (mineLines.join('\n') === theirLines.join('\n')) {
      merged.push(...mineLines) // the same change made on both sides
    } else if (start === end) {
      merged.push(...theirLines, ...mineLines) // both inserted here: keep both
    } else if (strategy === 'mine') {
      conflicts++
      merged.push(...mineLines)
    } else {
      conflicts++
      merged.push(CONFLICT_MARKERS.mine, ...mineLines, CONFLICT_MARKERS.separator, ...theirLines, CONFLICT_MARKERS.theirs)
    }
  }
  merged.push(...baseLines.slice(cursor))
  return { text: merged.join('\n'), conflicts }
}

// After re-parsing a merged file, gives entries that still exist the ids they
// had before (matched by key, in order), so open dialogs, focus and list
// animations stay attached to the same card / task.
export function adoptIds<Item extends { id: number }>(
  previous: Item[],
  next: Item[],
  keyOf: (item: Item) => string,
  onMatch?: (previousItem: Item, nextItem: Item) => void,
) {
  const unmatched = new Map<string, Item[]>()
  for (const item of previous) {
    const key = keyOf(item)
    unmatched.set(key, [...(unmatched.get(key) ?? []), item])
  }
  for (const item of next) {
    const match = unmatched.get(keyOf(item))?.shift()
    if (!match) continue
    item.id = match.id
    onMatch?.(match, item)
  }
}
