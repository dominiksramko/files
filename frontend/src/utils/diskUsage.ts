import type { SizeMeasure, UsageEntry, UsageTotals } from '@/types'
import { parentOf } from '@/utils/paths'

export function sizeOf(item: UsageTotals, measure: SizeMeasure): number {
  return measure === 'apparent' ? item.apparent : item.disk
}

// Largest first by measure, by name when equal.
export function sortBySize(entries: UsageEntry[], measure: SizeMeasure): UsageEntry[] {
  return [...entries].sort((a, b) => sizeOf(b, measure) - sizeOf(a, measure) || a.name.localeCompare(b.name))
}

// What a folder's items are parts of: the scan's total for it (which counts
// space shared by hard links once), or its items added up when the scan
// doesn't have the folder (made since).
export function folderSize(total: UsageTotals, entries: UsageEntry[], measure: SizeMeasure): number {
  if (total.files > 0 || total.dirs > 0 || sizeOf(total, measure) > 0) return sizeOf(total, measure)
  return entries.reduce((sum, entry) => sum + sizeOf(entry, measure), 0)
}

// The part of total that size is, from 0 to 1.
export function shareOf(size: number, total: number): number {
  if (total <= 0 || size <= 0) return 0
  return Math.min(1, size / total)
}

export function formatShare(share: number): string {
  if (share <= 0) return '0%'
  if (share < 0.01) return '<1%'
  return `${Math.round(share * 100)}%`
}

// Whether the page can go into entry (a folder, not a link to one).
export function canOpen(entry: UsageEntry): boolean {
  return entry.is_dir && !entry.link
}

// Where "Show in Files" goes: a folder opens, a file is previewed in its folder.
export function filesLocation(entry: UsageEntry): { path?: string; preview?: string } {
  const folder = entry.is_dir ? entry.path : parentOf(entry.path)
  const query: { path?: string; preview?: string } = folder ? { path: folder } : {}
  if (!entry.is_dir) query.preview = entry.path
  return query
}

// The page's URL query: the scanned folder, the folder shown, the tab and the
// size measure, leaving out the defaults.
export function usageQuery(state: { root: string; at: string; tab: 'contents' | 'largest'; measure: SizeMeasure }): Record<string, string> {
  const query: Record<string, string> = {}
  if (state.root) query.path = state.root
  if (state.at !== state.root) query.at = state.at
  if (state.tab === 'largest') query.tab = 'largest'
  if (state.measure === 'apparent') query.size = 'apparent'
  return query
}

export interface SmallEntries {
  count: number
  size: number
}

// Splits entries (largest first) into those big enough to show as tiles, at
// least minShare of total, and the rest, added up into one tile.
export function groupSmallEntries(entries: UsageEntry[], measure: SizeMeasure, total: number, minShare: number): { shown: UsageEntry[]; rest: SmallEntries | null } {
  const shown: UsageEntry[] = []
  const rest: SmallEntries = { count: 0, size: 0 }
  for (const entry of entries) {
    const size = sizeOf(entry, measure)
    if (size <= 0) continue
    if (shareOf(size, total) >= minShare) shown.push(entry)
    else {
      rest.count++
      rest.size += size
    }
  }
  return { shown, rest: rest.count > 0 ? rest : null }
}

export interface TreemapTile<T> {
  item: T
  x: number
  y: number
  width: number
  height: number
}

// Lays items out as rectangles filling width × height, each with an area in
// proportion to its size, kept as close to square as it can (the squarified
// treemap). Items are placed largest first; those without a size are left out.
export function squarify<T>(items: { item: T; size: number }[], width: number, height: number): TreemapTile<T>[] {
  const sized = items.filter((entry) => entry.size > 0).sort((a, b) => b.size - a.size)
  const total = sized.reduce((sum, entry) => sum + entry.size, 0)
  if (!sized.length || width <= 0 || height <= 0) return []
  const scale = (width * height) / total
  const areas = sized.map((entry) => ({ item: entry.item, area: entry.size * scale }))

  const tiles: TreemapTile<T>[] = []
  let left = 0
  let top = 0
  let freeWidth = width
  let freeHeight = height

  // How far from square the worst tile of row would be, laid along side.
  const worstRatio = (row: typeof areas, side: number) => {
    const rowArea = row.reduce((sum, entry) => sum + entry.area, 0)
    const largest = Math.max(...row.map((entry) => entry.area))
    const smallest = Math.min(...row.map((entry) => entry.area))
    return Math.max((side * side * largest) / (rowArea * rowArea), (rowArea * rowArea) / (side * side * smallest))
  }

  // Places row along the shorter side of the free space, and takes it off.
  const placeRow = (row: typeof areas) => {
    const rowArea = row.reduce((sum, entry) => sum + entry.area, 0)
    if (freeWidth >= freeHeight) {
      const columnWidth = rowArea / freeHeight
      let y = top
      for (const entry of row) {
        const tileHeight = entry.area / columnWidth
        tiles.push({ item: entry.item, x: left, y, width: columnWidth, height: tileHeight })
        y += tileHeight
      }
      left += columnWidth
      freeWidth -= columnWidth
    } else {
      const rowHeight = rowArea / freeWidth
      let x = left
      for (const entry of row) {
        const tileWidth = entry.area / rowHeight
        tiles.push({ item: entry.item, x, y: top, width: tileWidth, height: rowHeight })
        x += tileWidth
      }
      top += rowHeight
      freeHeight -= rowHeight
    }
  }

  let row: typeof areas = []
  for (const entry of areas) {
    const side = Math.min(freeWidth, freeHeight)
    if (row.length === 0 || worstRatio([...row, entry], side) <= worstRatio(row, side)) {
      row.push(entry)
    } else {
      placeRow(row)
      row = [entry]
    }
  }
  if (row.length) placeRow(row)
  return tiles
}
