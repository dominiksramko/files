// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import type { FileEntry } from '@/types'
import FilePreview from '@/components/files/FilePreview.vue'

// jsdom has no media queries, and the video player (plyr) asks one as it's
// imported: before the imports.
vi.hoisted(() => {
  window.matchMedia = (query: string) =>
    ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }) as unknown as MediaQueryList
})

const cleanups: (() => void)[] = []
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  vi.useRealTimers()
})

// A file with no viewer of its own, so the preview shows its fallback.
const entry: FileEntry = {
  name: 'data.bin', path: 'data.bin', is_dir: false, size: 3, mod_time: '2026-10-21T10:00:00Z', mime: 'application/octet-stream', has_thumb: false,
}

describe('FilePreview', () => {
  it('has the close button first in its top bar, on the left as in the modals', async () => {
    const closes: number[] = []
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => h(FilePreview, { entry, hasPrev: false, hasNext: false, index: 0, total: 1, onClose: () => closes.push(1) }),
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    await nextTick()

    const close = document.querySelector<HTMLButtonElement>('button[aria-label="Close"]')!
    const topBar = close.parentElement!
    expect(topBar.firstElementChild).toBe(close)
    expect(topBar.textContent).toContain('data.bin')
    close.click()
    expect(closes).toHaveLength(1)
  })

  // The preview over files that all show the fallback, moving between them as
  // FilesView does; shown: the open file's index, or null when closed.
  function mountOver(files: FileEntry[], shown: { value: number | null }, steps: string[]) {
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => {
        const index = shown.value
        return h(FilePreview, {
          entry: index === null ? null : files[index] ?? null,
          prevEntry: index === null ? null : files[index - 1] ?? null,
          nextEntry: index === null ? null : files[index + 1] ?? null,
          hasPrev: index !== null && index > 0,
          hasNext: index !== null && index < files.length - 1,
          index: index ?? -1,
          total: files.length,
          onPrev: () => { steps.push('prev'); shown.value = (shown.value ?? 0) - 1 },
          onNext: () => { steps.push('next'); shown.value = (shown.value ?? -1) + 1 },
          onClose: () => (shown.value = null),
        })
      },
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
  }

  const files: FileEntry[] = ['a.bin', 'b.bin', 'c.bin', 'd.bin'].map((name) => ({ ...entry, name, path: name }))

  it('moves on each arrow key at once, without waiting for the slide', async () => {
    vi.useFakeTimers()
    const steps: string[] = []
    const shown = ref<number | null>(0)
    mountOver(files, shown, steps)
    await nextTick()

    for (const key of ['ArrowRight', 'ArrowRight', 'ArrowRight', 'ArrowLeft']) {
      window.dispatchEvent(new KeyboardEvent('keydown', { key }))
      await nextTick()
    }
    expect(steps).toEqual(['next', 'next', 'next', 'prev'])
    expect(shown.value).toBe(2)
  })

  it("stays closed when it's closed while a slide is still running", async () => {
    vi.useFakeTimers()
    const steps: string[] = []
    const shown = ref<number | null>(1)
    mountOver(files, shown, steps)
    await nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    vi.advanceTimersByTime(1000)
    await nextTick()
    // A move that came only once the slide ended made FilesView open the folder's first file.
    expect(steps).toEqual(['next'])
    expect(shown.value).toBeNull()
  })
})

describe('FilePreview and Escape', () => {
  async function mountPreview() {
    const closes: number[] = []
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => h(FilePreview, { entry, hasPrev: false, hasNext: false, index: 0, total: 1, onClose: () => closes.push(1) }),
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
    await nextTick()
    return closes
  }
  const escape = () => new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })

  it('closes on Escape', async () => {
    const closes = await mountPreview()
    window.dispatchEvent(escape())
    expect(closes).toHaveLength(1)
  })

  it('stays open when something over it already handled the Escape (a menu closing)', async () => {
    const closes = await mountPreview()
    const handled = escape()
    handled.preventDefault()
    window.dispatchEvent(handled)
    expect(closes).toHaveLength(0)
  })
})
