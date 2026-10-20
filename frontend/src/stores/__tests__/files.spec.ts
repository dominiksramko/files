// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useFilesStore } from '@/stores/files'
import { files as filesApi, NotFoundError } from '@/utils/api'
import type { FileEntry } from '@/types'

// A stand-in EventSource that records what was opened and closed.
class FakeEventSource {
  static opened: FakeEventSource[] = []
  closed = false
  onerror: (() => void) | null = null
  private listeners = new Map<string, (event: MessageEvent) => void>()
  constructor(public url: string) { FakeEventSource.opened.push(this) }
  addEventListener(name: string, listener: (event: MessageEvent) => void) { this.listeners.set(name, listener) }
  close() { this.closed = true }
  emit(name: string, data: unknown) { this.listeners.get(name)?.({ data: JSON.stringify(data) } as MessageEvent) }
}

beforeEach(() => {
  setActivePinia(createPinia())
  FakeEventSource.opened = []
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('files store reset', () => {
  it('closes the live stream, drops the listing and never reconnects', async () => {
    const files = useFilesStore()
    const navigation = files.navigate('docs')
    const stream = FakeEventSource.opened[0]!
    expect(stream.url).toBe('/api/v1/files/watch?path=docs')
    stream.emit('snapshot', [{ name: 'secret.txt', path: 'docs/secret.txt' } as FileEntry])
    await navigation
    expect(files.entries).toHaveLength(1)

    files.reset()
    expect(stream.closed).toBe(true)
    expect(files.entries).toEqual([])
    expect(files.currentPath).toBe('')

    stream.onerror?.()
    window.dispatchEvent(new Event('online'))
    await vi.advanceTimersByTimeAsync(120_000)
    expect(FakeEventSource.opened).toHaveLength(1)
  })

  it('asks for the listing when the stream fails (a 401 there ends the session)', async () => {
    const list = vi.spyOn(filesApi, 'list').mockRejectedValue(new Error('offline'))
    const files = useFilesStore()
    void files.navigate('docs').catch(() => {})
    FakeEventSource.opened[0]!.onerror?.()
    expect(list).toHaveBeenCalledWith('docs')
    files.reset()
  })

  it('stops following a folder that is gone, and says where to go', async () => {
    vi.spyOn(filesApi, 'list').mockRejectedValue(new NotFoundError('Directory not found', 'projects'))
    const files = useFilesStore()
    const navigation = files.navigate('projects/old')
    const stream = FakeEventSource.opened[0]!
    stream.emit('snapshot', [])
    await navigation
    stream.onerror?.()
    await vi.advanceTimersByTimeAsync(1_000)
    expect(files.folderGone).toBeNull() // given a moment to reappear
    await vi.advanceTimersByTimeAsync(5_000)
    expect(files.folderGone).toBe('projects')
    const opened = FakeEventSource.opened.length
    await vi.advanceTimersByTimeAsync(120_000)
    expect(FakeEventSource.opened).toHaveLength(opened) // no more reconnects to the missing folder
  })

  it('keeps following a folder that comes back', async () => {
    const list = vi.spyOn(filesApi, 'list').mockRejectedValueOnce(new NotFoundError('Directory not found', 'projects'))
    list.mockResolvedValue([])
    const files = useFilesStore()
    const navigation = files.navigate('projects/synced')
    FakeEventSource.opened[0]!.emit('snapshot', [])
    await navigation
    FakeEventSource.opened[0]!.onerror?.()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(files.folderGone).toBeNull()
    files.reset()
  })
})
