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

  // The preview over files that all show the fallback, as FilesView passes them;
  // shown: the open file's index, or null when closed.
  function mountOver(files: FileEntry[], shown: { value: number | null }, steps: string[]) {
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp({
      render: () => {
        const index = shown.value
        return h(FilePreview, {
          entry: index === null ? null : files[index],
          prevEntry: index === null ? null : files[index - 1] ?? null,
          nextEntry: index === null ? null : files[index + 1] ?? null,
          hasPrev: index !== null && index > 0,
          hasNext: index !== null && index < files.length - 1,
          index: index ?? -1,
          total: files.length,
          onPrev: () => steps.push('prev'),
          onNext: () => steps.push('next'),
          onClose: () => (shown.value = null),
        })
      },
    })
    app.mount(root)
    cleanups.push(() => { app.unmount(); root.remove() })
  }

  const files: FileEntry[] = ['a.bin', 'b.bin', 'c.bin'].map((name) => ({ ...entry, name, path: name }))

  it('moves to the next file when the slide an arrow key starts ends', async () => {
    vi.useFakeTimers()
    const steps: string[] = []
    mountOver(files, ref<number | null>(1), steps)
    await nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    expect(steps).toEqual([])
    vi.advanceTimersByTime(300)
    expect(steps).toEqual(['next'])
  })

  it("doesn't move on when it's closed while the slide is still running", async () => {
    vi.useFakeTimers()
    const steps: string[] = []
    const shown = ref<number | null>(1)
    mountOver(files, shown, steps)
    await nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft' }))
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(shown.value).toBeNull()
    await nextTick()
    vi.advanceTimersByTime(300)
    // With nothing open, a late "next" made FilesView open the folder's first file.
    expect(steps).toEqual([])
  })
})
