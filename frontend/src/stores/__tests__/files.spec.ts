// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useFilesStore } from '@/stores/files'
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
})
