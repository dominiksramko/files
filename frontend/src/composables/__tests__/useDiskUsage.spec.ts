import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import type { UsageListing } from '@/types'
import { useDiskUsage } from '@/composables/useDiskUsage'
import { ScanNeededError, usage as usageApi } from '@/utils/api'

// Stands in for EventSource: the test sends the server's events with emit().
class FakeEventSource {
  static opened: FakeEventSource[] = []
  url: string
  closed = false
  onerror: (() => void) | null = null
  listeners = new Map<string, ((event: { data: string }) => void)[]>()
  constructor(url: string) {
    this.url = url
    FakeEventSource.opened.push(this)
  }
  addEventListener(name: string, listener: (event: { data: string }) => void) {
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), listener])
  }
  emit(name: string, payload: unknown = {}) {
    for (const listener of this.listeners.get(name) ?? []) listener({ data: JSON.stringify(payload) })
  }
  close() {
    this.closed = true
  }
}

function listing(at: string): UsageListing {
  const totals = { files: 1, dirs: 0, disk: 4096, apparent: 10 }
  return { root: 'media', at, scanned_at: '2026-10-20T12:00:00Z', unreadable: 0, root_total: totals, total: totals, entries: [] }
}

function setup(at = 'media') {
  const root = ref('media')
  const shownAt = ref(at)
  const scope = effectScope()
  const state = scope.run(() => useDiskUsage(root, shownAt))!
  return { root, at: shownAt, scope, ...state }
}

beforeEach(() => {
  FakeEventSource.opened = []
  vi.stubGlobal('EventSource', FakeEventSource)
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('useDiskUsage', () => {
  it('shows a scanned folder straight away', async () => {
    vi.spyOn(usageApi, 'list').mockImplementation(async (_root, at) => listing(at))
    const { listing: shown, at, scanning } = setup()
    await vi.waitFor(() => expect(shown.value?.at).toBe('media'))
    at.value = 'media/clips'
    await vi.waitFor(() => expect(shown.value?.at).toBe('media/clips'))
    expect(scanning.value).toBe(false)
    expect(FakeEventSource.opened).toHaveLength(0)
  })

  it('scans a folder without a scan, with progress, then shows it', async () => {
    let scanned = false
    vi.spyOn(usageApi, 'list').mockImplementation(async (_root, at) => {
      if (!scanned) throw new ScanNeededError('not scanned')
      return listing(at)
    })
    const { listing: shown, scanning, queued, progress } = setup()
    await vi.waitFor(() => expect(FakeEventSource.opened).toHaveLength(1))
    const source = FakeEventSource.opened[0]!
    expect(source.url).toBe('/api/v1/files/usage/scan?path=media')
    expect(scanning.value).toBe(true)

    source.emit('queued')
    expect(queued.value).toBe(true)
    source.emit('progress', { files: 5, dirs: 1, disk: 8192, apparent: 500 })
    expect(queued.value).toBe(false)
    expect(progress.value?.files).toBe(5)

    scanned = true
    source.emit('done', { scanned_at: '2026-10-20T12:00:00Z' })
    expect(source.closed).toBe(true)
    expect(scanning.value).toBe(false)
    await vi.waitFor(() => expect(shown.value?.at).toBe('media'))
  })

  it('reports a failed scan and doesn\'t reconnect after a broken one', async () => {
    vi.spyOn(usageApi, 'list').mockRejectedValue(new ScanNeededError('not scanned'))
    const { error, scanning, rescan } = setup()
    await vi.waitFor(() => expect(FakeEventSource.opened).toHaveLength(1))
    FakeEventSource.opened[0]!.emit('failed', { error: 'not a folder' })
    expect(error.value).toBe('not a folder')
    expect(FakeEventSource.opened[0]!.closed).toBe(true)

    rescan()
    const second = FakeEventSource.opened[1]!
    expect(scanning.value).toBe(true)
    expect(error.value).toBeNull()
    second.onerror?.()
    expect(second.closed).toBe(true)
    expect(scanning.value).toBe(false)
    expect(error.value).toMatch(/stopped/)
  })

  it('stops the scan when the page closes', async () => {
    vi.spyOn(usageApi, 'list').mockRejectedValue(new ScanNeededError('not scanned'))
    const { scope } = setup()
    await vi.waitFor(() => expect(FakeEventSource.opened).toHaveLength(1))
    scope.stop()
    expect(FakeEventSource.opened[0]!.closed).toBe(true)
  })

  it('drops an answer about a folder no longer shown', async () => {
    let answerFirst: (value: UsageListing) => void = () => {}
    vi.spyOn(usageApi, 'list').mockImplementation((_root, at) =>
      at === 'media/slow' ? new Promise((resolve) => { answerFirst = resolve }) : Promise.resolve(listing(at)))
    const { listing: shown, at } = setup('media/slow')
    await nextTick()
    at.value = 'media/fast'
    await vi.waitFor(() => expect(shown.value?.at).toBe('media/fast'))
    answerFirst(listing('media/slow'))
    await new Promise((resolve) => setTimeout(resolve))
    expect(shown.value?.at).toBe('media/fast')
  })

  it('deletes, then shows the new sizes and drops the deleted from the largest files', async () => {
    const list = vi.spyOn(usageApi, 'list').mockImplementation(async (_root, at) => listing(at))
    vi.spyOn(usageApi, 'largest').mockResolvedValue([
      { ...listing('media').root_total, name: 'big.bin', path: 'media/clips/big.bin', is_dir: false, size: 10, mod_time: '', mime: '', has_thumb: false, scanned: true },
      { ...listing('media').root_total, name: 'keep.bin', path: 'media/keep.bin', is_dir: false, size: 10, mod_time: '', mime: '', has_thumb: false, scanned: true },
    ])
    const remove = vi.spyOn(usageApi, 'delete').mockResolvedValue({ freed: { files: 1, dirs: 1, disk: 8192, apparent: 10 }, root_total: listing('media').root_total })
    const state = setup()
    await vi.waitFor(() => expect(state.listing.value).not.toBeNull())
    await state.loadLargest('disk')
    expect(state.largest.value).toHaveLength(2)

    const freed = await state.remove(['media/clips'])
    expect(freed.disk).toBe(8192)
    expect(remove).toHaveBeenCalledWith('media', ['media/clips'])
    expect(state.largest.value?.map((entry) => entry.path)).toEqual(['media/keep.bin'])
    expect(list).toHaveBeenCalledTimes(2)
  })
})
