import { describe, expect, it } from 'vitest'
import type { UsageEntry } from '@/types'
import {
  canOpen,
  filesLocation,
  folderSize,
  formatShare,
  groupSmallEntries,
  shareOf,
  sizeOf,
  sortBySize,
  squarify,
  usageStateFrom,
  withUsageState,
} from '../diskUsage'

function entry(path: string, disk: number, apparent = disk, extra: Partial<UsageEntry> = {}): UsageEntry {
  return {
    name: path.split('/').pop()!,
    path,
    is_dir: false,
    size: apparent,
    mod_time: '2026-10-20T10:00:00Z',
    mime: '',
    has_thumb: false,
    files: 0,
    dirs: 0,
    disk,
    apparent,
    scanned: false,
    ...extra,
  }
}

describe('sizes', () => {
  it('picks the measure', () => {
    const item = entry('a', 4096, 10)
    expect(sizeOf(item, 'disk')).toBe(4096)
    expect(sizeOf(item, 'apparent')).toBe(10)
  })

  it('sorts largest first by the measure, then by name', () => {
    const entries = [entry('b', 100, 5), entry('a', 100, 50), entry('c', 10, 500)]
    expect(sortBySize(entries, 'disk').map((item) => item.name)).toEqual(['a', 'b', 'c'])
    expect(sortBySize(entries, 'apparent').map((item) => item.name)).toEqual(['c', 'a', 'b'])
    expect(entries[0]!.name).toBe('b') // not sorted in place
  })

  it('takes a folder\'s size from the scan, or adds up its items', () => {
    const sharing = [entry('big.iso', 2000), entry('hard.iso', 2000)] // hard links: one space
    expect(folderSize({ files: 2, dirs: 0, disk: 2000, apparent: 4000 }, sharing, 'disk')).toBe(2000)
    expect(folderSize({ files: 0, dirs: 0, disk: 0, apparent: 0 }, sharing, 'disk')).toBe(4000)
    expect(folderSize({ files: 0, dirs: 0, disk: 0, apparent: 0 }, [], 'disk')).toBe(0)
  })

  it('gives shares of a total', () => {
    expect(shareOf(25, 100)).toBe(0.25)
    expect(shareOf(5, 0)).toBe(0)
    expect(shareOf(150, 100)).toBe(1) // the scan's total is older than the item
    expect(formatShare(0.614)).toBe('61%')
    expect(formatShare(0.004)).toBe('<1%')
    expect(formatShare(0)).toBe('0%')
  })
})

describe('locations', () => {
  it('goes into folders, not into links to them', () => {
    expect(canOpen(entry('media', 0, 0, { is_dir: true }))).toBe(true)
    expect(canOpen(entry('to-media', 0, 0, { is_dir: true, link: true }))).toBe(false)
    expect(canOpen(entry('a.txt', 0))).toBe(false)
  })

  it('shows a file in its folder and opens a folder', () => {
    expect(filesLocation(entry('media/big.bin', 1))).toEqual({ path: 'media', preview: 'media/big.bin' })
    expect(filesLocation(entry('top.txt', 1))).toEqual({ preview: 'top.txt' })
    expect(filesLocation(entry('media/clips', 1, 1, { is_dir: true }))).toEqual({ path: 'media/clips' })
  })
})

describe('the modal in the URL', () => {
  it('is closed without ?usage, and opens on the home folder with it empty', () => {
    expect(usageStateFrom({ path: 'docs' })).toBeNull()
    expect(usageStateFrom({ usage: '' })).toEqual({ root: '', at: '', tab: 'contents', measure: 'disk' })
    expect(usageStateFrom({ usage: null })).toEqual({ root: '', at: '', tab: 'contents', measure: 'disk' })
  })

  it('reads the folder looked at, the tab and the measure', () => {
    expect(usageStateFrom({ usage: 'media', usage_at: 'media/clips', usage_tab: 'largest', usage_size: 'apparent' }))
      .toEqual({ root: 'media', at: 'media/clips', tab: 'largest', measure: 'apparent' })
    expect(usageStateFrom({ usage: '', usage_at: 'media' })?.at).toBe('media')
  })

  it('shows the scanned folder for a folder outside it', () => {
    expect(usageStateFrom({ usage: 'media', usage_at: 'docs' })?.at).toBe('media')
    expect(usageStateFrom({ usage: 'media', usage_at: 'media-old/x' })?.at).toBe('media')
    expect(usageStateFrom({ usage: 'media', usage_tab: 'other', usage_size: 'other' }))
      .toEqual({ root: 'media', at: 'media', tab: 'contents', measure: 'disk' })
  })

  it('puts only what differs from the defaults in the URL, next to the browser\'s own keys', () => {
    expect(withUsageState({ path: 'docs', preview: 'docs/a.txt' }, { root: '', at: '', tab: 'contents', measure: 'disk' }))
      .toEqual({ path: 'docs', preview: 'docs/a.txt', usage: '' })
    expect(withUsageState({ path: 'docs' }, { root: 'media', at: 'media/clips', tab: 'largest', measure: 'apparent' }))
      .toEqual({ path: 'docs', usage: 'media', usage_at: 'media/clips', usage_tab: 'largest', usage_size: 'apparent' })
  })

  it('replaces the previous state, and closing removes it all', () => {
    const open = { path: 'docs', usage: 'media', usage_at: 'media/clips', usage_tab: 'largest', usage_size: 'apparent' }
    expect(withUsageState(open, { root: 'media', at: 'media', tab: 'contents', measure: 'disk' })).toEqual({ path: 'docs', usage: 'media' })
    expect(withUsageState(open, null)).toEqual({ path: 'docs' })
  })

  it('reads back what it writes', () => {
    const state = { root: 'media', at: 'media/clips', tab: 'largest', measure: 'apparent' } as const
    expect(usageStateFrom(withUsageState({}, state))).toEqual(state)
  })
})

describe('groupSmallEntries', () => {
  it('adds up what is too small for a tile', () => {
    const entries = [entry('big', 900), entry('mid', 80), entry('tiny1', 15), entry('tiny2', 5), entry('empty', 0)]
    const { shown, rest } = groupSmallEntries(entries, 'disk', 1000, 0.05)
    expect(shown.map((item) => item.name)).toEqual(['big', 'mid'])
    expect(rest).toEqual({ count: 2, size: 20 })
  })

  it('has no rest when everything shows', () => {
    expect(groupSmallEntries([entry('a', 50), entry('b', 50)], 'disk', 100, 0.01).rest).toBeNull()
  })
})

describe('squarify', () => {
  const area = (tile: { width: number; height: number }) => tile.width * tile.height
  const overlaps = (a: { x: number; y: number; width: number; height: number }, b: typeof a) =>
    a.x + 1e-6 < b.x + b.width && b.x + 1e-6 < a.x + a.width && a.y + 1e-6 < b.y + b.height && b.y + 1e-6 < a.y + a.height

  it('fills the space with areas in proportion to the sizes', () => {
    const sizes = [6, 6, 4, 3, 2, 2, 1]
    const tiles = squarify(sizes.map((size, index) => ({ item: index, size })), 600, 400)
    expect(tiles).toHaveLength(sizes.length)
    const total = sizes.reduce((sum, size) => sum + size, 0)
    for (const tile of tiles) {
      expect(area(tile)).toBeCloseTo((sizes[tile.item]! / total) * 600 * 400, 6)
      expect(tile.x).toBeGreaterThanOrEqual(-1e-6)
      expect(tile.y).toBeGreaterThanOrEqual(-1e-6)
      expect(tile.x + tile.width).toBeLessThanOrEqual(600 + 1e-6)
      expect(tile.y + tile.height).toBeLessThanOrEqual(400 + 1e-6)
    }
    for (let i = 0; i < tiles.length; i++) {
      for (let j = i + 1; j < tiles.length; j++) {
        expect(overlaps(tiles[i]!, tiles[j]!)).toBe(false)
      }
    }
  })

  it('keeps tiles close to square', () => {
    const tiles = squarify(Array.from({ length: 16 }, (_, index) => ({ item: index, size: 1 })), 400, 400)
    for (const tile of tiles) {
      expect(Math.max(tile.width / tile.height, tile.height / tile.width)).toBeLessThan(2.5)
    }
  })

  it('places the largest first', () => {
    const tiles = squarify([{ item: 'small', size: 1 }, { item: 'large', size: 9 }], 100, 100)
    expect(tiles[0]).toMatchObject({ item: 'large', x: 0, y: 0 })
  })

  it('gives one item the whole space and leaves out empty ones', () => {
    expect(squarify([{ item: 'a', size: 5 }, { item: 'b', size: 0 }], 300, 200)).toEqual([{ item: 'a', x: 0, y: 0, width: 300, height: 200 }])
    expect(squarify([], 300, 200)).toEqual([])
    expect(squarify([{ item: 'a', size: 5 }], 0, 200)).toEqual([])
  })
})
